package agent

import (
	"context"
	"fmt"
	"reia-clash/internal/mihomo"
	"reia-clash/internal/platform/coreservice"
	"time"
)

func startupService(root string, registration coreservice.Registration) (mihomo.ProcessLifecycle, bool, string) {
	lifecycle, enabled, err := mihomo.LoadServiceLifecycle(root)
	if registration.Installed || registration.Running {
		if err != nil || !enabled {
			// An actual OS registration takes precedence over a missing/broken
			// marker. Never start a local kernel alongside that service.
			err = mihomo.EnableServiceLifecycle(root)
			if err == nil {
				lifecycle, enabled, err = mihomo.LoadServiceLifecycle(root)
			}
		}
		if err != nil {
			detail := fmt.Errorf("系统服务已注册，但控制配置不可用，请修复服务: %w", err)
			return unavailableService{detail}, true, detail.Error()
		}
		return lifecycle, true, ""
	}
	if err != nil {
		return nil, false, "内核服务记录不可用，请修复服务: " + err.Error()
	}
	if enabled && !usableStartupService(lifecycle, false) {
		return nil, false, "系统服务未注册且控制端点不可达，已回退普通模式；可在设置中修复服务"
	}
	return lifecycle, enabled, ""
}

type unavailableService struct{ err error }

func (s unavailableService) Start(context.Context) error { return s.err }
func (s unavailableService) Status() (bool, string)      { return false, s.err.Error() }
func (s unavailableService) Restart() error              { return s.err }
func (s unavailableService) Shutdown() error             { return s.err }

func usableStartupService(lifecycle mihomo.ProcessLifecycle, registered bool) bool {
	if registered {
		return true
	}
	// Preserve reachable legacy helpers until the user migrates them explicitly.
	probe, ok := lifecycle.(interface{ Probe(context.Context) error })
	if !ok {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return probe.Probe(ctx) == nil
}
