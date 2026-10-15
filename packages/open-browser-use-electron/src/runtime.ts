import { randomUUID } from 'node:crypto';
import { spawn, type ChildProcess } from 'node:child_process';
import { once, EventEmitter } from 'node:events';
import type { Duplex } from 'node:stream';
import type { WebContentsView, WebContents, Session, BaseWindow } from 'electron';
import { IabConnections, type Backend, type IabEvent, type Operation, type GrantOptions } from './transport.js';
import { frame } from './wire.js';
import { FeedbackController, type VisualFeedbackOptions } from './feedback.js';

export type ElectronAdapter = {
  createView(session: Session, contents?: WebContents): WebContentsView;
  createSession(partition: string): Session;
};
type Tab = {
  id: number; scopeId: string; view: WebContentsView; contents: WebContents; lifetime: 'temporary' | 'persistent';
  touched: Set<string>; marks: Map<string, 'handoff' | 'deliverable'>;
  attached: boolean; mouseReady: boolean; pending: number; queue: Promise<any>; removers: (() => void)[];
  loading: boolean; pendingUrl?: string; loadError?: { code: number; description: string; url: string };
};
type ScopeState = { id: string; profileId: string; tabs: Map<number, Tab>; name: string; closing?: boolean };
type Surface = { window: BaseWindow; owner: WebContents; tabId?: number; bounds: { x: number; y: number; width: number; height: number }; remove: () => void };
const pageCommands = new Set([
  'Page.enable', 'Page.disable', 'Page.navigate', 'Page.reload', 'Page.stopLoading',
  'Page.getNavigationHistory', 'Page.navigateToHistoryEntry', 'Page.captureScreenshot',
  'Page.getLayoutMetrics', 'Page.close',
  'Runtime.enable', 'Runtime.disable', 'Runtime.evaluate', 'Runtime.callFunctionOn',
  'Runtime.getProperties', 'Runtime.releaseObject', 'Runtime.releaseObjectGroup',
  'DOM.enable', 'DOM.disable', 'DOM.getDocument', 'DOM.describeNode', 'DOM.querySelector',
  'DOM.querySelectorAll', 'DOM.getBoxModel', 'DOM.getContentQuads', 'DOM.getOuterHTML',
  'DOM.resolveNode', 'DOM.focus', 'DOM.scrollIntoViewIfNeeded',
  'Accessibility.enable', 'Accessibility.disable', 'Accessibility.getFullAXTree',
  'Input.dispatchMouseEvent', 'Input.dispatchKeyEvent', 'Input.insertText', 'Input.synthesizeScrollGesture',
  'Network.enable', 'Network.disable', 'Network.getResponseBody',
]);
export function assertUrl(value: unknown): string {
  if (value === 'about:blank') return value;
  if (typeof value !== 'string') throw new Error('Invalid URL');
  const url = new URL(value);
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password) throw new Error('IAB navigation only permits http, https and about:blank');
  return value;
}

