import test from 'node:test';
import assert from 'node:assert/strict';
import { normalizeAddress } from '../address.mjs';

test('addresses without a scheme open websites and preserve paths/query/hash',()=>{
  assert.equal(normalizeAddress(' github.com/ifuryst '),'https://github.com/ifuryst');
  assert.equal(normalizeAddress('example.com:8443/a?q=test#top'),'https://example.com:8443/a?q=test#top');
  assert.equal(normalizeAddress('//example.com/a'),'https://example.com/a');
});
test('local addresses with ports use HTTP rather than becoming a custom scheme',()=>{
  assert.equal(normalizeAddress('localhost:5173/a'),'http://localhost:5173/a');
  assert.equal(normalizeAddress('127.0.0.1:8080/a'),'http://127.0.0.1:8080/a');
  assert.equal(normalizeAddress('[::1]:8080/a'),'http://[::1]:8080/a');
});
test('explicit web schemes and blank tabs remain supported',()=>{
  assert.equal(normalizeAddress('https://example.com'),'https://example.com/');
  assert.equal(normalizeAddress('http://example.com/a'),'http://example.com/a');
  assert.equal(normalizeAddress('about:blank'),'about:blank');
});
test('invalid input, executable/file schemes and URL credentials remain rejected',()=>{
  for (const value of ['',null,'not a website','https://','file:///tmp/test','javascript:123','data:text/html,test','ftp://example.com','https://user:password@example.com']) assert.throws(()=>normalizeAddress(value));
});
