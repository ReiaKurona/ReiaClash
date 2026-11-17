const fs = require('node:fs');
const vm = require('node:vm');
const html = fs.readFileSync('test-webui.html', 'utf8');
const source = html.match(/<script>([\s\S]*?)<\/script>/)[1];
new vm.Script(source);
console.log('WebUI JavaScript syntax OK');

const assert = require('node:assert/strict');
const elements = new Map();
const element = id => {
  if (!elements.has(id)) elements.set(id, { checked: false, disabled: false, value: 'mixed', style: {}, textContent: '' });
  return elements.get(id);
};
const calls = [];
let geoLowMemory = true;
const context = vm.createContext({
  window: { location: { hostname: '127.0.0.1', protocol: 'http:', origin:'http://127.0.0.1:25500', href:'http://127.0.0.1:25500/theme#/nodes' } },
  document: { getElementById: element },
  AbortController,
  setTimeout: () => 0, clearTimeout() {}, setInterval() {},
  fetch: async (url, options = {}) => {
    calls.push({ url, options });
    if (url.endsWith('/api/agent/cache/clear') || url.endsWith('/api/agent/logs/clear')) {
      return { ok: true, json: async () => ({ success: true, cleared: { cache_files: 2, dns_records: 3, log_files: 2, events: 4 } }) };
    }
    if (url.endsWith('/api/agent/geo-low-memory')) {
      if (options.method === 'POST') geoLowMemory = JSON.parse(options.body).enable;
      return { ok: true, json: async () => ({ success: true, enabled: geoLowMemory, restart_requested: true }) };
    }
    return { ok: true, json: async () => url.endsWith('/api/system/status')
      ? { autostart_enabled: true, core_service_supported: true, core_service_enabled: true, core_service_registration: { backend: 'windows-scm', name: 'ReiaClashCore', installed: true, running: true } }
      : url.endsWith('/real-status') ? { settings: { tun_stack: 'mixed', outbound_mode: 'rule', geo_low_memory: geoLowMemory } } : { success: true } };
  }
});
vm.runInContext(source.replace(/\n  init\(\);/, '\n'), context);
(async () => {
  for (const action of ['cache', 'logs', 'open-cache', 'open-config']) {
    const button = { disabled: false };
    await context.agentMaintenance(action, button);
    const call = calls.at(-1);
    assert.equal(call.options.method, 'POST');
    assert.equal(call.url.endsWith(action.startsWith('open-') ? '/api/agent/directory/open' : `/api/agent/${action}/clear`), true);
    if (action.startsWith('open-')) assert.deepEqual(JSON.parse(call.options.body), { directory: action.slice(5) });
    assert.equal(button.disabled, false);
    assert.equal(element('notice').className, 'success');
  }
  calls.length = 0;
  assert.match(html, /id="pref-geo-low-memory" checked/);
  await context.toggleGeoLowMemory(false);
  const geoRequest = calls.find(c => c.url.endsWith('/api/agent/geo-low-memory'));
  assert.deepEqual(JSON.parse(geoRequest.options.body), { enable: false });
  assert.equal(element('pref-geo-low-memory').checked, false);
  assert.equal(element('pref-geo-low-memory').disabled, false);
  await context.toggleGeoLowMemory(true);
  assert.equal(element('pref-geo-low-memory').checked, true);
  calls.length = 0;
  assert.match(html, /<option value="mips">/);
  element('pref-tun-stack').value = 'mips';
  await context.updatePrefSettings('tun_stack');
  const stackPatch = calls.find(c => c.url.endsWith('/api/agent/settings'));
  assert.deepEqual(JSON.parse(stackPatch.options.body), { tun_stack: 'mips' });
  calls.length = 0;
  element('pref-allow-lan').checked = true;
  await context.updatePrefSettings('allow_lan');
  const patch = calls.find(c => c.url.endsWith('/api/agent/settings'));
  assert.deepEqual(JSON.parse(patch.options.body), { allow_lan: true });
  await context.toggleAutoStart(true);
  assert.equal(element('pref-autostart').checked, true);
  assert.equal(element('pref-autostart').disabled, false);
  await context.toggleCoreService(true);
  assert.equal(element('pref-core-service').checked, true);
  assert.equal(element('pref-core-service').disabled, false);
  assert.match(element('core-service-status').textContent, /已注册，运行中/);
  const healthyFetch = context.fetch;
  const pathError = '路径 /home/test/<example>/reia-service 权限为 0775，无法操作';
  for (const [route, run] of [
    ['/api/system/core-service', () => context.toggleCoreService(true, true)],
    ['/api/agent/settings', () => context.updatePrefSettings('use_tun')],
    ['/api/agent/master', () => context.toggleMaster()],
  ]) {
    context.fetch = async (url, options) => url.endsWith(route)
      ? { ok: false, status: 412, json: async () => ({ message: pathError }) }
      : healthyFetch(url, options);
    await run();
    assert.equal(element('core-service-operation-error').hidden, false);
    assert.equal(element('core-service-operation-error').textContent, pathError, 'keep copyable path diagnostics as literal text');
    await context.syncAutoStart();
    assert.equal(element('core-service-operation-error').textContent, pathError, 'polling must preserve the operation error');
  }
  context.fetch = healthyFetch;
  await context.toggleCoreService(true);
  assert.equal(element('core-service-operation-error').hidden, true, 'a successful retry clears the previous error');
  context.fetch = async (url, options) => url.endsWith('/api/system/status')
    ? { ok: true, json: async () => ({ core_service_enabled: true, core_service_supported: true, core_service_registration: { backend: 'windows-scm', name: 'ReiaClashCore', installed: false, running: false } }) }
    : healthyFetch(url, options);
  await context.syncAutoStart();
  assert.equal(element('pref-core-service').checked, false, 'a local marker must not masquerade as an installed service');
  assert.match(element('core-service-status').textContent, /注册\/修复/);
  let attempts = 0;
  context.fetch = async () => { attempts++; throw new Error('network disconnected'); };
  await assert.rejects(context.agentFetch('http://127.0.0.1:25500/api/system/core-service', { method: 'POST' }));
  assert.equal(attempts, 1, 'privileged requests must never be replayed automatically');
  vm.runInContext(`
    panelPrefs = {}; panelSiteName = '';
    proxyState = { order: ['Z', 'A'], proxies: {
      A: { type:'Selector', all:['Node10','Node2'], now:'Node10' },
      Z: { type:'Selector', all:['Node10','Node2'], now:'Node10' },
      GLOBAL: { type:'Selector', all:['A','Z'], now:'A' }
    }};
  `, context);
  const groups = mode => Array.from(vm.runInContext(`orderedGroupNames(proxyState.proxies, '${mode}')`, context));
  assert.deepEqual(groups('rule'), ['Z','A'], 'subscription order must beat JSON key order');
  assert.deepEqual(groups('global'), ['GLOBAL','Z','A']);
  vm.runInContext(`panelSiteName = 'A'`, context);
  assert.deepEqual(groups('rule'), ['A','Z'], 'XBoard brand group must be first');
  assert.deepEqual(groups('global'), ['GLOBAL','A','Z']);
  assert.deepEqual(Array.from(vm.runInContext(`orderedNodes('Z',proxyState.proxies)`,context)), ['Node10','Node2']);
  vm.runInContext(`panelPrefs.sort = { Z:'name' }`,context);
  assert.deepEqual(Array.from(vm.runInContext(`orderedNodes('Z',proxyState.proxies)`,context)), ['Node2','Node10']);
  const selectedNodes = ['Node10','Node2'].map(node => ({ dataset:{ node }, classList:{ toggle(key,value){ this.selected = value; } } }));
  const selectionLabel = {};
  context.testGroupView = { querySelector:() => selectionLabel, querySelectorAll:() => selectedNodes };
  vm.runInContext(`groupViews.set('Z',testGroupView)`,context);
  let rebuilds = 0, reloads = 0;
  context.renderGroups = () => { rebuilds++; };
  context.loadProxyGroups = () => { reloads++; };
  context.fetch = async () => ({ ok:true, json:async () => ({ success:true }) });
  await context.selectNode('Z','Node2');
  assert.equal(rebuilds,0); assert.equal(reloads,0);
  assert.equal(selectedNodes[0].classList.selected,false); assert.equal(selectedNodes[1].classList.selected,true);
  assert.match(selectionLabel.textContent,/Node2/);
  console.log('Subscription ordering, XBoard priority and in-place node switching OK');
  for (const name of ['connectTraffic','connectMemory','connectLogs','syncAutoStart','fetchIP','fetchCoreVersion','pollAgentEvents']) context[name] = () => {};
  context.syncRealStatus = async () => {};
  let available = false;
  const handshakes = [];
  context.fetch = async (url, options) => {
    if (!available) throw new Error('Agent not started yet');
    handshakes.push(JSON.parse(options.body));
    return {ok:true,json:async()=>({mihomo_controller:{secret:'test',api_base:'http://127.0.0.1:9090'}})};
  };
  await context.scanAgent();
  assert.equal(vm.runInContext('agentConnected',context),false);
  available = true;
  await context.scanAgent();
  assert.equal(vm.runInContext('agentConnected',context),true);
  assert.equal(handshakes.at(-1).panel_url,'http://127.0.0.1:25500/theme#/nodes');
  context.window.location.href = 'http://127.0.0.1:25500/theme#/connections';
  await context.scanAgent();
  assert.equal(handshakes.at(-1).panel_url,context.window.location.href);
  context.window.location.protocol = 'file:';
  context.window.location.origin = 'null';
  context.window.location.href = 'file:///C:/Users/tcs15/Desktop/ReiaClash/test-webui.html';
  await context.scanAgent();
  assert.equal(handshakes.at(-1).panel_url,context.window.location.href);
  assert.equal(handshakes.at(-1).panel_origin,'null');
  available = false; await context.scanAgent();
  assert.equal(vm.runInContext('agentConnected',context),false);
  available = true; await context.scanAgent();
  assert.equal(vm.runInContext('agentConnected',context),true);
  console.log('Automatic Agent scan/reconnect and full panel URL pairing payload OK');
  console.log('WebUI LAN, autostart, service controls and mutation retry checks OK');
})().catch(error => { console.error(error); process.exitCode = 1; });
