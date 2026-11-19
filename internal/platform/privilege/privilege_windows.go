//go:build windows

package privilege

import (
	"fmt"
	"syscall"
)

var shell32 = syscall.NewLazyDLL("shell32.dll")
var isUserAnAdmin = shell32.NewProc("IsUserAnAdmin")

func RequireTunPrivilege() error {
	admin, _, _ := isUserAnAdmin.Call()
	if admin == 0 {
		return fmt.Errorf("当前进程没有 TUN 所需的 Windows 管理员权限")
	}
	return nil
}
