//go:build linux && !android

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
	branding "reia-clash/ico"
	"strings"
)

func entryPath() (string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "autostart", "reia-clash.desktop"), nil
}

func Set(enabled bool, executable string) error {
	path, err := entryPath()
	if err != nil {
		return err
	}
	if !enabled {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if executable == "" {
		executable, err = os.Executable()
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	// Desktop Entry 的 Exec 字段不使用 shell；双引号和反斜杠需单独转义。
	quoted := `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(executable) + `" --background --login-autostart`
	content := "[Desktop Entry]\nType=Application\nName=ReiaClash\nExec=" + quoted + "\nX-GNOME-Autostart-enabled=true\n"
	if icon, err := branding.DesktopIconPath(); err == nil {
		content += "Icon=" + strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\r", `\r`).Replace(icon) + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return fmt.Errorf("写入 XDG 开机启动项失败: %w", err)
	}
	return nil
}

func Enabled(executable string) bool {
	path, err := entryPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}
