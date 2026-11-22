# 项目状态与验证记录

更新日期：2026-11-19。此文件描述代码和本轮验证范围，运行中的进程状态只代表检查当时。
接手入口：[context.md](../context.md)；详细设计：[architecture.md](architecture.md)；接口：[api.md](api.md)。
本次过程与下一步验收顺序见 [2026-11-17 任务交接](handoff-2026-11-17.md)。

## 2026-11-22 退出恢复、内核记忆与托盘内存

- Windows meow-rs 接管 DNS 前保存出站网卡 IPv4、IPv6 配置，内核退出后按记录恢复本项目残留的回环设置并刷新 DNS 缓存。已观察到接管且内核把原来的静态 DNS 清空时，也恢复已保存的静态地址。继续后台运行保持接管。完全退出恢复系统代理备份，Windows 也会清理明确指向项目混合端口的残留代理。
- 注销服务后完全退出保留 TUN 偏好并暂停接管。已确认 meow-rs 的冷启动发现优先，版本探测超时继续交给监督器。托盘展示运行内核与进程常驻内存。
- Windows 上已验证读取两族 DNS 恢复记录、DNS 归属判断、真实内存读取、完整退出保留偏好和已确认 meow-rs 冷启动顺序。未改动正在使用的网卡 DNS 或重启现有服务，真实退出后的系统恢复仍待实测。旧会话没有客户端 DNS 备份时无法重建未知的静态地址。

## 2026-11-22 meow-rs 实际接管与登录静默代理

- 本机控制器曾回报 `tun.enable=false`，而双栈分段默认路由指向 meow-tun。Windows 原生路由探测确认 meow-tun，Google 域名返回 Fake-IP 且 ICMP 有回复。通过系统路由发出的 HTTPS 请求返回 204。
- meow-rs 的 TUN 设置改为受管进程停启并确认接管，Windows 状态接口以实际路由补充控制器回报。静默代理可独立开启并注册登录启动项，三平台启动入口显式传递登录参数。
- Go 测试、静态检查、Windows 构建、Linux/macOS amd64 交叉构建、测试 WebUI 检查和主题桌面及移动回归通过。主题回归覆盖登录启动关闭时直接开启静默代理。正在运行的系统服务未重启，新版受管停启及真实登录待在用户会话验证。

## 2026-11-22 meow-rs 回环 DNS 冲突与运行配置查看

- 本机只读检查确认 UDP `127.0.0.1:53`、混合代理端口和控制器由同一个 meow-rs 进程持有，当时控制器报告 TUN 已关闭。Windows 的 meow-rs 热启动遇到回环 DNS 地址冲突时，Agent 重启一次受管内核并验证 TUN，失败沿用设置回滚。托盘与两种前端都以实际接管状态发出下一次命令。
- 本机查看页提供原始订阅和当前内核配置，按运行中的二进制选择 YAML。Go 测试覆盖实际 TUN 与保存偏好不一致时的重复启动和暂停、配置文件选择以及查看页安全响应。Go 静态检查、三平台构建、测试 WebUI 检查、主题桌面和移动回归通过。真实特权服务继续运行，自动重启恢复尚未在该服务上执行。

## 2026-11-22 meow-rs DNS 接管与原始订阅查看

- Windows/Linux 的 meow-rs 在订阅启用 DNS 且明确设置通配 UDP 53 时生成 `dns-hijack: ["any:53"]`，启动和热切换共用同一策略。mihomo 通过原有配置路径运行。meow-rs 计划停止及启动超时退出前尝试关闭 TUN。
- Aether M3 和测试 WebUI 新增原始 YAML 查看入口。Agent 提供只读本机顶层页面，对原文做 HTML 转义并禁用跨源读取、嵌入和缓存。
- Go 测试和静态检查、两种真实 Windows 内核配置预检、Windows/Linux amd64/macOS amd64 构建、主题桌面与移动回归和 ZIP 打包通过。隔离 meow-rs v0.29 验证控制器热切换 DNS 配置，并从独立本地监听端口取得 Fake-IP。实际 TUN、海外 DoH 出口以及强制退出后的系统 DNS 恢复仍待实测。

## 2026-11-22 meow-rs 出站与订阅状态确认

