import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
const escape = value => String(value).replace(/[&<>"']/g, c => ({ '&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;', "'":'&#39;' }[c]));
export async function startFixture() {
  const stylesheet = await readFile(new URL('./fixture.css',import.meta.url));
  const slowResponses = new Map();
  const renderPage = (url,response) => {
    response.setHeader('Content-Type', 'text/html; charset=utf-8');
    if (url.pathname==='/strict-csp') response.setHeader('Content-Security-Policy',"default-src 'self'; style-src 'none'; img-src 'none'; script-src 'unsafe-inline'");
    if (url.pathname === '/login') response.setHeader('Set-Cookie', 'iab_example=logged-in; Path=/; SameSite=Lax');
    let name = url.pathname.slice(1) || 'Welcome';
    try { name = decodeURIComponent(name); } catch {}
    const title = escape(name);
    response.end(`<!doctype html><html><head><meta charset="utf-8"><title>${title} · IAB</title>
      <link rel="stylesheet" href="/fixture.css"></head>
      <body><main class="page"><h1>${title}</h1><div class="controls">
      <section class="control"><h2>计数</h2><div class="count-row"><strong id="count">0</strong><button id="increment">加 1</button></div></section>
      <section class="control"><h2><label for="message">消息</label></h2><form id="demo-form"><input id="message" placeholder="输入消息" autocomplete="off"><button id="submit" type="submit">提交</button></form><p id="submitted" class="message-status" role="status"></p></section>
      </div><details><summary>更多</summary><button id="popup">打开新标签</button><p id="status" hidden>Ready</p></details></main>
      <script>
      window.trustedInput=false;
      window.trustedSubmit=false;
      document.querySelector('#demo-form').onsubmit=e=>{e.preventDefault();window.trustedSubmit=e.isTrusted;document.querySelector('#submitted').textContent='已提交：'+document.querySelector('#message').value;document.querySelector('#submitted').dataset.state='sent'};
      document.querySelector('#message').addEventListener('input',e=>{window.trustedInput=e.isTrusted});
      document.querySelector('#increment').onclick=e=>{window.trustedClick=e.isTrusted;const n=document.querySelector('#count');n.textContent=Number(n.textContent)+1};
      document.querySelector('#popup').onclick=()=>window.open('/popup','_blank');
      window.addEventListener('message',e=>{if(e.origin===location.origin){document.querySelector('#status').textContent=e.data;document.querySelector('#status').hidden=false}});
      if(window.opener) window.opener.postMessage('Popup connected',location.origin);
      </script></body></html>`);
  };
  const server = createServer((request,response)=>{
    const url=new URL(request.url,'http://localhost');
    if (url.pathname==='/fixture.css') { response.setHeader('Content-Type','text/css; charset=utf-8'); response.end(stylesheet); return; }
    if (url.pathname==='/slow') {
      slowResponses.set(response,url); response.once('close',()=>slowResponses.delete(response)); return;
    }
    renderPage(url,response);
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  return { server, releaseSlowPages:()=>{for (const [response,url] of slowResponses) renderPage(url,response);slowResponses.clear();}, url: `http://127.0.0.1:${server.address().port}`, close: () => new Promise(resolve => server.close(resolve)) };
}
