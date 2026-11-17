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
		return fmt.Errorf("TUN 需要 Windows 管理员权限；请以管理员身份重新启动 ReiaClash 后重试")
	}
	return nil
}
