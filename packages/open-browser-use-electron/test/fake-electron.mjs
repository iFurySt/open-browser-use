import { EventEmitter } from 'node:events';
export function fakeElectron() {
  const sessions = new Map();
  const views = [];
  return { views, sessions,
    createSession(partition) {
      if (!sessions.has(partition)) sessions.set(partition, Object.assign(new EventEmitter(), {
        isPersistent: () => partition.startsWith('persist:'), setPermissionRequestHandler() {}, setPermissionCheckHandler() {},
      }));
      return sessions.get(partition);
    },
    createView(session, contents) {
      const debuggerApi = Object.assign(new EventEmitter(), {
        attached: false, attach() { this.attached = true; }, isAttached() { return this.attached; },
        detach() { this.attached = false; this.emit('detach'); },
        commands: [], async sendCommand(method, params) { this.commands.push({ method, params }); return {}; },
      });
      const wc = contents ?? Object.assign(new EventEmitter(), {
        session, debugger: debuggerApi, url: 'about:blank', destroyed: false,
        getTitle() { return this.url; }, getURL() { return this.url; }, isDestroyed() { return this.destroyed; },
        async loadURL(url) { this.url = url; }, close() { this.destroyed = true; this.emit('destroyed'); },
        isolatedCalls: [], async executeJavaScriptInIsolatedWorld(worldId,scripts) { this.isolatedCalls.push({worldId,scripts}); return true; },
        setWindowOpenHandler(handler) { this.popup = handler; },
      });
      const view = { webContents: wc, setBounds(bounds) { this.bounds = bounds; }, setVisible(visible) { this.visible = visible; } };
      views.push(view); return view;
    },
  };
}
