# Aether M3 与 ReiaClash 接入

2026-11-17 在客户端目录下的 `Xboard-Theme-Aether-M3` 完成源码接入。主题依然使用原 XBoard API、认证、路由混淆与 MD3 布局；客户端控制使用独立 loopback 适配器。

## 当前行为

- 首页每约 5 秒验证 `127.0.0.1:25500/api/health` 并握手，不通过直接探测 9090 判断客户端存在。未检测到 ReiaClash 时隐藏客户端控制卡片；启动后自动出现，暂停代理或内核未就绪仍可管理。
- 首页新增 ReiaClash 控制台：真正的 Agent 总开关、系统代理/TUN 偏好、订阅状态、客户端设置与手动订阅入口。已有布局配置只补入新卡，不重置其他卡片顺序和尺寸。
- 套餐订阅卡保留原业务入口。首页代理总开关、状态和出口 IP 同在 `ReiaConsoleCard.vue`。Trace 在启停、模式或节点变化后刷新，探测失败时明确显示暂不可用。`ClientConfigCard.vue` 通过原文件 SHA-256 回归。
- 节点、连接、规则、日志继续使用主题已有页面；托管模式/节点切换、代理设置、重启/升级交给 Agent。原生查询及内核 DNS/Fake-IP 缓存操作使用握手返回的控制器密钥。节点组使用 Agent 的 group_order，并保留同名站点优先和规则/全局/直连的显示约定。
- 首页客户端控制使用主题现有的 Material Web 按钮与开关。客户端设置页接入 TUN 协议栈和网卡名、LAN、Geo 低内存、用户自启、系统服务安装/修复/卸载、内核更新及 Agent 缓存/日志/目录维护。代理接管或系统授权由用户点击触发。
- 没有客户端卡片时保留一个小型“连接本机客户端”入口，用于浏览器要求用户手势授予本地网络权限的情况；不会伪造授权或自动拉起程序。
- 连接页提供实时活动连接、会话内历史、速率、搜索、详情和断开。日志页提供级别与正则搜索、暂存、暂停显示、复制和自动重连。规则页提供规则集刷新，节点页保留订阅顺序、支持搜索与组测速。
- 设置、节点、规则、订阅、连接和日志使用 Aether M3 色板与 Material Web 组件。图标先读取 XBoard 的 `custom_icon_url`。CSS 外链按样式表加载，WOFF2 先查完整 URL 对应的浏览器缓存，未命中时由匿名沙盒下载。留空、失败或超时后使用主题内置字体。内置文件随附 Apache 2.0 许可证。 [Material Symbols](https://developers.google.com/fonts/docs/material_symbols)

## 订阅同步

网页检测到客户端且用户已登录后，从 XBoard `/user/getSubscribe` 重新取得当前账号的 `subscribe_url`，推送至 Agent。页面保持运行时每 4 小时重新获取并推送，URL 变化会立即同步；账号切换或组件卸载时丢弃尚未推送的旧响应。在途推送期间收到新 URL 变化，会排队重新获取最新链接。

网页计时器会受后台节流/休眠影响，恢复可见时检查是否到期。XBoard 的认证只发给站点自身，控制器密钥只在页面内存中，loopback 请求不使用站点 axios 拦截器、不发送站点 Authorization/Cookie。自动同步错误不包含订阅凭证。

Agent 接收合法新 URL 后先持久化覆盖旧 URL，再下载、校验并应用。就绪内核通过原生配置接口热加载，下载或校验失败时保留原有可用配置，并使用新 URL 重试。内容未变化且已有成功记录时跳过加载。下载使用 Clash.Meta 标识和 YAML Accept，以获取兼容的配置格式。

Agent 随用户登录自启或手动冷启动时立即刷新已保存 URL，此后每 12 小时刷新；失败按 1 分钟起、最长 30 分钟退避重试。这里“离线刷新”指不依赖网页打开，仍需要 Agent 运行以及网络可用，不新增独立的系统计划任务，也不默认更改用户自启偏好。服务模式退出托盘保留 Agent，所以原有后台刷新继续运行。

订阅操作串行，应用阶段与服务切换协调；运行配置与订阅文件增加事务锁，设置/Geo 变更从锁内读取最新订阅，避免把后台新订阅覆盖回旧内容。代理设置失败回滚时保留最新订阅 URL 和成功时间。

手动兜底页路由为 `/clash/subscription`（实际地址栏继续使用主题路由混淆），提供安装 URL、刷新已保存订阅和连接本机按钮。客户端根目录测试 WebUI 原有手动导入也保留。

## 主要源码

| 文件 | 职责 |
| --- | --- |
| Xboard-Theme-Aether-M3/src/api/reia.ts | Agent 身份检测、完整 URL 配对、内存凭据、写操作门控、超时及 WebSocket |
| Xboard-Theme-Aether-M3/src/api/clash.ts | 主题旧组件接口适配，托管写操作交给 Agent |
| Xboard-Theme-Aether-M3/src/stores/clash.ts | 自动发现/断开/重连、状态与流、账号订阅联动、卸载清理 |
| Xboard-Theme-Aether-M3/src/utils/reiaSubscriptionSync.ts | 4 小时与链接变化同步、账号/过期响应保护 |
| Xboard-Theme-Aether-M3/src/components/clash/ReiaConsoleCard.vue | 首页控制台 |
| Xboard-Theme-Aether-M3/src/views/clash/SubscriptionView.vue | 手动订阅兜底 |
| internal/agent/subscription.go | URL 覆盖、下载/校验/应用、启动/12 小时刷新、退避与状态 |
| internal/mihomo/profile_store.go | 配置事务锁、读取最新订阅后重建运行配置 |
| internal/httpapi/handlers_subscription.go | 订阅状态与刷新接口 |

## 构建、打包与验证

客户端与主题需要一起更新。客户端最新四组件位于 `.build/validation/windows-amd64`、`linux-amd64`、`darwin-amd64`；运行中的旧程序没有被本轮停止或替换。主题包位于 `Xboard-Theme-Aether-M3/MD3-Theme.zip`，用于上传到现有 XBoard 的主题管理，本轮没有发布线上站点。

在主题目录：

```text
node --test scripts/reia.test.mjs
npm run build
node scripts/reia-browser-test.mjs
npm run pack
```

Windows PowerShell 限制 npm.ps1 时使用 npm.cmd。浏览器测试使用本机 Chrome，可用 CHROME_PATH 指定。受限宿主测试使用 `CHROME_TEST_NO_SANDBOX=1`。测试服务使用随机 loopback 端口并模拟站点、Agent、内核和 WebSocket。桌面与移动暗色场景都检查本地字体、就绪、写入互锁、连接详情和日志重连。打包会移除 `dist/assets/index.html`，再次浏览器测试前须重新 build。

外链字体测试使用独立的本地 HTTP 源，覆盖带查询参数的 CSS、无凭据沙盒 WOFF2、缓存复用与失败回退。配置腾讯云 COS 等对象存储时，沙盒发出的 WOFF2 GET 使用 `Origin: null`，资源须允许此无凭据的跨域读取。 [腾讯云 COS 跨域设置](https://cloud.tencent.com/document/product/436/13318)

客户端新增回归：

```text
go test ./internal/agent ./internal/httpapi -run TestSubscription -count=1
go test ./internal/mihomo -run TestBackgroundProfile -count=1
```

待部署实测包括真实 XBoard 订阅输出及令牌轮换、浏览器 HTTPS 到 loopback 的权限、目标桌面的 TUN/服务授权和休眠后的定时行为。验证记录见 [项目状态](status.md)。
