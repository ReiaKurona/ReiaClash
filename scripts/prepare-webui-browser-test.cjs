// Produce an isolated browser fixture: all API and WebSocket traffic is mocked.
const fs = require('node:fs');
let html = fs.readFileSync('test-webui.html', 'utf8');
const setup = `<script>
window.REIA_THEME = {siteName:'品牌站'};
window.WebSocket = class { close() {} };
window.testCalls = [];
window.testProxies = {
  A:{type:'Selector',all:['Node10','Node2'],now:'Node10'},
  Z:{type:'Selector',all:['Node10','Node2'],now:'Node10'},
  '品牌站':{type:'Selector',all:['Node10','Node2'],now:'Node10'},
  GLOBAL:{type:'Selector',all:['A','Z','品牌站'],now:'A'},
  Node10:{type:'HTTP',history:[{delay:200}]},Node2:{type:'HTTP',history:[{delay:100}]}
};
window.fetch = async (url, options = {}) => {
  testCalls.push({url,options});
  let data = {success:true};
  if(url.endsWith('/api/handshake')) data = {agent_version:'test',mihomo_controller:{secret:'fake-test'}};
  if(url.endsWith('/real-status')) data = {settings:{tun_stack:'mips',outbound_mode:'rule',geo_low_memory:true}};
  if(url.endsWith('/api/system/status')) data = {core_service_supported:true};
  if(url.endsWith('/api/core/proxies')) data = {proxies:testProxies,group_order:['Z','品牌站','A']};
  if(url.endsWith('/api/core/select')) { const request = JSON.parse(options.body); testProxies[request.group].now = request.node; }
  if(url.endsWith('/api/core/connections')) data = {connections:[{id:'test-id',metadata:{host:'example.com',process:'browser',destinationPort:443},chains:['品牌站','Node2'],rule:'MATCH',download:1024}]};
  if(url.includes('/api/core/dns/query?')) data = {Status:0,Answer:[{data:'192.0.2.1'}]};
  if(url.endsWith('/api/core/dns/records')) data = {scope:'test DNS log',limit:200,records:[{at:new Date().toISOString(),source:'kernel',payload:'[DNS] example.com --> 192.0.2.1 A'}]};
  if(url.includes('ipify')) data = {ip:'192.0.2.1'};
  if(url.includes('/events?')) data = {events:[]};
  if(url.endsWith('/api/agent/cache/clear') || url.endsWith('/api/agent/logs/clear')) data = {success:true,cleared:{cache_files:1,dns_records:2,log_files:2,events:3}};
  return {ok:true,status:200,json:async()=>data};
};
</script>`;
html = html.replace('<script>', setup + '\n<script>');
const checks = `<script>
(async () => {
 const assert = (value, message) => { if(!value) throw new Error(message); };
 await connectAgent();
 const handshake = testCalls.find(call=>call.url.endsWith('/api/handshake'));
 const pairing = JSON.parse(handshake.options.body);
 assert(pairing.panel_url === window.location.href && pairing.panel_url.startsWith('file:///'),'file page did not report its exact file URL');
 switchTab('nodes'); await loadProxyGroups();
 const names = [...document.querySelectorAll('.group-name')].map(el=>el.textContent);
 assert(names.join(',') === '品牌站,Z,A', 'subscription/brand/GLOBAL order: '+names);
 const brand = groupViews.get('品牌站'), container = document.getElementById('groups-container');
 const node = [...brand.querySelectorAll('.node-card')].find(el=>el.dataset.node==='Node2');
 const before = testCalls.filter(call=>call.url.endsWith('/api/core/proxies')).length;
 await node.onclick();
 assert(groupViews.get('品牌站') === brand && brand.parentElement === container, 'node switching replaced the group frame');
 assert(node.classList.contains('selected'), 'selected node did not update');
 assert(testCalls.filter(call=>call.url.endsWith('/api/core/proxies')).length === before, 'selection reloaded all proxies');
 const sort = brand.querySelector('.node-sort'); sort.value='name'; sort.dispatchEvent(new Event('change'));
 assert(groupViews.get('品牌站').querySelector('.node-card').dataset.node==='Node2','manual name sorting failed');
 await switchTab('connections');
 assert(document.getElementById('connection-rows').children.length===1,'connection rows missing');
 assert(document.getElementById('connection-rows').textContent.includes('example.com'),'connection data missing');
 await switchTab('dns'); document.getElementById('dns-name').value='example.com'; await queryDNS();
 assert(document.getElementById('dns-result').textContent.includes('192.0.2.1'),'DNS answer missing');
 assert(document.getElementById('dns-record-rows').textContent.includes('[DNS]'),'DNS history missing');
 await restartKernel();
 assert(testCalls.some(call=>call.url.endsWith('/api/core/restart') && call.options.method==='POST'),'restart button did not call API');
 assert(document.getElementById('tab-dns').parentElement.classList.contains('container'),'network tabs are outside panel');
 await switchTab('ota');
 for (const [action, id] of [['cache','clear-agent-cache'],['logs','clear-agent-logs'],['open-cache','open-cache-directory'],['open-config','open-config-directory']]) {
   const button = document.getElementById(id);
   await agentMaintenance(action, button);
   assert(!button.disabled, 'maintenance button stayed disabled');
   assert(document.getElementById('notice').className === 'success', 'maintenance request failed');
 }
 assert(testCalls.some(call=>call.url.endsWith('/api/agent/directory/open') && JSON.parse(call.options.body).directory==='config'),'configuration directory request missing');
 const healthyFetch = window.fetch;
 const pathError = '路径 /home/test/<example>/reia-service 权限为 0775，无法操作';
 window.fetch = async (url, options) => url.endsWith('/api/system/core-service')
   ? {ok:false,status:412,json:async()=>({message:pathError})} : healthyFetch(url,options);
 await toggleCoreService(true, true);
 const detail = document.getElementById('core-service-operation-error');
 assert(!detail.hidden && detail.textContent === pathError && detail.children.length === 0,'service path error was lost or rendered as HTML');
 await syncAutoStart();
 assert(detail.textContent === pathError,'status polling replaced operation details');
 window.fetch = healthyFetch;
 await toggleCoreService(true, true);
 assert(detail.hidden && detail.textContent === '', 'successful repair did not clear stale details');
 document.body.dataset.browserTest='passed';
} )().catch(error=>{ document.body.dataset.browserTest='failed'; const el=document.createElement('pre');el.id='browser-test-error';el.textContent=error.stack;document.body.appendChild(el); });
</script>`;
html = html.replace('</body>', checks + '\n</body>');
fs.mkdirSync('.build', {recursive:true});
fs.writeFileSync('.build/webui-browser-test.html', html);
console.log('Isolated browser test fixture ready');
