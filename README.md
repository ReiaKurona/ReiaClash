# ReiaClash

ReiaClash 用 Go agent 管理独立运行的 mihomo，由浏览器 WebUI 承担常规 Clash 面板交互。桌面托盘是单独进程，不创建本地窗口。当前仓库的测试面板是 [test-webui.html](test-webui.html)。

## 项目文档与接手入口

文档更新于 **2026-11-17**。下次开启修改流程，先读 [接手上下文](context.md)，再按需要查看以下说明：

| 文档 | 内容 |
| --- | --- |
| [context.md](context.md) | 用户已确认的行为、最近修复根因、重点文件与下一次修改流程 |
| [本次任务交接](docs/handoff-2026-11-17.md) | 2026-11-17 两轮 Linux 修复、图标接入、产物、验证限制和下次实测步骤 |
| [项目状态](docs/status.md) | 已实现能力、Windows 实测结果、三平台构建范围与待办 |
| [详细架构](docs/architecture.md) | 四个程序、单实例、权限、服务切换、数据目录、退出语义与模块职责 |
| [API 说明](docs/api.md) | WebUI 配对、状态、服务、自启、Geo、节点、连接、DNS 与更新接口 |
| [前端移植与嵌入](docs/frontend-integration.md) | 面向其他站点/XBoard 的快速接入、JavaScript 接入类、最小页面与浏览器权限处理 |
| [Aether M3 接入](docs/xboard-integration.md) | 首页客户端控制台、4 小时网页同步、12 小时后台刷新、手动兜底与主题打包 |
| [AGENTS.md](AGENTS.md) | 后续代码助手的阅读入口与维护约定 |

最近一次 Windows 服务迁移、卸载重装、退出托盘保留 WebUI 和重复启动实测已通过；Linux/macOS 本轮为构建验证，实测边界见项目状态。

2026-11-17 第二轮修正：Linux 卸载后的 `org.freedesktop.systemd1.NoSuchUnit` 会正确识别为服务已不存在，不再误报“系统服务尚未完全移除”。内核发现覆盖程序目录里的标准名和版本化 mihomo；用户所有的原生 Unix 二进制缺执行位时自动补用户执行位，再做版本验证。无需先下载更新才能使用有效的同目录内核；显式选择的已下载更新仍优先。

用户提供的 JPEG 已命名为 `ico/reia-clash.jpeg`。四个 Windows 程序、三平台托盘、WebUI favicon、Linux 桌面/通知和 macOS 应用入口统一使用该图；生成方式和进程图标的平台限制见 [图标说明](ico/README.md)。本轮产物在 `.build/validation/windows-amd64`、`linux-amd64`、`darwin-amd64`，旧运行程序未停止/覆盖。Linux/macOS 的实际桌面显示与 TUN 仍需实机验证。

前端快速移植可复制 [reia-client.mjs](docs/examples/reia-client.mjs) 与 [embed.html](docs/examples/embed.html) 到站点静态目录；接口参考见上方 API 文档。接入类不依赖框架，不需要把整个测试面板搬到站点。

本地 `Xboard-Theme-Aether-M3` 发现本项目 Agent 时显示控制台，提供总开关、客户端设置和出口 IP。原套餐订阅与其他客户端导入入口继续可用。主题发现客户端后及每 4 小时获取当前账号订阅并推送，链接变化即时覆盖。客户端启动时及每 12 小时用保存链接后台刷新，失败保留可用配置并重试。手动安装页为 `/clash/subscription`，测试 WebUI 也提供手动安装入口。后台刷新由联网的 Agent 独立完成。

主题的 ReiaClash 控制卡通过 Cloudflare `1.1.1.1` Trace 展示出口 IP。设置与管理页采用同一套 Material Web 控件，包含实时连接、会话内历史、日志筛选与重连、规则集更新和节点测速。图标优先使用 XBoard 的 `custom_icon_url`，WOFF2 保留匿名沙盒下载，内置 Material Symbols 提供回退。 [主题接入与验证](docs/xboard-integration.md)

