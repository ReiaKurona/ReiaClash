//go:build darwin

package coreservice

import (
	"context"
	"fmt"
	"html"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	plistLabel = "com.reiaclash.core"
	plistPath  = "/Library/LaunchDaemons/" + plistLabel + ".plist"
)

// Supported 表示 macOS 平台支持特权内核服务安装器（通过 AppleScript 提权并由 launchd 守护）。
func Supported() bool { return true }

// RequestInstall 请求 macOS 管理员提权，并安装/启动特权内核服务。
func RequestInstall(ctx context.Context, dataDir string) error {
	executable, err := Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return err
	}
	command := shellQuote(executable) + " --install-core-service --data-dir " + shellQuote(absDataDir)
	script := "do shell script " + strconv.Quote(command) + " with administrator privileges"
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(callCtx, "osascript", "-e", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("macOS 特权服务授权安装失败：%s", strings.TrimSpace(string(out)))
	}
	return nil
}

// RequestUninstall 请求 macOS 管理员提权，并移除特权内核服务。
func RequestUninstall(ctx context.Context, dataDir string) error {
	executable, err := Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}
	script := "do shell script " + strconv.Quote(shellQuote(executable)+" --uninstall-core-service --data-dir "+shellQuote(dataDir)) + " with administrator privileges"
	callCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(callCtx, "osascript", "-e", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("macOS 特权服务授权移除失败：%s", strings.TrimSpace(string(out)))
	}
	return nil
}

// InstallPrivileged 在由管理员授权启动的特权进程中执行，注册 launchd plist 并加载运行。
func InstallPrivileged(dataDir, requestedUID string) error {
	executable, err := Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return err
	}

	if err := UninstallPrivileged(absDataDir, requestedUID); err != nil {
		return err
	}

	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>--core-service</string>
        <string>--data-dir</string>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>AbandonProcessGroup</key>
    <false/>
    <key>ExitTimeOut</key>
    <integer>20</integer>
</dict>
</plist>
`, plistLabel, html.EscapeString(executable), html.EscapeString(absDataDir))

	if err := os.WriteFile(plistPath, []byte(plistContent), 0644); err != nil {
		return fmt.Errorf("写入 launchd plist 失败: %w", err)
	}

	if out, err := exec.Command("launchctl", "load", "-w", plistPath).CombinedOutput(); err != nil {
		return fmt.Errorf("启动 macOS 特权服务失败: %s (%w)", strings.TrimSpace(string(out)), err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:25501", 300*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("macOS 内核服务未在 10 秒内启动，请检查 core-service.log")
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// UninstallPrivileged 卸载 launchd 服务并删除 plist。
func UninstallPrivileged(dataDir, requestedUID string) error {
	loaded, err := daemonLoaded()
	if err != nil {
		return err
	}
	if loaded {
		if output, err := exec.Command("launchctl", "bootout", "system/"+plistLabel).CombinedOutput(); err != nil {
			return fmt.Errorf("停止 macOS 内核服务失败: %s (%w)", strings.TrimSpace(string(output)), err)
		}
		deadline := time.Now().Add(20 * time.Second)
		for {
			loaded, err = daemonLoaded()
			if err != nil {
				return err
			}
			if !loaded {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("等待 macOS 服务退出超时")
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func daemonLoaded() (bool, error) {
	output, err := exec.Command("launchctl", "print", "system/"+plistLabel).CombinedOutput()
	if err == nil {
		return true, nil
	}
	text := strings.ToLower(string(output))
	if strings.Contains(text, "could not find service") || strings.Contains(text, "no such process") {
		return false, nil
	}
	return false, fmt.Errorf("查询 macOS 内核服务失败: %s (%w)", strings.TrimSpace(string(output)), err)
}
