//go:build linux && !android

package privilege

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

const capNetAdmin = 12

func RequireTunPrivilege() error {
	if syscall.Geteuid() == 0 || effectiveCapability(capNetAdmin) {
		return nil
	}
	return fmt.Errorf("TUN 需要 root 或 CAP_NET_ADMIN 系统网络权限；当前 ReiaClash 进程未获得该权限")
}

// Linux 可以通过文件或服务 capability 向非 root 进程授予网络管理权限，
// 因此同时检查 CapEff，不能只看当前 UID 是否为 0。
func effectiveCapability(bit uint) bool {
	file, err := os.Open("/proc/self/status")
	if err != nil {
		return false
	}
	defer file.Close()
	for scanner := bufio.NewScanner(file); scanner.Scan(); {
		name, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok || name != "CapEff" {
			continue
		}
		mask, err := strconv.ParseUint(strings.TrimSpace(value), 16, 64)
		return err == nil && mask&(uint64(1)<<bit) != 0
	}
	return false
}
