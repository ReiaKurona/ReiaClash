// Command reia is the silent desktop entry point. Keep agent and tray console
// builds available for debugging. Windows builds use -H=windowsgui.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	branding "reia-clash/ico"
	"reia-clash/internal/platform/launch"
	"runtime"
	"slices"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if base, e := os.UserConfigDir(); e == nil {
			dir := filepath.Join(base, "ReiaClash")
			if os.MkdirAll(dir, 0700) == nil {
				_ = os.WriteFile(filepath.Join(dir, "launcher-error.log"), []byte(err.Error()), 0600)
			}
		}
		os.Exit(1)
	}
}

func run() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if err := branding.InstallDesktopEntries(); err != nil {
		fmt.Fprintln(os.Stderr, "ReiaClash 桌面图标:", err)
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	// The tray owns an atomic OS lock. Process-name scans cannot prevent races.
	var args []string
	if slices.Contains(os.Args[1:], "--background") {
		args = append(args, "--background")
	}
	if slices.Contains(os.Args[1:], "--login-autostart") {
		args = append(args, "--login-autostart")
	}
	return launch.StartBackground(filepath.Join(filepath.Dir(executable), "reia-tray"+suffix), args...)
}
