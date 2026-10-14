import { createRequire } from 'node:module';
import type { Socket } from 'node:net';

export type ProcessIdentity = { pid: number; uid: number; startTime: string };
type NativePeer = { processIdentity(pid: number): ProcessIdentity; peerIdentity(fd: number): ProcessIdentity };
let native: NativePeer | undefined;
function adapter(): NativePeer {
  if (!native) {
    if (!['darwin', 'linux'].includes(process.platform)) throw new Error('AUTH_UNAVAILABLE: socket peer verification is not supported on this platform');
    try { native = createRequire(import.meta.url)('../build/Release/iab_peer.node'); }
    catch { throw new Error('AUTH_UNAVAILABLE: build the native peer adapter before enabling sockets'); }
  }
  return native!;
}
export function processIdentity(pid = process.pid): ProcessIdentity { return adapter().processIdentity(pid); }
export function peerIdentity(socket: Socket): ProcessIdentity {
  // Private Node handle access is confined to this version-tested adapter.
  const fd = (socket as unknown as { _handle?: { fd?: number } })._handle?.fd;
  if (!Number.isInteger(fd) || fd! < 0) throw new Error('AUTH_UNAVAILABLE: missing native socket descriptor');
  return adapter().peerIdentity(fd!);
}
export function sameIdentity(a: ProcessIdentity, b: ProcessIdentity): boolean {
  return a.pid === b.pid && a.uid === b.uid && a.startTime === b.startTime;
}
