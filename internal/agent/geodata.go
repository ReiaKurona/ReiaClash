package agent

import (
	"context"
	"errors"
	"reia-clash/internal/config"
)

// SetGeoLowMemory updates only the managed loader. Restarting applies it to
// both the local supervisor and the privileged helper, including when paused.
func (a *App) SetGeoLowMemory(ctx context.Context, enabled bool) (bool, error) {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	previous := a.Prefs.Snapshot().GeoLowMemory
	if previous == enabled {
		return false, nil
	}
	if _, err := a.Prefs.Update(func(s *config.Settings) error { s.GeoLowMemory = enabled; return nil }); err != nil {
		return false, err
	}
	restore := func(cause error) (bool, error) {
		_, settingsErr := a.Prefs.Update(func(s *config.Settings) error { s.GeoLowMemory = previous; return nil })
		return false, errors.Join(cause, settingsErr, a.Core.PersistCurrentRuntimeConfig())
	}
	if err := a.Core.PersistCurrentRuntimeConfig(); err != nil {
		return restore(err)
	}
	if a.Core.SupervisorStarted() || a.Core.UsesExternalLifecycle() {
		if err := a.Core.Restart(); err != nil {
			return restore(err)
		}
		a.AddEvent("info", "Geo 加载模式已保存，正在重启内核以应用："+config.GeodataLoader(enabled))
		return true, nil
	}
	return false, nil
}
