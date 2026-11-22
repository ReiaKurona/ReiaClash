package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"reia-clash/internal/config"
	"reia-clash/internal/update"
)

type ResourceEntry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Bytes     int64  `json:"bytes"`
	UpdatedAt int64  `json:"updated_at,omitempty"`
}

type ResourceStatus struct {
	Entries       []ResourceEntry `json:"entries"`
	AutoUpdate    bool            `json:"auto_update"`
	IntervalHours int             `json:"interval_hours"`
	Kinds         []string        `json:"kinds"`
	LastUpdated   int64           `json:"last_updated"`
}

func (a *App) Resources() (ResourceStatus, error) {
	dir, err := config.MihomoDataDir()
	if err != nil {
		return ResourceStatus{}, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return ResourceStatus{}, err
	}
	s := a.Prefs.Snapshot()
	status := ResourceStatus{AutoUpdate: s.ResourceAutoUpdate, IntervalHours: max(1, s.ResourceIntervalHours), LastUpdated: s.ResourceLastUpdated, Kinds: []string{}}
	if s.ResourceKinds != "" {
		status.Kinds = strings.Split(s.ResourceKinds, ",")
	}
	for _, resource := range update.GeoResources() {
		item := ResourceEntry{ID: resource.ID, Name: resource.Name}
		for _, entry := range entries {
			if strings.EqualFold(entry.Name(), resource.Name) {
				if stat, err := entry.Info(); err == nil && stat.Mode().IsRegular() {
					item.Bytes, item.UpdatedAt = stat.Size(), stat.ModTime().Unix()
				}
				break
			}
		}
		status.Entries = append(status.Entries, item)
	}
	status.Entries = append(status.Entries, ResourceEntry{ID: "rules", Name: "订阅中的 HTTP 规则集"})
	return status, nil
}

func resourceKinds(kinds []string) ([]string, error) {
	allowed := []string{"rules"}
	for _, r := range update.GeoResources() {
		allowed = append(allowed, r.ID)
	}
	result := make([]string, 0, len(allowed))
	for _, kind := range kinds {
		if !slices.Contains(allowed, kind) {
			return nil, fmt.Errorf("未知的资源类型")
		}
		if !slices.Contains(result, kind) {
			result = append(result, kind)
		}
	}
	return result, nil
}

func (a *App) SetResourceSchedule(enabled bool, interval int, kinds []string) error {
	kinds, err := resourceKinds(kinds)
	if err != nil {
		return err
	}
	if interval < 1 || interval > 168 || (enabled && len(kinds) == 0) {
		return fmt.Errorf("更新间隔应为 1–168 小时，启用时需要选择资源")
	}
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	_, err = a.Prefs.Update(func(s *config.Settings) error {
		chosen := strings.Join(kinds, ",")
		if s.ResourceKinds != chosen || (!s.ResourceAutoUpdate && enabled) {
			s.ResourceLastUpdated = 0
		}
		s.ResourceKinds, s.ResourceIntervalHours, s.ResourceAutoUpdate = chosen, interval, enabled
		return nil
	})
	return err
}

func (a *App) SyncResources(parent context.Context, kinds []string) error {
	return a.syncResources(parent, kinds, false)
}

func (a *App) syncResources(parent context.Context, kinds []string, scheduled bool) error {
	kinds, err := resourceKinds(kinds)
	if err != nil {
		return err
	}
	if len(kinds) == 0 {
		return fmt.Errorf("请选择要同步的资源")
	}
	if !a.resourceMu.TryLock() {
		return fmt.Errorf("资源正在同步")
	}
	defer a.resourceMu.Unlock()
	ctx, cancel := context.WithTimeout(parent, 4*time.Minute)
	defer cancel()
	dir, err := config.MihomoDataDir()
	if err != nil {
		return err
	}
	var failures error
	geoChanged := false
	for _, r := range update.GeoResources() {
		if !slices.Contains(kinds, r.ID) {
			continue
		}
		if err := a.Updater.SyncGeoResource(ctx, dir, r); err != nil {
			failures = errors.Join(failures, fmt.Errorf("%s: %w", r.Name, err))
		} else {
			geoChanged = true
		}
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(failures, err)
	}
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	if a.exiting {
		return fmt.Errorf("客户端正在退出")
	}
	if geoChanged && a.Core.RunningBinary() != "" {
		if err := a.Core.ReloadRuntime(ctx); err != nil {
			failures = errors.Join(failures, fmt.Errorf("Geo 文件已保存，将在下次启动加载：%w", err))
		}
	}
	if slices.Contains(kinds, "rules") {
		failures = errors.Join(failures, a.refreshRuleProviders(ctx))
	}
	if failures != nil {
		return failures
	}
	_, err = a.Prefs.Update(func(s *config.Settings) error {
		// A manual subset must not postpone the scheduled set.
		if scheduled && s.ResourceAutoUpdate && s.ResourceKinds == strings.Join(kinds, ",") {
			s.ResourceLastUpdated = time.Now().Unix()
		}
		return nil
	})
	return err
}

func (a *App) refreshRuleProviders(ctx context.Context) error {
	client := a.Core.Client()
	var response struct {
		Providers map[string]struct {
			VehicleType string `json:"vehicleType"`
		} `json:"providers"`
	}
	if err := client.GetJSON(ctx, "/providers/rules", &response); err != nil {
		return err
	}
	var failures error
	for name, provider := range response.Providers {
		if !strings.EqualFold(provider.VehicleType, "http") {
			continue
		}
		if err := ctx.Err(); err != nil {
			return errors.Join(failures, err)
		}
		if _, _, err := client.Do(ctx, http.MethodPut, "/providers/rules/"+url.PathEscape(name), nil); err != nil {
			failures = errors.Join(failures, err)
		}
	}
	return failures
}

func (a *App) resourceUpdateLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	var nextAttempt time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		settings := a.Prefs.Snapshot()
		if !settings.ResourceAutoUpdate || settings.ResourceKinds == "" || time.Now().Before(nextAttempt) {
			continue
		}
		if settings.ResourceLastUpdated != 0 && time.Since(time.Unix(settings.ResourceLastUpdated, 0)) < time.Duration(max(1, settings.ResourceIntervalHours))*time.Hour {
			continue
		}
		if err := a.syncResources(ctx, strings.Split(settings.ResourceKinds, ","), true); err != nil && ctx.Err() == nil {
			nextAttempt = time.Now().Add(10 * time.Minute)
			a.AddEvent("error", "外部资源自动同步失败："+err.Error())
		}
	}
}
