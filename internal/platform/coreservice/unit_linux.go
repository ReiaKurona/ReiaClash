//go:build linux && !android

package coreservice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const managedUnitMarker = "# Managed by ReiaClash. Do not edit.\n"

func installUnit(executable, dataDir string, uid, gid int) error {
	previous, readErr := os.ReadFile(unitPath)
	hadPrevious := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		return readErr
	}
	if hadPrevious && !strings.HasPrefix(string(previous), managedUnitMarker) {
		return fmt.Errorf("%s 已存在且不是由 ReiaClash 管理，拒绝覆盖", unitPath)
	}
	if hadPrevious {
		if err := disableAndStopUnit(); err != nil {
			return err
		}
	}
	restorePrevious := func() error {
		if hadPrevious {
			if err := writeManagedUnit(strings.TrimPrefix(string(previous), managedUnitMarker)); err != nil {
				return err
			}
		} else if err := removeManagedUnit(); err != nil {
			return err
		}
		if err := runSystemctl("daemon-reload"); err != nil {
			return err
		}
		if hadPrevious {
			return runSystemctl("enable", "--now", unitName)
		}
		return nil
	}
	if err := writeManagedUnit(renderUnit(executable, dataDir, uid, gid)); err != nil {
		return errors.Join(err, restorePrevious())
	}
	if err := runSystemctl("daemon-reload"); err != nil {
		return errors.Join(err, restorePrevious())
	}
	if err := runSystemctl("enable", "--now", unitName); err != nil {
		_ = disableAndStopUnit()
		return errors.Join(err, restorePrevious())
	}
	return nil
}

func uninstallUnit() error {
	if err := requireManagedUnit(); err != nil {
		if os.IsNotExist(err) {
			// A deleted unit file can still have a live process in systemd.
			registration := RegistrationStatus()
			if registration.Error != "" {
				return fmt.Errorf("无法确认旧服务状态: %s", registration.Error)
			}
			if registration.Running || registration.Installed {
				return runSystemctl("stop", unitName)
			}
			return nil
		}
		return err
	}
	if err := disableAndStopUnit(); err != nil {
		return err
	}
	if err := removeManagedUnit(); err != nil {
		return err
	}
	return runSystemctl("daemon-reload")
}

func writeManagedUnit(contents string) error {
	if err := requireManagedUnit(); err != nil && !os.IsNotExist(err) {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(unitPath), ".reia-clash-core-*.tmp")
	if err != nil {
		return fmt.Errorf("创建 systemd 服务文件失败：%w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.WriteString(managedUnitMarker + contents); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Chmod(0644); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, unitPath)
}

func requireManagedUnit() error {
	contents, err := os.ReadFile(unitPath)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(string(contents), managedUnitMarker) {
		return fmt.Errorf("%s 已存在且不是由 ReiaClash 管理，拒绝覆盖", unitPath)
	}
	return nil
}

func hasControlCharacters(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}

func removeManagedUnit() error {
	if err := requireManagedUnit(); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.Remove(unitPath)
}

func runSystemctl(args ...string) error {
	output, err := systemctlOutput(args...)
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("systemctl %s 失败：%s", strings.Join(args, " "), message)
	}
	return nil
}

func disableAndStopUnit() error {
	return disableAndStopSystemdUnit(unitName, runSystemctl, systemctlOutput)
}

func systemctlOutput(args ...string) ([]byte, error) {
	var systemctl string
	for _, candidate := range []string{"/usr/bin/systemctl", "/bin/systemctl"} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			systemctl = candidate
			break
		}
	}
	if systemctl == "" {
		return nil, fmt.Errorf("找不到 systemctl；当前系统可能没有运行 systemd")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, systemctl, args...)
	return command.CombinedOutput()
}
