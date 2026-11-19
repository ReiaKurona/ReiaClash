# ReiaClash 修改入口

开始修改前阅读：

1. [context.md](context.md)：最新用户约定、修复上下文和接手流程。
2. [docs/status.md](docs/status.md)：实现状态、实测范围和待办。
3. [docs/architecture.md](docs/architecture.md)：进程/权限/数据/服务边界。
4. 涉及接口或面板时阅读 [docs/api.md](docs/api.md)。
5. 涉及外部站点嵌入时阅读 [docs/frontend-integration.md](docs/frontend-integration.md)，接入类和示例位于 docs/examples/。

开发时保持：

- Windows、Linux、macOS 的平台边界；测试 WebUI 位于根目录 test-webui.html。
- 服务模式退出托盘后保留后台 Agent API 和内核服务，确保网页继续连接。
- 普通/服务内核共用实例锁，不能并发启动两份项目内核。
- 服务注册标记不等于真实注册或内核就绪；服务切换需验证系统状态与交接。
- 进程清理须核对归属，不能按名称批量结束 Clash/mihomo。
- 真实面板完整 URL（含 file:///）优先，data 始终位于应用旁。
- 不把密钥、真实订阅凭证或用户网络日志复制进项目文档。
- 按改动选择验证；文档修改只需核对内容与链接，不需要重启服务或触发授权。
- 更新行为/API 后同步上下文、状态、架构/API 和 README；区分已实现、已构建、已实测。

本地运行状态会变化，文档中的历史 PID 不可用于清理进程。当前工作目录未包含 Git 元数据。
