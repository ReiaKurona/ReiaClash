# ReiaClash 前端移植与嵌入指南

更新日期：2026-11-19。面向 XBoard 主题、Vue/React 应用、独立 Web 面板和其他站点。

| 文件 | 用途 |
| --- | --- |
| [API 完整索引](api.md) | 所有 Agent 路由、请求字段和生命周期含义 |
| [reia-client.mjs](examples/reia-client.mjs) | 无框架、无依赖的浏览器接入类，可复制到其他项目 |
| [最小可运行页面](examples/embed.html) | 自动发现、连接按钮、状态、启停、切模式、重启 |

## 1. 接入模型

网页里的 JavaScript 访问**当前浏览器所在电脑**的 ReiaClash：

```text
用户浏览器中的站点组件
  ├─ http://127.0.0.1:25500  Agent：配对、设置、服务、订阅、节点、DNS
  └─ http://127.0.0.1:9090   mihomo：可选的原生 REST / WebSocket
```

站点可以部署在任意支持该浏览器访问策略的 HTTPS 域名，无需为每个域名修改 Agent 白名单。
服务器端渲染、站点后端或 Nginx 在服务器访问 127.0.0.1，访问的是服务器自身，不能替代浏览器访问用户电脑。
25501 是客户端内部服务控制端口，前端不用接，也不会获得它的 token。

不要在页面加载时默认开启代理、安装系统服务或导入订阅。自动发现只做握手；有副作用的操作由用户点击触发。

本项目的 Aether M3 接入是用户明确授权的订阅自动化例外：发现 ReiaClash 后推送当前账号链接，并每 4 小时重新取得/推送；不自动开启代理或申请系统服务。客户端启动/每 12 小时独立刷新，手动安装页和原多客户端订阅卡保留。实现与验证见 [Aether M3 接入说明](xboard-integration.md)。通用示例仍由调用方决定是否启用自动推送。

## 2. 五分钟接入

把 examples/reia-client.mjs 复制到站点静态资源目录，例如 /assets/reia-client.mjs，在组件挂载后创建一个实例：

```javascript
import { ReiaClient } from '/assets/reia-client.mjs';

const client = new ReiaClient();

// 保留这个显式按钮，首次连接可能需要浏览器本地网络授权。
connectButton.onclick = async () => {
  try {
    await client.connect();
    const status = await client.status();
    renderStatus(status); // 只更新当前组件，不刷新站点页面。
  } catch (error) {
    showMessage(error.message);
  }
};

const stopDiscovery = client.startDiscovery(({ connected, error }) => {
  setConnectionState(connected);
  if (!connected) showConnectionHint(error?.message);
});

// 组件卸载时执行；不会停止用户的后台客户端。
function unmount() {
  stopDiscovery();
}
```

示例中的 connectButton/renderStatus 等由宿主组件提供。[embed.html](examples/embed.html) 提供了完整 DOM 实现。
把 embed.html 和模块放在同一静态目录后，通过 HTTP(S) 打开即可测试。
直接双击包含 ES module import 的 HTML 可能受到 file 模块加载限制；本地 file 面板可使用下节的内联 fetch 接法，或先将模块打包进入页面。
示例不会修改现有 test-webui.html，也不会加载整套客户端页面框架。

## 3. 不使用接入类的最小握手

```javascript
const page = new URL(window.location.href);
const response = await fetch('http://127.0.0.1:25500/api/handshake', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  credentials: 'omit',
  cache: 'no-store',
  targetAddressSpace: 'loopback',
  body: JSON.stringify({
    panel_origin: page.origin,
    panel_url: page.href,
  }),
});
const data = await response.json();
if (!response.ok) throw new Error(data.message || '本机连接失败');

// 只存在当前页面内存；不要把整个 data 写到日志或持久存储。
const controller = data.mihomo_controller;
```

响应结构：

```json
{
  "agent_version": "示例版本",
  "panel": {
    "paired": true,
    "panel_url": "https://panel.example.com/account#/clash",
    "open_url": "https://panel.example.com/account#/clash",
    "source": "paired",
    "network": "online"
  },
  "mihomo_controller": {
    "api_base": "http://127.0.0.1:9090",
    "secret": "仅作示例，使用真实握手返回值"
  },
  "features": ["rest", "websocket", "subscription", "autostart", "panel_pairing"]
}
```

- panel_url 必须是当前页面完整 URL，包含路径、查询和 SPA 片段，不能只传站点 Origin。
- 不要改写成客户端内置测试页，也不要默认替用户配对到兜底站点。
- HTTP(S) 请求的 panel_url 必须与浏览器自动发送的 Origin 同源；不要手动设置 Origin 请求头。
- file 页面使用 page.origin 的 null 字符串及真实 file:/// 绝对 URL。
- SPA 导航后下次握手会报告新地址；Agent/内核重启后重新握手更新内存密钥。
- features 是当前已列出的能力，不是完整路由能力清单；不能因为它没列 DNS 就认为 DNS API 不存在。

