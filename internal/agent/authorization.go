package agent

import (
	"context"
	"fmt"

	"reia-clash/internal/platform/privilege"
)

// requireTunAuthority accepts either the current process's system capability
// or an installed core service that owns the privileged Mihomo process.
func (a *App) requireTunAuthority(ctx context.Context) error {
	if a.Core.UsesExternalLifecycle() {
		return nil
	}
	if err := privilege.RequireTunPrivilege(); err != nil {
		a.AddEvent("info", "TUN 需要网络管理权限，请在系统授权窗口中确认安装内核服务")
		if installErr := a.setCoreService(ctx, true); installErr != nil {
			return fmt.Errorf("TUN 系统授权失败: %w", installErr)
		}
	}
	return nil
}
