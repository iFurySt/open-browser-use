import assert from 'node:assert/strict';
import { createConnection } from 'node:net';
import { once } from 'node:events';
import { frame } from '../../packages/open-browser-use-electron/dist/wire.js';

export async function runSmoke({ runtime, scope, fixture, surface, window, launch, present, savePage }) {
  const checks = [];
  const check = message => { checks.push(message); console.log(`IAB smoke: ${message}`); };
  const a = await scope.tabs.create({ url: `${fixture.url}/login` });
  const b = await scope.tabs.create({ url: `${fixture.url}/second` });
  assert.notEqual(a.id,b.id); assert.equal(scope.tabs.list().length,2);
  check('same session: independent tabs');
  const evaluate = async (id, expression) => {
    const result = await scope.call('executeCdp', { target:{tabId:id}, method:'Runtime.evaluate', commandParams:{expression,returnByValue:true,awaitPromise:true} });
    if (result.exceptionDetails) throw new Error(result.exceptionDetails.text);
    return result.result.value;
  };
  assert.match(await evaluate(b.id,'document.cookie'), /iab_example=logged-in/); check('named profile: shared login cookie');
  present(b.id);
  await scope.call('executeCdp', { target:{tabId:a.id}, method:'Runtime.evaluate', commandParams:{expression:'document.querySelector("#message").focus()'} });
  await scope.call('executeCdp', { target:{tabId:a.id}, method:'Input.insertText', commandParams:{text:'typed in hidden tab'} });
  assert.equal(await evaluate(a.id,'document.querySelector("#message").value'),'typed in hidden tab');
  assert.equal(await evaluate(a.id,'window.trustedInput'),true); check('CDP input: hidden tab, trusted input event');
  const point = await evaluate(a.id,'(()=>{const r=document.querySelector("#increment").getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()');
  for (const type of ['mouseMoved','mousePressed','mouseReleased']) await scope.call('executeCdp', {
    target:{tabId:a.id}, method:'Input.dispatchMouseEvent', commandParams:{type,...point,button:type==='mouseMoved'?'none':'left',buttons:type==='mousePressed'?1:0,clickCount:type==='mouseMoved'?0:1},
  });
  const clickDeadline = Date.now()+1000;
  while (await evaluate(a.id,'document.querySelector("#count").textContent')==='0' && Date.now()<clickDeadline) await new Promise(r=>setTimeout(r,20));
  assert.equal(await evaluate(a.id,'document.querySelector("#count").textContent'),'1');
  assert.equal(await evaluate(a.id,'window.trustedClick'),true); check('CDP mouse: hidden tab, trusted click event');
  const other = runtime.scopes.open({ id:'other-task', profileId:'work' });
  await assert.rejects(other.call('executeCdp',{target:{tabId:a.id},method:'Runtime.evaluate'}));
  await assert.rejects(scope.call('executeCdp',{target:{tabId:a.id},method:'Browser.close'})); check('scope ownership and global CDP rejection');
  for (const mode of ['private','socket','cli-private','cli-socket']) {
    const before = scope.tabs.list().length; const result = await launch(mode);
    assert.equal(scope.tabs.list().length,before+1);
    assert.ok(mode.startsWith('cli') ? result.result.id : result.tabId); check(`${mode}: real process / RPC / page`);
  }
  // CLI and worker processes have exited and their grants were revoked. This
  // unregistered process receives no browser response from the known endpoint.
  const endpoint = await runtime.connections.openSocket();
  const raw = createConnection(endpoint); let bytes = 0;
  raw.on('error',()=>{}); raw.on('data',data=>{bytes+=data.length});
  const closed = new Promise(resolve => raw.once('close',resolve));
  raw.once('connect',()=>raw.write(frame({jsonrpc:'2.0',id:'anonymous',method:'getInfo',params:{session_id:scope.id}})));
  await closed; assert.equal(bytes,0); check('anonymous known-path socket: rejected before RPC');
  present(a.id);
  await evaluate(a.id,'document.querySelector("#popup").click()');
  const deadline=Date.now()+5000;
  while (await evaluate(a.id,'document.querySelector("#status").textContent')!=='Popup connected' && Date.now()<deadline) await new Promise(r=>setTimeout(r,50));
  assert.equal(await evaluate(a.id,'document.querySelector("#status").textContent'),'Popup connected'); check('popup: separate tab with preserved opener');
  surface.hide(); assert.ok(scope.tabs.list().length>=7);
  const capture = await scope.call('executeCdp',{target:{tabId:a.id},method:'Page.captureScreenshot',commandParams:{format:'png'}});
  assert.ok(capture.data.length>100); check('hidden page screenshot and retained lifecycle');
  await savePage(Buffer.from(capture.data,'base64'));
  present(a.id);
  // Exercise the example's real, sender-validated IPC through its own renderer.
  const ui = expression => window.webContents.executeJavaScript(expression);
  const action = (name,value) => ui(`window.iabExample.action(${JSON.stringify(name)},${JSON.stringify(value) ?? 'undefined'})`);
  const waitFor = async predicate => {
    const until = Date.now()+5000;
    while (!(await predicate())) { if (Date.now()>=until) throw new Error('Browser panel UI timed out'); await new Promise(r=>setTimeout(r,25)); }
  };
  await waitFor(()=>ui(`document.querySelectorAll('[role="tab"]').length === ${scope.tabs.list().length}`));
  const layout = await ui(`(()=>{const r=document.querySelector('#viewport').getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height,label:document.body.textContent.includes('Native WebContentsView')}})()`);
  assert.equal(layout.label,false); assert.equal(layout.x,280); assert.equal(layout.y,134);
  const nativeBounds = () => window.contentView.children.find(view=>view.webContents && view.webContents !== window.webContents).getBounds();
  assert.deepEqual(nativeBounds(),{x:layout.x,y:layout.y,width:layout.width,height:layout.height});
  assert.ok(layout.width>=600 && layout.height>400); check('browser panel: tab strip, toolbar and native viewport aligned');
  window.setContentSize(960,760);
  await waitFor(()=>ui("Math.round(document.querySelector('#viewport').getBoundingClientRect().width)===680"));
  const small = await ui("(()=>{const r=document.querySelector('#viewport').getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height}})()");
  await action('viewport',small); assert.equal(small.height,626);
  assert.deepEqual(nativeBounds(),small);
  window.setContentSize(1240,828);
  await waitFor(()=>ui("Math.round(document.querySelector('#viewport').getBoundingClientRect().width)===960"));
  await assert.rejects(action('viewport',{x:0,y:0,width:960,height:760}));
  check('browser panel: resizing stays below chrome, invalid bounds rejected');
  const manual = await action('create');
  await action('navigate',`${fixture.url}/ui-target`);
  await waitFor(()=>evaluate(manual.id,"location.pathname==='/ui-target'"));
  await waitFor(()=>ui(`document.querySelector('#address').value === ${JSON.stringify(`${fixture.url}/ui-target`)}`));
  await action('back'); await waitFor(()=>evaluate(manual.id,"location.pathname==='/manual'"));
  await action('forward'); await waitFor(()=>evaluate(manual.id,"location.pathname==='/ui-target'"));
  await action('reload'); await waitFor(()=>evaluate(manual.id,"document.readyState==='complete'"));
  await assert.rejects(action('navigate','file:///etc/passwd'));
  await action('hide'); assert.equal((await ui('window.iabExample.state()')).selected,undefined);
  await action('select',manual.id); await action('close',manual.id);
  assert.ok(!scope.tabs.list().some(tab=>tab.id===manual.id));
  await action('select',a.id);
  await waitFor(()=>ui(`document.querySelector('[role="tab"][aria-selected="true"]')?.title === ${JSON.stringify(`${fixture.url}/login`)}`));
  check('browser panel: address navigation, history, reload, hide, restore and close');
  assert.equal(await ui("document.querySelectorAll('[data-step]').length"),5);
  await assert.rejects(action('demo-step',2));
  const first = await action('demo-step',1); const second = await action('demo-step',2);
  assert.notEqual(first.tab1,second.tab2);
  assert.equal((await ui('window.iabExample.state()')).selected,second.tab2);
  const clicked = await action('demo-step',3);
  assert.equal(clicked.detail.count,1); assert.equal(clicked.detail.trustedClick,true);
  assert.equal((await ui('window.iabExample.state()')).selected,first.tab1);
  assert.equal(await evaluate(second.tab2,'document.querySelector("#count").textContent'),'0');
  const captureTab1 = await scope.call('executeCdp',{target:{tabId:first.tab1},method:'Page.captureScreenshot',commandParams:{format:'png'}});
  await savePage(Buffer.from(captureTab1.data,'base64'),'demo-tab1.png');
  const submitted = await action('demo-step',4);
  assert.equal(submitted.detail.submitted,'Submitted: Hello from CLI');
  assert.equal(submitted.detail.trustedInput,true); assert.equal(submitted.detail.trustedSubmit,true);
  assert.equal((await ui('window.iabExample.state()')).selected,second.tab2);
  const captureTab2 = await scope.call('executeCdp',{target:{tabId:second.tab2},method:'Page.captureScreenshot',commandParams:{format:'png'}});
  await savePage(Buffer.from(captureTab2.data,'base64'),'demo-tab2.png');
  await action('demo-step',5); assert.equal(scope.tabs.list().length,0);
  check('CLI private demo: five manual steps, trusted click/input/submit, explicit tabs and close-all');
  const automated = await action('demo-play');
  assert.deepEqual(automated.completed,[1,2,3,4,5]);
  assert.equal(automated.results.find(result=>result.step===3).detail.count,1);
  assert.equal(automated.results.find(result=>result.step===4).detail.trustedSubmit,true);
  assert.equal(scope.tabs.list().length,0);
  check('CLI private demo: one-click replay completes all steps and cleans pages');
  const restarted = await action('demo-step',1);
  assert.deepEqual(restarted.completed,[1]); assert.equal(scope.tabs.list().length,1);
  await action('demo-step',5);
  await ui("window.__demoPlayback=window.iabExample.action('demo-play');true");
  await waitFor(async()=>!!(await ui('window.iabExample.state()')).demo.playing);
  await assert.rejects(action('demo-step',2));
  await action('demo-stop');
  const stopped = await ui('window.__demoPlayback');
  assert.equal(stopped.playing,false);
  const idle = (await ui('window.iabExample.state()')).demo;
  assert.equal(idle.playing,false); assert.equal(idle.busy,false); assert.ok(idle.completed.length<5);
  await action('demo-step',5); assert.equal(scope.tabs.list().length,0);
  check('CLI private demo: stop finishes safely and manual cleanup remains available');
  return { passed:true, electron:process.versions.electron, platform:process.platform, checks, tabs:scope.tabs.list().length };
}
