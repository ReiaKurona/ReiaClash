# 本机 API 与 WebUI 对接

更新日期：2026-11-19。以 internal/httpapi/server.go 路由和处理器为准。
Agent 基址是 http://127.0.0.1:25500，也提供 IPv6 loopback。控制接口不随 LAN 开关开放。

前端第一次移植请先看 [嵌入与快速接入指南](frontend-integration.md)，可直接复制 [JavaScript 接入类](examples/reia-client.mjs) 和 [最小示例页面](examples/embed.html)。
本页作为完整路由与响应语义参考。当前没有独立的 /v1 API 版本前缀，更新时应按客户端源码快照核对字段。

Agent 路由不要求 mihomo Bearer 密钥；核心 :9090 原生 REST 使用握手返回的 secret。浏览器负责发送 Origin，不要在 JavaScript 中伪造 Origin 请求头。
调用方在用户电脑的浏览器执行请求，不能把服务器自己的 127.0.0.1 当作用户客户端地址。

JSON 请求使用 Content-Type: application/json；有 JSON body 的处理器限制 1 MiB，拒绝未知字段和多余 JSON。
不同端点的响应并非统一包装：部分返回 success，部分直接返回 mihomo 原生数据。
一般错误形如 {"success":false,"code":"...","message":"..."}，不要仅凭 HTTP 200 或 UI 勾选推断内核已就绪。
以下字段示例不包含任何真实密钥或订阅凭证。

## 健康、面板和配对

| 方法 | 路径 | 请求/用途 |
| --- | --- | --- |
| GET | /api/health | status、agent_version、agent_pid；健康不等于内核 ready |
| POST | /api/handshake | panel_origin、panel_url；配对并取得 mihomo 控制信息 |
| GET | /api/agent/panel | paired、panel_url、open_url、source、network |
| GET | /panel | 当前面板目标的重定向，不缓存 |
| GET | /api/panel/open | /panel 的兼容入口 |
| GET | / | 内置测试 WebUI |
| GET | /test-webui.html | 内置测试 WebUI 显式路径 |

握手请求示例：
```json
{"panel_origin":"https://next.rka.jp","panel_url":"https://next.rka.jp/#/proxies"}
```

本地文件示例：
```json
{"panel_origin":"null","panel_url":"file:///C:/Users/tcs15/Desktop/ReiaClash/test-webui.html"}
```