- 修复内核切换时用当前内核配置校验候选内核的回归。原始订阅从 `profile.yaml` 读取，两种运行配置分别写入 `mihomo.yaml` 和 `meow.yaml`。切换无需重新下载订阅。
- meow-rs 尊重缓存订阅的 IPv6 偏好，补齐默认 TUN IPv6 地址并保留系统 DNS。切换验证加入系统 A、AAAA 和 IPv6 出站，面板分开展示 IPv6 探测结果，连接页优先使用嗅探域名。
- 本轮通过两种真实 Windows 二进制的双向配置预检，覆盖旧 `mihomo.yaml` 含 meow 专用字段的场景。隔离的 meow-rs v0.29.0 已从 IPv6 UDP 上游取得 AAAA，关闭真实网卡接管时验证了 TUN 配置热更新及控制器回读。Go 测试、静态检查和 Windows、Linux amd64、macOS amd64 构建通过。Chrome 桌面与移动回归覆盖 IPv6 单独展示、探测失败保留 IPv4、连接表显示嗅探域名，主题已打包。现用 DNS、物理网卡 IPv6 路由和实际 TUN 接管仍待实测。

- 切换到 meow-rs 时对比旧内核已通过的混合端口出站和手动节点检查，候选内核验证失败会回退。已选 meow-rs 从暂停恢复时也检查混合端口出站。日志可见当前版本跳过 TUIC 节点，部分 REALITY 节点握手失败，首次可用到后来失效的具体变化尚未复现。
- Windows 和 Linux 的 meow-rs 启动与热更新统一使用其 TUN 路由配置，保持系统 DNS，订阅未配置协议嗅探时补充 HTTP/TLS 域名识别。Aether M3 通过 Agent 的成功尝试编号确认结果，短暂接口故障后重新核对，保留上次成功记录。
- Go 测试、静态检查、Windows 构建及 Linux/macOS amd64 交叉构建通过。Windows meow-rs v0.29.0 通过生成配置检查和独立混合端口到本地 HTTP 上游转发测试。主题类型检查、11 项单元测试、Chrome 桌面及移动端回归和 ZIP 打包通过。当前订阅节点、Windows/Linux 实机 TUN 和线上主题部署待验收。

## 订阅热加载安全路径修复

- 普通 mihomo 使用 YAML 原文，meow-rs v0.29.0 使用 Base64 YAML，两种都通过控制器的 `payload` 字段提交。原配置路径、安全路径和 data 目录保持原设置。
- Windows 隔离进程已复现原版 mihomo 拒绝目录外 path，并实测两个内核接受各自的 payload 且配置实时生效。模拟控制器回归覆盖两种格式、资源配置重载和拒绝后的文件回滚。三平台已重新构建，现有运行中的 Agent 和系统服务未重启。

## 登录静默代理与外部资源

- 主题登录自启下新增静默代理。登录时检查网络、DNS、内核及混合端口，沿用系统授权流程，同步检查接管后连通性。手动打开客户端仍遵守冷启动暂停规则。
- 存储与目录集中提供缓存、订阅、日志清理，日志单独计数，data 只允许查看和打开。订阅需明确确认，并暂停自动导入直至用户手动导入。
- 外部资源页支持固定源 Geo 数据及订阅中的 HTTP 规则集同步和定时更新。订阅热加载提交内联 YAML `payload`，已确认的内核保留选择。
- Go 回归、Go 静态检查、三平台构建、主题类型检查和 Chrome 桌面/移动端回归通过。已打包主题。未触发真实登录自启、UAC 或在使用中的配置下下载覆盖 Geo 文件。

## 冷启动和 DNS 接管

- 普通 Agent 与系统服务冷启动时暂停代理，保留配置偏好和已运行后台服务的重连语义。Windows meow-rs 禁用改写系统 DNS 的 TUN 选项，计划重启/停止前先尽力关闭 TUN。
- Windows 独立端口实测从已保存开启状态冷启动仍暂停，保留 TUN + mips 偏好且 API 正常。Windows 配置生成、Go 测试、静态检查和三平台构建通过。真实特权服务切换未在此测试执行。

## 代理组节点记忆

Agent 保存面板手动选中的组与节点，普通内核和服务内核启动以及订阅热加载时共用该记录。模拟控制器验证了重建 Manager 后恢复有效节点和保留暂时不可用的节点；未切换正在运行的特权服务。

## 应用目录 OTA 和存储子页