主题上传包为 `Xboard-Theme-Aether-M3/MD3-Theme.zip`，需同时更新 `.build/validation/` 中对应平台的四个客户端组件。当前完成本地构建和模拟浏览器验证，尚未发布线上主题或验证真实站点的浏览器权限/订阅输出。详细测试与部署步骤见 [接入说明](docs/xboard-integration.md)。

Linux TUN 授权后出现 `bad unit file setting` 的问题已修正：WorkingDirectory 使用单路径格式，坏单元仍显示为已注册并可修复。更新四个客户端程序及测试 WebUI 后，在设置点击“修复服务”，按提示授权以重新生成 systemd 单元。当前 Linux 修复已交叉构建，桌面授权与 TUN 收发仍需实机复测。

2026-11-17 继续修正 Linux 授权后的路径权限兼容：对当前用户所有的服务程序/配置根目录去掉组与其他用户写位，兼容常见的 0775 构建产物；允许使用 root 所有且无组/其他用户写位的系统安装程序。配置目录和 token 仍须属于登录用户，应用旁 data 仍须对该用户可写；客户端会在授权前探测并返回具体路径错误，不递归修改所有者。Linux 操作日志改由用户 Agent 写入，避免 root 日志不可读。根目录测试 WebUI 的服务设置区会保留可复制的错误原文；若更新四组件并“修复服务”后仍失败，请复制该处完整文字。本轮尚未取得 Linux 现场完整错误，桌面授权和 TUN 成功仍需实机确认。

两个接管选项均关闭时，点击启动会使用原版 mihomo 的 TUN + mips，并按需请求系统授权。meow-rs 切换会在交接前准备配置所需的 GeoIP 数据。

Aether M3 可在主题「客户端设置 → 内核维护」切换内核及检查或应用更新。主题默认 favicon 和站点 Logo 使用 ReiaClash 图标，桌面通知由托盘以 ReiaClash 发送。主题包位于 `Xboard-Theme-Aether-M3/MD3-Theme.zip`。详见 [主题接入](docs/xboard-integration.md)。

meow-rs 切换会比对原有可用出站。检测失败会保留原内核和节点选择。Windows/Linux 的 meow-rs 按订阅中明确声明的通配 UDP 53 规则接管 DNS。验证范围见 [状态](docs/status.md)。

内核类型和手动选中的代理组节点会记忆。重启、换内核或重载订阅后，会恢复仍存在的组与节点。

内核切换使用已缓存的原始订阅，分别生成 `mihomo.yaml` 和 `meow.yaml`。meow-rs 会按订阅启用 TUN IPv6，面板出口卡单独报告 IPv6。订阅页可在本机独立标签查看原始订阅和当前内核 YAML。更新客户端时应同时更新并重启 Agent 和已安装的后台内核服务。

Aether M3 的「开启时静默启动代理」会同时注册登录启动项。登录时才自动接管，网络、DNS 或内核未就绪会保持暂停，TUN 授权仍用系统对话框。设置的「外部资源管理」可以同步和定时更新 Geo 文件与远程规则集。「存储与目录」提供缓存、订阅和日志清除，应用旁 `data/` 仅统计与打开。

测试面板的“内核维护”页可用 `/api/core/type` 在 mihomo 与 meow-rs 之间切换。切换时原 mihomo 继续提供下载代理，待官方发布包和配置通过校验后再交接。首次 meow-rs 启动失败会自动恢复 mihomo，也可在面板主动切回。meow-rs 使用自己的 TUN 实现，原 mihomo TUN stack 偏好仍用于切回后的 mihomo。

两种内核的正式二进制放在应用目录。Windows 的 meow-rs 和 Wintun 放在应用目录下的 `meow/`。手动更新立即验证和交接，静默包保存在 `updates/update.pack`，暂停接管、启动客户端（含登录自启）或重启内核时自动应用。每种内核保留最近三份 OTA 备份。应用目录需要可以写入。主题「客户端设置 → 存储与目录」集中展示升级缓存、内核工作目录和配置目录容量。详见 [API](docs/api.md)。

