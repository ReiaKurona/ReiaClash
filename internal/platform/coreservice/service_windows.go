//go:build windows

package coreservice

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"reia-clash/internal/platform/launch"
)

const taskName = "ReiaClashCore"

// Supported 表示支持 UAC 授权安装 Windows SCM 系统服务。
func Supported() bool { return true }

// RequestInstall 请求 Windows UAC 管理员提权，并安装/启动 SCM 服务。
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

	// 使用 PowerShell Start-Process -Verb RunAs 触发原生 Windows UAC 提权窗口
	psScript := fmt.Sprintf(
		`$ErrorActionPreference = 'Stop'; $proc = Start-Process -FilePath '%s' -ArgumentList '--install-core-service', '--data-dir', '"%s"' -WindowStyle Hidden -Verb RunAs -PassThru -Wait; exit $proc.ExitCode`,
		strings.ReplaceAll(executable, "'", "''"),
		strings.ReplaceAll(absDataDir, "'", "''"),
	)
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(callCtx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", psScript)
	launch.HideCommand(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if detail, readErr := os.ReadFile(filepath.Join(absDataDir, "core-service-operation.log")); readErr == nil && len(detail) > 0 {
			msg = strings.TrimSpace(string(detail))
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("Windows 特权服务提权安装失败：%s", msg)
	}
	return nil
}

// RequestUninstall 请求 Windows UAC 管理员提权，并移除特权内核服务。
func RequestUninstall(ctx context.Context, dataDir string) error {
	executable, err := Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}

	psScript := fmt.Sprintf(
		`$ErrorActionPreference = 'Stop'; $proc = Start-Process -FilePath '%s' -ArgumentList '--uninstall-core-service', '--data-dir', '"%s"' -WindowStyle Hidden -Verb RunAs -PassThru -Wait; exit $proc.ExitCode`,
		strings.ReplaceAll(executable, "'", "''"),
		strings.ReplaceAll(dataDir, "'", "''"),
	)
	callCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(callCtx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", psScript)
	launch.HideCommand(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if detail, readErr := os.ReadFile(filepath.Join(dataDir, "core-service-operation.log")); readErr == nil && len(detail) > 0 {
			msg = strings.TrimSpace(string(detail))
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("Windows 特权服务提权移除失败：%s", msg)
	}
	return nil
}

// uninstallLegacyTask is only used to migrate/remove pre-SCM installations.
func uninstallLegacyTask(requestedUID string) error {
	_ = launch.Command("schtasks.exe", "/end", "/tn", taskName).Run()
	if out, err := launch.Command("schtasks.exe", "/delete", "/tn", taskName, "/f").CombinedOutput(); err != nil {
		return fmt.Errorf("删除 Windows 特权任务失败: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}
