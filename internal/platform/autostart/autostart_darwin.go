//go:build darwin

package autostart

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
)

func entryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", "com.reiaclash.agent.plist"), nil
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
	// plist 是 XML；对可变的可执行文件路径做实体转义后再写入。
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>Label</key><string>com.reiaclash.agent</string>
<key>ProgramArguments</key><array><string>%s</string><string>--background</string></array>
<key>RunAtLoad</key><true/></dict></plist>`, html.EscapeString(executable))
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return fmt.Errorf("写入 LaunchAgent 失败: %w", err)
	}
	return nil
}

func Enabled(executable string) bool {
	path, err := entryPath()
	if err != nil {
		return false
	}
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return true
}
