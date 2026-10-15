import assert from 'node:assert/strict';
import { createConnection } from 'node:net';
import { once } from 'node:events';
import { frame } from '../../packages/open-browser-use-electron/dist/wire.js';
import { cursorBitmap } from '../../packages/open-browser-use-electron/dist/cursor.generated.js';
import { connectExampleMcp } from './mcp.mjs';

export async function runSmoke({ runtime, scope, fixture, surface, window, launch, launchMcp, present, savePage }) {
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
  // The MCP protocol uses stdin/stdout; browser bootstrap and RPC stay on FD 3
  // or the authenticated socket. Never bridge agent stdio to the private FD.
  for (const transport of ['private','socket']) {
    const mcp = await connectExampleMcp(await launchMcp(transport));
    let tabId;
    try {
      const tools = await mcp.request('tools/list');
      assert.ok(tools.tools.some(tool=>tool.name==='snapshot'));
      assert.ok(!tools.tools.some(tool=>tool.name==='user_tabs'));
      const opened = await mcp.callTool('open_tab',{url:`${fixture.url}/mcp-${transport}`});
      assert.equal(opened.isError,false); tabId = opened.structuredContent.id;
      const list = await mcp.callTool('tabs');
      assert.ok(list.structuredContent.result.some(tab=>tab.id===tabId));
      assert.equal((await mcp.callTool('tabs',{session_id:'other-task'})).isError,true);
      assert.equal((await mcp.callTool('page_info',{tab_id:a.id})).isError,false);
      assert.equal((await mcp.callTool('snapshot',{tab_id:tabId})).isError,false);
      const image = await mcp.callTool('screenshot',{tab_id:tabId});
      assert.equal(image.content[0].type,'image');
      assert.equal(Buffer.from(image.content[0].data,'base64').subarray(1,4).toString(),'PNG');
      assert.equal((await mcp.callTool('cdp',{tab_id:tabId,method:'Browser.close'})).isError,true);
      const foreign = await other.tabs.create();
      assert.equal((await mcp.callTool('snapshot',{tab_id:foreign.id})).isError,true);
      await other.tabs.close(foreign.id);
      const input = await evaluate(tabId,'(()=>{const r=document.querySelector("#message").getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()');
      assert.equal((await mcp.callTool('click',{tab_id:tabId,...input})).isError,false);
      assert.equal((await mcp.callTool('type_text',{tab_id:tabId,text:'Hello from MCP'})).isError,false);
      assert.equal(await evaluate(tabId,'document.querySelector("#message").value'),'Hello from MCP');
      assert.equal(await evaluate(tabId,'window.trustedInput'),true);
      const button = await evaluate(tabId,'(()=>{const r=document.querySelector("#increment").getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()');
      assert.equal((await mcp.callTool('click',{tab_id:tabId,...button})).isError,false);
      const deadline = Date.now()+2000;
      while (await evaluate(tabId,'document.querySelector("#count").textContent')==='0' && Date.now()<deadline) await new Promise(r=>setTimeout(r,20));
      assert.equal(await evaluate(tabId,'document.querySelector("#count").textContent'),'1');
      assert.equal(await evaluate(tabId,'window.trustedClick'),true);
    } finally { await mcp.close(); }
    assert.ok(scope.tabs.list().some(tab=>tab.id===tabId));
    check(`mcp-${transport}: stdio lifecycle, scoped tabs, image, trusted mouse/text, EOF retains pages`);
  }
  const restricted = await connectExampleMcp(await launchMcp('private',{operations:['tabs.read']}));
  try {
    assert.equal((await restricted.callTool('tabs')).isError,false);
    assert.equal((await restricted.callTool('open_tab')).isError,true);
    assert.equal((await restricted.callTool('snapshot',{tab_id:a.id})).isError,true);
    restricted.revoke();
    assert.equal((await restricted.callTool('tabs')).isError,true);
  } finally { await restricted.close(); }
  check('mcp: operation grants and revocation remain enforced by the IAB host');
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
  // Drive Chromium's real Enter default action, rather than calling
  // requestSubmit, so address navigation also covers the keyboard path.
  const pressEnter = async () => {
    window.webContents.debugger.attach('1.3');
    try {
      await window.webContents.debugger.sendCommand('Input.dispatchKeyEvent',{type:'keyDown',key:'Enter',code:'Enter',windowsVirtualKeyCode:13,text:'\r'});
      await window.webContents.debugger.sendCommand('Input.dispatchKeyEvent',{type:'keyUp',key:'Enter',code:'Enter',windowsVirtualKeyCode:13});
    } finally { window.webContents.debugger.detach(); }
  };
  const action = (name,value) => ui(`window.iabExample.action(${JSON.stringify(name)},${JSON.stringify(value) ?? 'undefined'})`);
  const waitFor = async (predicate,timeoutMs=5000) => {
    const until = Date.now()+timeoutMs;
    while (!(await predicate())) { if (Date.now()>=until) throw new Error('Browser panel UI timed out: '+JSON.stringify(await ui('({error:document.querySelector("#error").textContent,address:document.querySelector("#address").value,focus:document.activeElement?.id})'))); await new Promise(r=>setTimeout(r,25)); }
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
  await waitFor(()=>ui(`document.querySelector('#address').value===${JSON.stringify(`${fixture.url}/manual`)}`));
  await ui(`(()=>{const input=document.querySelector('#address');input.focus();input.value=${JSON.stringify(`${fixture.url}/ui-target`)};document.querySelector('#address-form').requestSubmit();input.blur();return true})()`);
  await waitFor(()=>evaluate(manual.id,"location.pathname==='/ui-target'"));
  await waitFor(()=>ui(`document.querySelector('#address').value === ${JSON.stringify(`${fixture.url}/ui-target`)}`));
  await waitFor(()=>ui("!document.querySelector('#back').disabled"));
  await ui("document.querySelector('#back').click();true"); await waitFor(()=>evaluate(manual.id,"location.pathname==='/manual'"));
  await waitFor(()=>ui("!document.querySelector('#forward').disabled"));
  await ui("document.querySelector('#forward').click();true"); await waitFor(()=>evaluate(manual.id,"location.pathname==='/ui-target'"));
  await evaluate(manual.id,'window.__beforeReload=true');
  await ui("document.querySelector('#reload').click();true"); await waitFor(()=>evaluate(manual.id,"document.readyState==='complete' && window.__beforeReload===undefined"));
  await assert.rejects(action('navigate','file:///etc/passwd'));
  await action('hide'); assert.equal((await ui('window.iabExample.state()')).selected,undefined);
  await action('select',manual.id); await action('close',manual.id);
  assert.ok(!scope.tabs.list().some(tab=>tab.id===manual.id));
  await action('select',a.id);
  await waitFor(()=>ui(`document.querySelector('[role="tab"][aria-selected="true"]')?.title === ${JSON.stringify(`${fixture.url}/login`)}`));
  check('browser panel: address navigation, history, reload, hide, restore and close');
  await waitFor(()=>ui(`document.querySelector('#address').value===${JSON.stringify(`${fixture.url}/login`)}`));
  // Exercise the same fixed IPC the main process sends for Cmd/Ctrl+L. The
  // smoke window need not take OS foreground focus from the developer.
  window.webContents.send('iab-example:focus-address');
  await waitFor(()=>ui("document.querySelector('#address').selectionStart===0 && document.querySelector('#address').selectionEnd===document.querySelector('#address').value.length"));
  // The address bar now transitions between idle, hover and focus colors.
  await waitFor(()=>ui("getComputedStyle(document.querySelector('#address-form')).borderColor==='rgb(76, 132, 223)'"));
  const focused=await ui(`(()=>{const input=document.querySelector('#address');return {start:input.selectionStart,end:input.selectionEnd,length:input.value.length,selection:getComputedStyle(input,'::selection').backgroundColor,border:getComputedStyle(document.querySelector('#address-form')).borderColor}})()`);
  assert.equal(focused.start,0); assert.equal(focused.end,focused.length);
  assert.equal(focused.selection,'rgb(54, 121, 213)'); assert.equal(focused.border,'rgb(76, 132, 223)');
  await savePage((await window.webContents.capturePage()).toPNG(),'address-focused.png');
  const slowUrl=`${fixture.url}/slow`;
  await ui(`(()=>{const input=document.querySelector('#address');input.value=${JSON.stringify(slowUrl.replace('http://',''))};return true})()`);
  await pressEnter();
  await waitFor(async()=>!!(await ui('window.iabExample.state()')).tabs.find(tab=>tab.id===a.id)?.loading);
  await waitFor(()=>ui(`document.querySelector('[data-tab-id="${a.id}"]').classList.contains('loading') && document.querySelector('#address-form').classList.contains('loading')`));
  assert.equal((await ui('window.iabExample.state()')).tabs.find(tab=>tab.id===a.id).pendingUrl,slowUrl);
  assert.equal(await ui("document.querySelector('#address').value"),slowUrl);
  await savePage((await window.webContents.capturePage()).toPNG(),'address-loading.png');
  await action('select',b.id);
  await waitFor(()=>ui("!document.querySelector('#address-form').classList.contains('loading')"));
  assert.equal((await ui('window.iabExample.state()')).tabs.find(tab=>tab.id===a.id).loading,true);
  fixture.releaseSlowPages();
  await waitFor(()=>evaluate(a.id,'location.pathname==="/slow" && document.readyState==="complete"'));
  await waitFor(async()=>!(await ui('window.iabExample.state()')).tabs.find(tab=>tab.id===a.id).loading);
  assert.equal((await ui('window.iabExample.state()')).selected,b.id);
  await action('select',a.id);
  await waitFor(()=>ui("!document.querySelector('#address-form').classList.contains('loading')"));
  check('address: scheme-less Enter, clear focus/selection and live loading before slow server responds');
  await ui("window.__failedNavigation=window.iabExample.action('navigate','http://127.0.0.1:0/unreachable').then(()=>false,()=>true);true");
  assert.equal(await ui('window.__failedNavigation'),true);
  await waitFor(async()=>{const tab=(await ui('window.iabExample.state()')).tabs.find(tab=>tab.id===a.id);return !tab.loading && !!tab.loadError});
  await waitFor(()=>ui("document.querySelector('#status').textContent.includes('加载失败')"));
  const recovered=`http://localhost:${new URL(fixture.url).port}/recovered`;
  await ui(`(()=>{const input=document.querySelector('#address');input.focus();input.value=${JSON.stringify(recovered.replace('http://',''))};return true})()`);
  await pressEnter();
  await waitFor(()=>evaluate(a.id,'location.pathname==="/recovered" && document.readyState==="complete"'));
  await waitFor(async()=>{const tab=(await ui('window.iabExample.state()')).tabs.find(tab=>tab.id===a.id);return !tab.loading && !tab.loadError});
  await waitFor(()=>ui("!document.querySelector('#status').textContent.includes('加载失败')"));
  check('address: failed navigation stops loading, localhost:port retry clears the error');
  await action('feedback',true);
  await scope.visualFeedback.setActive(a.id,true);
  await scope.call('executeCdp',{target:{tabId:a.id},method:'Input.dispatchMouseEvent',commandParams:{type:'mouseMoved',x:80,y:80}});
  const overlay = await evaluate(a.id,'(()=>{const e=document.getElementById("obu-iab-visual-feedback");return {visible:!!e,cursor:e?.dataset.cursorVisible,edge:e?.dataset.edgeGlow,x:e?.dataset.x,closed:e?.shadowRoot===null,isolated:typeof globalThis.__obuIabFeedback,hit:document.elementFromPoint(80,80)?.id}})()');
  assert.equal(overlay.visible,true); assert.equal(overlay.cursor,'true'); assert.equal(overlay.edge,'true'); assert.equal(overlay.x,'80');
  assert.equal(overlay.closed,true); assert.equal(overlay.isolated,'undefined'); assert.notEqual(overlay.hit,'obu-iab-visual-feedback');
  const feedbackImage=await scope.call('executeCdp',{target:{tabId:a.id},method:'Page.captureScreenshot',commandParams:{format:'png'}});
  await savePage(Buffer.from(feedbackImage.data,'base64'),'feedback.png');
  const feedbackGuest=window.contentView.children.find(view=>view.webContents && view.webContents!==window.webContents).webContents;
  const edgeStyle=()=>feedbackGuest.executeJavaScriptInIsolatedWorld(999,[{code:`(()=>{const c=globalThis.__obuIabFeedback;return {shadow:getComputedStyle(c.edge).boxShadow,hidden:c.edge.hidden,cursor:c.root.dataset.cursorVisible}})()`}]);
  await runtime.visualFeedback.configure({edgeGlowColor:'#A3c'});
  assert.equal(runtime.visualFeedback.getOptions().edgeGlowColor,'#aa33cc');
  assert.match((await edgeStyle()).shadow,/rgba\(170, 51, 204, 0\.32\)/);
  await assert.rejects(runtime.visualFeedback.configure({edgeGlow:false,edgeGlowColor:'invalid'}),/hex color/);
  assert.equal((await edgeStyle()).hidden,false);
  await runtime.visualFeedback.configure({edgeGlow:false});
  assert.deepEqual({hidden:(await edgeStyle()).hidden,cursor:(await edgeStyle()).cursor},{hidden:true,cursor:'true'});
  await runtime.visualFeedback.configure({edgeGlowColor:'#22C55E'});
  assert.equal((await edgeStyle()).hidden,true);
  await runtime.visualFeedback.configure({edgeGlow:true});
  assert.match((await edgeStyle()).shadow,/rgba\(34, 197, 94, 0\.32\)/);
  const coloredImage=await scope.call('executeCdp',{target:{tabId:a.id},method:'Page.captureScreenshot',commandParams:{format:'png'}});
  await savePage(Buffer.from(coloredImage.data,'base64'),'feedback-custom-color.png');
  await runtime.visualFeedback.configure({edgeGlowColor:'#4f8fe8'});
  check('visual feedback: live custom color, atomic validation and independent edge toggle preserve the cursor');
  await action('feedback',false);
  assert.equal(await evaluate(a.id,'!!document.getElementById("obu-iab-visual-feedback")'),false);
  await action('feedback',true);
  await scope.visualFeedback.setActive(a.id,false); await scope.visualFeedback.setActive(b.id,true);
  assert.equal(await evaluate(a.id,'!!document.getElementById("obu-iab-visual-feedback")'),false);
  await scope.call('executeCdp',{target:{tabId:b.id},method:'Input.dispatchMouseEvent',commandParams:{type:'mouseMoved',x:50,y:50}});
  await runtime.visualFeedback.configure({cursor:false,edgeGlow:true});
  assert.equal(await evaluate(b.id,'document.getElementById("obu-iab-visual-feedback").dataset.cursorVisible'),'false');
  await runtime.visualFeedback.configure({cursor:true,edgeGlow:false});
  assert.equal(await evaluate(b.id,'document.getElementById("obu-iab-visual-feedback").dataset.edgeGlow'),'false');
  await scope.visualFeedback.setActive(b.id,false); await action('feedback',true);
  check('visual feedback: isolated, nonblocking, scoped activity and independent/disabled options');
  await scope.visualFeedback.setActive(a.id,true);
  await scope.call('executeCdp',{target:{tabId:a.id},method:'Page.navigate',commandParams:{url:`${fixture.url}/strict-csp`}});
  await waitFor(()=>evaluate(a.id,'location.pathname==="/strict-csp" && document.readyState==="complete" && !!document.getElementById("obu-iab-visual-feedback")').catch(()=>false));
  assert.equal(await evaluate(a.id,'document.getElementById("obu-iab-visual-feedback").dataset.cursorVisible'),'false');
  await scope.call('executeCdp',{target:{tabId:a.id},method:'Input.dispatchMouseEvent',commandParams:{type:'mouseMoved',x:80,y:80}});
  // Inspect through the trusted host's isolated world; the page cannot read the
  // closed shadow. Verify the actual bitmap, not only that an overlay exists.
  const guest = window.contentView.children.find(view=>view.webContents && view.webContents !== window.webContents).webContents;
  await runtime.visualFeedback.configure({edgeGlowColor:'#F0A'});
  const strictShadow=await guest.executeJavaScriptInIsolatedWorld(999,[{code:'getComputedStyle(globalThis.__obuIabFeedback.edge).boxShadow'}]);
  assert.match(strictShadow,/rgba\(255, 0, 170, 0\.32\)/);
  const rendered = await guest.executeJavaScriptInIsolatedWorld(999,[{code:`(()=>{const canvas=globalThis.__obuIabFeedback.cursorLayer.querySelector('canvas');return {width:canvas.width,height:canvas.height,displayWidth:canvas.style.width,displayHeight:canvas.style.height,pixels:Array.from(canvas.getContext('2d').getImageData(0,0,canvas.width,canvas.height).data)}})()`}]);
  assert.equal(rendered.width,cursorBitmap.width); assert.equal(rendered.height,cursorBitmap.height);
  assert.equal(rendered.displayWidth,'23px'); assert.equal(rendered.displayHeight,'24px');
  // Canvas may round translucent RGB by one byte through premultiplication;
  // fully opaque pixels and alpha must retain the original asset exactly.
  const original=Buffer.from(cursorBitmap.rgba,'base64');
  for (let i=0;i<original.length;i+=4) {
    assert.equal(rendered.pixels[i+3],original[i+3]);
    if (original[i+3]===255) assert.deepEqual(rendered.pixels.slice(i,i+3),Array.from(original.subarray(i,i+3)));
  }
  await scope.call('executeCdp',{target:{tabId:a.id},method:'Input.dispatchMouseEvent',commandParams:{type:'mousePressed',x:80,y:80,button:'left',buttons:1,clickCount:1}});
  const ripple = await guest.executeJavaScriptInIsolatedWorld(999,[{code:`(()=>{const r=globalThis.__obuIabFeedback.ring.getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2}})()`}]);
  assert.ok(Math.abs(ripple.x-80)<0.01 && Math.abs(ripple.y-80)<0.01,'Click ripple remains centered while scaling');
  await scope.call('executeCdp',{target:{tabId:a.id},method:'Input.dispatchMouseEvent',commandParams:{type:'mouseReleased',x:80,y:80,button:'left',buttons:0,clickCount:1}});
  const strictFeedbackImage=await scope.call('executeCdp',{target:{tabId:a.id},method:'Page.captureScreenshot',commandParams:{format:'png'}});
  await savePage(Buffer.from(strictFeedbackImage.data,'base64'),'feedback-strict-csp.png');
  await runtime.visualFeedback.configure({edgeGlowColor:'#4f8fe8'});
  await scope.visualFeedback.setActive(a.id,false);
  await scope.call('executeCdp',{target:{tabId:a.id},method:'Input.dispatchMouseEvent',commandParams:{type:'mouseMoved',x:80,y:80}});
  await waitFor(()=>evaluate(a.id,'!document.getElementById("obu-iab-visual-feedback")'));
  check('visual feedback: navigation resets coordinates, custom color renders under strict CSP, transient activity expires');
  while (scope.tabs.list().length) await action('close',scope.tabs.list().at(-1).id);
  assert.equal(await ui("document.querySelectorAll('[data-step]').length"),5);
  await waitFor(()=>ui("[...document.querySelectorAll('[data-step]')].every(step=>!step.disabled)"));
  await assert.rejects(action('demo-step',3),/请先点击.*Tab 1/);
  await assert.rejects(action('demo-step',4),/请先点击.*Tab 2/);
  // Manual actions are independent: opening Tab 2 does not require Tab 1.
  const second = await action('demo-step',2); const first = await action('demo-step',1);
  assert.notEqual(first.tab1,second.tab2);
  assert.equal((await ui('window.iabExample.state()')).selected,first.tab1);
  const clicked = await action('demo-step',3);
  assert.equal(clicked.detail.count,1); assert.equal(clicked.detail.trustedClick,true);
  assert.equal((await ui('window.iabExample.state()')).selected,first.tab1);
  assert.equal(await evaluate(second.tab2,'document.querySelector("#count").textContent'),'0');
  const captureTab1 = await scope.call('executeCdp',{target:{tabId:first.tab1},method:'Page.captureScreenshot',commandParams:{format:'png'}});
  await savePage(Buffer.from(captureTab1.data,'base64'),'demo-tab1.png');
  const submitted = await action('demo-step',4);
  assert.equal(submitted.detail.submitted,'已提交：Hello from CLI');
  assert.equal(submitted.detail.trustedInput,true); assert.equal(submitted.detail.trustedSubmit,true);
  assert.equal((await ui('window.iabExample.state()')).selected,second.tab2);
  const pageLayout = await evaluate(second.tab2,'(()=>{const controls=[...document.querySelectorAll(".control")].map(e=>{const r=e.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height}});return {width:innerWidth,scrollWidth:document.documentElement.scrollWidth,controls,background:getComputedStyle(document.body).backgroundColor}})()');
  assert.equal(pageLayout.controls.length,2); assert.equal(pageLayout.background,'rgb(255, 255, 255)');
  assert.ok(pageLayout.scrollWidth<=pageLayout.width && pageLayout.controls.every(control=>control.width>0 && control.height>0));
  assert.ok(pageLayout.controls[1].y>=pageLayout.controls[0].y+pageLayout.controls[0].height);
  const captureTab2 = await scope.call('executeCdp',{target:{tabId:second.tab2},method:'Page.captureScreenshot',commandParams:{format:'png'}});
  await savePage(Buffer.from(captureTab2.data,'base64'),'demo-tab2.png');
  const third = await action('demo-step',5);
  assert.notEqual(third.tab3,first.tab1); assert.notEqual(third.tab3,second.tab2);
  assert.equal(third.detail.url,`${fixture.url}/github-profile`);
  assert.equal((await ui('window.iabExample.state()')).selected,third.tab3);
  assert.equal(await evaluate(third.tab3,'location.pathname'),'/github-profile');
  assert.ok(scope.tabs.list().some(tab=>tab.id===first.tab1) && scope.tabs.list().some(tab=>tab.id===second.tab2));
  assert.equal(await evaluate(first.tab1,'document.querySelector("#count").textContent'),'1');
  assert.equal(await evaluate(second.tab2,'document.querySelector("#submitted").textContent'),'已提交：Hello from CLI');
  check('CLI private demo: third tab opens independently, existing page state is retained');
  assert.equal(scope.tabs.list().length,3);
  await assert.rejects(action('demo-step',6));
  await waitFor(()=>ui("document.querySelector('#step-progress').textContent==='5 / 5'"));
  check('CLI private demo: five manual steps retain all three tabs and trusted input results');
  // Use real button clicks after completion, not only direct IPC calls: an
  // enabled backend alone would miss a renderer that still locks old steps.
  const clickStep = async number => {
    await waitFor(()=>ui(`!document.querySelector('[data-step="${number}"]').disabled`));
    await ui(`document.querySelector('[data-step="${number}"]').click();true`);
  };
  const waitIdle = () => waitFor(async()=>!(await ui('window.iabExample.state()')).demo.busy,30_000);
  for (const count of [2,3]) {
    await clickStep(3);
    await waitFor(()=>evaluate(first.tab1,`document.querySelector('#count').textContent==='${count}'`),30_000);
    await waitIdle();
  }
  await evaluate(second.tab2,"window.__repeatSubmits=0;document.querySelector('#demo-form').addEventListener('submit',()=>window.__repeatSubmits++);true");
  for (const count of [1,2]) {
    await clickStep(4);
    await waitFor(()=>evaluate(second.tab2,`window.__repeatSubmits===${count}`),30_000);
    await waitIdle();
    assert.equal(await evaluate(second.tab2,"document.querySelector('#message').value"),'Hello from CLI');
  }
  let latest=third;
  for (const [step,key] of [[1,'tab1'],[2,'tab2'],[5,'tab3']]) {
    const previous=latest[key];
    await clickStep(step);
    await waitFor(async()=>{const demo=(await ui('window.iabExample.state()')).demo;return demo[key]!==previous && !demo.busy},30_000);
    latest=(await ui('window.iabExample.state()')).demo;
    assert.notEqual(latest[key],previous);
  }
  assert.equal(scope.tabs.list().length,6);
  assert.equal(await evaluate(first.tab1,"document.querySelector('#count').textContent"),'3');
  assert.equal(await evaluate(second.tab2,'window.__repeatSubmits'),2);
  assert.equal(await evaluate(latest.tab1,"document.querySelector('#count').textContent"),'0');
  assert.equal(await evaluate(latest.tab3,'location.pathname'),'/github-profile');
  assert.deepEqual(latest.completed,[1,2,3,4,5]); assert.equal(latest.results.length,5);
  await action('close',latest.tab1);
  await assert.rejects(action('demo-step',3),/请先点击.*Tab 1/);
  await clickStep(1);
  await waitFor(async()=>{const demo=(await ui('window.iabExample.state()')).demo;return demo.tab1!==latest.tab1 && !demo.busy},30_000);
  assert.equal(scope.tabs.list().length,6);
  await waitFor(()=>ui("document.querySelector('#step-progress').textContent==='5 / 5' && [...document.querySelectorAll('[data-step]')].every(step=>!step.disabled)"));
  check('manual actions: all five repeat via real buttons, new tabs preserve prior results, missing targets stay guarded');
  const automated = await action('demo-play');
  assert.deepEqual(automated.completed,[1,2,3,4,5]);
  assert.equal(automated.results.find(result=>result.step===3).detail.count,1);
  assert.equal(automated.results.find(result=>result.step===4).detail.trustedSubmit,true);
  assert.equal(automated.results.find(result=>result.step===5).detail.url,`${fixture.url}/github-profile`);
  assert.equal(scope.tabs.list().length,3);
  assert.ok(!scope.tabs.list().some(tab=>[first.tab1,second.tab2,third.tab3].includes(tab.id)));
  assert.equal(await evaluate(automated.tab1,'document.querySelector("#count").textContent'),'1');
  assert.equal(await evaluate(automated.tab2,'document.querySelector("#submitted").textContent'),'已提交：Hello from CLI');
  check('CLI private demo: replay resets old tabs, completes five steps and retains results');
  await waitFor(()=>ui(`!![...document.querySelectorAll('[role="tab"]')].find(tab=>tab.title===${JSON.stringify(`${fixture.url}/Tab%201`)})`));
  await ui(`[...document.querySelectorAll('[role="tab"]')].find(tab=>tab.title===${JSON.stringify(`${fixture.url}/Tab%201`)}).click();true`);
  await waitFor(async()=>(await ui('window.iabExample.state()')).selected===automated.tab1);
  assert.equal(await ui("document.querySelector('#address').disabled"),false);
  await ui(`(()=>{const input=document.querySelector('#address');input.focus();input.value=${JSON.stringify(`${fixture.url}/inspect`)};document.querySelector('#address-form').requestSubmit();input.blur();return true})()`);
  await waitFor(()=>evaluate(automated.tab1,'location.pathname==="/inspect"'));
  await waitFor(()=>ui("!document.querySelector('#back').disabled"));
  await ui("document.querySelector('#back').click();true"); await waitFor(()=>evaluate(automated.tab1,'location.pathname==="/Tab%201"'));
  await waitFor(()=>ui("!document.querySelector('#forward').disabled"));
  await ui("document.querySelector('#forward').click();true"); await waitFor(()=>evaluate(automated.tab1,'location.pathname==="/inspect"'));
  await evaluate(automated.tab1,'window.__beforeReload=true');
  await ui("document.querySelector('#reload').click();true"); await waitFor(()=>evaluate(automated.tab1,'window.__beforeReload===undefined && document.readyState==="complete"'));
  assert.equal(await evaluate(automated.tab2,'document.querySelector("#submitted").textContent'),'已提交：Hello from CLI');
  check('retained tabs: real tab button, address form, back/forward and refresh remain usable');
  const manualOpen = await action('demo-step',1);
  assert.deepEqual(manualOpen.completed,[1,2,3,4,5]); assert.equal(scope.tabs.list().length,4);
  assert.ok(scope.tabs.list().some(tab=>tab.id===automated.tab2));
  await ui("window.__demoPlayback=window.iabExample.action('demo-play');true");
  await waitFor(async()=>{const d=(await ui('window.iabExample.state()')).demo;return d.playing && d.completed.includes(1) && d.tab1!==manualOpen.tab1});
  await assert.rejects(action('demo-step',2));
  await action('demo-stop');
  const stopped = await ui('window.__demoPlayback');
  assert.equal(stopped.playing,false);
  const idle = (await ui('window.iabExample.state()')).demo;
  assert.equal(idle.playing,false); assert.equal(idle.busy,false); assert.ok(idle.completed.length<5);
  assert.ok(scope.tabs.list().length>0);
  check('CLI private demo: stop completes safely and preserves the opened pages');
  await ui("window.__demoPlayback=window.iabExample.action('demo-play');true");
  await waitFor(async()=>{const d=(await ui('window.iabExample.state()')).demo;return d.playing && d.current===3});
  const active=(await ui('window.iabExample.state()')).demo;
  assert.equal(await ui("document.querySelector('#address').disabled"),false);
  assert.equal(await ui("[...document.querySelectorAll('[role=tab]')].some(tab=>tab.disabled)"),false);
  await ui(`[...document.querySelectorAll('[role="tab"]')].find(tab=>tab.title===${JSON.stringify(`${fixture.url}/Tab%202`)}).click();true`);
  await waitFor(()=>ui(`document.querySelector('[role=tab][aria-selected=true]')?.title===${JSON.stringify(`${fixture.url}/Tab%202`)}`));
  await ui(`(()=>{const input=document.querySelector('#address');input.focus();input.value=${JSON.stringify(`${fixture.url}/user-controlled`)};document.querySelector('#address-form').requestSubmit();input.blur();return true})()`);
  await action('select',active.tab1);
  const takenOver=await ui('window.__demoPlayback');
  await waitFor(()=>evaluate(active.tab2,'location.pathname==="/user-controlled"'));
  assert.ok(!takenOver.completed.includes(4));
  assert.equal(await evaluate(active.tab1,'document.querySelector("#count").textContent'),'1');
  check('manual takeover: switch during playback, navigate captured tab after current step, cancel remaining steps');
  return { passed:true, electron:process.versions.electron, platform:process.platform, checks, tabs:scope.tabs.list().length };
}
