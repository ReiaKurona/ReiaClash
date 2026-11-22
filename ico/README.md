# ReiaClash 图标

原始图片由用户提供，已从根目录 `images.jpeg` 移至 [reia-clash.jpeg](reia-clash.jpeg)。生成过程只缩放/转换格式，保持原图内容。

| 文件 | 使用位置 |
| --- | --- |
| reia-clash.jpeg | 原始素材 |
| reia-clash-64.png | 三平台托盘；测试 WebUI 内联 favicon |
| reia-clash.png | Linux 桌面/通知导出的图片、macOS 应用图标 |
| reia-clash.ico | Windows 多尺寸图标及 Aether M3 默认 favicon |
| reia-clash.icns | macOS Finder 应用入口 |
| cmd 下各组件的 icon_windows_*.syso | 四个 Windows EXE 的资源图标，含 386/amd64/arm64 |

修改 JPEG 后，在项目根目录运行：

```text
go generate ./ico
```

生成器位于 scripts/gen-icons，只使用 Go 标准库处理图片；Windows COFF 资源由固定版本 [go-winres v0.3.3](https://github.com/tc-hib/go-winres/releases/tag/v0.3.3) 生成，首次执行需要下载该开发工具。日常 `go build` 使用已生成的资源，不需要再次下载工具。

Linux 启动 reia 或 reia-tray 时，在用户 XDG 数据目录的 applications 下注册四组件的 .desktop 元数据；主入口可见，后台组件设 NoDisplay。图标从内嵌 PNG 导出到用户缓存 ReiaClash/icons，不依赖安装目录可写。已有非本项目管理的同名 .desktop 文件不会覆盖。用户自启入口与桌面通知也引用此图片。

macOS 的 sh build.sh 在构建四组件后生成四个 .app 入口，入口调用同目录的真实二进制，以保持应用旁 data 与服务路径。单独打包构建输出可运行 `go run ./scripts/package-macos -dir 构建目录`（从项目根目录执行）。托盘通过 Cocoa 设置应用图标。

Windows 四组件的文件/进程图标来自 EXE 内嵌资源，托盘来自内嵌 PNG。四个 EXE 的产品名和文件描述均为 ReiaClash。Windows 托盘在创建窗口前注册 `ReiaClash.Windows`，提供统一显示名与图标。Linux/macOS 的纯后台进程没有通用的二进制图标资源；进程查看器是否采用桌面元数据由系统决定。独立的上游 mihomo 二进制未被改写，避免改动其资源和校验结果。

桌面通知由托盘统一发送，应用名与消息标题使用 ReiaClash。Aether M3 的 favicon、未配置站点 Logo 时的默认图片由同一图标生成；自定义站点 Logo 和 Material Symbols 字体配置仍按原有方式生效。
