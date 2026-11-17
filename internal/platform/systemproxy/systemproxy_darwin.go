//go:build darwin

package systemproxy

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type proxyServiceState struct {
	Service              string `json:"service"`
	HTTPServer           string `json:"http_server"`
	HTTPPort             string `json:"http_port"`
	HTTPEnabled          bool   `json:"http_enabled"`
	HTTPSServer          string `json:"https_server"`
	HTTPSPort            string `json:"https_port"`
	HTTPSEnabled         bool   `json:"https_enabled"`
	AutoProxyURL         string `json:"auto_proxy_url"`
	AutoProxyEnabled     bool   `json:"auto_proxy_enabled"`
	AutoDiscoveryEnabled bool   `json:"auto_discovery_enabled"`
}

func Enabled() bool {
	services, err := serviceNames()
	if err != nil || len(services) == 0 {
		return false
	}
	tool, err := networksetupPath()
	if err != nil {
		return false
	}
	state, err := readProxyService(tool, services[0])
	return err == nil && state.HTTPEnabled && state.HTTPServer == "127.0.0.1" && state.HTTPPort == "7890" && !state.AutoProxyEnabled && !state.AutoDiscoveryEnabled
}

func Set(enabled bool, backupFile string) error {
	if enabled {
		services, err := serviceNames()
		if err != nil {
			return err
		}
		original, err := loadOrSaveProxySnapshot(services, backupFile)
		if err != nil {
			return err
		}
		if err := runAuthorizedNetworksetup(proxyOnCommands(original), proxyRestoreCommands(original)); err != nil {
			if current, readErr := readProxySnapshot(services); readErr == nil && proxySnapshotsEqual(original, current) {
				_ = os.Remove(backupFile)
			}
			return fmt.Errorf("设置 macOS 系统代理失败，授权窗口可能已取消: %w", err)
		}
		return nil
	}
	data, err := os.ReadFile(backupFile)
	if err != nil {
		if os.IsNotExist(err) {
			services, serviceErr := serviceNames()
			if serviceErr == nil {
				var cmds [][]string
				for _, s := range services {
					cmds = append(cmds, []string{"-setwebproxystate", s, "off"}, []string{"-setsecurewebproxystate", s, "off"})
				}
				_ = runAuthorizedNetworksetup(cmds, nil)
			}
			return nil
		}
		return err
	}
	var original []proxyServiceState
	if err := json.Unmarshal(data, &original); err != nil {
		return err
	}
	if err := runAuthorizedNetworksetup(proxyRestoreCommands(original), proxyOnCommands(original)); err != nil {
		return fmt.Errorf("恢复 macOS 系统代理失败，授权窗口可能已取消: %w", err)
	}
	_ = os.Remove(backupFile)
	return nil
}

