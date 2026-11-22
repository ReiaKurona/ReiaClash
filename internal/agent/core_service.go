package agent

import (
	"context"
	"errors"
	"fmt"

	"reia-clash/internal/config"
	"reia-clash/internal/mihomo"
	"reia-clash/internal/platform/privilege"
)

// SetCoreService 在 WebUI 请求时安装或移除平台提供的特权内核服务。
// 浏览器 API agent 始终运行在当前登录用户会话中。
func (a *App) SetCoreService(ctx context.Context, enable bool) error {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	return a.setCoreService(ctx, enable)
}

func (a *App) setCoreService(ctx context.Context, enable bool) error {
	if a.exiting {
		return fmt.Errorf("ReiaClash 正在退出")
	}
	if enable == a.Core.UsesExternalLifecycle() {
		if enable {
			registration := a.serviceProvider().RegistrationStatus()
			if registration.Installed && registration.Running && !registration.LegacyHelper {
				return a.Core.Start(ctx)
			}
			// Repair missing registrations or migrate the legacy Windows task.
			return a.installCoreService(ctx)
		}
		registration := a.serviceProvider().RegistrationStatus()
		if !registration.Installed && registration.Error == "" {
			_, marked, markerErr := mihomo.LoadServiceLifecycle(a.Paths.Root)
			if !marked && markerErr == nil {
				return nil
			}
		}
		return a.removeCoreService(ctx, true)
	}
	if !a.serviceProvider().Supported() {
		return fmt.Errorf("当前平台尚未提供 Mihomo 特权服务安装器")
	}
	if enable {
		return a.installCoreService(ctx)
	}
	return a.removeCoreService(ctx, true)
}

func (a *App) RepairCoreService(ctx context.Context) error {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	if a.exiting {
		return fmt.Errorf("ReiaClash 正在退出")
	}
	if !a.serviceProvider().Supported() {
		return fmt.Errorf("当前平台不支持系统服务")
	}
	return a.installCoreService(ctx)
}

func (a *App) installCoreService(ctx context.Context) error {
	if a.Core.BinaryPath() == "" {
		return fmt.Errorf("请先在 WebUI 中安装 mihomo 内核，再启用特权服务")
	}
	// Store the actual resolved kernel path so the OS service never relies on cwd.
	binary := a.Core.BinaryPath()
	if _, err := a.Prefs.Update(func(s *config.Settings) error { s.ActiveBinary = binary; return nil }); err != nil {
		return err
	}
	if err := mihomo.EnsureServiceCredentials(a.Paths.Root); err != nil {
		return fmt.Errorf("准备内核服务认证密钥失败：%w", err)
	}

	// 先停止用户态内核，释放 API 端口，再让系统服务启动接管后的内核。
	wasExternal := a.Core.UsesExternalLifecycle()
	if !wasExternal {
		if err := a.Core.Shutdown(); err != nil {
			cleanupErr := mihomo.DisableServiceLifecycle(a.Paths.Root)
			return errors.Join(err, cleanupErr, a.resumeLocalCore())
		}
	}
	if err := a.serviceProvider().RequestInstall(ctx, a.Paths.Root); err != nil {
		registration := a.serviceProvider().RegistrationStatus()
		if registration.Installed {
			if markErr := mihomo.EnableServiceLifecycle(a.Paths.Root); markErr != nil {
				return errors.Join(err, markErr)
			}
			if lifecycle, enabled, loadErr := mihomo.LoadServiceLifecycle(a.Paths.Root); loadErr == nil && enabled {
				a.Core.SetLifecycle(lifecycle)
				a.Prefs.SetRuntimeTunAuthorization(true)
				if registration.Running {
					return errors.Join(err, a.Core.Start(a.context()))
				}
				return fmt.Errorf("系统服务已注册，但安装/启动尚未完成: %w", err)
			}
		}
		if wasExternal {
			return err
		}
		cleanupErr := mihomo.DisableServiceLifecycle(a.Paths.Root)
		return errors.Join(err, cleanupErr, a.resumeLocalCore())
	}
	registration := a.serviceProvider().RegistrationStatus()
	if !registration.Installed || !registration.Running {
		return fmt.Errorf("系统服务安装未完成，实际注册/运行状态不正常: %s", registration.Error)
	}
	if err := mihomo.EnableServiceLifecycle(a.Paths.Root); err != nil {
		return err
	}

	lifecycle, enabled, err := mihomo.LoadServiceLifecycle(a.Paths.Root)
	if err != nil || !enabled {
		if err == nil {
			err = fmt.Errorf("特权服务已安装，但本机控制配置缺失")
		}
		return err
	}
	a.Core.SetLifecycle(lifecycle)
	a.Prefs.SetRuntimeTunAuthorization(true)
	if err := a.Core.Start(a.context()); err != nil {
		return fmt.Errorf("特权服务已安装，但暂时无法连接：%w", err)
	}
	if err := a.Prefs.Reload(); err != nil {
		return err
	}
	a.AddEvent("info", "Mihomo 特权服务已安装，内核崩溃后会由系统服务管理器自动恢复")
	return nil
}

func (a *App) removeCoreService(ctx context.Context, resumeLocal bool) error {
	localStarted := !a.Core.UsesExternalLifecycle() && a.Core.SupervisorStarted()
	if localStarted {
		if err := a.Core.Shutdown(); err != nil {
			return err
		}
	}
	if err := a.serviceProvider().RequestUninstall(ctx, a.Paths.Root); err != nil {
		if localStarted {
			return errors.Join(err, a.Core.Start(a.context()))
		}
		return err
	}
	registration := a.serviceProvider().RegistrationStatus()
	if registration.Installed || registration.Running || registration.Error != "" {
		return fmt.Errorf("系统服务尚未完全移除，已阻止启动普通内核: %s", registration.Error)
	}
	a.Core.SetLifecycle(nil)
	if err := mihomo.DisableServiceLifecycle(a.Paths.Root); err != nil {
		return err
	}
	a.Prefs.SetRuntimeTunAuthorization(privilege.RequireTunPrivilege() == nil)
	if _, err := a.Prefs.Update(func(s *config.Settings) error {
		if !resumeLocal {
			s.Enabled = false
		} else {
			s.UseTun = false
			if !s.UseSystemProxy {
				s.Enabled = false
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := a.Core.PersistCurrentRuntimeConfig(); err != nil {
		return err
	}
	if !resumeLocal {
		return nil
	}
	if err := a.Core.Start(a.context()); err != nil {
		return err
	}
	a.AddEvent("info", "Mihomo 特权服务已移除，内核已切回当前用户进程管理")
	return nil
}

func (a *App) resumeLocalCore() error {
	a.Core.SetLifecycle(nil)
	a.Prefs.SetRuntimeTunAuthorization(privilege.RequireTunPrivilege() == nil)
	return a.Core.Start(a.context())
}

func (a *App) context() context.Context {
	a.runCtxMu.RLock()
	ctx := a.runCtx
	a.runCtxMu.RUnlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
