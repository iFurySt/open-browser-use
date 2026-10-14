import electron from 'electron';
import { IabRuntime } from './runtime.js';
export { IabRuntime, IabScope } from './runtime.js';
export type { GrantOptions, Operation, Bootstrap } from './transport.js';
export function createIabRuntime(options: { appId: string }): IabRuntime {
  if (!electron.app?.isReady()) throw new Error('createIabRuntime must run in Electron main after app.whenReady()');
  const runtime = new IabRuntime({
    createSession: partition => electron.session.fromPartition(partition),
    createView: (session, contents) => new electron.WebContentsView({
      ...(contents ? { webContents: contents } : {}),
      webPreferences: { session, sandbox: true, contextIsolation: true, nodeIntegration: false, webSecurity: true, webviewTag: false },
    }),
  }, options.appId);
  return runtime;
}
