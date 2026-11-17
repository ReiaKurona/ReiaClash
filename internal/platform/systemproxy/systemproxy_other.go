//go:build !windows && (!linux || android) && !darwin

package systemproxy

import "fmt"

func Enabled() bool { return false }
func Set(enabled bool, backupFile string) error {
	return fmt.Errorf("当前平台尚未实现系统代理适配")
}