- 正式内核移至应用目录，旧版暂存路径在启动时迁移。Windows meow-rs 与 Wintun 同目录。每种内核保留最近三份 OTA 备份。
- Windows 独立端口实测真实 mihomo 从固定路径运行，暂停代理、手动重启内核和 Agent 冷启动均能应用离线 update.pack。文件回滚、三份备份保留、原有用户备份保护及包校验回归通过。
- Aether M3 的存储子页与目录打开请求通过 Chrome 桌面和移动端回归，主题 ZIP 已打包。Go 测试、静态检查和三平台构建通过。现有特权服务、登录自启及线上站点未在本轮切换或部署。
 
## 2026-11-22 GeoIP 启动、接管默认值和订阅合并

- Windows 服务日志定位到 meow-rs 在控制器就绪前等待缺失的 Country MMDB。切换前经旧内核准备 Country MMDB，延长 meow-rs 启动窗口，报告控制器、混合代理端口和 TUN 验证阶段。现场特权内核切换未重跑。
- 总开关启动时没有接管方式会应用原版 mihomo、TUN 和 mips。关闭最后一种接管方式时同步关闭总开关。模拟控制器的 mips 回退测试通过。
- 订阅同步去重、GeoIP 引用检测、Go 测试和静态检查通过。Aether M3 类型检查、Chrome 桌面与移动端回归及主题打包通过。线上站点验收待部署。

## 2026-11-22 Aether M3 与通知品牌

- 实际主题的内核维护支持双向切换、检查更新、手动与静默更新。通过健康接口同步操作锁，减少握手与状态重读。
- Windows 四个 EXE 的图标和 ReiaClash 版本资源已逐个核对。macOS 四个应用入口与 Aether M3 ZIP 内置图标已核对。桌面通知统一通过托盘以 ReiaClash 发送。
- Go 常规测试和静态检查、Windows/Linux/macOS 构建、主题类型检查及 Chrome 桌面/移动端 DOM 回归通过。主题 ZIP 已生成。线上 XBoard 与实际桌面通知中心显示待部署后核对。

## 2026-11-22 内核交接与面板状态

- 实现 mihomo / meow-rs 切换、原代理下载、校验后交接及失败回退。Windows 官方 meow-rs 包携带的 wintun.dll 随版本安装。
- 实现手动和静默更新的启动确认与旧受管理二进制清理。Agent 会处理暂存包并清理上次中断留下的包。
- WebUI 合并健康与状态轮询，握手随 URL 或 Agent 实例变化更新。连接表复用现有行，DNS 无变化时保留现有内容。
- Windows 普通模式处理受限路径查询和已受 Job 约束时的嵌套 Job 限制。原问题电脑的安装目录权限、桌面启动环境和 TUN 授权仍待现场复测。
- Windows 真实 mihomo 回归、Go 静态检查、WebUI 回归及 Windows、Linux amd64、macOS amd64 四组件构建通过。meow-rs v0.29.0 Windows 官方包 SHA-256 和配置预检通过。Linux/macOS 图形桌面、特权服务和真实 TUN 未在本轮环境运行。
- Windows 独立端口验证通过：静默更新完成后删除旧受管理可执行文件和临时包，通过运行中的 mihomo 下载并切换到 meow-rs，再切回 mihomo。另在验证副本注入 meow-rs 启动失败，确认系统恢复原 mihomo。

## 2026-11-22 Windows 安装包

- 从 `installer/` 生成 `.build/installer/ReiaClash-Setup.exe`，大小为 44,904,996 字节。保留 mihomo、meow-rs、Wintun、离线 WebUI、预置 Geo 数据和规则，排除运行生成的日志、流量及缓存。
- 使用 7-Zip 25.00 的 LZMA2 `-mx=9` 固实压缩，指定 256 MiB 字典，归档记录的有效字典为 192 MiB。Inno Setup 7.1.1 编译包含桌面与开始菜单快捷方式，以及安装结束后可选运行。成品内嵌归档通过完整性检查，14 个文件均与素材的 SHA-256 一致。新版未在真实桌面交互安装。

## 2026-11-19 Windows 安装包

- 已生成 Windows x64 `.build/installer/ReiaClash-Setup.exe`。从发布安装包取出内嵌 7z 并解压，14 个文件的 SHA-256 与打包目录一致，包括预置 GeoIP、GeoSite、规则集和离线 WebUI。
- 7-Zip 26.06 确认归档启用固实压缩。脚本传入 256 MiB 字典，此批素材约 114 MiB，编码器自动写入 128 MiB 的 LZMA2 字典。
- 隔离安装测试将默认目录和快捷方式重定向到工作区，验证带空格和中文的自定义路径、14 个安装文件、快捷方式目标和独立勾选、卸载。完成页的启动选项和正式桌面、开始菜单路径留待真实 Windows 用户会话验证。

