package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"reia-clash/internal/config"
	"reia-clash/internal/platform/systemproxy"
)

type SettingsPatch struct {
	UseSystemProxy      *bool   `json:"use_system_proxy"`
	UseTun              *bool   `json:"use_tun"`
	TunStack            *string `json:"tun_stack"`
	AllowLAN            *bool   `json:"allow_lan"`
	AutoUpdateEnabled   *bool   `json:"auto_update_enabled"`
	UpdateIntervalHours *int    `json:"update_interval_hours"`
	OutboundMode        *string `json:"outbound_mode"`
}

// A failed proxy setting must not roll back a newer subscription received
// independently while it was applying to the kernel.
func restoreManagedPreferences(current *config.Settings, previous config.Settings) {
	previous.SubscriptionURL = current.SubscriptionURL
	previous.SubscriptionUpdatedAt = current.SubscriptionUpdatedAt
	*current = previous
}

type Status struct {
	IsActuallyIntercepting bool            `json:"is_actually_intercepting"`
	RealSystemProxy        bool            `json:"real_system_proxy"`
	RealTun                bool            `json:"real_tun"`
	MihomoRunning          bool            `json:"mihomo_running"`
	ActualMode             string          `json:"actual_mode,omitempty"`
	RealAllowLAN           bool            `json:"real_allow_lan"`
	LastError              string          `json:"last_error,omitempty"`
	Settings               config.Settings `json:"settings"`
}

func (a *App) SetEnabled(ctx context.Context, enabled bool) error {
	old := a.Prefs.Snapshot()
	if enabled && !old.UseSystemProxy && !old.UseTun {
		return fmt.Errorf("请先在设置中启用系统代理或 TUN，再启动流量接管")
	}
	if enabled && old.UseTun {
		if err := a.requireTunAuthority(ctx); err != nil {
			return err
		}
		a.Prefs.SetRuntimeTunAuthorization(true)
	}
	next, err := a.Prefs.Update(func(settings *config.Settings) error {
		settings.Enabled = enabled
		return nil
	})
	if err != nil {
		return err
	}
	if !old.UseTun {
		if err := a.applySystemProxy(ctx, next); err != nil {
			_, _ = a.Prefs.Update(func(settings *config.Settings) error { restoreManagedPreferences(settings, old); return nil })
			return err
		}
		return nil
	}
	if err := a.applySettings(ctx, next); err != nil {
		_, _ = a.Prefs.Update(func(settings *config.Settings) error { restoreManagedPreferences(settings, old); return nil })
		_ = a.applySettings(ctx, old)
		return err
	}
	return nil
}

func (a *App) UpdateSettings(ctx context.Context, patch SettingsPatch) error {
	old := a.Prefs.Snapshot()
	next, err := a.Prefs.Update(func(settings *config.Settings) error {
		if patch.UseSystemProxy != nil {
			settings.UseSystemProxy = *patch.UseSystemProxy
		}
		if patch.UseTun != nil {
			settings.UseTun = *patch.UseTun
		}
		if patch.TunStack != nil {
			if !config.ValidTunStack(*patch.TunStack) {
				return fmt.Errorf("不支持的 TUN 协议栈: %s", *patch.TunStack)
			}
			settings.TunStack = *patch.TunStack
		}
		if patch.AutoUpdateEnabled != nil {
			settings.AutoUpdateEnabled = *patch.AutoUpdateEnabled
		}
		if patch.UpdateIntervalHours != nil {
			if *patch.UpdateIntervalHours < 1 || *patch.UpdateIntervalHours > 168 {
				return fmt.Errorf("更新间隔必须是 1 到 168 小时")
			}
			settings.UpdateIntervalHours = *patch.UpdateIntervalHours
		}
		if patch.OutboundMode != nil {
			if *patch.OutboundMode != "rule" && *patch.OutboundMode != "global" && *patch.OutboundMode != "direct" {
				return fmt.Errorf("不支持的代理模式: %s", *patch.OutboundMode)
			}
			settings.OutboundMode = *patch.OutboundMode
		}
		if patch.AllowLAN != nil {
			settings.AllowLAN = *patch.AllowLAN
		}
		return nil
	})
	if err != nil {
		return err
	}
	if next.Enabled && patch.UseTun != nil && *patch.UseTun {
		if err := a.requireTunAuthority(ctx); err != nil {
			_, _ = a.Prefs.Update(func(settings *config.Settings) error { restoreManagedPreferences(settings, old); return nil })
			return err
		}
		a.Prefs.SetRuntimeTunAuthorization(true)
	}
	if patch.OutboundMode != nil || patch.AllowLAN != nil {
		if err := a.persistHotMode(ctx, old, next); err != nil {
			return err
		}
	}
	// 这些偏好不影响 mihomo YAML，因此不需要重启内核。
	if patch.UseTun == nil && patch.TunStack == nil {
		if patch.UseSystemProxy != nil {
			if err := a.applySystemProxy(ctx, next); err != nil {
				if _, restoreErr := a.Prefs.Update(func(settings *config.Settings) error { restoreManagedPreferences(settings, old); return nil }); restoreErr != nil {
					return fmt.Errorf("%w；恢复原设置失败: %v", err, restoreErr)
				}
				return err
			}
			return nil
		}
		return nil
	}
	// TUN preferences are dormant while interception is paused.
	if !next.Enabled && !old.Enabled {
		return nil
	}
	if err := a.applySettings(ctx, next); err != nil {
		_, _ = a.Prefs.Update(func(settings *config.Settings) error { restoreManagedPreferences(settings, old); return nil })
		_ = a.applySettings(ctx, old)
		return err
	}
	return nil
}