func proxySnapshotsEqual(left, right []proxyServiceState) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func loadOrSaveProxySnapshot(services []string, backupFile string) ([]proxyServiceState, error) {
	data, err := os.ReadFile(backupFile)
	if err == nil {
		var original []proxyServiceState
		if err := json.Unmarshal(data, &original); err != nil {
			return nil, fmt.Errorf("读取 macOS 代理备份失败: %w", err)
		}
		return original, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	original, err := readProxySnapshot(services)
	if err != nil {
		return nil, err
	}
	data, err = json.Marshal(original)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(backupFile), 0700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(backupFile, data, 0600); err != nil {
		return nil, err
	}
	return original, nil
}

func readProxySnapshot(services []string) ([]proxyServiceState, error) {
	var original []proxyServiceState
	tool, err := networksetupPath()
	if err != nil {
		return nil, err
	}
	for _, service := range services {
		state, err := readProxyService(tool, service)
		if err != nil {
			return nil, err
		}
		original = append(original, state)
	}
	return original, nil
}

func readProxyService(tool, service string) (proxyServiceState, error) {
	web, webErr := exec.Command(tool, "-getwebproxy", service).Output()
	secure, secureErr := exec.Command(tool, "-getsecurewebproxy", service).Output()
	autoURL, autoURLErr := exec.Command(tool, "-getautoproxyurl", service).Output()
	autoDiscovery, autoDiscoveryErr := exec.Command(tool, "-getproxyautodiscovery", service).Output()
	if webErr != nil || secureErr != nil || autoURLErr != nil || autoDiscoveryErr != nil {
		return proxyServiceState{}, fmt.Errorf("读取 macOS 系统代理状态失败: %s", service)
	}
	state := parseProxyState(service, web, secure)
	autoValues := parseValues(autoURL)
	discoveryValues := parseValues(autoDiscovery)
	state.AutoProxyURL = autoValues["URL"]
	state.AutoProxyEnabled = autoValues["Enabled"] == "Yes"
	state.AutoDiscoveryEnabled = strings.EqualFold(discoveryValues["Auto Proxy Discovery"], "On")
	return state, nil
}

func proxyOnCommands(original []proxyServiceState) [][]string {
	var commands [][]string
	for _, state := range original {
		service := state.Service
		if state.AutoProxyEnabled {
			commands = append(commands, []string{"-setautoproxystate", service, "off"})
		}
		if state.AutoDiscoveryEnabled {
			commands = append(commands, []string{"-setproxyautodiscovery", service, "off"})
		}
		commands = append(commands,
			[]string{"-setwebproxy", service, "127.0.0.1", "7890"},
			[]string{"-setsecurewebproxy", service, "127.0.0.1", "7890"},
			[]string{"-setwebproxystate", service, "on"},
			[]string{"-setsecurewebproxystate", service, "on"},
		)
	}
	return commands
}

func proxyRestoreCommands(original []proxyServiceState) [][]string {
	var commands [][]string
	for _, state := range original {
		httpHost, httpPort := state.HTTPServer, state.HTTPPort
		if httpHost == "" {
			httpHost = "127.0.0.1"
		}
		if httpPort == "" {
			httpPort = "80"
		}
		httpsHost, httpsPort := state.HTTPSServer, state.HTTPSPort
		if httpsHost == "" {
			httpsHost = "127.0.0.1"
		}
		if httpsPort == "" {
			httpsPort = "443"
		}
		httpState, httpsState := "off", "off"
		if state.HTTPEnabled {
			httpState = "on"
		}
		if state.HTTPSEnabled {
			httpsState = "on"
		}
		commands = append(commands,
			[]string{"-setwebproxy", state.Service, httpHost, httpPort},
			[]string{"-setsecurewebproxy", state.Service, httpsHost, httpsPort},
			[]string{"-setwebproxystate", state.Service, httpState},
			[]string{"-setsecurewebproxystate", state.Service, httpsState},
			[]string{"-setautoproxystate", state.Service, proxyState(state.AutoProxyEnabled)},
			[]string{"-setproxyautodiscovery", state.Service, proxyState(state.AutoDiscoveryEnabled)},
		)
	}
	return commands
}

func proxyState(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}

func parseProxyState(service string, web, secure []byte) proxyServiceState {
	webValues, secureValues := parseValues(web), parseValues(secure)
	return proxyServiceState{
		Service: service, HTTPServer: webValues["Server"], HTTPPort: webValues["Port"],
		HTTPEnabled: webValues["Enabled"] == "Yes", HTTPSServer: secureValues["Server"],
		HTTPSPort: secureValues["Port"], HTTPSEnabled: secureValues["Enabled"] == "Yes",
	}
}

func parseValues(output []byte) map[string]string {
	values := make(map[string]string)
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			values[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return values
}

func serviceNames() ([]string, error) {
	tool, err := networksetupPath()
	if err != nil {
		return nil, err
	}
	out, err := exec.Command(tool, "-listallnetworkservices").Output()
	if err != nil {
		return nil, fmt.Errorf("读取 macOS 网络服务失败: %w", err)
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("macOS 未找到网络服务")
	}
	var names []string
	for _, line := range lines[1:] {
		name := strings.TrimSpace(line)
		if name != "" && !strings.HasPrefix(name, "*") {
			names = append(names, name)
		}
	}
	return names, nil
}
