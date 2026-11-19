package agent

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"reia-clash/internal/config"
	"reia-clash/internal/platform/systemproxy"
)

type SettingsPatch struct {
	UseSystemProxy      *bool   `json:"use_system_proxy"`
	UseTun              *bool   `json:"use_tun"`
	TunStack            *string `json:"tun_stack"`
	TunDevice           *string `json:"tun_device"`
	AllowLAN            *bool   `json:"allow_lan"`
	AutoUpdateEnabled   *bool   `json:"auto_update_enabled"`
	UpdateIntervalHours *int    `json:"update_interval_hours"`
	OutboundMode        *string `json:"outbound_mode"`
}

// A failed proxy setting must not roll back a newer subscription received
// independently while it was applying to the kernel.
func restoreManagedPreferences(current *config.Settings, previous config.Settings) {
	current.Enabled, current.UseSystemProxy, current.UseTun = previous.Enabled, previous.UseSystemProxy, previous.UseTun
	current.TunStack, current.TunDevice, current.AllowLAN = previous.TunStack, previous.TunDevice, previous.AllowLAN
	current.OutboundMode = previous.OutboundMode
	current.AutoUpdateEnabled, current.UpdateIntervalHours = previous.AutoUpdateEnabled, previous.UpdateIntervalHours
}

type Status struct {
	IsActuallyIntercepting bool            `json:"is_actually_intercepting"`
	RealSystemProxy        bool            `json:"real_system_proxy"`
	RealTun                bool            `json:"real_tun"`
	RealTunDevice          string          `json:"real_tun_device,omitempty"`
	MihomoRunning          bool            `json:"mihomo_running"`
	ActualMode             string          `json:"actual_mode,omitempty"`
	RealAllowLAN           bool            `json:"real_allow_lan"`
	LastError              string          `json:"last_error,omitempty"`
	Settings               config.Settings `json:"settings"`
}

var ErrInvalidTunDevice = errors.New("TUN 网卡名无效：Windows/Linux 使用 Reia 开头的 4–15 位字母、数字、连字符或下划线，macOS 使用 utun 加编号，留空自动选择")

func (a *App) SetEnabled(ctx context.Context, enabled bool) error {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()

	return a.changeSettings(ctx, enabled, func(settings *config.Settings) error {
		if enabled && !settings.UseSystemProxy && !settings.UseTun {
			return fmt.Errorf("请先在设置中启用系统代理或 TUN，再启动流量接管")
		}
		settings.Enabled = enabled
		return nil
	})
}