## 2026-11-19 融合主题管理页

- 字体优先使用 `custom_icon_url`，WOFF2 继续走匿名沙盒和浏览器缓存。开发页与打包 Blade 使用同一个加载器。Chrome 模拟独立字体源验证了带参数的 CSS、无凭据 WOFF2 冷加载、重复访问命中缓存和 404 回退，主题包已重新打包。未访问用户实际 COS 地址。
- Cloudflare Trace 已接入 Agent 和旧客户端的浏览器探测。使用固定地址验证真实端点返回 HTTP 200、`ip`、`loc` 与允许跨域响应头，验证输出不记录公网 IP。实际用户的代理和去广告规则仍需端到端验收。
- 融合主题的连接、日志、节点、规则、设置、内核概览和订阅已统一 Material Web 样式。首页合并总开关和出口 IP，套餐业务入口仍可用。连接历史和日志缓冲仅存在页面内存。
- 主题类型检查、生产构建和 10 项协议/解析回归通过。Chrome 在桌面浅色及移动暗色下验证了本地图标、旧读响应拦截、写入互锁、就绪变化、TUN 栈与 LAN 确认状态、连接详情、日志重连、规则集刷新和节点选择。
- 完整 Go 测试、静态检查与根目录测试 WebUI 验证通过。新版 Agent 已为 Windows、Linux 和 macOS amd64 构建至 `.build/validation/`，打包通过并核对主题 ZIP 包含本地图标及许可证。线上 XBoard 部署尚未验证。

## 2026-11-19 托盘退出与 Windows 权限验证

- 退出选择、保留后台 API、确认注销后跳过普通内核接管以及取消或卸载失败的回归通过。Windows 句柄路径解析和共享实例锁的改动恢复了受限宿主中的内核交接与跨进程排重测试。
- `go test ./...`、`go vet ./...`、根目录测试 WebUI 的接入协议和模拟浏览器验证通过。Windows、Linux、macOS 的 amd64 四组件已构建至 `.build/validation/`。
- Windows `SHELLEXECUTEINFOW` 布局与授权前取消测试在 amd64 和 386 运行通过，arm64 交叉编译通过。尚未触发实际 UAC、卸载正在运行的系统服务或更改代理。Windows、Linux、macOS 桌面退出对话框仍待对应平台交互验证。

## 2026-11-19 出口 IP 与热加载验证

- XBoard 类型检查、生产构建、协议用例和隔离 Chrome DOM 验证通过。浏览器模拟 IP 回环查询失败，然后启动代理返回新 IP。主题安装包位于 `Xboard-Theme-Aether-M3/MD3-Theme.zip`。
- 根目录测试 WebUI 脚本、接入类用例和隔离浏览器回归通过。Chrome 在受限宿主使用测试脚本的隔离配置与 `CHROME_TEST_NO_SANDBOX=1`。
- 使用随项目提供的 Mihomo v1.19.34 和隔离端口验证同一进程接收 TUN 配置 PATCH 与配置文件 PUT。TUN 保持关闭，未操作系统代理或运行中的服务。真实系统网卡、授权与公网出口仍待现场验证。
- Windows、Linux、macOS 的 amd64 四组件已构建到 `.build/validation/`。此轮完整 Go 测试曾受 Windows 可执行路径解析和跨进程锁权限限制，随后由托盘退出与 Windows 权限修复解决。

## 当前功能

