import { app, BrowserWindow, ipcMain } from 'electron';
import { createIabRuntime } from 'open-browser-use-electron';
import { mkdtempSync } from 'node:fs';
import { mkdir, writeFile } from 'node:fs/promises';
import { join, dirname, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { startFixture } from './fixture.mjs';
import { runSmoke } from './smoke.mjs';
import { createExampleClients } from './integrations.mjs';
import { createActionDemo } from './demo.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const smoke = process.argv.includes('--smoke');
const userData = smoke ? process.env.OBU_IAB_SMOKE_USER_DATA ?? mkdtempSync(join(tmpdir(), 'obu-iab-example-')) : undefined;
if (userData) app.setPath('userData', userData);
// Do not await app.whenReady() at ESM top level: Electron waits for its entry
// module to finish evaluating before it completes application startup.
app.whenReady().then(main).catch(error => { console.error(error); app.exit(1); });
async function main() {
  if (smoke) console.log('IAB smoke: Electron ready');
  let shuttingDown = false;
  const fixture = await startFixture();
  const runtime = createIabRuntime({ appId: 'electron-example' });
  runtime.profiles.open({ id: 'work' });
  const scope = runtime.scopes.open({ id: 'example-task', profileId: 'work' });
  const window = new BrowserWindow({ width: 1240, height: 850, minWidth: 960, minHeight: 760,
    webPreferences: { preload: join(here, 'preload.cjs'), sandbox: true, contextIsolation: true, nodeIntegration: false },
  });
  window.webContents.on('will-navigate', event => event.preventDefault());
  const surface = runtime.surfaces.register({ id: 'browser-panel', window, owner: window.webContents });
  let selected;
  const changed = () => { if (!shuttingDown && !window.isDestroyed()) window.webContents.send('iab-example:changed'); };
  const present = id => { surface.present(id); selected = id; changed(); };
  runtime.subscribe(event => {
    if (shuttingDown || event.scopeId !== scope.id) return;
    if (event.method === 'onTabClosed' && selected === event.params.tabId) {
      const remaining = scope.tabs.list(); selected = undefined;
      if (remaining.length) present(remaining.at(-1).id);
    }
    if (['onTabCreated','onTabClosed','onTabUpdated'].includes(event.method)) changed();
  });
  const resize = () => { const [width,height] = window.getContentSize(); surface.setBounds({ x: 280, y: 134, width: Math.max(1,width-280), height: Math.max(1,height-134) }); };
  window.on('resize', resize); resize();
  const clients = createExampleClients({runtime,scopeId:scope.id,cliPath:resolve(here,'../../build/obu-iab'),directory:here});
  const runCli = args => clients.launchCli('private',args);
  const demo = createActionDemo({cli:runCli,fixtureUrl:fixture.url,present,changed,paceMs:smoke?0:1000});
  // Reference integrations are kept in code and smoke, separate from the demo UI.
  async function launch(mode) {
    const result = mode.startsWith('cli-')
      ? await clients.launchCli(mode.endsWith('socket')?'socket':'private',['open-tab',`${fixture.url}/${mode}`])
      : await clients.launchNode(mode==='socket'?'socket':'private',`${fixture.url}/${mode}`);
    present(mode.startsWith('cli-')?result.result.id:result.tabId); return result;
  }
  const hostUrl = pathToFileURL(join(here,'index.html')).href;
  const selectedCdp = (method, commandParams = {}, tabId = selected) => {
    if (tabId === undefined) throw new Error('请选择一个标签');
    return runCli(['cdp',String(tabId),method,JSON.stringify(commandParams)]).then(response=>response.result);
  };
  ipcMain.handle('iab-example:action', async (event, message) => {
    if (shuttingDown) return { tabs: [], selected: undefined };
    if (event.sender !== window.webContents || event.senderFrame !== window.webContents.mainFrame || event.senderFrame.url !== hostUrl) throw new Error('Unauthorized renderer');
    if ((demo.snapshot().busy || demo.snapshot().playing) && !['state','viewport','demo-stop'].includes(message?.action)) throw new Error('演示正在运行');
    switch (message?.action) {
      case 'state': {
        const tabs = scope.tabs.list(), tabId = selected; let history;
        if (tabId !== undefined) history = await selectedCdp('Page.getNavigationHistory',{},tabId).catch(() => undefined);
        return {tabs,selected:tabId,demo:demo.snapshot(),canGoBack:!!history && history.currentIndex>0,canGoForward:!!history && history.currentIndex<history.entries.length-1};
      }
      case 'viewport': {
        const bounds = message.value, [width,height] = window.getContentSize();
        if (!bounds || !['x','y','width','height'].every(key=>Number.isSafeInteger(bounds[key])) ||
            bounds.x<280 || bounds.y<134 || bounds.width<1 || bounds.height<1 ||
            bounds.x+bounds.width>width || bounds.y+bounds.height>height) throw new Error('Invalid browser viewport');
        surface.setBounds({x:bounds.x,y:bounds.y,width:bounds.width,height:bounds.height}); return {};
      }
      case 'select': if (!scope.tabs.list().some(t => t.id === message.value)) throw new Error('Unknown tab'); present(message.value); return {};
      case 'hide': surface.hide(); selected = undefined; changed(); return {};
      case 'close': await runCli(['close-tab',String(message.value)]); return {};
      case 'navigate': return selectedCdp('Page.navigate',{url:message.value});
      case 'reload': return selectedCdp('Page.reload');
      case 'back': case 'forward': {
        const tabId = selected;
        const history = await selectedCdp('Page.getNavigationHistory',{},tabId);
        const entry = history.entries[history.currentIndex+(message.action==='back'?-1:1)];
        if (entry) await selectedCdp('Page.navigateToHistoryEntry',{entryId:entry.id},tabId); return {};
      }
      case 'create': { const {result:tab} = await runCli(['open-tab',message.value || `${fixture.url}/manual`]); present(tab.id); return tab; }
      case 'demo-step': return demo.step(message.value);
      case 'demo-play': return demo.play();
      case 'demo-stop': return demo.stop();
      default: throw new Error('Unknown example action');
    }
  });
  await window.loadFile(join(here,'index.html'));

  async function shutdown() {
    if (shuttingDown) return;
    shuttingDown = true;
    demo.stop();
    await runtime.dispose(); await fixture.close();
    if (!window.isDestroyed()) window.destroy();
    ipcMain.removeHandler('iab-example:action');
    // The smoke launcher removes the disposable profile after Electron exits;
    // Chromium's Session storage writers outlive individual WebContents.
    app.exit();
  }
  app.on('window-all-closed', () => { void shutdown(); });
  app.on('before-quit', event => { if (!shuttingDown) { event.preventDefault(); void shutdown(); } });
  if (smoke) {
    try {
      const output = resolve(here,'../../.cache/iab-smoke'); await mkdir(output,{recursive:true});
      const savePage = (png,name='page.png') => writeFile(join(output,name), png);
      const report = await runSmoke({ runtime, scope, fixture, surface, window, launch, present, savePage });
      await writeFile(join(output,'ui.png'), (await window.webContents.capturePage()).toPNG());
      await writeFile(join(output,'report.json'), JSON.stringify(report,null,2));
      console.log(JSON.stringify(report)); await shutdown();
    } catch (error) { shuttingDown = true; console.error(error); await runtime.dispose(); await fixture.close(); app.exit(1); }
  }
}
