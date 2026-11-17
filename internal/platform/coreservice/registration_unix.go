//go:build !windows

package coreservice

import (
	"context"
	"os"
	"reia-clash/internal/platform/launch"
	"runtime"
	"strings"
	"time"
)

func RegistrationStatus() Registration {
	result := Registration{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	switch runtime.GOOS {
	case "linux":
		result.Backend, result.Name = "systemd", "reia-clash-core.service"
		output, err := launch.CommandContext(ctx, "systemctl", "show", result.Name, "--property=LoadState,ActiveState,FragmentPath,LoadError").CombinedOutput()
		result = systemdRegistration(string(output), err)
	case "darwin":
		result.Backend, result.Name = "launchd", "com.reiaclash.core"
		_, err := os.Stat("/Library/LaunchDaemons/" + result.Name + ".plist")
		result.Installed = err == nil
		if err != nil && !os.IsNotExist(err) {
			result.Error = err.Error()
		}
		if result.Installed {
			output, err := launch.CommandContext(ctx, "launchctl", "print", "system/"+result.Name).CombinedOutput()
			result.Running = err == nil && strings.Contains(string(output), "state = running")
			if err != nil {
				result.Error = err.Error()
			}
		}
	default:
		result.Backend = "unsupported"
	}
	return result
}
