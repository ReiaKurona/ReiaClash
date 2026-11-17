//go:build darwin

package privilege

import (
	"fmt"
	"syscall"
)

func RequireTunPrivilege() error {
	if syscall.Geteuid() != 0 {
		return fmt.Errorf("TUN 需要 root 或 CAP_NET_ADMIN 系统网络权限；当前 ReiaClash 进程未获得该权限")
	}
	return nil
}