HTTP(S) panel_url 必须与请求 Origin 同源；本地 file 页面应传完整 file:///，不能把 null 当成真实面板 URL。
旧 panel_origin 仍兼容，但省略完整 URL 不能产生虚假配对或覆盖已配对地址。
成功响应包含 agent_version、panel、mihomo_controller.api_base、mihomo_controller.secret 和 features。
secret 会交给当前网页 JavaScript。服务控制 token 不会返回给网页。
面板 URL 只在 Agent 内存保存；退出托盘保留 Agent 时配对仍有效，Agent 真正退出后清除。
配对 source 是 paired/default/builtin，network 是 checking/online/offline；未配对时 panel_url 可省略。
完整请求与响应示例见 [快速接入的握手章节](frontend-integration.md#3-不使用接入类的最小握手)。

浏览器可能要求用户授权访问本地网络；面板保留“连接本机”交互，周期扫描并在失联后重连。
跨协议跳转到 file 可能受浏览器限制，托盘和通知通过系统直接打开当前目标。

## 状态、设置与启停

| 方法 | 路径 | 请求/主要返回 |
| --- | --- | --- |
| GET | /api/agent/real-status | 实际内核、系统代理、TUN、模式、LAN 与持久设置。托盘请求 `?metrics=1` 时可附加 `core_memory_bytes` |
| GET | /api/agent/outbound-ip | Cloudflare Trace 观察到的公网出口。返回兼容字段 `ip`（优先 IPv4）、可选 `country_code`、机房代码 `colo` 和单独探测的 `ipv6`。已探测但未取得公网 IPv6 时 `ipv6` 为空字符串 |
| GET | /api/agent/events?since=0 | 增量事件列表，since 是事件 ID |
| GET | /api/system/status | 自启、服务注册/ready/error、settings |
| POST | /api/agent/master | {"enable":true} 或 false，总开关 |
| POST | /api/agent/settings | 下表所列设置的部分更新 |
| POST | /api/system/autostart | {"enable":true} 或 false，当前用户登录自启 |
| GET | /api/agent/geo-low-memory | 当前 Geo loader、enabled、restart_requested |
| POST | /api/agent/geo-low-memory | {"enable":true} 或 false；enable 必须显式是布尔值 |

real-status 主要字段：

- is_actually_intercepting、real_system_proxy、real_tun、mihomo_running。TUN 生效时可返回 `real_tun_device`。
- actual_mode、real_allow_lan、last_error、settings。
- settings.enabled 是意图；显示实际接管应结合真实状态字段。

总开关在普通 Agent 或内核系统服务冷启动时恢复为暂停。连接已经运行的服务会保留该会话状态。Windows meow-rs 依订阅启用 DNS 接管前保存网卡配置，退出恢复仍残留的回环 DNS。

settings 部分更新接受：

| 字段 | 类型 | 约束/用途 |
| --- | --- | --- |
| use_system_proxy | bool | 系统代理偏好 |
| use_tun | bool | TUN 偏好，需要接管时按需授权 |
| tun_stack | string | mixed / gvisor / system / mips |
| tun_device | string | 网卡名。Windows/Linux 使用 `Reia` 开头的 4–15 位 ASCII 字母、数字、连字符或下划线；macOS 使用 `utun` 加 0–9999 的编号。空字符串自动选择 |
| allow_lan | bool | 只调整代理入口，不开放控制接口 |
| auto_update_enabled | bool | 内核自动检查/更新偏好 |
| update_interval_hours | int | 1–168 |
| outbound_mode | string | rule / global / direct |

总开关、开机启动、服务模式、Geo loader 分别使用对应端点，不能直接把完整 settings 对象回传到部分更新接口。

设置网卡名可向 `POST /api/agent/settings` 发送 `{"tun_device":"ReiaWork"}`。暂停时保存至下次启用，活动 TUN 在原内核进程中重建监听，有短暂中断的可能。非法名称返回 HTTP 400 和 `invalid_tun_device`。握手的 `tun_device` 特性表示支持此字段。

`outbound_ip` 表示出口 IP API 可用，`outbound_ip_trace` 表示数据来自 `https://1.1.1.1/cdn-cgi/trace`。开启接管时，查询通过本机 Mihomo 入口，当前规则和模式仍决定出站。暂停时直接查询。查询失败返回 HTTP 502 和 `outbound_ip_unavailable`。旧 Agent 可能返回可选 `org`，Trace 版本返回可选 `colo`。
Geo loader 开启为 memconservative，关闭为 standard；restart_requested 只表示已发出重启，不表示重启已完成。

### 状态响应示例

GET /api/health：

```json
{"status":"ok","agent_version":"示例版本","agent_pid":1234}
```

GET /api/agent/real-status：下面是内核运行、总开关关闭的示例，不是“内核已退出”。

```json
{
  "is_actually_intercepting": false,
  "real_system_proxy": false,
  "real_tun": false,
  "mihomo_running": true,
  "actual_mode": "rule",
  "real_allow_lan": false,
  "settings": {
    "enabled": false,
    "use_system_proxy": false,
    "use_tun": true,
    "tun_stack": "mips",
    "tun_device": "ReiaClash",
    "allow_lan": false,
    "geo_low_memory": true,
    "autostart_enabled": false,
    "auto_update_enabled": false,
    "update_interval_hours": 24,
    "outbound_mode": "rule"
  }
}
```

last_error/actual_mode 在没有值时可能省略。settings 还可能含 subscription_url、active_binary、previous_binary；前端不要把这些信息整体写到站点日志。
GET /api/system/status 使用相同 settings 结构，并包含前述服务与自启字段。
core_service_registration 的 backend 示例为 windows-scm/systemd/launchd，name 为对应服务名；executable、legacy_helper、error 可省略。
system-status.core_service_error 是独立字段，应与 registration.error 一起用于错误展示。

## 系统服务

POST /api/system/core-service：
```json
{"enable":true}
```

修复/迁移：
```json
{"enable":true,"repair":true}
```

卸载：
```json
{"enable":false}
```

操作可能触发系统授权，前端需防止重复提交并允许足够等待时间。
Linux 在授权前检查用户路径和应用旁 data 的读写能力；当前用户所有的服务程序/配置根目录会去掉组与其他用户写位，root 所有且不可被组/其他用户写入的系统程序也可使用。配置目录与 token 仍须属于登录用户。路径不符合要求时返回原有错误响应，message 包含具体路径及对应的归属/权限或系统错误，不要在前端将其简化成统一“没有权限”。测试 WebUI 在服务设置区保留可复制的操作错误详情。授权后的安装器仍复查安全条件；本轮没有更改请求字段或响应格式。
成功响应有 success 与 core_service_enabled；最终状态再次查询 /api/system/status。

系统状态字段：

| 字段 | 意义 |
| --- | --- |
| core_service_supported | 当前构建是否有安装器，不保证系统依赖已就绪 |
| core_service_enabled | 该状态接口中为实际已注册且 Agent 已委托 |
| core_service_delegated | Agent 当前使用服务委托，包括待修复或旧版委托，配合实际注册状态使用 |
| core_service_ready | OS 报运行且内核服务状态确认核心运行 |
| core_service_error | 服务委托/内核的具体错误 |
| core_service_registration | backend、name、installed、running、executable、legacy_helper、error |
| autostart_enabled | 当前用户启动项 |
| settings | 当前持久偏好 |

注册开关可按 registration.installed/running 展示已安装状态，但必须单独呈现 ready/error。
文件标记、已注册、OS 运行中、内核就绪是不同状态，不能合并成一个成功判断。
Linux 移除后收到明确的 org.freedesktop.systemd1.NoSuchUnit，且无已加载/存活证据时，状态为未注册而非卸载失败；AccessDenied、总线故障或仍在运行/退出的服务继续按错误处理。请求和响应字段不变。
关闭服务会关闭 TUN 并切回普通管理；失败或取消不能假装已卸载。

POST /api/agent/exit：

- 服务模式：202，success:true、background:true，保留 Agent API 与内核；托盘收到结果后退出自身。
- 普通模式：恢复本项目系统代理、停止内核、恢复已记录的 DNS，202 后请求 Agent 退出。
- 该接口不关闭浏览器标签，也不是卸载服务接口。

该 API 自身不会远程移除托盘。只有托盘菜单自己的退出流程会在收到 API 响应后退出托盘，网页直接调用不可据此宣称托盘已关闭。

托盘在用户选择“注销服务并退出”后调用 `POST /api/agent/exit/uninstall-service`。接口要求确认系统服务已卸载，恢复本项目的系统代理备份，停止本地监督器并返回 HTTP 202，然后关闭 Agent。卸载失败或取消时返回错误并保留 Agent API。浏览器页面调用此端点返回 HTTP 403 和 `desktop_only`。

## 节点与订阅

| 方法 | 路径 | 请求/响应 |
| --- | --- | --- |
| GET | /api/core/proxies | mihomo proxies，附 group_order 订阅组顺序 |
| POST | /api/core/select | {"group":"组名","node":"节点名"} |
| POST | /api/core/group-delay | {"group":"组名"}，返回核心组测速数据 |
| POST | /api/subscription/url | {"url":"https://example.com/subscription"}，下载、校验并应用 |
| GET | /api/subscription/status | 订阅刷新状态，不返回 URL 或 token |
| POST | /api/subscription/refresh | 无需 URL，重新获取并应用已保存的最新订阅 |
| GET | /subscription/source | 本机浏览器顶层只读页面。默认显示订阅缓存 YAML，`?view=runtime` 显示当前运行内核的 YAML。返回 HTML，关闭 CORS、嵌入和缓存 |

`POST /api/core/select` 保存组与节点的手动选择。重启、换内核或热加载订阅后恢复仍存在的选项。

切节点会关闭现有连接以便后续流量使用新节点；前端只更新选中项，不刷新框架。
订阅内容中的客户端受管字段会被覆盖，原始订阅与生成配置分别保存。
实际订阅地址可能含凭证，不要写入接手文档或测试固定数据。

`POST /api/subscription/url` 在合法新 URL 被接受后先覆盖持久化 `settings.subscription_url`，再下载、校验并应用。后续下载或校验失败保留现有配置，后台用新 URL 重试。内容变化且内核就绪时通过原生 `PUT /configs?force=true` 的 `payload` 字段热加载生成的 YAML（mihomo 传原文，meow-rs 传 Base64），内容相同且此前刷新成功时跳过加载。成功响应兼容原有 success/message，失败使用原有错误格式。HTTP(S) 订阅受公网地址、重定向检查和 16 MiB 限制。

Agent 每次启动立即检查已保存 URL，运行期间每 12 小时刷新，失败退避 1～30 分钟；这不要求网页保持打开，不改变 Enabled、系统代理或 TUN 偏好。设置增加 `subscription_updated_at`（成功时间，Unix 秒，默认 0）。握手 features 增加 subscription_refresh。网页轮询、启动自动刷新、手动导入串行；代理设置回滚不覆盖新订阅元数据。

订阅状态的 `successful_attempt` 是当前 Agent 的成功刷新序号。主题遇到不确定的网络响应会比对操作前后的实际状态，避免把已完成的更新当成失败。

状态示例：`{"has_url":true,"refreshing":false,"last_success_at":1700000000,"last_attempt_at":1700000000,"next_refresh_at":1700043200,"refresh_interval_hours":12}`。时间为 Unix 秒，尚未成功时为 0；next_refresh_at 是常规周期时间，失败时实际可提前退避重试。last_error 可省略，只保留不含订阅凭证的概括错误。attempt/error 是本次 Agent 内存状态，成功时间持久化。

Aether M3 已按用户要求在发现客户端后及每 4 小时自动推送当前账号订阅，另保留手动安装页；接入及部署边界见 [主题接入](xboard-integration.md)。

## 连接与 DNS

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | /api/core/connections | mihomo 原生连接列表/统计 |
| DELETE | /api/core/connections/{id} | 关闭单条连接 |
| DELETE | /api/core/connections | 关闭全部连接 |
| GET | /api/core/dns/query?name=example.com&type=A | 通过内核解析并记录结果 |
| GET | /api/core/dns/records | 返回 records、capture_error、limit、scope |
| DELETE | /api/core/dns/records | 清空内存记录 |

DNS type 默认 A，还接受 AAAA、CNAME、MX、TXT、NS、SOA、PTR、SRV、HTTPS、SVCB。
记录字段包含 id、at、source，以及可选 name/type/payload/result/error/duration_ms。
source 为 manual 或 kernel；limit 为 200。仅保存本次 Agent 运行中的记录，不是完整 DNS 历史。
内核不支持某路由或日志格式变化时，应显示实际错误，不伪造查询/连接结果。

## Agent 缓存、日志和目录

| 方法 | 路径 | 请求与响应 |
| --- | --- | --- |
| POST | /api/agent/cache/clear | 无请求体；200，`success:true`，`cleared:{cache_files,dns_records}` |
| POST | /api/agent/logs/clear | 无请求体；200，`success:true`，`cleared:{log_files,events}` |
| POST | /api/agent/directory/open | `{"directory":"cache"}` 或 `{"directory":"config"}`；202，`success:true`、`directory`、`message` |

缓存清理只删除用户配置目录 `updates/` 内中断下载/解压产生的普通临时文件（`.mihomo-download.tmp`、`mihomo-*.tmp`、`mihomo-*.tmp.exe`），并清空当前 Agent 的 DNS 内存记录。更新正在下载/校验时拒绝清理，待完成后再调用。保留已校验的 `update.pack`、活动/待切换/回滚内核、`bin/`、应用旁的 `data/`、订阅、设置、密钥、服务记录和代理备份；不清除内核自己的 DNS/fake-IP 缓存。

日志清理原地截断 `agent.log`、`agent-startup.log`，并清空 Agent 内存事件；不删除文件，不重启 Agent，后续日志继续写入。事件/DNS ID 保持递增，已连接面板的增量游标仍有效。服务日志、系统 journal、网页当前显示的 Mihomo WebSocket 日志不在此范围。运行中可能立即产生新日志或记录。

打开目录由用户会话 Agent 调用 Windows Explorer、Linux `xdg-open` 或 macOS `open`。`cache` 对应 `updates/`，`work` 对应应用旁 `data/`，`config` 对应用户配置根目录；不接受任意路径。202 只表示已请求系统打开，实际文件管理器窗口取决于当前桌面会话。缺少/错误目录类型、未知字段或多余 JSON 返回 400；本地清理/启动文件管理器失败返回 500 `operation_failed`，可能已完成部分清理。无需系统提权。握手 `features` 增加 `agent_maintenance`。

`GET /api/agent/directory/sizes` 返回 `directories.cache/work/config`，各含 `bytes`、`files` 和 `partial`。统计逻辑文件大小，不跟随符号链接。配置根目录统计排除已单独统计的 `updates/`。`POST /api/agent/directory/open` 的 `work` 类型对应应用旁 `data/`。
 
### 登录启动、外部资源与清理

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| POST | /api/system/autostart/proxy | `{"enable":true}` 注册登录启动项并保存静默启动代理。关闭此项只关闭静默代理。GET /api/system/status 返回 `silent_proxy_on_login` |
| GET | /api/agent/resources | 可同步条目、大小和更新时间，及定时更新偏好 |
| POST | /api/agent/resources/settings | `{"enable":true,"interval_hours":24,"kinds":["country","geosite","rules"]}` |
| POST | /api/agent/resources/sync | `{"kinds":["country","rules"]}`，同步指定项 |
| POST | /api/agent/config/subscription/clear | `{"confirm":true}`，清除本机订阅及缓存配置，暂停网页自动导入 |
 
资源 ID 为 `country`、`geoip`、`metadb`、`geosite` 和 `rules`。Geo 使用固定发布源，`rules` 同步当前配置中的 HTTP 规则集。计划可设置 1–168 小时间隔。清除订阅保留客户端偏好、授权凭据和节点记忆。手动导入恢复自动同步。目录容量接口另含 `directories.logs`，日志清理只处理固定日志文件。工作目录没有清除接口。

## 重启与更新

| 方法 | 路径 | 请求/用途 |
| --- | --- | --- |
| POST | /api/core/restart | 手动重启当前管理模式下的内核，返回 202 |
| POST | /api/agent/restart | 重启兼容入口 |
| GET | /api/core/type | 返回 `{"success":true,"kind":"mihomo"}`，类型也可能是 `meow-rs` |
| POST | /api/core/type | `{"kind":"meow-rs"}` 或 `{"kind":"mihomo"}`。目标内核启动验证完成后返回 200 |
| GET | /api/agent/check-update | 查询当前内核类型的官方稳定版和适配资产 |
| POST | /api/agent/upgrade | 手动模式验证目标内核启动后返回 200。静默模式验证并保存 `updates/update.pack` 后返回 200，`pending:true` 表示等待应用 |

切换到 meow-rs 时，Agent 通过正在运行的 mihomo mixed 代理获取发布元数据和资产。校验归档 SHA-256、可执行文件和当前配置后开始交接。交接期间下载请求和生命周期请求可能持续数十秒。启动验证失败会尝试恢复上一个可用内核，并返回操作错误。

`GET /api/agent/real-status` 包含 `core_kind`。轻量轮询可使用 `GET /api/health` 的 `agent_pid` 和 `core_busy`，忙碌期间暂停详细状态和列表轮询。完整面板 URL 变化或 Agent PID 变化后重新握手。固化更新包在暂停接管、Agent 启动（含自启）或手动重启内核时应用。重启接口仍返回 202，面板可随后通过状态接口确认运行情况。

请求 `POST /api/agent/master` 开启且两种接管方式均未配置时，Agent 选择原版 mihomo、TUN 和 mips。需要权限时走原系统授权流程。

`POST /api/subscription/url` 可传 `sync_only:true`。当前链接一致且最近 12 小时已有成功刷新时直接确认，URL 变化仍立即应用。省略时保留手动导入语义。

## 响应、错误与重试约定

| 情况 | HTTP/返回 | 前端处理 |
| --- | --- | --- |
| 普通设置、节点选择、服务操作成功 | 通常 200，success:true，部分附 settings/状态 | 完成后读取实际状态 |
| 重启、更新、退出请求接受 | 202，success:true | 不把 202 当作所有异步操作已经完成 |
| 无效 JSON、未知字段、无效参数 | 400，invalid_request 或具体参数码 | 修正提交字段 |
| Host 或配对来源不符 | 403，loopback_only/origin_mismatch 等 | 核对访问地址与实际网页来源 |
| 权限错误 | 通常 412，permission_required | 显示系统授权结果，查询当前状态 |
| 核心路由不支持 | 通常 502，mihomo_route_unavailable | 当前功能显示不可用 |
| 网络/核心不可达 | 通常 503，mihomo_unavailable | 保持 Agent 面板并显示内核错误 |
| 其他操作失败 | 通常 500，operation_failed | 展示 message，不假定状态已回滚 |

```json
{"success":false,"code":"permission_required","message":"示例：当前操作需要系统网络权限"}
```

错误码可扩展，个别错误按底层类型映射，不能假定所有取消授权都必然是 412。
原生核心转发可能直接返回其 JSON，404/方法错误等也可能是非 JSON；接入层应保留 HTTP status 并处理空响应。
Agent 没有前端写操作的通用幂等键协议；超时/断网后不自动重发重启、安装、卸载、订阅导入或更新，应先查询实际状态。
服务授权操作前端可等待最多约 5 分钟，读请求通常取 10 秒；用户取消浏览器请求不等于取消后台 OS 操作。

## 内部服务端点

127.0.0.1:25501 的 GET /status、POST /start、/restart、/stop 使用独立 Bearer token，仅供 Agent 与特权服务交互。
/stop 停的是内核监督器，不是从 SCM/systemd/launchd 卸载或结束服务宿主。
WebUI 不应直接调用这些端点，也不应获取 core-service.token。
原生 mihomo :9090 的 REST、logs/traffic/memory WebSocket 使用握手返回的 mihomo secret。
