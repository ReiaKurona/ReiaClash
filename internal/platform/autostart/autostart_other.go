//go:build !windows && (!linux || android) && !darwin

package autostart

import "fmt"

func Set(enabled bool, executable string) error {
	return fmt.Errorf("当前系统暂不支持桌面开机启动")
}

func Enabled(executable string) bool { return false }
