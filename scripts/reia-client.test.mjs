import { test } from 'node:test';
import assert from 'node:assert/strict';
import { ReiaClient, ReiaError } from '../docs/examples/reia-client.mjs';

const json = (value, status = 200) => new Response(JSON.stringify(value), { status });
const handshake = secret => ({ agent_version: 'test', panel: { paired: true },
  mihomo_controller: { api_base: 'http://127.0.0.1:9090', secret } });
const page = 'https://panel.example.com/account?lang=zh#/clash';

test('handshake preserves real page URL, file URL and route changes', async () => {
  let url = page;
  const bodies = [];
  const client = new ReiaClient({ getPanelURL: () => url, fetchImpl: async (target, options) => {
    assert.equal(target, 'http://127.0.0.1:25500/api/handshake');
    assert.equal(options.credentials, 'omit');
    assert.equal(options.targetAddressSpace, 'loopback');
    assert.equal(options.headers.Authorization, undefined);
    bodies.push(JSON.parse(options.body));
    return json(handshake('memory-only'));
  } });
  await client.connect();
  assert.deepEqual(bodies[0], { panel_origin: 'https://panel.example.com', panel_url: page });
  url = 'https://panel.example.com/account#/connections';
  await client.connect();
  assert.equal(bodies[1].panel_url, url);
  url = 'file:///C:/Users/demo/ReiaClash/test-webui.html';
  await client.connect();
  assert.deepEqual(bodies[2], { panel_origin: 'null', panel_url: url });
});

test('core credentials refresh after pairing, disappear after disconnect, and never go to Agent', async () => {
  let secret = 'first';
  const client = new ReiaClient({ getPanelURL: () => page, fetchImpl: async (url, options) => {
    if (url.endsWith('/api/handshake')) return json(handshake(secret));
    if (url.includes(':25500/')) assert.equal(options.headers.Authorization, undefined);
    else assert.equal(options.headers.Authorization, `Bearer ${secret}`);
    return json({ version: 'test' });
  } });
  await client.connect();
  await client.core('/version');
  secret = 'second';
  await client.connect();
  await client.core('/version');
  await client.status();
  client.disconnect();
  assert.equal(client.controller, null);
  await assert.rejects(client.core('/version'), { code: 'not_connected' });
});

test('a failed latest handshake clears credentials while an older reply cannot restore them', async () => {
  let complete;
  let failed = false;
  const client = new ReiaClient({ getPanelURL: () => page, fetchImpl: async () => {
    if (failed) throw new TypeError('blocked or offline');
    return new Promise(resolve => { complete = resolve; });
  } });
  const pending = client.connect();
  client.disconnect();
  complete(json(handshake('stale')));
  await assert.rejects(pending, { code: 'aborted' });
  assert.equal(client.connected, false);
  client.session = handshake('old');
  failed = true;
  await assert.rejects(client.connect(), { code: 'network_error' });
  assert.equal(client.connected, false);
});

test('discovery retries handshake and stopping prevents late updates or extra requests', async () => {
  let calls = 0;
  const states = [];
  const client = new ReiaClient({ getPanelURL: () => page, fetchImpl: async () => {
    if (++calls === 1) throw new TypeError('Agent not yet started');
    return json(handshake('reconnected'));
  } });
  let stop;
  await new Promise((resolve, reject) => {
    const timer = setTimeout(() => { stop?.(); reject(new Error('discovery did not reconnect')); }, 1000);
    stop = client.startDiscovery(state => {
      states.push(state.connected);
      if (state.connected) { stop(); clearTimeout(timer); resolve(); }
    }, { intervalMs: 5 });
  });
  const stoppedAt = calls;
  await new Promise(resolve => setTimeout(resolve, 20));
  assert.deepEqual(states, [false, true]);
  assert.equal(calls, stoppedAt);
  assert.equal(client.controller, null);
});

test('mutations never retry, serialize and release the gate after timeout', async () => {
  let writes = 0;
  const client = new ReiaClient({ fetchImpl: async (_url, options) => {
    writes++;
    return new Promise((_, reject) => options.signal.addEventListener('abort', () => reject(new Error('aborted')), { once: true }));
  } });
  const pending = client.setService(true, { timeoutMs: 15 });
  await assert.rejects(client.restart(), { code: 'operation_in_progress' });
  await assert.rejects(pending, { code: 'timeout', uncertain: true });
  assert.equal(writes, 1);
  assert.equal(client.mutationPending, false);
  client.fetchImpl = async () => json({ success: true });
  await client.restart();
});

