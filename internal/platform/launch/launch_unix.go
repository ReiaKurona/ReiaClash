//go:build (linux || darwin) && !android

package launch

import (
	"io"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
)

func HideCommand(command *exec.Cmd) {}

func configureBackground(command *exec.Cmd) { command.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

// StartBackground 将子进程放入独立会话并丢弃输出，托盘启动时不会弹出控制台窗口。
func StartBackground(executable string, args ...string) error {
	command := exec.Command(executable, args...)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}

// OpenURL 在默认浏览器中打开指定网址
func OpenURL(targetURL string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", targetURL).Start()
	}
	return exec.Command("xdg-open", targetURL).Start()
}

// IsProcessRunning 判断指定进程名是否已在运行
func IsProcessRunning(name string) bool {
	out, err := exec.Command("pgrep", "-f", name).Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}
