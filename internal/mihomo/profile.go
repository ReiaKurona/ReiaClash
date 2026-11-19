package mihomo

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"reia-clash/internal/config"
)

var managedKeys = map[string]bool{
	"geodata-loader": true,
	"allow-lan":      true, "bind-address": true, "external-controller": true,
	"external-controller-cors": true, "external-controller-pipe": true,
	"external-controller-tls": true, "external-controller-unix": true,
	"listeners": true, "log-level": true, "mode": true,
	"mixed-port": true, "port": true, "redir-port": true,
	"secret": true, "socks-port": true, "tproxy-port": true, "tun": true,
}

// BuildConfig 在保留订阅中的节点、策略组、规则、提供器、DNS 和注释的同时，
// 覆盖由 ReiaClash 管理的监听地址、密钥、模式和 TUN 设置。
func BuildConfig(profile []byte, settings config.Settings, secret string) ([]byte, error) {
	clean, err := stripManagedKeys(profile)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	bind := "127.0.0.1"
	if settings.AllowLAN {
		bind = "*"
	}
	fmt.Fprintf(&out, "mixed-port: 7890\nallow-lan: %t\nbind-address: %q\nmode: %s\n", settings.AllowLAN, bind, settings.OutboundMode)
	fmt.Fprintf(&out, "log-level: info\nexternal-controller: 127.0.0.1:9090\nsecret: %q\n", secret)
	fmt.Fprintf(&out, "geodata-loader: %s\n", config.GeodataLoader(settings.GeoLowMemory))
	out.WriteString("external-controller-cors:\n  allow-origins:\n    - '*'\n  allow-private-network: true\n")
	out.WriteString("tun:\n  enable: ")
	if settings.Enabled && settings.UseTun && settings.TunAuthorized {
		out.WriteString("true\n")
	} else {
		out.WriteString("false\n")
	}
	stack := settings.TunStack
	if !config.ValidTunStack(stack) {
		stack = config.Defaults().TunStack
	}
	fmt.Fprintf(&out, "  stack: %s\n  auto-route: true\n  auto-detect-interface: true\n", stack)
	if settings.TunDevice != "" && config.ValidTunDevice(settings.TunDevice) {
		fmt.Fprintf(&out, "  device: %q\n", settings.TunDevice)
	}
	out.Write(clean)
	if len(clean) > 0 && clean[len(clean)-1] != '\n' {
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

// stripManagedKeys 只处理 YAML 顶层由 agent 接管的键，避免订阅里的重复键
// 覆盖 loopback、secret 或授权后的系统设置。其他内容原样交给 mihomo 校验。
func stripManagedKeys(src []byte) ([]byte, error) {
	var kept []string
	scanner := bufio.NewScanner(bytes.NewReader(src))
	scanner.Buffer(make([]byte, 4096), 16<<20)
	var skipping bool
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if indent == 0 && !strings.HasPrefix(trimmed, "#") && strings.Contains(trimmed, ":") {
			key := strings.Trim(strings.TrimSpace(strings.SplitN(trimmed, ":", 2)[0]), `"'`)
			skipping = managedKeys[key]
		}
		if !skipping {
			kept = append(kept, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return []byte(strings.Join(kept, "\n")), nil
}