## 4. 自动发现与连接状态

接入类每次握手重新读取 getPanelURL，默认取当前 location.href。扫描间隔默认 5 秒，不会重放写操作。
自动发现不需要先访问 /api/health，健康接口可另外用于调试和取得 agent_pid。

| 状态 | 判断 | 前端显示/动作 |
| --- | --- | --- |
| Agent 不可达或浏览器阻止访问 | fetch 网络失败 | 提示启动客户端/允许本地访问，保留连接按钮 |
| Agent 已连接 | 握手成功 | 可加载设置与状态，不代表内核已经启动 |
| 内核运行 | real-status.mihomo_running | 可请求核心数据 |
| 代理实际接管 | real-status.is_actually_intercepting | 显示“代理运行中” |
| 服务已注册 | system-status.core_service_registration.installed | 服务开关可显示已安装 |
| 服务内核已就绪 | system-status.core_service_ready | 显示“服务可用” |

网络异常不能单独证明“用户没安装客户端”；权限、CSP 和客户端未启动都可能使 fetch 报错。
网页目前没有专用协议可直接拉起客户端；未连接时提示用户启动 reia，不要声称网页可以自动安装或启动桌面服务。
断连时停止核心 WebSocket、清除旧内存凭据；组件卸载时停止定时器和请求。

## 5. 常用操作与接入类方法

接入类支持 agent(path, options) 访问任意已文档化的 Agent 路由，以及下列快捷方法。
所有方法返回原始 JSON；不会统一增加 data 或 success 包装。

| 方法 | 对应操作 |
| --- | --- |
| connect() / startDiscovery(callback) | 真实 URL 配对 / 周期重连，返回的停止函数用于卸载 |
| health() / status() / systemStatus() / panel() | 健康 / 实际状态 / 系统状态 / 面板配对状态 |
| events(since) | 拉取增量事件 |
| setEnabled(boolean) | 总开关 |
| settings(patch) | 部分设置更新 |
| setAutostart(boolean) | 登录后启动客户端 |
| setService(boolean, {repair}) | 安装/卸载/修复真实 OS 服务 |
| geoLowMemory() / setGeoLowMemory(boolean) | Geo loader 查询/设置 |
| proxies() / select(group,node) / delay(group) | 节点数据、切换、测速 |
| importSubscription(url) | 导入可由客户端访问的绝对订阅 URL |
| subscriptionStatus() / refreshSubscription() | 不含凭证的刷新状态 / 刷新已保存 URL |
| connections() / closeConnection(id) / closeConnections() | 连接管理 |
| dnsQuery(name,type) / dnsRecords() / clearDnsRecords() | DNS 查询与记录 |
| restart() | 手动重启内核 |
| outboundIP() | 获取公网出口 IP，`outbound_ip_trace` 表示 Agent 使用 Cloudflare Trace |
| checkUpdate() / upgrade(isSilent) | 检查/更新内核，默认静默暂存 |
| exitAgent() | 请求 Agent 退出或转后台，含义见下文 |
| core(path,options) | 使用内存 Bearer 密钥访问原生核心 REST |

方法末尾可传 {signal,timeoutMs}，setService 的 repair 与这两个参数放在同一对象。

```javascript
await client.settings({ use_system_proxy: true });
await client.setEnabled(true);
await client.settings({ outbound_mode: 'global' }); // rule / global / direct
await client.settings({ allow_lan: true });
await client.settings({ use_tun: true, tun_stack: 'mixed' });
await client.setAutostart(true);
await client.setGeoLowMemory(true);
```

设置对象只接受 use_system_proxy、use_tun、tun_stack、allow_lan、auto_update_enabled、update_interval_hours、outbound_mode。
不要把 GET 返回的完整 settings 对象原样提交：总开关、自启、Geo、服务模式有独立端点。
Geo 低内存默认开启；启用/关闭分别生成 memconservative/standard。
TUN stack 接受 mixed/gvisor/system/mips，但目标内核是否接受需按版本验证。

### 服务、重启与操作等待

```javascript
await client.setService(true);                    // 可能弹 UAC/系统授权
await client.setService(true, { repair: true });  // 修复/迁移
const state = await client.systemStatus();
renderService({
  installed: state.core_service_registration.installed,
  ready: state.core_service_ready,
  error: state.core_service_error || state.core_service_registration.error,
});
```