| 功能 | 实现状态 | 说明 |
| --- | --- | --- |
| 静默启动与调试入口 | 已实现 | reia 静默启动；Agent/Tray 保留控制台；独立 reia-service |
| 规则/全局/直连 | 已修正 | 持久化并热应用，处理全局组选择与现有连接 |
| 系统代理与总开关 | 已实现 | 偏好与实际接管区分，首次不主动开启，备份恢复 |
| TUN 按需提权 | 已实现 | Windows 原生 UAC、Linux polkit、macOS 授权后交给系统服务 |
| TUN 协议栈与网卡名 | 已接入 | 新配置默认 mips，支持 `tun_device`。内核就绪时热应用活动 TUN 设置 |
| XBoard 公网出口 IP | 已接入 Trace | Agent 查询与浏览器兼容入口、IPv4/IPv6、动作后刷新、明确失败态 |
| XBoard Clash 管理面板 | 已更新 | Material Web、设置写入互锁、就绪更新、连接监控和日志重连 |
| 订阅热加载 | 已接入 | 运行内核通过 `PUT /configs?force=true` 载入变更，解析失败恢复旧配置 |
| 真正的 OS 服务 | 已实现 | Windows SCM、Linux systemd、macOS launchd |
| 服务修复/卸载 | 已修正 | 实际注册状态核对，旧进程清理，防止同时启动本地内核 |
| 进程唯一性 | 已加强 | 托盘锁、Agent/服务端口预占、普通/服务内核共享锁 |
| 应用旁 data 目录 | 已实现 | Geo 预置资源不跟随更新内核迁移 |
| 内核发现与更新 | 已修正 | 平台名/权限/-v 检查，支持不同包名及嵌套归档 |
| LAN 开关 | 已修正 | 作用于代理入口，控制 API 仍限 loopback |
| 用户登录自启 | 已接入 API/面板 | 与内核系统服务的自启是不同设置 |
| Geo 低内存 | 默认开启 | 独立 API，memconservative/standard，重启应用 |
| 节点顺序与局部切换 | 已实现 | 订阅顺序、自定义排序、XBoard 同名组优先 |
| 连接管理 | 已实现 | 列表、删除单条/全部连接，面板过滤 |
| DNS 查询与记录 | 已实现 | 日志与手动查询，内存最多 200 条 |
| 手动重启内核 | 已实现 | WebUI/托盘及 API |
| 面板发现与真实 URL 配对 | 已修正 | HTTP(S)/file 完整 URL、自动重连、运行时保存 |
| 通知与托盘打开面板 | 已实现 | 优先真实配对，在线站点兜底，明确离线用内置面板 |
| 托盘服务模式退出 | 已实现 | 继续后台运行、注销服务后完全退出，或取消 |
| 前端移植接入资料 | 已提供 | API 参考、嵌入指南、无框架接入类和最小 HTML 示例 |
| Agent 缓存/日志/目录 | 已实现 | 三个 POST API，内核维护页四个按钮；范围见 API 文档 |
| Linux systemd 路径与坏单元修复 | 已修正源码 | 分开路径/参数转义，识别 bad-setting，修复前验证服务停止 |
| Linux 授权后文件权限兼容 | 已修正源码 | 用户路径收紧写位、允许安全的 root 程序、授权前读写探测、用户身份操作日志、面板保留错误详情；现场待验证 |
| Linux 服务移除 NoSuchUnit | 已修正并回归 | 明确单元缺失不误报卸载失败；保留存活/过渡/真实错误 |
| 项目 JPEG 统一图标 | 已接入并构建 | 四组件 EXE 资源、三平台托盘、WebUI favicon、Linux 桌面/通知、macOS .app 入口；桌面实测边界见下文 |
| Aether M3 客户端控制台 | 已接入并本地构建 | 自动识别 Agent/显隐卡片、首页总开关/客户端设置、手动订阅页；原套餐/多客户端卡原文件保持不变 |
| 网页/客户端订阅自动刷新 | 已实现并回归 | 网页发现后/每 4 小时取最新 URL 推送；Agent 启动/每 12 小时后台刷新，新 URL 覆盖与失败恢复 |

“已实现”描述源码能力，不等于所有平台、发行版、内核版本均已实机验证。

## 2026-11-17 Aether M3 接入验证

实现与部署说明见 [主题接入](xboard-integration.md)。主题目录存在独立 Git 元数据，客户端根目录仍无 Git；未提交、发布或操作线上站点。

| 检查 | 结果 | 范围 |
| --- | --- | --- |
| npm run build | 通过 | Vue/TypeScript 检查及 Vite 生产构建；保留原依赖，复用用户已解决的安装环境 |
| 主题 node --test scripts/reia.test.mjs | 7 项通过 | Agent 身份/URL/凭据隔离、4 小时边界、URL 轮换与在途队列、账号/卸载保护、重试、原卡片哈希不变 |
| 主题模拟 Chrome 实际 DOM | 通过 | 未启动/后启动/断开/内核未就绪的显隐、首页 Agent 总开关、自动订阅、手动页与原卡片；使用临时 --no-sandbox 和测试专用虚拟 rAF |
| 后端 TestSubscription / TestBackgroundProfile | 通过 | 新 URL 失败后持久化/保留配置、串行覆盖、启动真实 worker、12 小时边界、重复内容不重启、概括状态不泄露 URL、偏好回滚与并发文件事务 |
| 根目录 WebUI 脚本及接入类协议 | 通过 | 兼容原接口；接入类增加状态/刷新方法 |
| npm run pack | 已生成主题 ZIP | Xboard-Theme-Aether-M3/MD3-Theme.zip；未上传真实站点 |
| 三平台静态检查与四组件 | 通过 | 新产物在 .build/validation 各平台目录；Linux 新订阅/HTTP/配置并发测试已交叉编译；原运行程序未停止或替换 |
| 完整 go test ./... | 未通过，已有宿主限制 | 自身路径解析和跨进程锁 Access is denied，与前轮记录相同；不宣称全绿 |

