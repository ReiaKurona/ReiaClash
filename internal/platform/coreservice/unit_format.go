package coreservice

import (
	"strconv"
	"strings"
)

// renderUnit 为 Mihomo helper 生成 systemd 服务配置。服务以登录用户身份运行，
// 只获得 CAP_NET_ADMIN，浏览器 API agent 不会以 root 身份运行。
func renderUnit(executable, dataDir string, uid, gid int) string {
	return "[Unit]\n" +
		"Description=ReiaClash privileged Mihomo service\n" +
		"After=network-online.target\n" +
		"Wants=network-online.target\n" +
		"StartLimitIntervalSec=0\n\n" +
		"[Service]\n" +
		"Type=simple\n" +
		"User=" + strconv.Itoa(uid) + "\n" +
		"Group=" + strconv.Itoa(gid) + "\n" +
		"WorkingDirectory=" + systemdWorkingDirectory(dataDir) + "\n" +
		"ExecStart=" + systemdQuote(executable) + " --core-service --data-dir " + systemdQuote(dataDir) + "\n" +
		"Restart=always\n" +
		"RestartSec=2s\n" +
		"KillMode=control-group\n" +
		"AmbientCapabilities=CAP_NET_ADMIN\n" +
		"CapabilityBoundingSet=CAP_NET_ADMIN\n" +
		"NoNewPrivileges=true\n" +
		"PrivateTmp=true\n" +
		"RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK\n" +
		"UMask=0077\n\n" +
		"[Install]\n" +
		"WantedBy=multi-user.target\n"
}

// WorkingDirectory is a single literal path: its parser expands % specifiers,
// but does not remove argument quotes or expand $ like ExecStart does.
func systemdWorkingDirectory(value string) string {
	return strings.ReplaceAll(value, "%", "%%")
}

// systemdQuote 按 unit 文件格式转义一个参数。systemd 会直接执行命令，不经过 shell。
func systemdQuote(value string) string {
	var out strings.Builder
	out.WriteByte('"')
	for _, char := range value {
		switch char {
		case '\\', '"':
			out.WriteByte('\\')
			out.WriteRune(char)
		case '$':
			out.WriteString("$$")
		case '%':
			out.WriteString("%%")
		default:
			out.WriteRune(char)
		}
	}
	out.WriteByte('"')
	return out.String()
}
