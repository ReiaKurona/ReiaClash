package agent

import (
	"context"
	"fmt"
)

// PrepareDesktopExit holds the service lock through uninstall and shutdown.
// The caller sends the HTTP response before requesting process exit.
func (a *App) PrepareDesktopExit(ctx context.Context, uninstallService bool) (background bool, err error) {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	if a.exiting {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	delegated := a.Core.UsesExternalLifecycle()
	if !uninstallService && delegated {
		return true, nil
	}
	registration := a.serviceProvider().RegistrationStatus()
	if uninstallService {
		if delegated || registration.Installed || registration.Running {
			if err := a.removeCoreService(ctx, false); err != nil {
				return false, err
			}
		} else if registration.Error != "" {
			return false, fmt.Errorf("无法确认内核服务注册状态: %s", registration.Error)
		}
	} else if registration.Installed || registration.Running {
		return true, nil
	}
	if err := a.PrepareExit(); err != nil {
		return false, err
	}
	if err := a.Core.Shutdown(); err != nil {
		return false, err
	}
	a.exiting = true
	return false, nil
}
