//go:build windows

package systemproxy

import (
	"encoding/json"
	"fmt"
	"os"
	"reia-clash/internal/platform/launch"
	"strings"
	"syscall"
)

const internetSettings = `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`

type registryValue struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type snapshot map[string]registryValue

var wininet = syscall.NewLazyDLL("wininet.dll")
var internetSetOption = wininet.NewProc("InternetSetOptionW")

func Enabled() bool {
	enabled, enabledOK := readValue("ProxyEnable")
	server, serverOK := readValue("ProxyServer")
	return enabledOK && serverOK && strings.EqualFold(enabled.Value, "0x1") && strings.Contains(server.Value, "127.0.0.1:7890")
}

func Set(enabled bool, backupFile string) error {
	if enabled {
		if _, err := os.Stat(backupFile); os.IsNotExist(err) {
			state := make(snapshot)
			for _, name := range []string{"ProxyEnable", "ProxyServer", "ProxyOverride", "AutoConfigURL", "AutoDetect"} {
				if value, ok := readValue(name); ok {
					state[name] = value
				}
			}
			bytes, err := json.Marshal(state)
			if err != nil {
				return err
			}
			if err := os.WriteFile(backupFile, bytes, 0600); err != nil {
				return err
			}
		}
		if err := writeValue("ProxyServer", "REG_SZ", "127.0.0.1:7890"); err != nil {
			return rollbackAfterFailure(err, backupFile)
		}
		if err := writeValue("ProxyOverride", "REG_SZ", "localhost;127.*;<local>"); err != nil {
			return rollbackAfterFailure(err, backupFile)
		}
		if err := writeValue("AutoConfigURL", "REG_SZ", ""); err != nil {
			return rollbackAfterFailure(err, backupFile)
		}
		if err := writeValue("AutoDetect", "REG_DWORD", "0"); err != nil {
			return rollbackAfterFailure(err, backupFile)
		}
		if err := writeValue("ProxyEnable", "REG_DWORD", "1"); err != nil {
			return rollbackAfterFailure(err, backupFile)
		}
		refreshWinINet()
		return nil
	}
	bytes, err := os.ReadFile(backupFile)
	if err != nil {
		if os.IsNotExist(err) {
			// 没有备份文件时，直接确保系统代理处于关闭状态
			_ = writeValue("ProxyEnable", "REG_DWORD", "0")
			refreshWinINet()
			return nil
		}
		return err
	}
	var state snapshot
	if err := json.Unmarshal(bytes, &state); err != nil {
		return err
	}
	for _, name := range []string{"ProxyEnable", "ProxyServer", "ProxyOverride", "AutoConfigURL", "AutoDetect"} {
		value, ok := state[name]
		if !ok {
			_ = launch.Command("reg.exe", "delete", internetSettings, "/v", name, "/f").Run()
			continue
		}
		if err := writeValue(name, value.Kind, value.Value); err != nil {
			return err
		}
	}
	_ = os.Remove(backupFile)
	refreshWinINet()
	return nil
}

func rollbackAfterFailure(cause error, backupFile string) error {
	if err := Set(false, backupFile); err != nil {
		return fmt.Errorf("%w；恢复原代理设置也失败: %v", cause, err)
	}
	return cause
}

func readValue(name string) (registryValue, bool) {
	out, err := launch.Command("reg.exe", "query", internetSettings, "/v", name).CombinedOutput()
	if err != nil {
		return registryValue{}, false
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 3 && strings.EqualFold(fields[0], name) {
			return registryValue{Kind: fields[1], Value: strings.Join(fields[2:], " ")}, true
		}
	}
	return registryValue{}, false
}

func writeValue(name, kind, value string) error {
	args := []string{"add", internetSettings, "/v", name, "/t", kind, "/d", value, "/f"}
	if output, err := launch.Command("reg.exe", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("更新 Windows 系统代理注册表失败: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func refreshWinINet() {
	const settingsChanged = uintptr(39)
	const refresh = uintptr(37)
	_, _, _ = internetSetOption.Call(0, settingsChanged, 0, 0)
	_, _, _ = internetSetOption.Call(0, refresh, 0, 0)
}
