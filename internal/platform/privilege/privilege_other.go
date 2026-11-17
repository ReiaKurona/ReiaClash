//go:build !windows && (!linux || android) && !darwin

package privilege

import "fmt"

func RequireTunPrivilege() error { return fmt.Errorf("当前平台尚未实现 TUN 权限适配") }
