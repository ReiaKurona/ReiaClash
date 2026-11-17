//go:build (linux || darwin) && !android

package kernelbinary

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"golang.org/x/sys/unix"
)

// An unpacked native binary often loses its executable bit when transferred
// from Windows. Repair only this user's file, without granting write access.
func prepareExecutable(path string) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	fd, err := unix.Open(resolved, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return fmt.Errorf("无法读取内核 %q: %w", path, err)
	}
	file := os.NewFile(uintptr(fd), resolved)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || int(stat.Uid) != os.Getuid() {
		return fmt.Errorf("内核 %q 没有执行权限，且不属于当前用户；请由所有者设置 chmod u+x", path)
	}
	var magic [4]byte
	if _, err := io.ReadFull(file, magic[:]); err != nil {
		return fmt.Errorf("内核 %q 无效: %w", path, err)
	}
	native := runtime.GOOS == "linux" && magic == [4]byte{0x7f, 'E', 'L', 'F'}
	if runtime.GOOS == "darwin" {
		for _, prefix := range [][4]byte{{0xfe, 0xed, 0xfa, 0xce}, {0xce, 0xfa, 0xed, 0xfe}, {0xfe, 0xed, 0xfa, 0xcf}, {0xcf, 0xfa, 0xed, 0xfe}, {0xca, 0xfe, 0xba, 0xbe}, {0xbe, 0xba, 0xfe, 0xca}} {
			native = native || magic == prefix
		}
	}
	if !native {
		return fmt.Errorf("内核 %q 不是当前系统的原生二进制，不自动授予执行权限", path)
	}
	if err := file.Chmod(info.Mode().Perm() | 0100); err != nil {
		return fmt.Errorf("无法为内核 %q 设置用户执行权限: %w", path, err)
	}
	return nil
}