真实 XBoard 订阅输出、账号令牌轮换、HTTPS 本地网络权限/CSP、长时间运行与目标桌面 TUN/服务授权仍需部署验证。自动订阅更新不自动开启代理或安装服务；“离线”指不依赖网页，Agent 必须运行且网络可用。

## 2026-11-17 第二轮验证：内核发现、卸载与图标

用户提供 NoSuchUnit 完整文字后，已修正卸载复查误判；内核查找补齐程序目录版本名和 Unix 用户执行位修复，保留探测错误。新增图标与生成/打包工具见 [ico](../ico/README.md)。

| 检查 | 结果 | 范围 |
| --- | --- | --- |
| go test ./internal/kernelbinary ./internal/platform/coreservice | 通过（Windows） | 程序目录版本名不依赖下载；标准名/显式更新优先；原始 NoSuchUnit 的属性与无属性形式；不吞 AccessDenied/总线故障/存活状态 |
| WebUI 脚本与接入类协议 | 通过 | 协议 9 项；favicon 随图片生成；没有改接口字段 |
| 隔离 Chrome DOM | 通过，临时 --no-sandbox | 全模拟接口，保留原默认脚本；含服务错误详情与旧页面行为 |
| go vet ./... | 三平台通过 | Windows、Linux/macOS amd64 |
| 三平台四组件 | 构建通过 | .build/validation/windows-amd64、linux-amd64、darwin-amd64；页面及 ico 资源已随产物复制 |
| Windows EXE 图标提取 | 四组件通过 | 通过系统 API 从新 EXE 提取图标，四张提取图 SHA-256 一致，人工查看确认是新素材 |
| macOS .app 入口 | 已生成 | 四个入口的 plist、包装脚本与 ICNS；尚未在 Finder/Cocoa 实测 |
| Linux 权限与桌面专用回归 | 已交叉编译，未执行 | coreservice.test、kernelbinary.test、ico.test；含原生二进制补执行位/拒绝脚本，XDG 图标导出、桌面字段转义 |
| 完整 go test ./... | 未通过，仍为已记录的宿主权限失败 | Agent/HTTP/内核管理测试解析自身程序路径 Access is denied；跨进程锁测试失败。其余已执行包通过；没有宣称全绿 |

未替换或停止当前旧进程，未操作系统服务、系统代理或 TUN。目标 Linux 仍需验证：不点击更新时使用同目录 mihomo；开启 TUN 后卸载服务，注册、进程及锁完成移交，普通内核单份运行；重复关闭不因 NoSuchUnit 失败。Linux/macOS 的桌面/托盘/进程查看器图标表现仍需目标系统检查。

## 2026-11-17 第一轮验证与限制

用户反馈 Linux 系统密码通过后 WebUI 仍提示路径/权限失败，尚未取得完整报错和发行版。源码修正了 0775 用户构建产物和 root 所有系统程序被安装器拒绝的情况，增加授权前用户访问探测与具体路径诊断，并避免 root 写用户操作日志。没有改变 TUN capability、实例锁、服务退出和应用旁 data 的边界。

