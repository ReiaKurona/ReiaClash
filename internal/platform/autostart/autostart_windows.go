//go:build windows

package autostart

import (
	"fmt"
	"os"
	"reia-clash/internal/platform/launch"
	"strings"
)

const runKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

func Set(enabled bool, executable string) error {
	if enabled {
		if executable == "" {
			path, err := os.Executable()
			if err != nil {
				return err
			}
			executable = path
		}
		command := `"` + strings.ReplaceAll(executable, `"`, `\"`) + `" --background --login-autostart`
		return launch.Command("reg.exe", "add", runKey, "/v", "ReiaClash", "/t", "REG_SZ", "/d", command, "/f").Run()
	}
	if launch.Command("reg.exe", "query", runKey, "/v", "ReiaClash").Run() != nil {
		return nil
	}
	if err := launch.Command("reg.exe", "delete", runKey, "/v", "ReiaClash", "/f").Run(); err != nil {
		return fmt.Errorf("移除 Windows 开机启动项失败: %w", err)
	}
	return nil
}

func Enabled(executable string) bool {
	return launch.Command("reg.exe", "query", runKey, "/v", "ReiaClash").Run() == nil
}
