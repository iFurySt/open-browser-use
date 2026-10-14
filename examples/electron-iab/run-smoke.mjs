import electron from 'electron';
import { spawn } from 'node:child_process';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const userData = await mkdtemp(join(tmpdir(),'obu-iab-example-'));
const child = spawn(electron, [dirname(fileURLToPath(import.meta.url)), '--smoke'], {
  stdio: 'inherit', env: { ...process.env, OBU_IAB_SMOKE_USER_DATA: userData },
});
for (const signal of ['SIGINT','SIGTERM']) process.once(signal, () => child.kill(signal));
try {
  const code = await new Promise((resolve, reject) => {
    child.once('error',reject); child.once('exit',code=>resolve(code??1));
  });
  process.exitCode = code;
} finally {
  // No live Chromium process can recreate storage during cleanup now.
  await rm(userData,{recursive:true,force:true,maxRetries:5,retryDelay:100});
}