以上是独立操作示例，不能在组件加载时整段连续执行。
接入类读请求默认 10 秒，握手 5 秒，写请求 5 分钟，exitAgent 默认 10 秒；同一实例只允许一个写操作在途。
操作中禁用对应开关/按钮，完成后重新读取实际状态。超时或取消只终止浏览器等待，不保证系统操作已撤销。
服务卸载、重启、更新返回后用状态确认，不自动重发安装、卸载或重启请求。

服务模式下 exitAgent() 返回 background:true，后台 API 继续运行。API 本身不会关闭托盘，托盘菜单是在调用成功后自行退出；网页不应显示“整个客户端已退出”。
普通模式下该接口会请求 Agent 退出；关闭网页或卸载组件则只需要停止前端定时器，不要调用 exitAgent()。

### Agent 维护、目录容量与打开本机目录

以下方法绑定用户点击事件，分别执行，不在页面加载或重连时自动调用：

```javascript
await client.clearAgentCache();       // 暂存下载/解压文件和本次 DNS 记录
await client.clearAgentLogs();        // 两个 Agent 日志文件及内存事件
await client.openDirectory('cache');  // 本机用户配置目录内的 updates/
await client.openDirectory('config'); // 本机用户配置根目录
```

`client.directorySizes()` 查询升级缓存、应用旁 data 和配置目录容量。使用 `client.openDirectory('work')` 打开应用旁 data。Aether M3 在「客户端设置 → 存储与目录」统一展示。

接入类同时提供 `setSilentProxyOnLogin`、`resources`、`setResourceSchedule` 和 `syncResources`。先在页面完成确认交互再调用 `clearSubscription`。该操作清空本机订阅缓存并暂停自动导入，目录、资源和订阅接口字段见 [API](api.md)。

