import { createInterface } from 'node:readline';

// Minimal dependency-free MCP client for this reference app and its smoke.
// Production hosts can give the same child stdin/stdout to their MCP SDK.
export async function connectExampleMcp({ child, revoke }) {
  if (!child.stdin || !child.stdout) throw new Error('MCP requires piped stdin/stdout');
  const lines = createInterface({ input:child.stdout });
  const pending = new Map();
  let sequence = 0, exited = false, errors = '';
  child.stderr?.on('data', chunk => { errors = (errors + chunk).slice(-2000); });
  const fail = error => {
    for (const entry of pending.values()) { clearTimeout(entry.timer); entry.reject(error); }
    pending.clear();
  };
  const closed = new Promise(resolve => child.once('close', code => {
    exited = true; lines.close(); fail(new Error('MCP process closed')); resolve(code);
  }));
  child.on('error', fail);
  child.stdin.on('error', fail);
  lines.on('line', line => {
    try {
      const message = JSON.parse(line), entry = pending.get(message.id);
      if (!entry) return;
      clearTimeout(entry.timer); pending.delete(message.id);
      if (message.error) entry.reject(new Error(message.error.message));
      else entry.resolve(message.result);
    } catch (error) { fail(error); child.kill(); }
  });
  const request = (method, params = {}) => new Promise((resolve,reject) => {
    if (exited) { reject(new Error('MCP process closed')); return; }
    const id = ++sequence;
    const timer = setTimeout(() => { pending.delete(id); reject(new Error('MCP request timed out')); child.kill(); },30_000);
    pending.set(id,{ resolve,reject,timer });
    child.stdin.write(JSON.stringify({jsonrpc:'2.0',id,method,params})+'\n');
  });
  const close = async () => {
    const timer = setTimeout(() => child.kill(),5000);
    try {
      if (!exited) child.stdin.end();
      const code = await closed;
      if (code !== 0) throw new Error(`MCP failed: ${errors}`);
    } finally { clearTimeout(timer); revoke(); }
  };
  try {
    await request('initialize',{protocolVersion:'2025-06-18',capabilities:{},clientInfo:{name:'iab-example',version:'1.0.0'}});
    child.stdin.write(JSON.stringify({jsonrpc:'2.0',method:'notifications/initialized'})+'\n');
    return {request,callTool:(name,args={})=>request('tools/call',{name,arguments:args}),close,revoke};
  } catch (error) { child.kill(); revoke(); throw error; }
}