| 检查 | 本轮结果 | 范围 |
| --- | --- | --- |
| go test ./internal/platform/coreservice | 通过（Windows） | 新增权限策略：0775 收紧、安全 root 程序、异主配置/token 拒绝、路径与模式诊断；既有 unit/注册/停止回归 |
| node scripts/check-webui.cjs | 通过 | 包含服务、TUN 设置和总开关三种失败原文保留、状态刷新不覆盖、重试成功清除、按文本展示 |
| node --test scripts/reia-client.test.mjs | 通过，9 项 | 未改请求/响应协议 |
| Linux 文件权限与日志符号链接回归 | 已交叉编译，未执行 | prepareServicePaths 精确改权限/无关文件保留、探测临时文件清理、缺 token 拒绝、日志替换不修改链接目标 |
| go vet ./... | 三平台通过 | Windows 本机；Linux/macOS amd64 交叉静态检查 |
| Windows、Linux/macOS amd64 四组件 | 构建通过 | .build/validation/windows、linux-amd64、darwin-amd64，未替换现有服务 |
| 隔离 Chrome DOM | 通过（临时 --no-sandbox） | 原脚本因宿主 GPU 进程权限崩溃失败；仅在全模拟接口的独立测试进程临时添加参数，默认启动脚本未改。含路径错误原文/文本安全/轮询保留/重试清除的真实 DOM 检查 |
| 完整 go test ./... | 未通过，宿主权限限制 | 项目内缓存绕过默认 Go 缓存拒绝访问后，仍有既有自身路径解析和跨进程锁测试 Access is denied，与前次记录一致 |

本轮没有注册/卸载或重启 Windows 系统服务，没有修改系统代理或启用 TUN。Linux 桌面认证、真实 systemd 交接与 TUN 收发仍须实测；路径修正不能视为现场故障已被实测解决。

验证时设置 GOCACHE 为项目内 .build/go-cache，跨平台构建另使用 .build/go-mod 作为 GOMODCACHE，避免默认缓存目录的读写限制。Linux 无需 root 可运行 `go test ./internal/platform/coreservice`，或执行已交叉构建的 `.build/validation/linux-amd64/coreservice.test`；测试不注册或启动系统服务。

## 2026-11-16 验证与限制

Linux 授权后的 bad unit file setting 已定位到 WorkingDirectory 的参数引号。已修正路径生成、坏单元的注册状态展示，以及修复/卸载时的停止确认。新增 Agent 缓存、日志清理与目录打开 API 已接入测试面板和 JavaScript 接入类。

| 检查 | 本轮结果 | 范围 |
| --- | --- | --- |
| 服务格式/注册状态/停止确认、更新缓存清理回归 | 通过 | 不注册系统服务；覆盖坏单元、存活/未知状态拒绝替换、更新进行中拒绝清理、内核保留 |
| 新增 Agent 维护和 HTTP API 回归 | 通过 | 保留配置/订阅/密钥/配对、日志旧句柄继续写入、事件/DNS 游标、固定目录与无效请求 |
| go vet ./... | 通过 | Windows 静态检查 |
| WebUI 脚本及接入类协议回归 | 通过 | 包括新增三个 POST 路由和四个面板操作 |
| 隔离 Chrome DOM | 通过 | 模拟所有 API，四个维护操作和既有 DOM 检查；当前宿主需 --no-sandbox 才能启动隔离测试 Chrome，默认脚本因宿主权限失败，未修改默认启动参数 |
| Windows 四组件构建 | 通过 | .build/validation/windows，未替换/重启运行中的服务 |
| Linux/macOS amd64 四组件交叉构建 | 通过 | .build/validation/linux-amd64、darwin-amd64，不代表桌面/TUN 实测 |
| Linux systemd-analyze 验证用例 | 已添加并交叉编译，未执行 | 在 Linux 运行 go test ./internal/platform/coreservice，无需 root，不安装或启动服务 |
| 完整 go test ./... | 未通过，受环境权限限制 | 已有服务/内核初始化测试的 filepath.EvalSymlinks 读取自身程序路径返回 Access is denied；跨进程锁测试也因权限失败。独立最小程序复现了路径解析错误，未修改这些无关实现 |
| 符号链接清理保护用例 | 跳过 | Windows 当前会话无创建符号链接权限；普通文件/目录和受限路径清理检查已通过 |

需在 Linux 更新四组件及测试页面后点击“修复服务”，复测桌面授权、真实 systemd 注册/就绪、TUN 收发及目录打开。本轮未操作现有 Windows 服务或系统代理。

## 2026-11-15 Windows 服务修复

问题链路：旧 mihomo 遗留 → 9090/7890 被占用 → 新 helper 虽占 25501 但内核未就绪 → 开关/SCM 状态误导。

本次处理包括：

