# ReiaClash 接手上下文

更新日期：2026-11-19。工作目录：`C:/Users/Administrator/Desktop/ReiaClash`。
本文件记录下一次修改必须先了解的决策和最近修复，替代 2026-11-12 的旧快照。

最新修改见 [Aether M3 接入](docs/xboard-integration.md)、[退出语义](#退出语义) 和 [出口 IP、TUN 配置与热加载](#2026-11-19-出口-iptun-配置与热加载)。此前 Linux/图标交接见 [2026-11-17 任务总结](docs/handoff-2026-11-17.md)。平台授权、服务与 TUN 的实测范围见 [项目状态](docs/status.md)。

## 2026-11-22 内核交接与面板轮询

测试面板支持 mihomo 与 meow-rs 双向切换。首次切换通过旧内核代理下载，先做完整性、版本和配置校验，再在单实例约束下交接。Agent 在新内核通过就绪检查后提交更新，按归属清理旧更新文件。静默更新包由 Agent 处理。状态面板通过轻量健康接口跟踪 Agent 和生命周期操作锁，保持原页面、输入和连接表操作控件。实测与构建范围见 [项目状态](docs/status.md)。

## 2026-11-22 Aether M3 和统一通知

上一轮内核交接和静默更新接入 Aether M3 原生内核维护页。默认 favicon 和站点 Logo 采用项目图标，四个 EXE 的图标、产品名和文件描述统一，桌面通知统一走托盘。详见 [Aether M3 接入](docs/xboard-integration.md) 与 [图标说明](ico/README.md)。

## 2026-11-22 GeoIP 启动与默认接管

切换 meow-rs 前通过仍运行的 mihomo 准备配置所需的 Country.mmdb。代理总开关在无接管方式时使用原版 mihomo、TUN 和 mips。主题订阅同步传 `sync_only`，避免多个面板反复下载同一份有效订阅。

## 应用目录 OTA 与存储统计

`updates/` 用于暂存。新版启动时迁移在暂存区运行的旧版内核。手动升级安装到应用目录，经共享监督器确认启动，失败恢复旧版。静默更新到 `update.pack` 并延至暂停、启动或重启时应用。每类保留三份备份。

## 代理节点记忆

Agent 按代理组保存面板手动选中的节点。启动、内核切换和重新加载订阅后，恢复当前内核仍提供的组和节点。

## 冷启动与 Windows DNS

新启动的普通内核监督器和系统服务均默认暂停接管。已实际运行的后台服务可由 Agent 或托盘重连并延续当前会话。主程序重启或关闭内核前先尝试关闭 meow-rs TUN，释放其 DNS 接管。

## 登录静默代理、目录与资源

主题的登录自启下增加静默代理偏好，只有登录启动入口才触发。启动前检查 DNS、网络和混合代理端口，沿用系统授权，接管后验证失败则尝试暂停。订阅热加载通过 `payload` 传入生成配置，已确认的内核不会因配置加载问题自动改回 mihomo。

存储页提供升级缓存、订阅配置和日志清理。订阅清理需确认，并暂停主题自动导入至下一次手动导入。工作目录只允许打开和统计。外部资源页可手动或定时同步官方 Geo 文件及配置里的 HTTP 规则集。

## meow-rs 出站与订阅状态

两种内核共用已验证的原始订阅 `profile.yaml`，分别生成 `mihomo.yaml` 和 `meow.yaml`。切换与 OTA 先以候选内核的配置格式校验原始缓存。后台刷新继续使用已保存的链接，也接受主题推送的链接。

订阅启用 IPv6 或 DNS AAAA 查询时，meow-rs 配置 IPv6 TUN 地址，保留显式禁用和自定义地址。进入 TUN 前后分别检查原本可用的 A、AAAA 和系统 IPv6 路径。出口卡独立显示混合代理的 IPv6 探测结果，连接表使用控制器可提供的嗅探域名。

切换到 meow-rs 时，对照旧内核已通过的混合代理和手动选择节点检查。候选未通过则回退；常规运行或订阅刷新时保留已确认的内核选择。Windows/Linux 的 meow-rs 在订阅开启 DNS 且明确接管任意地址的 UDP 53 时采用 `any:53`，其余订阅保留系统 DNS。订阅未指定 sniffer 时使用 HTTP/TLS/QUIC 域名识别。原始 YAML 从本机 Agent 独立页面读取，Aether M3 提供入口。

meow-rs 的 TUN 设置变更通过受管内核停启应用，等待旧进程退出后再读取新配置。Windows 通过两段 IPv4 默认路由确认 meow-tun 实际接管，修复控制器错误报告关闭时的开关。查看页可选择原始订阅或运行内核启动时指定的 YAML。开启登录静默代理会同时注册登录启动项。

## 阅读顺序

1. 本文件：用户要求、最近改动、不能回退的行为。
2. [项目状态](docs/status.md)：已完成内容、验证范围和待办。
3. [详细架构](docs/architecture.md)：进程、权限、数据、服务切换。
4. [API 说明](docs/api.md)：WebUI 与客户端对接。
   前端移植另见 [嵌入指南](docs/frontend-integration.md)，接入类与示例在 docs/examples/。
5. [README](README.md)：运行、构建和面板接入。

当前目录没有 Git 元数据。不要假设存在提交历史、分支或可用的 `git diff`；以源码和这些文档核对状态。

## 项目与用户目标

ReiaClash 用 Go 管理独立 mihomo 内核，以浏览器 WebUI 和桌面托盘提供客户端功能。测试面板是根目录 `test-webui.html`；当前已在本地 Xboard-Theme-Aether-M3 源码完成接入，默认在线入口仍是 `https://next.rka.jp`，线上部署尚未验证。
必须兼顾 Windows、Linux、macOS，不能只检查 Windows 的 exe 文件。
用户希望正常使用无需默认管理员/root，TUN 按需触发系统授权。

四个构建产物是 `reia`、`reia-agent`、`reia-tray`、`reia-service`，Windows 加 `.exe`。
`reia` 是静默总入口，原 Agent/Tray 保留控制台构建以便调试，服务使用独立程序。

## 已确定的行为

- 首次启动总开关 `enabled=false`，接管方式默认勾选 TUN（`mips`），系统代理偏好关闭，Geo 低内存开启。已有文件中的明确偏好继续使用。
- 规则、全局、直连都要实际应用；模式或节点切换后关闭现有连接，让新连接使用新选择。
- 控制接口始终只绑定 loopback；LAN 开关只开放 mihomo 代理入口，不开放 Agent/服务控制接口。
- Geo 资源放在应用程序旁的 `data/`，更新内核位置变化不能改变这一目录。
- 系统代理只恢复本项目保存的备份；不要恢复“无备份也强行关闭用户代理”的旧实现。
- TUN 不足权时通过 Windows UAC、Linux polkit 或 macOS 系统授权安装内核服务，Agent 保持用户会话身份。
- 节点组默认按订阅 YAML 顺序，组内节点按 mihomo 的 `all` 顺序。用户可自行排序，切换节点不重建页面框架。
- XBoard 的 `siteName` 同名组优先；规则模式隐藏 GLOBAL，全局模式显示并置顶 GLOBAL，直连模式隐藏节点列表。
- 前后端接受 `mixed/gvisor/system/mips` TUN stack。`settings.tun_device` 配置本项目网卡名，`real_tun_device` 报告内核当前网卡。Windows/Linux 新配置默认 `ReiaClash`，macOS 默认自动 `utun`。
- DNS 记录是 Agent 本次运行期间的日志与手动查询记录，最多 200 条，不是完整 DNS 抓包。

## 2026-11-19 Windows 安装程序

`build-installer.ps1` 默认递归读取 `installer/`，当前工作区实际素材目录为 `installler/`，脚本会自动回退。输出为 `.build/installer/ReiaClash-Setup.exe`。保持 `test-webui.html` 和预置 `data/` 与程序同级。安装目录可选，默认当前用户目录，桌面和开始菜单快捷方式独立可选，完成页询问是否启动 `reia.exe`。

安装包使用 7-Zip LZMA2 最高压缩级别，指定 256 MiB 字典并开启固实模式。Inno Setup 7 直接嵌入从 `installer/` 生成的归档，素材包含已放入目录的两种内核，并排除本机运行产生的日志、流量和选择缓存。校验结果见 [项目状态](docs/status.md)。

## 退出语义

用户原话：“只退出托盘和面板，后台内核服务继续运行”；随后明确反馈“退出后 WebUI 断开”仍是问题。

最终实现是：

- 服务模式点击“退出客户端”打开对话框。“继续后台运行”关闭托盘，保留后台 Agent、系统服务和 mihomo，WebUI 继续连接。“注销服务并退出”完成系统授权、核对已卸载和恢复系统代理备份后彻底退出。取消或失败时保留托盘。
- 普通模式退出：恢复已备份的系统代理，Windows 也会清理仍指向 ReiaClash 的残留代理。停止本项目内核并恢复已记录的 DNS。完全退出只暂停接管，保留内核与 TUN 偏好。
- Windows meow-rs 接管 DNS 前保存出站网卡的静态或自动配置，退出或下一次启动恢复仍残留的回环 DNS 并清掉系统 DNS 缓存。托盘信息显示运行内核及进程常驻内存。
- 关闭服务开关：系统授权后真正停止并卸载服务，关闭 TUN，再回到普通进程管理。
- 重新运行总入口：恢复托盘并复用现有 Agent/服务/内核，不能再生成一套后台进程。

`POST /api/agent/exit` 在服务模式返回 202 和 `background:true`，不关闭 Agent API。
托盘专用 `POST /api/agent/exit/uninstall-service` 在明确选择注销后使用，卸载成功后直接退出，跳过启动普通内核的步骤。

2026-11-19 Windows 授权改用 `ShellExecuteExW`，按需启动独立授权进程并等待其退出。通过文件句柄获取可执行程序真实路径，修复当前受限 Windows 环境可读文件但逐级解析路径返回 `Access is denied` 的情况。共享实例锁改用 `CreateMutexExW` 请求 `SYNCHRONIZE`，同名锁可正确报告已存在。

## 2026-11-19 出口 IP、TUN 配置与热加载

首页 ReiaClash 控制卡集中展示总开关与出口 IP。新版 Agent 的 `/api/agent/outbound-ip` 使用 Cloudflare Trace，握手声明 `outbound_ip_trace`，旧版由浏览器查询同一 Trace。接管、模式、节点变化后刷新并忽略过期响应。管理页使用 Aether M3 色板与 Material Web，连接和日志会自动重连，设置共享写入锁。TUN 网卡名继续通过客户端设置与测试 WebUI 管理。

Aether M3 图标优先读取 XBoard 的 `custom_icon_url`。开发入口和打包后的 Blade 共用 `scripts/icon-font-loader.js`。WOFF2 使用完整 URL 命中已有缓存，未命中走匿名沙盒；CSS 通过样式表加载，失败或超时才回退到内置字体。

Agent 在内核就绪时用 `/configs` 单独热应用模式、LAN、TUN 启停和活动网卡设置。TUN 读取实际启停、协议栈和指定网卡名，失败恢复原偏好和运行配置。订阅变化通过 `PUT /configs?force=true` 应用，内核不可达时沿用监督器恢复路径。初次授权移交、内核二进制更新和 Geo 加载器切换仍由监督器处理。详细验证见 [项目状态](docs/status.md)。

## 2026-11-17 Aether M3 接入与订阅刷新

用户明确要求原套餐订阅卡、多客户端导入以及其他业务页保持原样；仅改客户端控制。主题内原 SubscriptionCard.vue、ClientConfigCard.vue 的文件哈希已验证未变。新增首页 ReiaClash 控制台，依据 Agent 健康/握手自动显隐，而非任意 9090；暂停/内核未就绪仍可管理。旧组件适配至 Agent 管理设置/生命周期；模式选择与节点顺序保留项目约定。完整 URL（含主题混淆 hash）用于配对。

网页发现客户端后与每 4 小时重新取得当前账号 subscribe_url 并推送，链接变化立即同步；账号切换/卸载丢弃旧响应，客户端密钥仅内存，站点认证不发往 loopback。新增 /clash/subscription 手动兜底页，保留原多客户端卡。浏览器 LNA 仍可能需要点击“连接本机客户端”。

Agent 接受新链接先覆盖持久化旧链接，再尝试下载/校验；失败保留可用配置并用新链接重试。启动/每 12 小时后台刷新，失败按 1～30 分钟退避，内容未变不重启。新增 GET /api/subscription/status、POST /api/subscription/refresh；settings 增加 subscription_updated_at。设置/Geo 与订阅配置事务增加并发保护，设置回滚不能恢复旧订阅链接。

主要源码：主题 src/api/reia.ts、src/api/clash.ts、src/stores/clash.ts、src/utils/reiaSubscriptionSync.ts、ReiaConsoleCard.vue、SubscriptionView.vue；客户端 internal/agent/subscription.go、settings.go、internal/mihomo/profile_store.go、internal/httpapi/handlers_subscription.go。详见 [接入说明](docs/xboard-integration.md)。

主题类型/构建、7 项协议/同步/原卡片不变回归与模拟 Chrome 实际 DOM 通过。新后端订阅/配置并发回归通过；完整 Go 测试仍有旧宿主权限失败。主题包 MD3-Theme.zip 在主题目录，新客户端四组件在 .build/validation 各平台目录；本轮未发布线上主题或操作现有服务/代理。下次先用新主题与新客户端一起在目标站点验证真实订阅格式、权限、令牌轮换及长期定时行为，不能用旧根目录 EXE 代表新功能。

## 2026-11-17 第二轮：Linux 内核发现、服务移除与统一图标

用户补充了明确的卸载错误：`org.freedesktop.systemd1.NoSuchUnit "Unit reia-clash-core.service not found."`，并反馈项目目录内核需要先手动更新才能被找到。

- systemd 查询现在将无注册/存活证据的 NoSuchUnit 识别为正常缺失，包括 LoadError 字段和无属性的 D-Bus 错误。已加载、正在退出的服务以及 AccessDenied/总线故障仍保留错误，不能因此启动第二个内核。修正作用于安装恢复和卸载后的复查，无 API 格式变化。
- 发现顺序保留显式 updates 选择优先，其后在程序目录、工作目录查找标准名与 mihomo-/mihomo_ 版本名，再回退旧路径/bin。排除归档、配置与校验文件；不同平台候选仍需 -v 验证。
- Unix 解压/传输丢失执行位时，只对当前用户所有、文件头匹配的原生二进制补用户执行位；不改脚本或异主文件。找不到内核的事件保留各候选路径与探测错误，不再只显示泛化的“请下载内核”。Agent 与系统服务继续共用该实现。
- 用户新下载的 images.jpeg 已重命名为 ico/reia-clash.jpeg；派生 PNG/ICO/ICNS、四组件 Windows 资源、三平台托盘、WebUI favicon、Linux 桌面/通知及 macOS 应用入口均使用此图。图标生成/平台边界见 [图标说明](ico/README.md)。不要重新生成原来的蓝色 R 图标。
- Windows 新产物位于 .build/validation/windows-amd64，Linux/macOS 为同目录下 linux-amd64/darwin-amd64。运行中旧程序未停止或覆盖，服务/代理状态未调整。

权限/发现/systemd 回归、WebUI/协议/隔离浏览器、三平台静态检查与四组件构建通过。四个新 Windows EXE 均已提取图标并确认一致。Linux 专用权限及桌面回归只交叉编译；完整 Go 测试仍受已记录的路径解析/跨进程锁限制，不能宣称所有测试通过或 Linux TUN 已实测成功。

## 2026-11-17 Linux TUN 授权后路径权限修正

用户继续反馈：Linux 输入系统密码后，根目录测试 WebUI 报权限/路径无法操作。现场尚未提供完整后端错误和发行版，不能将本轮源码问题直接认定为该现场唯一根因，也不能宣布 Linux TUN 已实测成功。

源码确认安装器在 polkit 授权后才校验路径：用户构建常见的 0775 服务程序会被 `0022` 写权限检查拒绝；root 所有且 0755 的系统安装程序也被“必须属于登录用户”拒绝。本轮修正：

- 授权前以当前用户检查服务程序、配置根目录、控制 token，并探测配置根目录和应用旁 data 的实际写入能力；data 不迁移到用户缓存。
- 仅对当前用户所有的上述校验路径去掉组/其他用户写位（通常程序/目录 0775 → 0755；正常 token 已是 0600），不递归 chmod/chown。配置目录/token 仍必须属于该用户。root 所有的服务程序允许使用，但不能有组/其他用户写位，且当前用户须可读/执行。
- 特权进程继续复查 UID、类型和权限，拒绝授权期间被改成不安全的路径。报错包含具体路径，归属/模式错误附 UID 或权限值。
- Linux 提权命令只向 stderr 报错；用户态调用方原子写 core-service-operation.log，避免失败后留下 root 所有、用户不可读的操作日志。
- test-webui.html 服务设置区增加可选中复制的错误详情；服务操作、总开关和 TUN 设置失败时保留后端原文，状态轮询不会覆盖，下一次操作清除。接口格式不变。

已通过权限策略、既有服务平台回归及 WebUI/接入类回归。Linux 文件权限和日志链接保护用例已添加，需 Linux 执行；本机无可用 Linux/systemd 桌面。完整测试仍有已记录的 Windows 自身路径解析/跨进程锁权限失败，不能写成完整测试通过。构建和最终验证以 [状态文档](docs/status.md) 为准。

Linux 更新四组件及根目录测试页面后，以普通用户启动并点击“修复服务”。若仍失败，复制服务设置区完整错误文字进一步定位；不要为绕过检查使用 chmod -R 777 或默认以 root 运行 Agent。

## 2026-11-16 Linux 服务修复与维护接口

用户反馈 Linux TUN 授权后 systemd 报 `bad unit file setting`。源码确认 `WorkingDirectory` 复用了 ExecStart 的引号转义，而 systemd 的工作目录解析器不移除这些引号，因而拒绝该绝对路径。

- 工作目录改用单路径格式，仅转义 `%`；ExecStart 保持参数转义。
- 实际注册检测保留 `bad-setting/error/masked` 单元并返回具体加载错误；过渡状态仍视为服务可能存活，不启动第二个普通内核。
- 修复/卸载分开执行 disable 和 stop；坏单元 stop 报错时，仅在另查 ActiveState 确认为 inactive/failed 后允许替换或移除。
- 新增 POST `/api/agent/cache/clear`、`/api/agent/logs/clear`、`/api/agent/directory/open`。测试 WebUI 内核维护页增加四个按钮，接入类增加相应方法；范围见 [API](docs/api.md)。不改其他代理功能或退出语义。
- 缓存清理仅删除 updates 中断下载/解压临时文件及 DNS 内存记录；日志清理仅截断 Agent 两个日志文件及内存事件。保留配置、订阅、密钥、代理备份、服务日志、已下载内核及应用旁 data。打开目录由用户会话执行，cache=updates、config=用户配置根目录。

本轮新增回归、go vet、前端协议/脚本和隔离 Chrome DOM 检查通过；Windows 四组件及 Linux/macOS amd64 四组件构建通过，产物位于 `.build/validation/`。完整 go test 在当前受限环境仍有已有服务/锁测试失败：filepath.EvalSymlinks 读取自身程序路径返回 Access is denied，跨进程锁测试也受权限限制。符号链接用例因 Windows 未授予创建符号链接权限而跳过。不能写成本轮完整测试全绿或已完成 Linux TUN 实测。

Linux 用户更新四个程序及 test-webui.html 后，在设置点“修复服务”重新生成 unit。Linux systemd-analyze 验证用例已添加并交叉编译，需在 Linux 执行；本机无 Linux/systemd 桌面，不自动调整现有 Windows 服务。

## 最近一次 Windows 服务故障与根因

实机出现：旧服务进程退出，但旧 mihomo 遗留占用 9090/7890；新服务绑定了 25501，SCM 一度显示运行，自己的内核却不能启动。
还存在 START_PENDING 时 SCM 拒绝 STOP、失败服务不断自动重启、注册标记与真实状态不一致的问题。

关键修复：

1. Windows 安装/修复/卸载先禁用旧服务自启及失败恢复，再停止服务；卡住或处于 START_PENDING 时，使用从该 SCM 自有进程取得的稳定进程句柄终止它。
2. 特权清理核对可执行路径、启动参数中的项目配置/data 路径、进程创建时间；先清 helper，再重新取快照清内核，防止 helper 抢先重建子进程。
3. CIM 创建时间精度是微秒，原生 FILETIME 是 100 纳秒；比较时统一精度。此前直接相等比较导致合法旧进程被误判为 PID 已复用而跳过。
4. 本地与服务监督器共享数据目录对应的 kernel 实例锁；Shutdown 等待子进程和锁释放后，下一监督器才可接管。
5. Windows 子进程绑定 Job Object，Linux 使用 systemd 控制组，macOS 使用 launchd 进程组。
6. SCM 只有在自身监督器确认内核运行后才报告 Running。安装失败会停止错误重试，并保留具体诊断。
7. 启动时真实服务注册优先于本地标记。标记损坏/缺失时恢复控制配置或保持待修复的服务委托，不能另起本地内核。
8. 服务卸载后再次检查实际注册状态，仍注册/运行时不启动普通监督器。
9. 托盘退出门控阻止恢复线程重新拉起 Agent；重复运行 Agent 遇到已识别的本项目 API 会正常返回。

重点文件：

- `internal/platform/coreservice/scm_windows.go`：SCM 注册、停止、失败恢复、就绪。
- `internal/platform/coreservice/cleanup_windows.go`：精确归属和创建时间比较。
- `internal/platform/instance/`、`internal/mihomo/manager.go`：共享内核锁和交接。
- `internal/platform/launch/child_windows.go`：Job Object。
- `internal/coreservice/run.go`：服务运行期和自身内核就绪。
- `internal/agent/core_service.go`、`startup.go`：服务切换及启动恢复。
- `internal/httpapi/handlers_maintenance.go`、`internal/trayui/tray.go`：退出与后台 API 保留。

## 面板 URL 与通知：不要回退

WebUI 每 5 秒扫描/重连 Agent，握手上报当前完整 `panel_url`，支持 HTTP(S) 和本地 `file:///`。
例如本项目本地 HTML 的 file URL，不能改写为 127.0.0.1 的测试页。
已配对真实 URL 优先；未配对且在线/检测中使用 `https://next.rka.jp`；只有明确离线且未配对时使用内置面板。
URL 只放 Agent 内存，不写设置或日志。退出托盘但 Agent 尚在时，配对仍属于本次 Agent 运行。
托盘与通知点击都实时获取当前目标；通知文案保持简短。

## 2026-11-15 验证与现场状态

前端移植资料已新增：docs/frontend-integration.md、docs/examples/reia-client.mjs、docs/examples/embed.html。
接入类只封装现有接口，不修改后端服务协议；覆盖真实 URL 配对、自动发现、内存密钥刷新、超时、防重复写入和组件卸载。
模拟协议测试入口为 node --test scripts/reia-client.test.mjs；不会访问实际用户客户端。站点的浏览器授权/CSP 仍需部署后验证。

2026-11-15 已完成：

- `go test ./...`、`go vet ./...`。
- WebUI 脚本检查、独立 Chrome DOM 集成测试。
- Windows 四组件构建，Linux/macOS amd64 命令组件交叉构建。
- Windows 旧服务迁移及遗留内核清理实测。
- Windows 原生卸载：服务注册消失、旧服务和旧内核进程都退出。
- 随后重新安装：服务恢复运行、Agent API 自动连回。
- 退出托盘及重复启动：Agent/服务/内核 PID 未因重复启动而变化，每种后台角色保持一个。

最后一次现场检查：服务已注册、Running、ready，内核运行；Agent/Service/Mihomo/Tray 各一份。TUN 当时为关闭状态，不要为了接手文档自动打开。
PID 是临时值，不作为下一次结束进程的依据。Linux/macOS 本轮仅构建验证，不能写成已完成对应桌面实机验证。
带 `REIA_TEST_MIHOMO`、`REIA_TEST_AGENT` 的额外集成测试需显式设置条件；默认测试通过不代表所有可选测试已执行。

## 下次开启修改流程

先读文档，再检查相关源码和当前进程/端口/日志；不要假定上次运行状态仍然成立。
当前用户已接受上述退出语义和跨平台边界；后续修复应保持这些约定。
按新问题修改对应模块，并完成与改动相关的测试。文档任务不需要重启服务或触发 UAC。
功能、API 或生命周期变化后同步更新本文件、状态、架构/API 与 README，避免再次出现多份文档描述相反。
