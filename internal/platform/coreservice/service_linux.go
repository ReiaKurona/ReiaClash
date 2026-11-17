//go:build linux && !android

package coreservice

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const (
	unitName = "reia-clash-core.service"
	unitPath = "/etc/systemd/system/" + unitName
)

// Supported 表示当前 Linux 构建包含 systemd 服务安装器。
func Supported() bool { return true }

// RequestInstall 调用系统 polkit 授权提示，并安装以当前用户身份运行的 helper。
// helper 只获得 CAP_NET_ADMIN 权限。
func RequestInstall(ctx context.Context, dataDir string) error {
	executable, err := Executable()
	if err != nil {
		return err
	}
	// Run as the desktop user, before polkit. Only tighten these two owned
	// paths; never recursively chmod/chown the installation or user profile.
	if err := prepareServicePaths(executable, dataDir); err != nil {
		return fmt.Errorf("安装内核服务前检查失败：%w", err)
	}
	return requestPolkit(ctx, "--install-core-service", "--data-dir", dataDir, "--service-uid", strconv.Itoa(os.Getuid()))
}

// RequestUninstall 请求 polkit 授权并移除 ReiaClash 系统服务。
func RequestUninstall(ctx context.Context, dataDir string) error {
	return requestPolkit(ctx, "--uninstall-core-service", "--data-dir", dataDir, "--service-uid", strconv.Itoa(os.Getuid()))
}

func requestPolkit(ctx context.Context, args ...string) error {
	pkexec, err := exec.LookPath("pkexec")
	if err != nil {
		return fmt.Errorf("找不到 pkexec；请安装 polkit 后重试")
	}
	executable, err := Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}
	installing := false
	for _, argument := range args {
		if argument == "--install-core-service" {
			installing = true
			break
		}
	}
	if installing && isTemporaryPath(executable) {
		return fmt.Errorf("agent 当前运行在临时目录；请从固定安装目录启动后再注册内核服务")
	}
	commandArgs := append([]string{executable}, args...)
	callCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	command := exec.CommandContext(callCtx, pkexec, commandArgs...)
	output, err := command.CombinedOutput()
	// pkexec's root process must not create root-owned diagnostics in the
	// desktop profile. Persist its stderr here, under the caller's identity.
	var dataDir string
	for i, arg := range args {
		if arg == "--data-dir" && i+1 < len(args) {
			dataDir = args[i+1]
			break
		}
	}
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		_ = writeOperationDiagnostic(dataDir, message)
		return fmt.Errorf("特权服务操作失败：%s", message)
	}
	_ = writeOperationDiagnostic(dataDir, "")
	return nil
}