test('already aborted requests are not sent and do not retain the mutation gate', async () => {
  const abort = new AbortController(); abort.abort();
  const client = new ReiaClient({ fetchImpl: async () => assert.fail('request must not be sent') });
  await assert.rejects(client.setEnabled(true, { signal: abort.signal }), { code: 'aborted', uncertain: false });
  assert.equal(client.mutationPending, false);
});

test('JSON errors preserve status/code and native empty responses are accepted', async () => {
  const client = new ReiaClient({ fetchImpl: async () => json({ success: false, code: 'permission_required', message: '系统授权失败' }, 412) });
  await assert.rejects(client.setService(true), { name: 'ReiaError', code: 'permission_required', status: 412, message: '系统授权失败' });
  client.fetchImpl = async () => new Response(null, { status: 204 });
  assert.equal(await client.restart(), null);
  client.fetchImpl = async () => new Response('invalid body');
  await assert.rejects(client.restart(), { code: 'invalid_response', uncertain: true });
});

test('adapters use actual Go routes and request fields with encoded query/connection IDs', async () => {
  const calls = [];
  const client = new ReiaClient({ fetchImpl: async (url, options) => {
    calls.push({ path: new URL(url).pathname, method: options.method,
      body: options.body === undefined ? undefined : JSON.parse(options.body), url });
    return json({ success: true });
  } });
  await client.setAutostart(true);
  await client.setGeoLowMemory(false);
  await client.setService(true, { repair: true });
  await client.upgrade(false);
  await client.select('站点', '节点');
  await client.dnsQuery('example.com', 'AAAA');
  await client.closeConnection('connection id');
  await client.exitAgent();
  assert.deepEqual(calls.slice(0, 5).map(({ path, body }) => [path, body]), [
    ['/api/system/autostart', { enable: true }], ['/api/agent/geo-low-memory', { enable: false }],
    ['/api/system/core-service', { enable: true, repair: true }], ['/api/agent/upgrade', { is_silent: false }],
    ['/api/core/select', { group: '站点', node: '节点' }],
  ]);
  assert.equal(new URL(calls[5].url).searchParams.get('type'), 'AAAA');
  assert.equal(calls[6].path, '/api/core/connections/connection%20id');
  assert.equal(calls[6].method, 'DELETE');
  assert.equal(calls[7].path, '/api/agent/exit');
  await assert.rejects(client.closeConnection(''), { code: 'invalid_id' });
  assert.equal(calls.length, 8);
  await client.clearAgentCache();
  await client.clearAgentLogs();
  await client.openDirectory('config');
  await client.openDirectory('cache');
  assert.deepEqual(calls.slice(8).map(({ path, method, body }) => [path, method, body]), [
    ['/api/agent/cache/clear', 'POST', undefined], ['/api/agent/logs/clear', 'POST', undefined],
    ['/api/agent/directory/open', 'POST', { directory: 'config' }],
    ['/api/agent/directory/open', 'POST', { directory: 'cache' }],
  ]);
  await client.subscriptionStatus();
  await client.refreshSubscription();
  assert.deepEqual(calls.slice(12).map(({ path, method }) => [path, method]), [
    ['/api/subscription/status', 'GET'], ['/api/subscription/refresh', 'POST'],
  ]);
});

test('reject remote bases, external paths and unexpected handshake controllers', async () => {
  assert.throws(() => new ReiaClient({ agentBase: 'https://remote.example.com' }), ReiaError);
  const client = new ReiaClient({ getPanelURL: () => page, fetchImpl: async () => json({
    mihomo_controller: { api_base: 'http://remote.example.com:9090', secret: 'ignored' },
  }) });
  await assert.rejects(client.connect(), { code: 'invalid_handshake' });
  await assert.rejects(client.agent('//remote.example.com/path'), { code: 'invalid_path' });
  assert.equal(client.controller, null);
});
