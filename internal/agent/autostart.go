package agent

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reia-clash/internal/platform/network"
	"reia-clash/internal/platform/systemproxy"
	"runtime"
	"time"

	"reia-clash/internal/config"
	"reia-clash/internal/platform/autostart"
)

func (a *App) SetAutoStart(enabled bool) error {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	previous := autostart.Enabled("")
	executable, err := trayExecutable()
	if err != nil {
		return err
	}
	if err := autostart.Set(enabled, executable); err != nil {
		return err
	}
	_, err = a.Prefs.Update(func(settings *config.Settings) error {
		settings.AutoStart = enabled
		if !enabled {
			settings.SilentProxyOnLogin = false
		}
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

func (a *App) SetSilentProxyOnLogin(enabled bool) error {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	wasRegistered := autostart.Enabled("")
	var executable string
	if enabled {
		var err error
		executable, err = trayExecutable()
		if err != nil {
			return err
		}
		if err := autostart.Set(true, executable); err != nil {
			return err
		}
	}
	_, err := a.Prefs.Update(func(settings *config.Settings) error {
		settings.SilentProxyOnLogin = enabled
		if enabled {
			settings.AutoStart = true
		}
		return nil
	})
	if err != nil && enabled && !wasRegistered {
		return errors.Join(err, autostart.Set(false, executable))
	}
	return err
}

func (a *App) StartLoginProxy(parent context.Context) {
	if !a.Prefs.Snapshot().SilentProxyOnLogin || !autostart.Enabled("") {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		if a.Core.RunningBinary() != "" && loginNetworkReady(ctx) {
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(2 * time.Second):
		}
	}
	if ctx.Err() != nil {
		if parent.Err() == nil {
			a.AddEvent("error", "登录静默代理仍处于暂停：网络、DNS 或内核尚未就绪")
		}
		return
	}
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	if a.Core.UsesExternalLifecycle() {
		if err := a.Prefs.Reload(); err != nil {
			a.AddEvent("error", "读取登录代理状态失败："+err.Error())
			return
		}
	}
	if a.proxyIntent || a.Prefs.Snapshot().Enabled || !a.Prefs.Snapshot().SilentProxyOnLogin || !autostart.Enabled("") {
		return
	}
	// Allow the existing OS authorization dialog to finish with the user's consent.
	if err := a.setEnabled(parent, true); err != nil {
		a.AddEvent("error", "登录静默代理未启动："+err.Error())
		return
	}
	ready := loginNetworkReady(parent)
	if ready && a.Prefs.Snapshot().UseSystemProxy {
		_, err := a.OutboundIP(parent)
		ready = err == nil
	}
	if !ready && parent.Err() == nil {
		if err := a.setEnabled(parent, false); err != nil {
			if recovery := a.pauseFailedLoginProxy(parent); recovery != nil {
				a.AddEvent("error", "登录代理联网验证失败，自动暂停失败："+errors.Join(err, recovery).Error())
			} else {
				a.AddEvent("info", "登录代理联网验证失败，已停止接管并恢复暂停状态")
			}
		} else {
			a.AddEvent("error", "登录代理联网验证失败，已暂停接管")
		}
	}
}

func loginNetworkReady(ctx context.Context) bool {
	connection, err := net.DialTimeout("tcp", "127.0.0.1:7890", 500*time.Millisecond)
	if err != nil {
		return false
	}
	connection.Close()
	if !network.Online(ctx) {
		return false
	}
	// A direct IP probe alone does not detect a stale local DNS override.
	dns, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err = net.DefaultResolver.LookupHost(dns, "next.rka.jp")
	return err == nil
}

// Called with serviceMu held if hot TUN shutdown could not be verified.
func (a *App) pauseFailedLoginProxy(ctx context.Context) error {
	_, saveErr := a.Prefs.Update(func(s *config.Settings) error { s.Enabled = false; return nil })
	proxyErr := a.PrepareExit()
	if a.Core.UsesExternalLifecycle() && a.hasSystemProxyBackup() {
		proxyErr = systemproxy.Set(false, filepath.Join(a.Paths.Root, "system-proxy-backup.json"))
	}
	stopErr := a.Core.Shutdown()
	if err := errors.Join(saveErr, stopErr); err != nil {
		return errors.Join(err, proxyErr)
	}
	if err := a.Core.PersistCurrentRuntimeConfig(); err != nil {
		return errors.Join(err, proxyErr)
	}
	return errors.Join(proxyErr, a.Core.Start(ctx))
}