func writeOperationDiagnostic(dataDir, message string) error {
	file, err := os.CreateTemp(dataDir, ".core-service-operation-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.WriteString(message); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(dataDir, "core-service-operation.log"))
}

func prepareServicePaths(executable, dataDir string) error {
	resolvedExecutable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return fmt.Errorf("无法访问服务程序 %q: %w", executable, err)
	}
	for _, path := range []struct {
		name                  string
		directory, executable bool
	}{
		{executable, false, true},
		{dataDir, true, false},
		{filepath.Join(dataDir, "core-service.token"), false, false},
	} {
		resolved, err := filepath.EvalSymlinks(path.name)
		if err != nil {
			return fmt.Errorf("无法访问路径 %q: %w", path.name, err)
		}
		// Open and fstat/fchmod the same object; do not follow a replacement
		// symlink or block on a pipe while checking an executable/token.
		fd, err := unix.Open(resolved, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if err != nil {
			return fmt.Errorf("无法读取路径 %q: %w", resolved, err)
		}
		file := os.NewFile(uintptr(fd), resolved)
		info, statErr := file.Stat()
		if statErr == nil {
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok {
				statErr = fmt.Errorf("无法读取路径 %q 的所有者", resolved)
			} else {
				var mode os.FileMode
				mode, statErr = servicePathMode(resolved, info.Mode(), int(stat.Uid), os.Getuid(), path.directory, path.executable)
				if statErr == nil && mode != info.Mode().Perm() {
					statErr = file.Chmod(mode)
				}
			}
		}
		_ = file.Close()
		if statErr != nil {
			return fmt.Errorf("检查路径 %q 失败: %w", resolved, statErr)
		}
		if path.executable {
			if err := unix.Access(resolved, unix.X_OK); err != nil {
				return fmt.Errorf("无法执行服务程序 %q: %w", resolved, err)
			}
		}
	}
	// Probe writes as the service's actual UID, not as the root installer.
	if err := probeServiceDirectory(dataDir); err != nil {
		return err
	}
	// Geo/cache stays beside the application, even when the selected kernel
	// lives in updates. Polkit does not grant the service extra file access.
	kernelData := filepath.Join(filepath.Dir(resolvedExecutable), "data")
	if err := os.MkdirAll(kernelData, 0700); err != nil {
		return fmt.Errorf("服务用户无法创建应用数据目录 %q: %w", kernelData, err)
	}
	return probeServiceDirectory(kernelData)
}

func probeServiceDirectory(directory string) error {
	probe, err := os.CreateTemp(directory, ".service-access-*")
	if err != nil {
		return fmt.Errorf("服务用户无法写入目录 %q: %w", directory, err)
	}
	_ = probe.Close()
	return os.Remove(probe.Name())
}

func isTemporaryPath(path string) bool {
	relative, err := filepath.Rel(os.TempDir(), path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// InstallPrivileged 只由 pkexec 启动的特权进程调用。
func InstallPrivileged(dataDir, requestedUID string) error {
	uid, err := authorizePolkitUser(requestedUID)
	if err != nil {
		return err
	}
	executable, err := Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}
	dataDir, err = filepath.Abs(dataDir)
	if err != nil {
		return err
	}
	dataDir, err = filepath.EvalSymlinks(dataDir)
	if err != nil {
		return err
	}
	if hasControlCharacters(dataDir) || hasControlCharacters(executable) {
		return fmt.Errorf("agent 或数据目录路径不能包含控制字符")
	}
	if err := checkOwnedPath(dataDir, uid, true, false); err != nil {
		return fmt.Errorf("ReiaClash 数据目录校验失败：%w", err)
	}
	if err := checkOwnedPath(executable, uid, false, true); err != nil {
		return fmt.Errorf("ReiaClash 服务程序校验失败：%w", err)
	}
	serviceUser, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return fmt.Errorf("读取当前用户信息失败：%w", err)
	}
	groupID, err := strconv.Atoi(serviceUser.Gid)
	if err != nil {
		return fmt.Errorf("当前用户的主组编号无效")
	}
	if err := checkOwnedPath(filepath.Join(dataDir, "core-service.token"), uid, false, false); err != nil {
		return fmt.Errorf("服务控制密钥尚未准备好：%w", err)
	}
	if err := installUnit(executable, dataDir, uid, groupID); err != nil {
		return err
	}
	return nil
}

// UninstallPrivileged 只移除由 ReiaClash 管理的 systemd 单元。
func UninstallPrivileged(dataDir, requestedUID string) error {
	if _, err := authorizePolkitUser(requestedUID); err != nil {
		return err
	}
	return uninstallUnit()
}

func authorizePolkitUser(requestedUID string) (int, error) {
	uid, err := strconv.Atoi(requestedUID)
	if err != nil || uid < 0 {
		return 0, fmt.Errorf("当前用户编号无效")
	}
	if os.Geteuid() != 0 {
		return 0, fmt.Errorf("服务安装命令需要通过 polkit 获得管理员授权")
	}
	polkitUID, err := strconv.Atoi(os.Getenv("PKEXEC_UID"))
	if err != nil || polkitUID != uid {
		return 0, fmt.Errorf("授权用户与服务所属用户不一致")
	}
	return uid, nil
}

func checkOwnedPath(path string, uid int, directory, executable bool) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("无法读取路径 %q 的所有者", path)
	}
	if _, err := servicePathMode(path, info.Mode(), int(stat.Uid), uid, directory, executable); err != nil {
		return err
	}
	if info.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("路径 %q 权限为 %04o，不能允许组或其他用户写入；请从普通用户客户端重试", path, info.Mode().Perm())
	}
	return nil
}