export class IabRuntime implements Backend {
  readonly instanceId = randomUUID();
  readonly connections = new IabConnections(this);
  private events = new EventEmitter();
  private profileSessions = new Map<string, Session>();
  private sessionRemovers: (() => void)[] = [];
  private states = new Map<string, ScopeState>();
  private scopeDisposals = new Map<string, Promise<void>>();
  private tabsById = new Map<number, Tab>();
  private surfaceStates = new Map<string, Surface>();
  private workers = new Set<ChildProcess>();
  private nextTabId = 1;
  private closed = false;
  private disposing?: Promise<void>;
  private feedbackController: FeedbackController;
  constructor(private adapter: ElectronAdapter, readonly appId: string, feedback: VisualFeedbackOptions = {}) {
    this.feedbackController = new FeedbackController(feedback);
    if (!appId || appId.length > 100) throw new Error('appId is required');
    this.events.setMaxListeners(256);
  }
  readonly visualFeedback = {
    configure: (options: VisualFeedbackOptions) => { this.ensureOpen(); return this.feedbackController.configure(options); },
    getOptions: () => this.feedbackController.configuration,
  };
  async setFeedbackActive(scopeId: string,tabId: number,active: boolean): Promise<void> {
    this.ensureOpen(); await this.feedbackController.setActive(this.tab(scopeId,tabId).contents,active);
  }
  readonly profilesApi = {
    open: (options: { id: string; persistent?: boolean }): { id: string } => {
      this.ensureOpen();
      if (!options.id || options.id.length > 100) throw new Error('Invalid profile id');
      const key = `${options.persistent === false ? '' : 'persist:'}obu-iab:${encodeURIComponent(this.appId)}:${encodeURIComponent(options.id)}`;
      if (!this.profileSessions.has(options.id)) {
        const session = this.adapter.createSession(key);
        session.setPermissionRequestHandler((_wc, _permission, callback) => callback(false));
        session.setPermissionCheckHandler(() => false);
        const denyDownload = (event: { preventDefault(): void }) => event.preventDefault();
        session.on('will-download', denyDownload);
        this.sessionRemovers.push(() => session.off('will-download', denyDownload));
        this.profileSessions.set(options.id, session);
      } else if ((this.profileSessions.get(options.id)!.isPersistent?.() ?? options.persistent !== false) !== (options.persistent !== false)) {
        throw new Error('Cannot change profile persistence');
      }
      return { id: options.id };
    },
  };
  readonly profiles = this.profilesApi;
  readonly scopes = {
    open: (options: { id: string; profileId: string }): IabScope => {
      this.ensureOpen();
      if (!options.id || options.id.length > 200 || !this.profileSessions.has(options.profileId)) throw new Error('Scope requires a registered profile');
      const previous = this.states.get(options.id);
      if (previous?.closing) throw new Error('IAB_SCOPE_CLOSED');
      if (previous && previous.profileId !== options.profileId) throw new Error('Cannot change scope profile');
      if (!previous) this.states.set(options.id, { ...options, tabs: new Map(), name: options.id });
      return new IabScope(this, options.id);
    },
  };
  hasScope(id: string): boolean { return !this.closed && !this.disposing && this.states.has(id) && !this.states.get(id)!.closing; }
  subscribe(handler: (event: IabEvent) => void): () => void { this.events.on('event', handler); return () => { this.events.off('event', handler); }; }
  private emit(scopeId: string, method: string, params: Record<string, any>): void { this.events.emit('event', { scopeId, method, params }); }
  private ensureOpen(): void { if (this.closed || this.disposing) throw new Error('IAB_DISPOSED'); }
  private scope(id: string): ScopeState { const s = this.states.get(id); if (!s || s.closing || this.closed) throw new Error('IAB_SCOPE_CLOSED'); return s; }
  private tab(scopeId: string, id: unknown): Tab {
    if (!Number.isSafeInteger(id)) throw new Error('Explicit numeric tabId required');
    const tab = this.scope(scopeId).tabs.get(id as number);
    if (!tab || tab.contents.isDestroyed()) throw new Error('IAB_TAB_NOT_OWNED');
    return tab;
  }
  private snapshot(tab: Tab): Record<string, any> {
    return { id: tab.id, title: tab.contents.getTitle(), url: tab.contents.getURL(),
        active: [...this.surfaceStates.values()].some(s => s.tabId === tab.id), lifetime: tab.lifetime,
        loading: tab.loading, pendingUrl: tab.pendingUrl, loadError: tab.loadError };
  }
  private allocate(scopeId: string, contents?: WebContents): Tab {
    const scope = this.scope(scopeId);
    if (scope.tabs.size >= 32 || this.tabsById.size >= 128) throw new Error('IAB_TAB_LIMIT');
    const profile = this.profileSessions.get(scope.profileId)!;
    if (contents && contents.session !== profile) throw new Error('Popup profile mismatch');
    const view = this.adapter.createView(profile, contents);
    view.setBounds({ x: 0, y: 0, width: 800, height: 600 }); view.setVisible(false);
    const tab: Tab = { id: this.nextTabId++, scopeId, view, contents: view.webContents, lifetime: 'temporary', touched: new Set(), marks: new Map(), attached: false, mouseReady: false, pending: 0, queue: Promise.resolve(), removers: [], loading: false };
    scope.tabs.set(tab.id, tab); this.tabsById.set(tab.id, tab);
    const wc = view.webContents;
    this.feedbackController.register(wc);
    const updated = () => { if (!wc.isDestroyed()) this.emit(scopeId,'onTabUpdated',{tab:this.snapshot(tab)}); };
    const listen = (target: EventEmitter, name: string, listener: (...args: any[]) => void) => {
      target.on(name, listener); tab.removers.push(() => target.off(name, listener));
    };
    const checkNavigation = (event: { preventDefault(): void }, urlOrDetails: string | { url: string }) => {
      try { assertUrl(typeof urlOrDetails === 'string' ? urlOrDetails : urlOrDetails.url); }
      catch { event.preventDefault(); }
    };
    listen(wc, 'will-navigate', checkNavigation); listen(wc, 'will-frame-navigate', checkNavigation); listen(wc, 'will-redirect', checkNavigation);
    listen(wc, 'destroyed', () => this.forget(tab));
    const navigating = (details: any,url: string,_inPlace: boolean,isMainFrame: boolean) => {
      if (!(details.isMainFrame ?? isMainFrame)) return;
      const target=details.url ?? url;
      if (target?.startsWith('chrome-error:')) return;
      tab.pendingUrl=(details.isSameDocument ?? _inPlace)?undefined:target; tab.loadError=undefined; updated();
    };
    listen(wc,'did-start-navigation',navigating);
    listen(wc,'did-redirect-navigation',navigating);
    listen(wc,'did-start-loading',()=>{tab.loading=true;updated();});
    listen(wc,'did-stop-loading',()=>{tab.loading=false;tab.pendingUrl=undefined;updated();});
    listen(wc,'did-fail-load',(_event: any,code: number,description: string,url: string,isMainFrame: boolean)=>{
      if (!isMainFrame || code===-3) return; // Superseded/stopped navigation is not a site error.
      tab.loadError={code,description,url}; updated();
    });
    listen(wc, 'did-finish-load', () => { tab.mouseReady = false; });
    listen(wc, 'did-finish-load', () => this.feedbackController.resetDocument(wc));
    listen(wc, 'did-navigate-in-page', () => this.feedbackController.resetDocument(wc));
    listen(wc, 'page-title-updated', () => this.emit(scopeId, 'onTabUpdated', { tab: this.snapshot(tab) }));
    for (const event of ['did-navigate', 'did-navigate-in-page', 'did-stop-loading']) {
      listen(wc, event, () => { if (!wc.isDestroyed()) this.emit(scopeId, 'onTabUpdated', { tab: this.snapshot(tab) }); });
    }
    listen(wc, 'render-process-gone', () => { void this.destroyTab(tab).catch(() => {}); });
    listen(wc.debugger, 'message', (_event, method, params, sessionId) => {
      if (!sessionId && this.states.get(scopeId)?.tabs.has(tab.id)) this.emit(scopeId, 'onCDPEvent', { target: { tabId: tab.id }, method, params });
    });
    listen(wc.debugger, 'detach', () => { tab.attached = false; this.emit(scopeId, 'onCDPDetach', { tabId: tab.id }); });
    wc.setWindowOpenHandler(details => {
      try { assertUrl(details.url); } catch { return { action: 'deny' }; }
      if (!this.hasScope(scopeId) || scope.tabs.size >= 32 || this.tabsById.size >= 128) return { action: 'deny' };
      return { action: 'allow', createWindow: options => {
        // Electron internally supplies a precreated WebContents for some popup
        // dispositions; preserve it when present to retain opener semantics.
        const contents = (options as typeof options & { webContents?: WebContents }).webContents;
        const popup = this.allocate(scopeId, contents);
        return popup.contents;
      } };
    });
    this.emit(scopeId, 'onTabCreated', { tab: this.snapshot(tab) }); return tab;
  }
  async createTab(scopeId: string, options: { url?: string; turnId?: string } = {}): Promise<Record<string, any>> {
    const url = assertUrl(options.url ?? 'about:blank');
    const tab = this.allocate(scopeId);
    if (options.turnId) tab.touched.add(options.turnId);
    try { await tab.contents.loadURL(url); return this.snapshot(tab); }
    catch (error) { await this.closeTab(scopeId, tab.id); throw error; }
  }
  listTabs(scopeId: string): Record<string, any>[] { return [...this.scope(scopeId).tabs.values()].map(t => this.snapshot(t)); }
  private forget(tab: Tab): void {
    if (!this.tabsById.delete(tab.id)) return;
    this.feedbackController.release(tab.contents);
    this.states.get(tab.scopeId)?.tabs.delete(tab.id);
    for (const surface of this.surfaceStates.values()) if (surface.tabId === tab.id) {
      if (!surface.window.isDestroyed?.()) surface.window.contentView.removeChildView(tab.view);
      surface.tabId = undefined;
    }
    for (const remove of tab.removers) remove(); tab.removers = [];
    this.emit(tab.scopeId, 'onTabClosed', { tabId: tab.id });
  }
  async closeTab(scopeId: string, tabId: number): Promise<void> {
    await this.destroyTab(this.tab(scopeId, tabId));
  }
  private async destroyTab(tab: Tab): Promise<void> {
    this.forget(tab);
    if (!tab.contents.isDestroyed()) {
      const done = once(tab.contents, 'destroyed');
      tab.contents.close({ waitForBeforeUnload: false });
      await done;
    }
  }
  private async cdp(tab: Tab, method: string, params: Record<string, any>, authorized: () => boolean): Promise<any> {
    if (!pageCommands.has(method)) throw new Error('IAB CDP command is outside page permissions');
    if (params.includeCommandLineAPI) throw new Error('DevTools command-line helpers are not exposed to page grants');
    if (method === 'Page.navigate') assertUrl(params.url);
    if (method === 'Page.close') { await this.closeTab(tab.scopeId, tab.id); return {}; }
    if (!tab.contents.debugger.isAttached()) tab.contents.debugger.attach('1.3');
    tab.attached = true;
    // A never-presented native view can have DOM layout but no compositor hit
    // test data. Render one frame before mouse input without presenting/focusing
    // the tab. Real Electron smoke verifies the resulting event is trusted.
    if ((method === 'Input.dispatchMouseEvent' || method === 'Input.synthesizeScrollGesture') && !tab.mouseReady) {
      await tab.contents.debugger.sendCommand('Page.captureScreenshot', { format: 'png' });
      tab.mouseReady = true;
    }
    if (!authorized()) throw new Error('IAB_AUTH_REVOKED');
    if (method.startsWith('Input.')) await this.feedbackController.input(tab.contents,method,params);
    if (!authorized()) throw new Error('IAB_AUTH_REVOKED');
    const result = await tab.contents.debugger.sendCommand(method, params);
    if (method === 'Page.captureScreenshot') tab.mouseReady = true;
    if (['Page.navigate', 'Page.reload', 'Page.navigateToHistoryEntry'].includes(method)) tab.mouseReady = false;
    return result;
  }
  async dispatch(scopeId: string, method: string, params: Record<string, any> = {}, operations: readonly Operation[] = ['tabs.read', 'tabs.create', 'page.control'], authorized: () => boolean = () => true): Promise<any> {
    this.ensureOpen();
    if (!authorized()) throw new Error('IAB_AUTH_REVOKED');
    const scope = this.scope(scopeId);
    if (params.session_id !== undefined && params.session_id !== scopeId) throw new Error('IAB_SCOPE_MISMATCH');
    const need = (operation: Operation) => { if (!operations.includes(operation)) throw new Error('IAB_OPERATION_DENIED'); };
    switch (method) {
      case 'getInfo': return { name: 'Open Browser Use IAB', type: 'iab', version: '1.0.0', metadata: { instanceId: this.instanceId, scopeId },
        capabilities: { multiTab: true, fileUploads: false, downloads: false, clipboard: false, history: false, childTargets: false } };
      case 'ping': return 'pong';
      case 'getTabs': case 'getUserTabs': need('tabs.read'); return this.listTabs(scopeId);
      case 'createTab': need('tabs.create'); return this.createTab(scopeId, { url: params.url, turnId: params.turn_id });
      case 'nameSession': need('page.control'); scope.name = String(params.name).slice(0, 200); return { name: scope.name };
      case 'finalizeTabs': {
        need('page.control');
        if (!Array.isArray(params.keep)) throw new Error('keep must be an array');
        const kept = new Map<number, 'handoff' | 'deliverable'>();
        for (const item of params.keep) {
          const tab = this.tab(scopeId, item.tabId ?? item.id);
          if (!['handoff', 'deliverable'].includes(item.status)) throw new Error('Invalid disposition');
          kept.set(tab.id, item.status);
        }
        for (const t of [...scope.tabs.values()]) {
          if (kept.has(t.id)) { t.lifetime = 'persistent'; t.touched.clear(); t.marks.clear(); }
          else if (t.pending === 0) await this.closeTab(scopeId, t.id);
        }
        return this.listTabs(scopeId);
      }
      case 'turnEnded': {
        need('page.control');
        if (typeof params.turn_id !== 'string') throw new Error('turn_id required');
        for (const t of [...scope.tabs.values()]) if (t.touched.delete(params.turn_id)) {
          const mark = t.marks.get(params.turn_id); t.marks.delete(params.turn_id);
          if (mark) t.lifetime = 'persistent';
          if (!mark && t.lifetime === 'temporary' && t.touched.size === 0 && t.pending === 0) await this.closeTab(scopeId, t.id);
        }
        return this.listTabs(scopeId);
      }
    }
    need('page.control');
    const tab = this.tab(scopeId, params.tabId ?? params.target?.tabId);
    if (typeof params.turn_id === 'string') tab.touched.add(params.turn_id);
    if (method === 'markTab') {
      if (!['handoff', 'deliverable'].includes(params.status) || typeof params.turn_id !== 'string') throw new Error('Invalid mark');
      tab.marks.set(params.turn_id, params.status); return {};
    }
    if (method === 'attach') {
      if (!tab.contents.debugger.isAttached()) tab.contents.debugger.attach('1.3'); tab.attached = true; return {};
    }
    if (method === 'detach') { if (tab.attached) tab.contents.debugger.detach(); tab.attached = false; return {}; }
    if (method !== 'executeCdp') throw new Error('IAB method not supported');
    if (typeof params.method !== 'string' || params.target?.sessionId || params.sessionId) throw new Error('Invalid CDP target');
    tab.pending++;
    const run = tab.queue.then(() => {
      if (!authorized()) throw new Error('IAB_AUTH_REVOKED');
      return this.cdp(this.tab(scopeId, tab.id), params.method, params.commandParams ?? {}, authorized);
    });
    tab.queue = run.catch(() => {});
    try { return await run; } finally { tab.pending--; }
  }
  readonly surfaces = {
    register: (options: { id: string; window: BaseWindow; owner: WebContents }) => {
      this.ensureOpen();
      if (this.surfaceStates.has(options.id)) throw new Error('Surface already registered');
      const surface: Surface = { ...options, bounds: { x: 0, y: 0, width: 800, height: 600 }, remove: () => {} };
      const unbind = () => {
        const tab = surface.tabId === undefined ? undefined : this.tabsById.get(surface.tabId);
        if (tab) {
          if (!surface.window.isDestroyed?.()) surface.window.contentView.removeChildView(tab.view);
          tab.view.setVisible(false);
          tab.mouseReady = false;
        }
        surface.tabId = undefined;
      };
      const closed = () => { unbind(); this.surfaceStates.delete(options.id); options.window.off('closed', closed); };
      surface.remove = closed; options.window.on('closed', closed); this.surfaceStates.set(options.id, surface);
      return {
        present: (tabId: number) => {
          const tab = this.tabsById.get(tabId); if (!tab) throw new Error('Unknown tab');
          for (const other of this.surfaceStates.values()) if (other !== surface && other.tabId === tabId) throw new Error('Tab is already displayed in another surface');
          unbind(); surface.tabId = tabId; surface.window.contentView.addChildView(tab.view); tab.view.setBounds(surface.bounds); tab.view.setVisible(true); tab.mouseReady = false;
        },
        setBounds: (bounds: Surface['bounds']) => {
          if (Object.values(bounds).some(v => !Number.isSafeInteger(v) || v < 0 || v > 16384) || bounds.width < 1 || bounds.height < 1) throw new Error('Invalid bounds');
          surface.bounds = { ...bounds };
          if (surface.tabId !== undefined) {
            const tab = this.tabsById.get(surface.tabId);
            if (tab) { tab.view.setBounds(surface.bounds); tab.mouseReady = false; }
          }
        },
        hide: unbind,
        dispose: closed,
      };
    },
  };
  async launchClient(options: GrantOptions & { scopeId: string; command: string; args?: string[]; env?: NodeJS.ProcessEnv; transport?: 'private' | 'socket' }): Promise<{ child: ChildProcess; revoke: () => void }> {
    this.scope(options.scopeId);
    const transport = options.transport ?? 'private';
    if (transport === 'socket') await this.connections.openSocket();
    const child = spawn(options.command, options.args ?? [], { env: options.env, stdio: ['ignore', 'pipe', 'pipe', 'pipe'], shell: false });
    let grantId: string | undefined;
    try {
      await once(child, 'spawn'); this.workers.add(child);
      const stream = child.stdio[3] as Duplex;
      stream.on('error', () => {});
      if (transport === 'private') grantId = this.connections.servePrivate(stream, options.scopeId, options).grantId;
      else {
        const bootstrap = this.connections.grant(options.scopeId, transport, options, child.pid);
        grantId = bootstrap.grantId; stream.write(frame({ jsonrpc: '2.0', method: 'iab.bootstrap', params: bootstrap }));
      }
      const revoke = () => { if (grantId) this.connections.revoke(grantId); };
      child.once('exit', () => { revoke(); this.workers.delete(child); });
      return { child, revoke };
    } catch (error) {
      if (grantId) this.connections.revoke(grantId);
      this.workers.delete(child); child.kill(); throw error;
    }
  }
  disposeScope(id: string): Promise<void> {
    const previous = this.scopeDisposals.get(id);
    if (previous) return previous;
    const scope = this.states.get(id);
    if (!scope) return Promise.resolve();
    scope.closing = true; this.connections.revokeScope(id);
    const done = (async () => {
      // Closing an opener may synchronously destroy its popup as well.
      for (const tab of [...scope.tabs.values()]) await this.destroyTab(tab);
      this.states.delete(id);
    })().finally(() => this.scopeDisposals.delete(id));
    this.scopeDisposals.set(id, done); return done;
  }
  dispose(): Promise<void> {
    if (!this.disposing) this.disposing = this.finishDispose();
    return this.disposing;
  }
  private async finishDispose(): Promise<void> {
    if (this.closed) return;
    await this.connections.dispose();
    for (const worker of this.workers) worker.kill(); this.workers.clear();
    for (const id of [...this.states.keys()]) await this.disposeScope(id);
    for (const surface of this.surfaceStates.values()) surface.remove();
    for (const remove of this.sessionRemovers) remove(); this.sessionRemovers = [];
    this.profileSessions.clear();
    this.closed = true; this.events.removeAllListeners();
  }
}
export class IabScope {
  constructor(private runtime: IabRuntime, readonly id: string) {}
  readonly tabs = {
    create: (options: { url?: string; turnId?: string } = {}) => this.runtime.createTab(this.id, options),
    list: () => this.runtime.listTabs(this.id),
    close: (id: number) => this.runtime.closeTab(this.id, id),
  };
  readonly visualFeedback = {
    setActive: (tabId: number,active: boolean) => this.runtime.setFeedbackActive(this.id,tabId,active),
  };
  call(method: string, params: Record<string, any> = {}): Promise<any> { return this.runtime.dispatch(this.id, method, params); }
  dispose(): Promise<void> { return this.runtime.disposeScope(this.id); }
}
