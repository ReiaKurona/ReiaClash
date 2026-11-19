package coreservice

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"reia-clash/internal/platform/launch"
)

// Privileged commands and the long-lived service have a distinct process role.
func Executable() (string, error) {
	current, err := os.Executable()
	if err != nil {
		return "", err
	}
	current, err = launch.ResolveExecutable(current)
	if err != nil {
		return "", err
	}
	name := "reia-service"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	target := filepath.Join(filepath.Dir(current), name)
	info, err := os.Stat(target)
	if err != nil {
		return "", fmt.Errorf("找不到内核服务程序 %s，请重新构建客户端: %w", target, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("内核服务路径不是普通文件")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0 {
		return "", fmt.Errorf("内核服务程序缺少执行权限")
	}
	return target, nil
}