- 独立服务程序、真实 SCM 自有进程，兼容迁移旧 Agent 服务及旧计划任务。
- 先禁用旧服务启动/失败恢复，再完成停止；处理卡住与 START_PENDING 无法接受 STOP。
- 按路径、参数和创建时间识别所属进程，不按进程名或端口直接批量结束。
- 修正 CIM 与 FILETIME 的创建时间精度差异；使用稳定进程句柄防止结束复用的 PID。
- 共享内核实例锁，关闭与接管之间等待完成。
- 本服务内核就绪后再宣布 SCM Running；安装失败停止循环重试。
- 已注册服务的控制记录损坏时不回退启动第二个内核。
- 服务模式退出保留 Agent API；关闭服务开关才执行卸载与普通模式回退。

## 2026-11-15 验证结果

| 检查 | 本轮结果 | 范围 |
| --- | --- | --- |
| go test ./... | 通过 | 包含服务状态、退出、锁、清理归属、创建时间等回归 |
| go vet ./... | 通过 | 静态检查 |
| node scripts/check-webui.cjs | 通过 | JS 语法、节点顺序/局部更新、面板配对等 |
| node scripts/run-webui-browser-test.cjs | 通过 | 独立 Chrome 配置和模拟 API 下的实际 DOM |
| ./build.ps1 | 通过 | Windows 四个程序 |
| Linux amd64 命令组件交叉构建 | 通过 | 构建，不是 Linux 桌面运行验证 |
| macOS amd64 命令组件交叉构建 | 通过 | 构建，不是 macOS 桌面运行验证 |
| Windows 旧服务迁移 | 通过 | 真实注册、清理原遗留内核、单一新内核接管 |
| Windows 卸载与重装 | 通过 | 注册和旧进程移除，重装后 WebUI API 恢复 |
| 退出托盘后保持 WebUI | 通过 | API 仍可用，后台 PID 不因退出而替换 |
| 重复启动 | 通过 | Agent、服务、内核各一个；托盘也只一份 |
| node --test scripts/reia-client.test.mjs | 通过 | 模拟 fetch，验证配对、凭据更新、发现/卸载、超时及写入门控，不访问真实 Agent |

最后记录的 Windows 服务已正常注册且 ready。最后 TUN 为关闭状态；本轮服务测试不代表 TUN 实际收发流量测试。
可选真实内核与独立 Agent 集成测试依赖环境变量及无端口冲突等条件，默认测试可能跳过它们。
浏览器 DOM 测试使用模拟接口，不能替代真实远程站点的浏览器授权验证。

## 尚未完成与需要实测

- Linux/macOS 的桌面授权、服务安装/卸载、退出、TUN 和通知点击仍需对应系统验证。
- Linux 系统代理目前针对 GNOME/gsettings；KDE 等桌面尚无独立系统代理实现。
- 已在本地 Aether M3 主题仓库接入客户端；真实站点部署、浏览器权限及长期订阅定时仍待实测。
- 客户端组件自更新和平台签名流程尚未提供，当前更新器针对 mihomo。Windows 已提供离线安装包。
- 未提供 Android 应用/前台服务外壳；不能按桌面构建结果宣称 Android 已完成。
- DNS 记录不是完整流量捕获，不保存 Agent 启动前历史；内核日志格式变化可能影响采集。
- mips stack 已打通校验、持久化与生成配置，但应以目标内核验证结果为准。
- 资源预置仍需与订阅的 Geo 数据格式匹配；缺少资源时内核可能尝试联网下载。
- 当前目录无 Git 元数据，后续若需要版本管理应单独明确初始化或导入仓库的范围。

## 下次修改的验证命令

在项目目录按改动范围选择：

```powershell
go test ./...
go vet ./...
node scripts/check-webui.cjs
node scripts/run-webui-browser-test.cjs
node --test scripts/reia-client.test.mjs
./build.ps1
```

Linux/macOS 本机使用 sh build.sh。跨平台构建可在独立命令环境设置 GOOS/GOARCH 后构建 ./cmd/...，不要让交叉构建环境变量影响随后本机测试。
服务生命周期变化还需实机核对真实注册、端口归属、退出后 API、重复启动、卸载后的进程清理，并在结束时恢复用户原本希望运行的服务状态。
文档修改只核对源码一致性和文档链接即可，不需要执行上述运行测试。

## 更新此文档的时机

功能完成后更新实现状态；实测后写明平台与条件；失败或未测项目保留事实，不以交叉编译替代实机结论。
生命周期变化须同步 [接手上下文](../context.md) 和 [架构](architecture.md)，接口变化须同步 [API](api.md)。
