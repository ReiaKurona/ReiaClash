package coreservice

import (
	"fmt"
	"os"
)

// Only user-owned paths may be tightened. A system-installed executable is
// also valid when root owns it and neither group nor others can replace it.
func servicePathMode(path string, mode os.FileMode, owner, uid int, directory, executable bool) (os.FileMode, error) {
	if directory && !mode.IsDir() || !directory && !mode.IsRegular() {
		return 0, fmt.Errorf("路径 %q 类型不正确", path)
	}
	if owner != uid && !(executable && owner == 0) {
		return 0, fmt.Errorf("路径 %q 属于 UID %d，服务用户为 UID %d；请检查该路径的所有者", path, owner, uid)
	}
	if owner != uid && mode.Perm()&0022 != 0 {
		return 0, fmt.Errorf("系统程序 %q 权限为 %04o，不能允许组或其他用户写入", path, mode.Perm())
	}
	required := os.FileMode(0400)
	if directory {
		required = 0700
	} else if executable {
		required = 0500
	}
	if owner == uid && mode.Perm()&required != required {
		return 0, fmt.Errorf("路径 %q 权限为 %04o，当前用户缺少所需的读写或执行权限", path, mode.Perm())
	}
	return mode.Perm() &^ 0022, nil
}
