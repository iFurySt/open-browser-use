import { once } from 'node:events';
import { join } from 'node:path';

// Two reference cases, each with private/socket transport. The example UI uses
// only launchCli('private', ...); smoke continues to exercise all four paths.
export function createExampleClients({ runtime, scopeId, cliPath, directory }) {
  async function launch(options) {
    const { child } = await runtime.launchClient({ scopeId, ttlMs:30_000, ...options });
    let output = '', errors = '';
    const append = (key, chunk) => {
      if (key === 'out') output += chunk; else errors += chunk;
      if (output.length + errors.length > 1_048_576) child.kill();
    };
    child.stdout.on('data',chunk=>append('out',chunk));
    child.stderr.on('data',chunk=>append('err',chunk));
    const timer = setTimeout(()=>child.kill(),30_000);
    try {
      const [code] = await once(child,'close');
      if (code !== 0) throw new Error(`Client failed: ${errors.slice(0,1000)}`);
      return JSON.parse(output);
    } finally { clearTimeout(timer); }
  }
  return {
    launchNode: (transport,url) => launch({transport,command:process.execPath,args:[join(directory,'worker.mjs'),url],env:{...process.env,ELECTRON_RUN_AS_NODE:'1'}}),
    launchCli: (transport,args) => launch({transport,command:cliPath,args:['iab',...args]}),
    // Hand these pipes to the host agent's MCP client. FD 3 remains private.
    launchMcp: (transport = 'private', options = {}) => runtime.launchClient({
      scopeId, ttlMs:60_000, ...options, transport,
      command:cliPath, args:['iab','mcp'], stdin:'pipe',
    }),
  };
}
