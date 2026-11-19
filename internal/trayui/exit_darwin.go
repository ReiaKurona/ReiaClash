//go:build darwin

package trayui

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func showExitDialog(ctx context.Context) (exitChoice, error) {
	script := fmt.Sprintf(`try
	activate
	return button returned of (display dialog %q with title %q buttons {%q, %q, %q} default button %q cancel button %q)
on error number -128
	return ""
end try`, exitMessage, exitTitle, cancelExitLabel, uninstallLabel, keepServiceLabel, keepServiceLabel, cancelExitLabel)
	output, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", script).CombinedOutput()
	if err != nil {
		return cancelExit, fmt.Errorf("macOS 退出对话框失败: %w: %s", err, strings.TrimSpace(string(output)))
	}
	switch choice := strings.TrimSpace(string(output)); choice {
	case keepServiceLabel:
		return keepService, nil
	case uninstallLabel:
		return uninstallService, nil
	case "":
		return cancelExit, nil
	default:
		return cancelExit, fmt.Errorf("无法识别退出选项 %s", strconv.Quote(choice))
	}
}
