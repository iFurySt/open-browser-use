import { createRequire } from 'node:module';
import { spawnSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

// Private channels do not use this addon; Windows socket mode fails closed.
if (process.platform === 'darwin' || process.platform === 'linux') {
  const require = createRequire(import.meta.url);
  const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
  const args = [require.resolve('node-gyp/bin/node-gyp.js'), 'rebuild'];
  const localHeaders = resolve(dirname(process.execPath), '..');
  if (existsSync(resolve(localHeaders, 'include/node/node_api.h'))) args.push(`--nodedir=${localHeaders}`);
  const result = spawnSync(process.execPath, args, { cwd: root, stdio: 'inherit' });
  if (result.error) throw result.error;
  if (result.status !== 0) process.exit(result.status ?? 1);
}