func (a *App) applySettings(ctx context.Context, settings config.Settings) error {
	proxyEnabled := settings.Enabled && settings.UseSystemProxy
	backupPath := filepath.Join(a.Paths.Root, "system-proxy-backup.json")
	if !proxyEnabled && (systemproxy.Enabled() || a.hasSystemProxyBackup()) {
		if err := systemproxy.Set(false, backupPath); err != nil {
			return err
		}
	}
	if err := a.Core.ReloadProfile(); err != nil {
		return err
	}
	// Restart is asynchronous. Do not report success against the old controller
	// that can still answer briefly before the supervisor replaces it.
	readyCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for {
		var actual struct {
			Mode     string `json:"mode"`
			AllowLAN bool   `json:"allow-lan"`
			Tun      struct {
				Enable bool `json:"enable"`
			} `json:"tun"`
		}
		err := a.Core.Client().GetJSON(readyCtx, "/configs", &actual)
		wantTun := settings.Enabled && settings.UseTun && a.Prefs.Snapshot().TunAuthorized
		if err == nil && actual.Tun.Enable == wantTun && actual.AllowLAN == settings.AllowLAN && actual.Mode == settings.OutboundMode {
			break
		}
		select {
		case <-readyCtx.Done():
			return fmt.Errorf("等待内核应用设置失败: %w", readyCtx.Err())
		case <-time.After(150 * time.Millisecond):
		}
	}
	if settings.OutboundMode == "global" {
		if err := a.ensureGlobalProxy(ctx); err != nil {
			return err
		}
	}
	if proxyEnabled {
		readyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		if err := a.Core.WaitReady(readyCtx); err != nil {
			return fmt.Errorf("mihomo 尚未就绪，系统代理未启用：%w", err)
		}
		if !systemproxy.Enabled() {
			if err := systemproxy.Set(true, backupPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *App) persistHotMode(ctx context.Context, old, next config.Settings) error {
	if err := a.Core.PersistCurrentRuntimeConfig(); err != nil {
		_, _ = a.Prefs.Update(func(settings *config.Settings) error { restoreManagedPreferences(settings, old); return nil })
		return err
	}
	readyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := a.Core.WaitReady(readyCtx); err != nil {
		_, _ = a.Prefs.Update(func(settings *config.Settings) error { restoreManagedPreferences(settings, old); return nil })
		_ = a.Core.PersistCurrentRuntimeConfig()
		return fmt.Errorf("切换设置失败，mihomo 未就绪: %w", err)
	}
	if next.OutboundMode == "global" {
		if err := a.ensureGlobalProxy(ctx); err != nil {
			_, _ = a.Prefs.Update(func(settings *config.Settings) error { restoreManagedPreferences(settings, old); return nil })
			_ = a.Core.PersistCurrentRuntimeConfig()
			return err
		}
	}
	if _, _, err := a.Core.Client().Do(ctx, "PATCH", "/configs", map[string]any{"mode": next.OutboundMode, "allow-lan": next.AllowLAN, "bind-address": runtimeBindAddress(next.AllowLAN)}); err != nil {
		_, _ = a.Prefs.Update(func(settings *config.Settings) error { restoreManagedPreferences(settings, old); return nil })
		_ = a.Core.PersistCurrentRuntimeConfig()
		_, _, _ = a.Core.Client().Do(ctx, "PATCH", "/configs", map[string]any{"mode": old.OutboundMode, "allow-lan": old.AllowLAN, "bind-address": runtimeBindAddress(old.AllowLAN)})
		return err
	}
	// 模式切换后强制断开旧连接
	_ = a.Core.Client().CloseConnections(ctx)
	return nil
}

// Mihomo defaults GLOBAL to DIRECT. Pick a usable policy group when entering
// global mode, otherwise switching the mode alone appears to do nothing.
func (a *App) ensureGlobalProxy(ctx context.Context) error {
	var result struct {
		Proxies map[string]struct {
			Type string   `json:"type"`
			Now  string   `json:"now"`
			All  []string `json:"all"`
		} `json:"proxies"`
	}
	if err := a.Core.Client().GetJSON(ctx, "/proxies", &result); err != nil {
		return err
	}
	global, ok := result.Proxies["GLOBAL"]
	if !ok || global.Now != "DIRECT" {
		return nil
	}
	for _, name := range global.All {
		proxy := result.Proxies[name]
		if name == "GLOBAL" || name == "DIRECT" || name == "REJECT" || proxy.Type == "Direct" || proxy.Type == "Reject" || proxy.Type == "Compatible" {
			continue
		}
		_, _, err := a.Core.Client().Do(ctx, "PUT", "/proxies/GLOBAL", map[string]string{"name": name})
		return err
	}
	return nil
}

func runtimeBindAddress(allowLAN bool) string {
	if allowLAN {
		return "*"
	}
	return "127.0.0.1"
}

func (a *App) applySystemProxy(ctx context.Context, settings config.Settings) error {
	backupPath := filepath.Join(a.Paths.Root, "system-proxy-backup.json")
	if !settings.Enabled || !settings.UseSystemProxy {
		if systemproxy.Enabled() || a.hasSystemProxyBackup() {
			return systemproxy.Set(false, backupPath)
		}
		return nil
	}
	if systemproxy.Enabled() {
		return nil
	}
	readyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := a.Core.WaitReady(readyCtx); err != nil {
		return fmt.Errorf("mihomo 尚未就绪，系统代理未启用：%w", err)
	}
	return systemproxy.Set(true, backupPath)
}
