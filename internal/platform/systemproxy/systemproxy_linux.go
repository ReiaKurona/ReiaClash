//go:build linux && !android

package systemproxy

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func Enabled() bool {
	mode := get("org.gnome.system.proxy", "mode")
	host := strings.Trim(string(get("org.gnome.system.proxy.http", "host")), "'\n\r ")
	port := strings.TrimSpace(string(get("org.gnome.system.proxy.http", "port")))
	return strings.Contains(string(mode), "manual") && host == "127.0.0.1" && port == "7890"
}

func Set(enabled bool, backupFile string) error {
	if _, err := exec.LookPath("gsettings"); err != nil {
		return fmt.Errorf("当前桌面没有 gsettings 系统代理接口")
	}
	if enabled {
		if _, err := os.Stat(backupFile); os.IsNotExist(err) {
			state := map[string]string{
				"mode":       getString("org.gnome.system.proxy", "mode"),
				"http-host":  getString("org.gnome.system.proxy.http", "host"),
				"http-port":  getString("org.gnome.system.proxy.http", "port"),
				"https-host": getString("org.gnome.system.proxy.https", "host"),
				"https-port": getString("org.gnome.system.proxy.https", "port"),
			}
			bytes, err := json.Marshal(state)
			if err != nil {
				return err
			}
			if err := os.WriteFile(backupFile, bytes, 0600); err != nil {
				return err
			}
		}
		for _, args := range [][]string{
			{"set", "org.gnome.system.proxy.http", "host", "'127.0.0.1'"},
			{"set", "org.gnome.system.proxy.http", "port", "7890"},
			{"set", "org.gnome.system.proxy.https", "host", "'127.0.0.1'"},
			{"set", "org.gnome.system.proxy.https", "port", "7890"},
			{"set", "org.gnome.system.proxy", "mode", "'manual'"},
		} {
			if err := exec.Command("gsettings", args...).Run(); err != nil {
				cause := fmt.Errorf("设置 GNOME 系统代理失败: %w", err)
				if rollbackErr := Set(false, backupFile); rollbackErr != nil {
					return fmt.Errorf("%w；恢复原代理设置也失败: %v", cause, rollbackErr)
				}
				return cause
			}
		}
		return nil
	}
	bytes, err := os.ReadFile(backupFile)
	if err != nil {
		if os.IsNotExist(err) {
			_ = exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "'none'").Run()
			return nil
		}
		return err
	}
	var state map[string]string
	if err := json.Unmarshal(bytes, &state); err != nil {
		return err
	}
	for _, item := range []struct{ schema, key, saved string }{
		{"org.gnome.system.proxy.http", "host", "http-host"},
		{"org.gnome.system.proxy.http", "port", "http-port"},
		{"org.gnome.system.proxy.https", "host", "https-host"},
		{"org.gnome.system.proxy.https", "port", "https-port"},
		{"org.gnome.system.proxy", "mode", "mode"},
	} {
		if value := state[item.saved]; value != "" {
			if err := exec.Command("gsettings", "set", item.schema, item.key, value).Run(); err != nil {
				return fmt.Errorf("恢复 GNOME 系统代理设置失败: %w", err)
			}
		}
	}
	_ = os.Remove(backupFile)
	return nil
}

func get(schema, key string) []byte {
	out, err := exec.Command("gsettings", "get", schema, key).Output()
	if err != nil {
		return nil
	}
	return out
}

func getString(schema, key string) string {
	return strings.TrimSpace(string(get(schema, key)))
}