测试面板的“内核维护”页提供清理 Agent 缓存、清空 Agent 日志，设置的“存储与目录”集中提供容量与目录入口。接口均为 POST：`/api/agent/cache/clear`、`/api/agent/logs/clear`、`/api/agent/directory/open`（`{"directory":"cache"}` 或 `{"directory":"config"}`）。缓存清理只删除 updates 下载/解压暂存及 DNS 内存记录；日志只清空 Agent 两个日志文件和内存事件，保留配置、订阅、密钥、服务日志和已下载内核。详细响应见 [API](docs/api.md#agent-缓存日志和目录)。

2026-11-16 的回归、静态检查、前端脚本/协议及隔离浏览器检查已通过。2026-11-17 的检查及构建范围见 [项目状态](docs/status.md)。完整 Go 测试仍有当前宿主路径解析/跨进程锁权限失败；不能将交叉构建视为 Linux TUN 已实测成功。

## 当前已接入

- mihomo 子进程监督、崩溃重启、配置校验和原子更新。运行中的内核可热应用 TUN 启停、协议栈、网卡名、模式和 LAN，也可热加载订阅。订阅文件位于用户配置目录。[Mihomo API](https://wiki.metacubex.one/api/)
- API 服务只绑定 `127.0.0.1:25500` 和 `[::1]:25500`；请求 Host 只接受 loopback 或当前 DNS 记录全为 loopback 的本地别名。
- WebUI 握手验证浏览器 Origin，并按需求返回 mihomo API 地址和密钥。面板可以直接调用 mihomo REST/WebSocket；密钥会暴露给该页面脚本。
- 测试面板中的启停、代理模式、系统代理、TUN、节点选择、延迟、订阅、自动启动和内核维护接口。
- Windows、Linux、macOS 的用户级开机启动；Windows 注册表、GNOME 和 macOS 网络服务的系统代理适配；桌面托盘菜单提供代理模式、重启内核和退出。
- Linux systemd 特权内核服务：WebUI 触发 polkit 授权，服务以当前登录用户运行并只获得 `CAP_NET_ADMIN`；内核服务异常退出后由 systemd 自动重启。
- Chrome 从远程 HTTPS 面板首次访问本机服务时显示“连接本机”按钮；用户授权后页面自动重连。浏览器要求这次授权由用户操作触发。

## Windows 安装包

双击 `.build/installer/ReiaClash-Setup.exe` 安装。默认路径为 `%LOCALAPPDATA%\Programs\ReiaClash`，可以改到当前用户可写的目录。桌面和开始菜单的 ReiaClash 快捷方式可分别勾选，安装完成页可选择立即启动。

重新打包时，把 Windows x64 程序、`test-webui.html` 和预置 `data/` 放入 `installer/`。脚本也兼容现有的 `installler/` 目录。安装 7-Zip 和 Inno Setup 7 后在项目根目录执行：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\build-installer.ps1
```

脚本递归打包安装素材，排除 `data/` 下的日志库、流量库、缓存数据库和节点选择缓存。通过 `-PayloadDirectory`、`-OutputDirectory` 可指定目录，通过 `-SevenZip`、`-InnoCompiler` 可指定工具路径。压缩使用 7z LZMA2 最高级别 `-mx=9`、256 MiB 字典设定和固实模式。安装时解压随包提供的内核、Geo 数据、规则集和离线 WebUI。

## 运行

在仓库目录执行 `go run ./cmd/reia-agent --webui test-webui.html`，然后打开 `http://127.0.0.1:25500/`。桌面托盘入口为 `go run ./cmd/reia-tray`。正式打包时把 `reia`、`reia-agent`、`reia-tray`、`reia-service`、`test-webui.html`、`data/` 和对应平台的内核放在同一安装目录：Windows 内核文件名为 `mihomo.exe`，Linux/macOS 为 `mihomo`，Unix 程序需要执行权限。普通进程与特权服务共用查找逻辑：显式下载并选择的更新优先，其次应用目录、当前工作目录，再回退旧选择/缓存。每个候选文件均检查普通文件类型、执行权限和 `mihomo -v` 输出，不再只判断文件存在。端口 9090 或 7890 被其他 Clash 客户端占用时显示冲突并等待端口释放，不会停止其他客户端。

静默总入口为 `reia`（Windows 为 `reia.exe`）。Windows 执行 `./build.ps1`，Linux/macOS 执行 `sh build.sh`，会生成 `reia`、`reia-agent`、`reia-tray` 和独立服务程序 `reia-service`，打包时须一起放在安装目录；原有 agent、tray 保留控制台构建用于调试。自启优先注册静默总入口；升级后可在面板关闭再开启自启以更新旧启动项。这里的自启指当前用户登录后启动。

冷启动默认暂停代理，保留内核、节点、TUN 和系统代理偏好。完全退出恢复本机网络设置。Windows meow-rs 会记录并恢复由内核改写的网卡 DNS。托盘菜单显示当前内核和进程内存。重开托盘时，已在后台运行的服务会继续运行。

Geo 文件放在 **应用二进制同目录的 `data/`**，例如 `data/geoip.dat`、`data/geosite.dat`、`data/country.mmdb`。内核启动、校验和服务模式都使用此目录；设置、订阅、密钥和日志仍放在用户配置目录。目录需要对当前用户可写；即使内核升级到用户 updates 目录，也继续使用应用旁的同一份 data。文件缺失时 mihomo 仍可能按订阅配置下载，请预制匹配的文件。`go run` 使用临时二进制目录，测试预制数据及服务模式请先构建。

TUN 权限不足时按需弹出系统授权窗口，由特权程序注册内核服务。Agent 和 WebUI 继续在普通用户会话运行。Windows 通过原生 `ShellExecuteExW` 的 `runas` 请求 UAC，用授权后的 `reia-service.exe` 注册 SCM 服务 `ReiaClashCore`。Linux 使用 systemd、pkexec/polkit 及桌面认证代理，macOS 使用系统授权和 launchd。WebUI 显示实际的服务注册状态与内核就绪状态。旧版 Windows SYSTEM 计划任务会在重新开启或修复服务时迁移。 [Microsoft 管理员权限指南](https://learn.microsoft.com/en-us/windows/win32/secbp/running-with-administrator-privileges)

重复运行总入口或直接运行组件时，每种角色最多保留一份常驻实例：托盘使用 Windows 命名互斥体或 Unix 文件锁，Agent 在启动内核前独占 25500，内核服务在读写配置和启动内核前独占 25501。正常运行包含 tray、agent、mihomo；服务模式增加独立的 `reia-service`，不会再把特权服务显示成第二个 Agent，也不会同时运行用户态与服务态两份 mihomo。锁在进程退出后由系统释放，重启不会被陈旧锁文件阻止。

托盘发现已注册或已委托的服务时，点击“退出客户端”可以选择继续后台运行、注销服务并退出，或取消。继续后台运行保留服务、Agent 和 WebUI 连接。注销服务先完成系统授权和实际卸载，再恢复本项目保存的系统代理备份并退出 Agent 与托盘，中间无需启动普通内核。取消或卸载失败时托盘保持可用。WebUI 中关闭服务开关仍会卸载服务并切回普通模式。Linux 的托盘退出对话框使用 Zenity 或 KDialog。

普通内核监督器和服务监督器共用数据目录对应的实例锁，旧监督器完成停止并释放锁后才允许新实例接管。Windows 修复/卸载先禁用旧服务启动与失败恢复，再停止服务并清理旧版留下的进程；清理同时核对可执行路径、配置参数和创建时间，不按名称批量结束进程。服务只有在自身内核就绪后才报告 SCM 运行中；Windows 安装失败会停止错误重试并返回具体原因。设置通过系统实际注册状态显示开关，另行返回 `core_service_ready`、`core_service_error` 区分可用与异常状态。

旧版服务使用 `reia-agent --core-service` 时，升级后重新启动客户端并在设置点击“修复服务”，授权后迁移到 `reia-service`；Windows 服务名称仍为 `ReiaClashCore`。修复接口为 `POST /api/system/core-service`，请求体 `{"enable":true,"repair":true}`。

启动时会核对服务记录：系统未注册服务且已认证控制端点不可达时，回退普通模式并提示重新注册；仍可连接的旧 helper 会保留以便迁移。等待内核/特权服务不会阻塞控制 API 的健康检查。托盘启动 Agent 的早期错误保存在用户配置目录 `ReiaClash/agent-startup.log`，运行日志为 `agent.log`。恢复系统代理时只使用本客户端的备份记录；服务模式退出保留备份供后续关闭代理使用。

如果系统服务实际已注册，但本地控制标记缺失或损坏，会优先恢复服务委托或保留待修复状态，不会同时启动第二个普通内核。服务启动和特权操作的具体错误分别见用户配置目录中的 `core-service-startup.log`、`core-service-operation.log`。

更新器支持 ZIP、原始 GZIP、TAR、TAR.GZ/TGZ：按可执行文件特征识别内核，兼容版本化文件名和嵌套目录，不安装包内附带的服务单元、配置或脚本。更新包通过 SHA-256 校验后，提取出的内核还必须通过 `-v` 运行验证；多候选歧义、损坏或不兼容包会返回错误。

设置中的“Geo 低内存模式”默认开启，旧设置缺少此项时也默认为开启。开启生成 `geodata-loader: memconservative`，关闭生成 `geodata-loader: standard`；导入订阅不能覆盖此开关，GeoIP 的 DAT/MMDB 格式保持由订阅控制。运行中的本地/服务内核会收到重启请求，连接会短暂中断；未启动时保存并在下次启动应用。

新配置启动时总开关关闭，默认勾选 `mips` TUN，系统代理偏好关闭。WebUI 能通过 `POST /api/agent/settings` 保存 `tun_device`。Windows/Linux 新配置默认 `ReiaClash`，macOS 由内核自动选择 `utun`，具体格式见 [API](docs/api.md#状态设置与启停)。

新增 API：`GET /api/agent/geo-low-memory` 查询；`POST /api/agent/geo-low-memory` 设置，请求体为 `{"enable":true}` 或 `{"enable":false}`。响应包含 `enabled`、`geodata_loader`、`restart_requested`；缺少或错误类型的 `enable` 返回 400。此项也通过状态接口的 `settings.geo_low_memory` 返回。`restart_requested` 表示已发出重启请求，实际内核状态通过原有状态接口查询。

验证：`go test ./...`、`node scripts/check-webui.cjs`。可设置 `REIA_TEST_MIHOMO` 为内核绝对路径运行真实内核集成测试：用隔离的 loopback 端口验证规则/全局/直连的实际 HTTP 出站以及 LAN 配置热切换，不修改系统代理或 TUN。

节点页默认按订阅 YAML 中 `proxy-groups` 的顺序展示，各组节点按 Mihomo 返回的 `all` 原始顺序展示。支持名称/延迟排序、自定义拖动节点、调整组顺序和恢复订阅顺序，偏好保存在当前浏览器。切换节点只更新选中状态，不刷新页面或重建节点组。规则模式隐藏 GLOBAL，全局模式显示并置顶 GLOBAL，直连模式隐藏节点列表。

XBoard 主题接入可在加载面板脚本前提供 `window.REIA_THEME = {siteName: '站点名'}`，或运行时调用 `window.ReiaClashPanel.configure({siteName: '站点名'})`。同名节点组优先显示（全局模式下 GLOBAL 仍排第一），未提供站点名时保持订阅顺序。测试面板也可以手动填写站点名。这是主题适配入口，当前项目不包含 XBoard 主题源码。

网络检查 API（WebUI 已提供连接管理、DNS 检查和设置页重启按钮）：

| 方法 | 路径 | 功能 |
| --- | --- | --- |
| GET | `/api/core/proxies` | 原生节点数据，附加 `group_order` 订阅组顺序 |
| GET | `/api/core/connections` | 活跃连接、进程、规则、代理链和流量 |
| DELETE | `/api/core/connections/{id}` | 关闭单条连接 |
| DELETE | `/api/core/connections` | 关闭全部连接 |
| GET | `/api/core/dns/query?name=example.com&type=A` | 通过 Mihomo DNS 查询并记录结果 |
| GET / DELETE | `/api/core/dns/records` | 查看 / 清空 DNS 查询记录 |
| POST | `/api/core/restart` | 请求重启内核，兼容保留 `/api/agent/restart` |
| GET | `/api/agent/outbound-ip` | 获取 Trace 观察到的公网出口 IP、国家代码和 Cloudflare 机房代码 |

DNS 记录来自内核 debug 日志流及面板发起的查询，最多保留本次 Agent 运行中的 200 条。融合主题连接页在前台通过 WebSocket 读取内核快照，并在页面内存保留最近 300 条已观察到的结束连接。根目录测试 WebUI 使用 REST 轮询，可按域名、IP、进程、节点或规则过滤。

可选浏览器验证：`node scripts/run-webui-browser-test.cjs`，通过独立 Chrome 无头配置目录和模拟 API 检查实际 DOM 中的节点排序、站点名优先、切换节点保留原节点组元素，以及连接/DNS/重启按钮。可用 `CHROME_PATH` 指定 Chrome；测试不连接实际客户端或外部网站。

面板发现与配对：WebUI 每 5 秒自动尝试连接本机 Agent；连接后通过握手持续报告当前完整页面 URL，支持先开网页后启动客户端，以及 Agent 重启后重新配对。扫描受浏览器本地网络访问授权和后台定时器策略约束，首次需要时请点击“连接本机”并允许本地网络访问。

`POST /api/handshake` 新增 `panel_url`（完整 HTTP(S) 或本地 `file:///` 页面地址，保留路径、查询和路由片段），兼容原有 `panel_origin`；HTTP(S) 页面的 URL 必须与浏览器 Origin 同源。本地 HTML 必须上报完整 `file:///` 地址，不会被改写为本机 HTTP 测试页。例如 `file:///C:/Users/tcs15/Desktop/ReiaClash/test-webui.html` 配对后，`panel_url` 和 `open_url` 都返回此地址。未上报 URL 不会产生虚假配对或覆盖已有地址。面板 URL 只保存在当前 Agent 内存中，不写入 settings.json 或日志，退出后清除。`GET /api/agent/panel` 返回 `paired`、`panel_url`、`open_url`、`source`、`network`；`GET /panel`（兼容 `/api/panel/open`）根据当前状态返回重定向，不缓存结果。浏览器可能限制 HTTP 跳转至本地文件，托盘及通知点击直接通过系统打开 file URL。

托盘、通知点击的地址优先级：已配对时打开最新明确上报的真实面板 URL（包括本地 file URL），联网探测不能覆盖它；尚未配对时默认打开 `https://next.rka.jp`；只有未配对且已明确检测为离线时才使用项目内置 `test-webui.html`。启动时后台对 `next.rka.jp:443`、`1.1.1.1:443`、`223.5.5.5:443` 进行最长 3 秒的 TCP 可达性检测，任一成功视为在线，每 20 秒复查。检测不依赖 HTTP 代理，也不发送面板路径或账户信息。Agent 不可用且离线时，托盘可直接打开同目录 HTML 文件。

通知统一使用简短文案：“正在启动…”、“已启动，点击打开面板。”、“面板已连接，点击打开。”；不再把内部事件和长错误直接弹给用户，详细错误仍可在面板/日志查看。Linux 的启动通知使用替换更新，减少重复横幅。Windows 接收托盘气泡激活事件，Linux 使用 D-Bus 通知动作，macOS 使用通知中心 delegate；点击会在当时重新读取当前面板目标。桌面通知显示和动作仍取决于系统通知权限及桌面环境。Windows 通知点击桥接已有隐藏窗口消息测试；Linux/macOS 通知需要在对应桌面实际验证。

## 平台状态

Windows、Linux 和 macOS 共用 agent 核心；托盘、代理、开机启动和特权服务按上述方式适配。Linux 系统代理目前针对 GNOME/gsettings；其他桌面可使用 TUN。Android 还需要 Android 应用与前台服务外壳，当前仓库不包含该外壳。