缓存和日志清理不重启内核，保留设置、订阅、密钥和已下载内核；详细清理范围和响应计数见 [API](api.md#agent-缓存日志和目录)。打开目录返回 202 表示客户端已请求系统文件管理器打开，不保证当前桌面会话一定显示窗口。维护失败可能已清理部分内容，显示原始错误，不自动重复发送请求。网页的内核 WebSocket 日志显示与磁盘 Agent 日志是不同数据源。

支持 `tun_device` 的客户端可通过 `client.settings({ tun_device: 'ReiaWork' })` 保存网卡名。macOS 使用 `utun` 加编号，各平台均可传空字符串自动选择。`client.status()` 的 `real_tun_device` 用于显示活动网卡。`client.outboundIP()` 可在总开关、模式、TUN 或节点变化后重新调用，查询失败时显示暂不可用并取消上一次探测。[设置与状态 API](api.md#状态设置与启停)

## 6. 节点、顺序与 XBoard 适配

```javascript
const data = await client.proxies();
const subscriptionOrder = data.group_order;
const groupNodes = data.proxies['站点名'].all;
await client.select('站点名', '节点名');
// 只更新这一组的选中项，不刷新路由、不重建整个站点或节点框架。
```

不要依靠 JSON 对象 key 顺序作为订阅顺序；使用 group_order 和组内 all。
自定义排序可以保存在宿主站点的浏览器偏好中。节点切换会断开现有连接，新连接使用新节点。

规则模式：隐藏 GLOBAL，站点同名组优先，其余保持订阅顺序。
全局模式：GLOBAL 显示并置顶；直连模式：隐藏代理节点选择区。
现有测试面板支持 window.REIA_THEME = {siteName:'站点名'} 或 window.ReiaClashPanel.configure({siteName:'站点名'})。
这些全局对象属于现有面板，不由接入类自动创建；移植到新主题时在组件中实现同样排序策略。

订阅导入应传完整 HTTP(S) 订阅链接；不能指望客户端复用网站浏览器的登录 Cookie，也不能传站点相对路径。
当前订阅下载会拒绝解析到私网的订阅地址。面板显示节点名称、连接字段、DNS/日志时使用文本绑定。

## 7. 连接、DNS 与实时数据

```javascript
const connections = await client.connections(); // 原生 connections / uploadTotal / downloadTotal
await client.closeConnection('示例连接ID');
const result = await client.dnsQuery('example.com', 'A');
const { records, capture_error } = await client.dnsRecords();
const version = await client.core('/version');
```

连接列表在页面可见时按约 2 秒轮询即可；DNS 记录来自当前 Agent 的日志与手动查询，最多 200 条。
先确认 Agent 和核心状态，核心未就绪时不要对所有页面连续弹出同一错误。

WebSocket 使用握手中的 api_base/secret，REST 用 Authorization，浏览器 WebSocket 用 token 查询参数：

```javascript
function openCoreSocket(client, path) {
  const controller = client.controller;
  if (!controller) throw new Error('请先连接本机');
  const url = new URL(path, controller.api_base);
  if (url.origin !== new URL(controller.api_base).origin) throw new Error('仅允许本机核心路径');
  url.protocol = 'ws:';
  url.searchParams.set('token', controller.secret);
  try {
    return new WebSocket(url.href, { targetAddressSpace: 'loopback' });
  } catch {
    return new WebSocket(url.href);
  }
}

const traffic = openCoreSocket(client, '/traffic');
traffic.onmessage = event => {
  const { up, down } = JSON.parse(event.data); // 每秒字节数
  renderTraffic(up, down);
};
// 组件卸载或 Agent 断连时：traffic.close();
```

常用流是 /traffic、/memory（inuse）、/logs?level=info（type/payload）。
`/connections?interval=1000` 提供活动连接快照。`/logs?level=info&format=structured` 可启用结构化日志，面板同时兼容旧的 `type` 和 `payload`。
接入类不自动管理 WebSocket；宿主组件应在断线后适度延迟重连，重连前使用最新 controller，卸载时清理定时器和 socket。
不要记录包含 token 的完整 WebSocket URL；不要将控制密钥上传站点后端或保存在 localStorage。

## 8. Vue/React/SSR 和 iframe

- Vue：在 onMounted 创建实例/发现循环，onBeforeUnmount 停止发现、轮询、WebSocket；把状态写入组件响应式变量。
- React：在 useEffect 创建实例与订阅并返回 cleanup；不要在 render 中创建计时器；Strict Mode 的重复挂载必须能正常清理。
- SSR：模块可导入，但访问本机和读取 window.location 必须在浏览器挂载后进行；服务端不执行自动发现。
- 跨域 iframe：握手应上报 iframe 自己的 URL/Origin，不能冒充父页面 Origin。若要配对到父站点地址，让父页面直接运行接入组件，或使用同源部署。

组件只负责自己的渲染和订阅，不替换宿主 document、location 或路由实例。
接入类 disconnect()/停止发现只清理页面会话，不发送服务停止或卸载命令。

## 9. 站点策略和故障处理

站点使用 CSP 时，connect-src 应包含实际本机目标，例如：

```text
connect-src 'self' http://127.0.0.1:25500 http://127.0.0.1:9090 ws://127.0.0.1:9090;
```

这是需要合并到现有策略的连接指令示例，不要覆盖宿主其他 API 域名或 script-src。
如需旧 Agent 的浏览器出口探测，将 `https://1.1.1.1` 加入 `connect-src`。新版 Agent 在本机代为查询 Trace。
若站点禁止内联脚本，将 embed.html 的脚本逻辑放入已有前端构建产物，或使用宿主提供的 nonce；不要为示例放宽整站脚本策略。
浏览器本地网络权限、HTTPS/混合内容策略、iframe 限制仍受浏览器和部署环境约束；CSP 放行不等于浏览器已授权。
不识别 targetAddressSpace 的环境不能靠该参数绕过访问限制。

| 错误 | 处理 |
| --- | --- |
| network_error / fetch TypeError | 检查客户端运行、浏览器站点权限、CSP，保留手动连接 |
| origin_mismatch / invalid_panel_url | 检查当前完整页面 URL 和 iframe 实际来源 |
| permission_required，通常 412 | 显示系统授权提示和后端原始 message，授权失败后重新查状态 |
| mihomo_unavailable，通常 503 | Agent 可连接但核心不可用，显示内核状态/修复入口 |
| mihomo_route_unavailable，通常 502 | 当前内核没有对应路由，按功能展示失败 |
| operation_failed，通常 500 | 显示具体 message，再查询实际状态 |
| operation_in_progress | 不重复提交，等待当前写操作结束 |
| timeout / aborted 且 uncertain=true | 写操作可能已执行，先查状态，用户决定是否重试 |

Agent 仍有可扩展错误码，不能只按 message 文本判断授权或状态。
内核原生接口的错误格式可能与 Agent 不同；接入类会保留 HTTP status 并提供 message。

## 10. 移植验收

1. 先打开站点、后启动客户端，自动连接；先启动客户端、再打开站点也可连接。
2. 本地网络权限尚未允许时显示可操作的连接按钮，允许后恢复。
3. SPA 路由变化后，托盘打开最新真实页面地址；file 面板不改写成本机测试 HTTP 页。
4. Agent 已连但核心未就绪时，能正常显示设置和错误，不误报代理已启用。
5. 节点/模式切换不刷新宿主框架，服务开关区分注册、ready 和错误。
6. 取消系统授权/操作超时不会自动重复安装服务；服务模式退出托盘后页面仍可连接。
7. 组件反复挂载/卸载没有重复定时器、socket 或网络请求；页面卸载不停止用户后台代理。

本指南按当前源码接口编写，没有新增远程云 API 或更改客户端服务协议。
接入类的模拟协议回归可执行 node --test scripts/reia-client.test.mjs；浏览器权限和目标站点 CSP 仍需在实际站点验收。