func (a *App) UpdateSettings(ctx context.Context, patch SettingsPatch) error {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()

	return a.changeSettings(ctx, patch.UseTun != nil && *patch.UseTun, func(settings *config.Settings) error {
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
		if patch.TunDevice != nil {
			if !config.ValidTunDevice(*patch.TunDevice) {
				return ErrInvalidTunDevice
			}
			settings.TunDevice = *patch.TunDevice
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
}

func (a *App) changeSettings(ctx context.Context, requestTun bool, change func(*config.Settings) error) error {
	if a.exiting {
		return fmt.Errorf("ReiaClash 正在退出")
	}
	old := a.Prefs.Snapshot()
	next := old
	if err := change(&next); err != nil {
		return err
	}
	authorizeTun := requestTun && next.Enabled && next.UseTun && !old.TunAuthorized
	if next == old && !authorizeTun {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Complete service handover before saving an enabled TUN preference.
	if authorizeTun {
		if err := a.requireTunAuthority(ctx); err != nil {
			return err
		}
		a.Prefs.SetRuntimeTunAuthorization(true)
	}
	next, err := a.Prefs.Update(change)
	if err != nil {
		return err
	}

	previousTun, targetTun := tunEnabled(old), tunEnabled(next)
	modeChanged := old.OutboundMode != next.OutboundMode
	lanChanged := old.AllowLAN != next.AllowLAN
	tunPreferencesChanged := old.TunStack != next.TunStack || old.TunDevice != next.TunDevice
	tunChanged := previousTun != targetTun || (targetTun && tunPreferencesChanged)
	persistConfig := modeChanged || lanChanged || tunChanged || tunPreferencesChanged
	proxyChanged := (old.Enabled && old.UseSystemProxy) != (next.Enabled && next.UseSystemProxy)
	patchAttempted, proxyAttempted := false, false
	rollback := func(cause error) error {
		_, settingsErr := a.Prefs.Update(func(current *config.Settings) error {
			restoreManagedPreferences(current, old)
			return nil
		})
		if settingsErr != nil {
			return errors.Join(cause, settingsErr)
		}
		if persistConfig {
			cause = errors.Join(cause, a.Core.PersistCurrentRuntimeConfig())
		}
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if patchAttempted {
			cause = errors.Join(cause, a.patchCoreSettings(recovery, old, modeChanged, lanChanged, tunChanged))
		}
		if proxyAttempted {
			cause = errors.Join(cause, a.applySystemProxy(recovery, old))
		}
		return cause
	}
	if persistConfig {
		if err := a.Core.PersistCurrentRuntimeConfig(); err != nil {
			return rollback(err)
		}
	}
	if proxyChanged && !(next.UseSystemProxy && next.Enabled) {
		proxyAttempted = true
		if err := a.applySystemProxy(ctx, next); err != nil {
			return rollback(err)
		}
	}
	if modeChanged || lanChanged || tunChanged {
		readyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err := a.Core.WaitReady(readyCtx)
		cancel()
		if err != nil {
			return rollback(fmt.Errorf("切换设置失败，mihomo 未就绪: %w", err))
		}
		patchAttempted = true
		if err := a.patchCoreSettings(ctx, next, modeChanged, lanChanged, tunChanged); err != nil {
			return rollback(err)
		}
	}
	if proxyChanged && next.Enabled && next.UseSystemProxy {
		proxyAttempted = true
		if err := a.applySystemProxy(ctx, next); err != nil {
			return rollback(err)
		}
	}
	return nil
}

func tunEnabled(settings config.Settings) bool {
	return settings.Enabled && settings.UseTun && settings.TunAuthorized
}

func (a *App) patchCoreSettings(ctx context.Context, settings config.Settings, modeChanged, lanChanged, tunChanged bool) error {
	payload := make(map[string]any)
	if modeChanged {
		if settings.OutboundMode == "global" {
			if err := a.ensureGlobalProxy(ctx); err != nil {
				return err
			}
		}
		payload["mode"] = settings.OutboundMode
	}
	if lanChanged {
		payload["allow-lan"] = settings.AllowLAN
		payload["bind-address"] = runtimeBindAddress(settings.AllowLAN)
	}
	if tunChanged {
		payload["tun"] = map[string]any{
			"enable": tunEnabled(settings), "stack": settings.TunStack, "device": settings.TunDevice,
			"auto-route": true, "auto-detect-interface": true,
		}
	}
	client := a.Core.Client()
	if _, _, err := client.Do(ctx, "PATCH", "/configs", payload); err != nil {
		return err
	}
	if tunChanged {
		var actual struct {
			Tun struct {
				Enable bool   `json:"enable"`
				Stack  string `json:"stack"`
				Device string `json:"device"`
			} `json:"tun"`
		}
		if err := client.GetJSON(ctx, "/configs", &actual); err != nil {
			return err
		}
		if actual.Tun.Enable != tunEnabled(settings) ||
			(actual.Tun.Enable && (!strings.EqualFold(actual.Tun.Stack, settings.TunStack) ||
				(settings.TunDevice != "" && actual.Tun.Device != settings.TunDevice))) {
			return fmt.Errorf("内核未应用 TUN 设置，请检查内核版本及日志")
		}
	}
	if modeChanged {
		_ = client.CloseConnections(ctx)
	}
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
