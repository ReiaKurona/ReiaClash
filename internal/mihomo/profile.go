package mihomo

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/netip"
	"runtime"
	"strings"

	"go.yaml.in/yaml/v3"
	"reia-clash/internal/config"
	"reia-clash/internal/kernelbinary"
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
	meow := kernelbinary.IsMeow(settings.ActiveBinary)
	if meow {
		out.WriteString("  auto-route: true\n")
	} else {
		fmt.Fprintf(&out, "  stack: %s\n  auto-route: true\n  auto-detect-interface: true\n", stack)
	}
	if settings.TunDevice != "" && config.ValidTunDevice(settings.TunDevice) {
		fmt.Fprintf(&out, "  device: %q\n", settings.TunDevice)
	}
	if meow {
		tun, err := MeowTunOptions(profile)
		if err != nil {
			return nil, err
		}
		if tun.DNSHijack != nil {
			if len(tun.DNSHijack) == 0 {
				out.WriteString("  dns-hijack: []\n")
			} else {
				fmt.Fprintf(&out, "  dns-hijack: [%q]\n", tun.DNSHijack[0])
			}
		}
		if tun.IPv6Address != "" {
			fmt.Fprintf(&out, "  inet6-address: %q\n", tun.IPv6Address)
			if !hasTopLevelKey(clean, "ipv6") {
				out.WriteString("ipv6: true\n")
			}
		}
		if preserveMeowSystemDNS() && !hasTopLevelKey(clean, "sniffer") {
			out.WriteString("sniffer:\n  enable: true\n  parse-pure-ip: true\n  override-destination: false\n  sniff:\n    HTTP: {ports: [80, 8080]}\n    TLS: {ports: [443, 8443]}\n    QUIC: {ports: [443]}\n")
		}
	}
	out.Write(clean)
	if len(clean) > 0 && clean[len(clean)-1] != '\n' {
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

type MeowTUN struct {
	IPv6Address string
	DNSHijack   []string
}

func MeowTunOptions(profile []byte) (MeowTUN, error) {
	var options struct {
		IPv6 *bool `yaml:"ipv6"`
		DNS  struct {
			Enable bool `yaml:"enable"`
			IPv6   bool `yaml:"ipv6"`
		} `yaml:"dns"`
		Tun struct {
			Address   yaml.Node `yaml:"inet6-address"`
			DNSHijack yaml.Node `yaml:"dns-hijack"`
		} `yaml:"tun"`
	}
	if err := yaml.Unmarshal(profile, &options); err != nil {
		return MeowTUN{}, err
	}
	tun := MeowTUN{}
	if preserveMeowSystemDNS() {
		tun.DNSHijack = []string{}
		var entries []string
		if options.DNS.Enable && options.Tun.DNSHijack.Decode(&entries) == nil && requestsUDP53Hijack(entries) {
			tun.DNSHijack = []string{"any:53"}
		}
	}
	if options.IPv6 != nil && !*options.IPv6 {
		return tun, nil
	}
	address := options.Tun.Address
	if address.Kind == yaml.AliasNode && address.Alias != nil {
		address = *address.Alias
	}
	if address.Kind == yaml.SequenceNode {
		if len(address.Content) == 0 {
			return tun, nil
		}
		address = *address.Content[0]
	}
	if address.Kind != 0 && address.Tag != "!!null" {
		value := strings.TrimSpace(address.Value)
		if value == "" && address.Tag == "!!str" {
			return tun, nil
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil || !prefix.Addr().Is6() || prefix.Addr().Is4In6() {
			return MeowTUN{}, fmt.Errorf("订阅 tun.inet6-address 需要有效的 IPv6 CIDR")
		}
		tun.IPv6Address = prefix.String()
		return tun, nil
	}
	if options.IPv6 != nil || options.DNS.IPv6 {
		tun.IPv6Address = "fdfe:dcba:9876::1/126"
	}
	return tun, nil
}

func requestsUDP53Hijack(entries []string) bool {
	for _, entry := range entries {
		entry = strings.ToLower(strings.TrimSpace(entry))
		host, port, err := net.SplitHostPort(strings.TrimPrefix(entry, "udp://"))
		if err == nil && port == "53" && (host == "any" || host == "0.0.0.0" || host == "::") {
			return true
		}
	}
	return false
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

func preserveMeowSystemDNS() bool { return runtime.GOOS == "windows" || runtime.GOOS == "linux" }

func hasTopLevelKey(profile []byte, key string) bool {
	for _, line := range bytes.Split(profile, []byte{10}) {
		if len(line) == 0 || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
			continue
		}
		name, _, found := strings.Cut(string(line), ":")
		if found && strings.Trim(strings.TrimSpace(name), `"'`) == key {
			return true
		}
	}
	return false
}
