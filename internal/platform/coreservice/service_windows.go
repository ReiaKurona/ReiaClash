//go:build windows

package coreservice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"

	"reia-clash/internal/platform/launch"
)

const taskName = "ReiaClashCore"

// Supported 表示支持 UAC 授权安装 Windows SCM 系统服务。
func Supported() bool { return true }

// RequestInstall requests normal Windows UAC consent for the service helper.
func RequestInstall(ctx context.Context, dataDir string) error {
	return requestWindowsService(ctx, dataDir, "--install-core-service", "安装")
}

func RequestUninstall(ctx context.Context, dataDir string) error {
	return requestWindowsService(ctx, dataDir, "--uninstall-core-service", "移除")
}

func requestWindowsService(ctx context.Context, dataDir, operation, label string) error {
	executable, err := Executable()
	if err != nil {
		return err
	}
	executable, err = launch.ResolveExecutable(executable)
	if err != nil {
		return fmt.Errorf("无法访问 Windows 服务程序 %q: %w", executable, err)
	}
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return err
	}

	handle, err := startElevated(ctx, executable, []string{operation, "--data-dir", absDataDir})
	if err != nil {
		switch {
		case errors.Is(err, windows.ERROR_CANCELLED):
			return fmt.Errorf("已取消 Windows 管理员授权: %w", err)
		case errors.Is(err, windows.ERROR_ACCESS_DENIED), errors.Is(err, windows.ERROR_ACCESS_DISABLED_BY_POLICY),
			errors.Is(err, windows.ERROR_ELEVATION_REQUIRED), errors.Is(err, windows.ERROR_SERVICE_DISABLED):
			return fmt.Errorf("Windows 拒绝启动管理员授权程序，请检查 UAC、Application Information 服务或组策略: %w", err)
		default:
			return fmt.Errorf("无法启动 Windows 管理员授权程序: %w", err)
		}
	}
	defer windows.CloseHandle(handle)
	code, err := waitElevated(handle)
	if err != nil {
		return fmt.Errorf("等待 Windows 特权服务%s完成失败: %w", label, err)
	}
	if code == 0 {
		return nil
	}
	detail, _ := os.ReadFile(filepath.Join(absDataDir, "core-service-operation.log"))
	if message := strings.TrimSpace(string(detail)); message != "" {
		return fmt.Errorf("Windows 特权服务%s失败: %s", label, message)
	}
	return fmt.Errorf("Windows 特权服务%s失败，安装器退出码 %d", label, code)
}

// uninstallLegacyTask is only used to migrate/remove pre-SCM installations.
func uninstallLegacyTask(requestedUID string) error {
	_ = launch.Command("schtasks.exe", "/end", "/tn", taskName).Run()
	if out, err := launch.Command("schtasks.exe", "/delete", "/tn", taskName, "/f").CombinedOutput(); err != nil {
		return fmt.Errorf("删除 Windows 特权任务失败: %s (%w)", strings.TrimSpace(string(out)), err)
	}
	return nil
}
