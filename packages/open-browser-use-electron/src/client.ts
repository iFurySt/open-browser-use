import { Socket, createConnection } from 'node:net';
import type { Duplex } from 'node:stream';
import { OpenBrowserUseClient, OpenBrowserUseBrowser, type BrowserUseRequestParams, type JsonValue, type NotificationHandler } from 'open-browser-use-sdk';
import { FramedChannel, type Message } from './wire.js';
import { peerIdentity, sameIdentity } from './peer.js';
import type { Bootstrap } from './transport.js';

export class IabClient extends OpenBrowserUseClient {
  private channel: FramedChannel;
  private pending = new Map<string, { resolve: (value: any) => void; reject: (error: Error) => void; timer: NodeJS.Timeout }>();
  private handlers = new Set<NotificationHandler>();
  private sequence = 0;
  constructor(channel: FramedChannel, readonly bootstrap: Bootstrap, timeoutMs = 10_000) {
    super({ socketPath: bootstrap.endpoint ?? 'private-channel', sessionId: bootstrap.scopeId, timeoutMs });
    this.bootstrap = { ...bootstrap };
    this.channel = channel;
    channel.on('message', (m: Message) => {
      if (m.id === undefined) {
        if (typeof m.method === 'string') for (const h of this.handlers) h({ method: m.method, params: m.params as JsonValue });
        return;
      }
      const p = this.pending.get(String(m.id));
      if (!p) return;
      clearTimeout(p.timer); this.pending.delete(String(m.id));
      if (m.error) p.reject(new Error(m.error.message)); else p.resolve(m.result);
    });
    channel.on('closed', () => {
      for (const p of this.pending.values()) { clearTimeout(p.timer); p.reject(new Error('IAB connection closed')); }
      this.pending.clear(); this.handlers.clear();
    });
  }
  override async connect(): Promise<this> { if (this.channel.stream.destroyed) throw new Error('IAB connection closed'); return this; }
  override close(): void { this.channel.close(); }
  override onNotification(handler: NotificationHandler): () => void { this.handlers.add(handler); return () => { this.handlers.delete(handler); }; }
  private send(method: string, params: Record<string, any>): Promise<any> {
    if (this.channel.stream.destroyed) return Promise.reject(new Error('IAB connection closed'));
    const id = String(++this.sequence);
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => { this.pending.delete(id); reject(new Error('IAB request timed out')); }, this.timeoutMs);
      this.pending.set(id, { resolve, reject, timer });
      this.channel.send({ jsonrpc: '2.0', id, method, params });
    });
  }
  async authenticate(): Promise<this> {
    const b = this.bootstrap;
    const result = await this.send('auth.connect', { authVersion: b.authVersion, instanceId: b.instanceId, grantId: b.grantId, secret: b.secret });
    if (result?.authenticated !== true || result.scopeId !== this.sessionId) throw new Error('IAB authentication failed');
    this.channel.maxFrame = 8 * 1024 * 1024;
    // Do not keep the lease secret in long-lived client metadata.
    b.secret = ''; return this;
  }
  override request(method: string, params: BrowserUseRequestParams = {}): Promise<JsonValue> {
    if (params.session_id !== undefined && params.session_id !== this.sessionId) return Promise.reject(new Error('IAB scope mismatch'));
    return this.send(method, { ...params, session_id: this.sessionId, turn_id: params.turn_id ?? this.turnId });
  }
  browser(): OpenBrowserUseBrowser { return new OpenBrowserUseBrowser({ client: this }); }
}

export async function connectSocketIab(bootstrap: Bootstrap): Promise<IabClient> {
  if (bootstrap.transport !== 'socket' || !bootstrap.endpoint || !bootstrap.serverIdentity) throw new Error('Invalid private bootstrap');
  const socket = createConnection(bootstrap.endpoint);
  const timer = setTimeout(() => socket.destroy(new Error('IAB socket connection timed out')), 5000);
  try {
    await new Promise<void>((resolve, reject) => { socket.once('connect', resolve); socket.once('error', reject); });
    clearTimeout(timer);
    if (!sameIdentity(peerIdentity(socket), bootstrap.serverIdentity)) throw new Error('IAB server identity mismatch');
    return await new IabClient(new FramedChannel(socket), bootstrap).authenticate();
  } catch (error) { clearTimeout(timer); socket.destroy(); throw error; }
}

export async function connectPrivateIab(stream: Duplex = new Socket({ fd: 3, readable: true, writable: true })): Promise<IabClient> {
  const channel = new FramedChannel(stream);
  try {
    const bootstrap = await new Promise<Bootstrap>((resolve, reject) => {
      const timer = setTimeout(() => { cleanup(); reject(new Error('IAB bootstrap timed out')); }, 5000);
      const cleanup = () => { clearTimeout(timer); channel.off('message', receive); channel.off('closed', closed); };
      const closed = () => { cleanup(); reject(new Error('IAB private channel closed')); };
      const receive = (m: Message) => {
        if (m.method !== 'iab.bootstrap' || m.params?.authVersion !== 1) { channel.close(); return; }
        cleanup(); resolve(m.params as Bootstrap);
      };
      channel.on('message', receive); channel.on('closed', closed);
    });
    if (bootstrap.transport === 'socket') { channel.close(); return await connectSocketIab(bootstrap); }
    return await new IabClient(channel, bootstrap).authenticate();
  } catch (error) { channel.close(); throw error; }
}
