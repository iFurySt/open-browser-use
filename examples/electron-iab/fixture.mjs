import { createServer } from 'node:http';
const escape = value => String(value).replace(/[&<>"']/g, c => ({ '&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;', "'":'&#39;' }[c]));
export async function startFixture() {
  const server = createServer((request, response) => {
    const url = new URL(request.url, 'http://localhost');
    response.setHeader('Content-Type', 'text/html; charset=utf-8');
    if (url.pathname==='/strict-csp') response.setHeader('Content-Security-Policy',"default-src 'self'; style-src 'none'; img-src 'none'; script-src 'unsafe-inline'");
    if (url.pathname === '/login') response.setHeader('Set-Cookie', 'iab_example=logged-in; Path=/; SameSite=Lax');
    let name = url.pathname.slice(1) || 'Welcome';
    try { name = decodeURIComponent(name); } catch {}
    const title = escape(name);
    response.end(`<!doctype html><html><head><meta charset="utf-8"><title>${title} · IAB</title>
      <style>body{font:17px system-ui;color:#183b35;background:#f3faf7;margin:0;padding:44px}h1{font-size:38px}input,button{font:inherit;padding:12px;border:1px solid #9ab9af;border-radius:8px}button{background:#d7efe2;cursor:pointer}article{background:white;padding:28px;border-radius:16px;max-width:660px}label{display:block;margin:24px 0 10px}small{color:#50746a}</style></head>
      <body><small>OPEN BROWSER USE · LOCAL TEST PAGE</small><h1>${title}</h1><article>
      <p>Each tab owns a real page. Tasks can share a named login profile.</p>
      <form id="demo-form"><label for="message">Try typing with the agent</label><input id="message" placeholder="Your message"><button id="submit" type="submit">Submit</button><p id="submitted" role="status">Not submitted</p></form>
      <p><button id="increment">Increment</button> <strong id="count">0</strong></p>
      <button id="popup">Open a popup tab</button><p id="status">Ready</p>
      <p><a href="/login">Set demo login cookie</a> · <a href="/second">Visit another page</a></p></article>
      <script>
      window.trustedInput=false;
      window.trustedSubmit=false;
      document.querySelector('#demo-form').onsubmit=e=>{e.preventDefault();window.trustedSubmit=e.isTrusted;document.querySelector('#submitted').textContent='Submitted: '+document.querySelector('#message').value};
      document.querySelector('#message').addEventListener('input',e=>{window.trustedInput=e.isTrusted});
      document.querySelector('#increment').onclick=e=>{window.trustedClick=e.isTrusted;const n=document.querySelector('#count');n.textContent=Number(n.textContent)+1};
      document.querySelector('#popup').onclick=()=>window.open('/popup','_blank');
      window.addEventListener('message',e=>{if(e.origin===location.origin)document.querySelector('#status').textContent=e.data});
      if(window.opener) window.opener.postMessage('Popup connected',location.origin);
      </script></body></html>`);
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  return { server, url: `http://127.0.0.1:${server.address().port}`, close: () => new Promise(resolve => server.close(resolve)) };
}
