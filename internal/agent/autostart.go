package agent

import (
	"os"
	"path/filepath"
	"runtime"

	"reia-clash/internal/config"
	"reia-clash/internal/platform/autostart"
)

func (a *App) SetAutoStart(enabled bool) error {
	previous := a.Prefs.Snapshot().AutoStart
	executable, err := trayExecutable()
	if err != nil {
		return err
	}
	if err := autostart.Set(enabled, executable); err != nil {
		return err
	}
	_, err = a.Prefs.Update(func(settings *config.Settings) error {
		settings.AutoStart = enabled
		return nil
	})
	if err != nil {
		_ = autostart.Set(previous, executable)
		return err
	}
	if enabled {
		a.AddEvent("info", "ReiaClash 已注册开机启动")
	} else {
		a.AddEvent("info", "ReiaClash 已取消开机启动")
	}
	return nil
}

func trayExecutable() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	launcherName := "reia"
	if runtime.GOOS == "windows" {
		launcherName += ".exe"
	}
	launcherPath := filepath.Join(filepath.Dir(executable), launcherName)
	if info, err := os.Stat(launcherPath); err == nil && !info.IsDir() {
		return launcherPath, nil
	}
	name := "reia-tray"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	trayPath := filepath.Join(filepath.Dir(executable), name)
	if _, err := os.Stat(trayPath); err == nil {
		return trayPath, nil
	}
	return executable, nil
}
