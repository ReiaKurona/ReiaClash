//go:build darwin

package systemproxy

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// runAuthorizedNetworksetup 通过 macOS 原生授权对话框请求管理员权限，
// 一次性执行整组代理设置，并在任一命令失败时恢复之前的状态。
func runAuthorizedNetworksetup(changes, rollback [][]string) error {
	osascript := "/usr/bin/osascript"
	if err := requireExecutable(osascript); err != nil {
		return fmt.Errorf("找不到 macOS 系统授权工具 osascript: %w", err)
	}
	networksetup, err := networksetupPath()
	if err != nil {
		return fmt.Errorf("找不到 macOS 网络设置工具 networksetup: %w", err)
	}

	appleScript := authorizedShellScript(networksetupTransaction(networksetup, changes, rollback))
	output, err := exec.Command(osascript, "-e", appleScript).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("macOS 管理员授权或网络设置失败: %s", message)
	}
	return nil
}

// authorizedShellScript 把 shell 脚本拆成 AppleScript 文本列表，避免使用
// 临时脚本文件：管理员授权时执行的内容不会被本机其他进程替换。
func authorizedShellScript(shellScript string) string {
	lines := strings.Split(strings.TrimSuffix(shellScript, "\n"), "\n")
	quotedLines := make([]string, 0, len(lines))
	for _, line := range lines {
		quotedLines = append(quotedLines, appleScriptQuote(line))
	}
	return strings.Join([]string{
		"set oldDelimiters to AppleScript's text item delimiters",
		"set AppleScript's text item delimiters to linefeed",
		"set shellBody to {" + strings.Join(quotedLines, ", ") + "} as text",
		"set AppleScript's text item delimiters to oldDelimiters",
		"do shell script (\"/bin/sh -c \" & quoted form of shellBody) with administrator privileges",
	}, "\n")
}

// networksetupTransaction 在批量设置中途失败时运行整组回滚命令。
func networksetupTransaction(networksetup string, changes, rollback [][]string) string {
	var script strings.Builder
	script.WriteString("#!/bin/sh\nset -e\n")
	if len(rollback) > 0 {
		script.WriteString("cleanup() {\n")
		script.WriteString("  result=$?\n  trap - EXIT\n")
		script.WriteString("  if [ \"$result\" -ne 0 ]; then\n    set +e\n")
		for _, command := range rollback {
			script.WriteString("    ")
			script.WriteString(networksetupCommand(networksetup, command))
			script.WriteString(" || true\n")
		}
		script.WriteString("  fi\n  exit \"$result\"\n}\ntrap cleanup EXIT\n")
	}
	for _, command := range changes {
		script.WriteString(networksetupCommand(networksetup, command))
		script.WriteByte('\n')
	}
	if len(rollback) > 0 {
		script.WriteString("trap - EXIT\n")
	}
	return script.String()
}

// networksetupCommand 将系统工具和每个参数分别做 shell 转义。
func networksetupCommand(networksetup string, arguments []string) string {
	var command strings.Builder
	command.WriteString(shellQuote(networksetup))
	for _, argument := range arguments {
		command.WriteByte(' ')
		command.WriteString(shellQuote(argument))
	}
	return command.String()
}

// shellQuote 转义一个 shell 参数，避免网络服务名称被当成命令执行。
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// appleScriptQuote 生成 AppleScript 双引号字符串字面量。
func appleScriptQuote(value string) string {
	value = strings.NewReplacer("\\", "\\\\", `"`, `\"`, "\r", "\\r", "\n", "\\n").Replace(value)
	return `"` + value + `"`
}

func networksetupPath() (string, error) {
	path := "/usr/sbin/networksetup"
	if err := requireExecutable(path); err != nil {
		return "", fmt.Errorf("找不到 macOS 网络设置工具 networksetup: %w", err)
	}
	return path, nil
}

func requireExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return fmt.Errorf("%s 不是可执行文件", path)
	}
	return nil
}
