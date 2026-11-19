//go:build linux && !android

package trayui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func showExitDialog(ctx context.Context) (exitChoice, error) {
	var zenityErr error
	if path, err := exec.LookPath("zenity"); err == nil {
		out, err := exec.CommandContext(ctx, path, "--question", "--title="+exitTitle, "--text="+exitMessage,
			"--ok-label="+keepServiceLabel, "--cancel-label="+cancelExitLabel, "--extra-button="+uninstallLabel).Output()
		if ctx.Err() != nil {
			return cancelExit, ctx.Err()
		}
		if strings.TrimSpace(string(out)) == uninstallLabel {
			return uninstallService, nil
		}
		if err == nil {
			return keepService, nil
		}
		var failed *exec.ExitError
		if errors.As(err, &failed) && failed.ExitCode() == 1 && len(failed.Stderr) == 0 {
			return cancelExit, nil
		}
		zenityErr = fmt.Errorf("Zenity 退出对话框失败: %w", err)
	}
	if path, err := exec.LookPath("kdialog"); err == nil {
		out, err := exec.CommandContext(ctx, path, "--title", exitTitle, "--menu", exitMessage,
			"keep", keepServiceLabel, "uninstall", uninstallLabel).Output()
		if ctx.Err() != nil {
			return cancelExit, ctx.Err()
		}
		if err == nil {
			switch strings.TrimSpace(string(out)) {
			case "keep":
				return keepService, nil
			case "uninstall":
				return uninstallService, nil
			}
			return cancelExit, fmt.Errorf("无法识别 KDialog 的退出选项")
		}
		var failed *exec.ExitError
		if errors.As(err, &failed) && failed.ExitCode() == 1 && len(failed.Stderr) == 0 {
			return cancelExit, nil
		}
		return cancelExit, fmt.Errorf("KDialog 退出对话框失败: %w", err)
	}
	if zenityErr != nil {
		return cancelExit, zenityErr
	}
	return cancelExit, fmt.Errorf("找不到 Zenity 或 KDialog，无法显示退出选项")
}
