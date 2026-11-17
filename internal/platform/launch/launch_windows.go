//go:build windows

package launch

import (
	"io"
	"os/exec"
	"strings"
	"syscall"
)

const createNoWindow = 0x08000000

func HideCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

func configureBackground(command *exec.Cmd) { HideCommand(command) }

// StartBackground 隐藏由托盘启动的 agent 子进程，避免 Windows 创建控制台窗口。
func StartBackground(executable string, args ...string) error {
	command := exec.Command(executable, args...)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}

// OpenURL 在默认浏览器中打开指定网址
func OpenURL(targetURL string) error {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
	return cmd.Start()
}

// IsProcessRunning 判断指定进程名是否已在运行
func IsProcessRunning(name string) bool {
	out, err := Command("tasklist", "/fi", "imagename eq "+name, "/fo", "csv", "/nh").Output()
	if err != nil {
		return false
	}
	s := string(out)
	return len(s) > 0 && !strings.Contains(s, "No tasks") && !strings.Contains(s, "没有运行的任务") && strings.Contains(strings.ToLower(s), strings.ToLower(name))
}
