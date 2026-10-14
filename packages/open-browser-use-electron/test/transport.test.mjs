import test from 'node:test';
import assert from 'node:assert/strict';
import { createConnection } from 'node:net';
import { lstat } from 'node:fs/promises';
import { dirname } from 'node:path';
import { once } from 'node:events';
import { IabConnections } from '../dist/transport.js';
import { connectSocketIab } from '../dist/client.js';
import { frame } from '../dist/wire.js';

const supported = ['darwin', 'linux'].includes(process.platform);
async function setup(t) {
  const handlers = new Set(), calls = [];
  const backend = { instanceId: 'test-instance', hasScope: id => ['a','b'].includes(id),
    async dispatch(scope, method, params) { calls.push({ scope, method, params }); return { scope, method }; },
    subscribe(fn) { handlers.add(fn); return () => handlers.delete(fn); },
  };
  const connections = new IabConnections(backend); t.after(() => connections.dispose());
  const endpoint = await connections.openSocket();
  const grant = (scope = 'a', options = {}) => connections.grant(scope, 'socket', { ttlMs: 5000, ...options }, process.pid);
  return { connections, endpoint, grant, calls, emit: e => { for (const h of handlers) h(e); } };
}
test('private directory and socket permissions, native peer plus scoped handshake', { skip: !supported }, async t => {
  const { connections, endpoint, grant, calls } = await setup(t);
  assert.equal((await lstat(dirname(endpoint))).mode & 0o777, 0o700);
  assert.equal((await lstat(endpoint)).mode & 0o777, 0o600);
  const bootstrap = grant(); const client = await connectSocketIab(bootstrap); t.after(() => client.close());
  assert.equal(client.bootstrap.secret, ''); assert.equal((await client.getInfo()).scope, 'a');
  await assert.rejects(client.request('getInfo', { session_id: 'b' }));
  assert.equal(calls.length, 1);
  connections.revoke(bootstrap.grantId);
  await assert.rejects(client.getInfo());
});
test('unregistered peer and unauthenticated getInfo never reach backend', { skip: !supported }, async t => {
  const { endpoint, grant, calls } = await setup(t);
  const plain = createConnection(endpoint); const closed = once(plain, 'close'); plain.on('error', () => {});
  await closed; assert.equal(calls.length, 0);
  grant();
  const raw = createConnection(endpoint); raw.on('error', () => {});
  await once(raw, 'connect'); const done = once(raw, 'close');
  raw.write(frame({ jsonrpc:'2.0', id:'anon', method:'getInfo', params: { session_id:'a' } }));
  await done; assert.equal(calls.length, 0);
});
test('wrong, expired and revoked leases are rejected, even for a registered peer', { skip: !supported }, async t => {
  const { connections, grant, calls } = await setup(t);
  const valid = grant(); await assert.rejects(connectSocketIab({ ...valid, secret: 'A'.repeat(43) }));
  const expired = grant('a', { ttlMs: 20 }); await new Promise(r => setTimeout(r, 40));
  await assert.rejects(connectSocketIab(expired));
  connections.revoke(valid.grantId); await assert.rejects(connectSocketIab(valid)); assert.equal(calls.length, 0);
});
test('client checks server identity before sending the credential', { skip: !supported }, async t => {
  const { grant, calls } = await setup(t); const b = grant();
  await assert.rejects(connectSocketIab({ ...b, serverIdentity: { ...b.serverIdentity, startTime: 'wrong' } }), /identity mismatch/);
  assert.equal(calls.length, 0);
});
test('events stay scoped and stop immediately on expiry', { skip: !supported }, async t => {
  const { grant, emit } = await setup(t);
  const client = await connectSocketIab(grant('a', { ttlMs: 100 })); const events = [];
  client.onNotification(e => events.push(e));
  emit({ scopeId:'b', method:'onCDPEvent', params:{ private:'b' } });
  emit({ scopeId:'a', method:'onCDPEvent', params:{ public:'a' } });
  await new Promise(r => setTimeout(r, 20)); assert.equal(events.length, 1);
  await new Promise(r => setTimeout(r, 110));
  emit({ scopeId:'a', method:'onCDPEvent', params:{} });
  await assert.rejects(client.getInfo()); assert.equal(events.length, 1);
});
test('oversized pre-auth frames are rejected with bounded memory and no dispatch', { skip: !supported }, async t => {
  const { endpoint, grant, calls } = await setup(t); grant();
  const raw = createConnection(endpoint); raw.on('error', () => {}); await once(raw, 'connect');
  const closed = once(raw, 'close'); raw.write(Buffer.from([255,255,255,255])); await closed; assert.equal(calls.length, 0);
});
test('server rejects scope override on the wire, independently of client guards', { skip: !supported }, async t => {
  const { grant, calls } = await setup(t);
  const client = await connectSocketIab(grant());
  // A malicious client can bypass its own convenience API; the server is the boundary.
  client.channel.send({ jsonrpc:'2.0', id:'attack', method:'getTabs', params:{session_id:'b'} });
  await new Promise(resolve => client.channel.once('closed',resolve));
  assert.equal(calls.length,0);
});
