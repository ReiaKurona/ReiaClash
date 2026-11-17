package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"reia-clash/internal/config"
	"reia-clash/internal/update"
)

func (a *App) CheckUpdate(ctx context.Context) (update.Info, error) {
	return a.Updater.Check(ctx)
}

func (a *App) UpgradeCore(ctx context.Context, silent bool) (update.Info, error) {
	info, stagedPath, err := a.Updater.Stage(ctx)
	if err != nil {
		return update.Info{}, err
	}
	previous := a.Core.BinaryPath()
	_, err = a.Prefs.Update(func(settings *config.Settings) error {
		settings.PreviousBinary = previous
		settings.ActiveBinary = stagedPath
		return nil
	})
	if err != nil {
		return update.Info{}, err
	}
	if !silent {
		if err := a.Core.Restart(); err != nil {
			_, _ = a.Prefs.Update(func(s *config.Settings) error { s.ActiveBinary = previous; s.PreviousBinary = ""; return nil })
			return update.Info{}, err
		}
	}
	a.AddEvent("info", fmt.Sprintf("mihomo %s 已下载并通过 SHA-256 校验", info.Tag))
	return info, nil
}

// autoUpdateLoop 只在 WebUI 开启自动更新后检查；下载的新版本先放入独立文件，
// 不覆盖当前进程正在运行的可执行文件，等内核下次重启时再切换。
func (a *App) autoUpdateLoop(ctx context.Context) {
	firstCheck := true
	for ctx.Err() == nil {
		settings := a.Prefs.Snapshot()
		delay := time.Duration(settings.UpdateIntervalHours) * time.Hour
		if firstCheck {
			delay = 30 * time.Second
			firstCheck = false
		}
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return
		}
		settings = a.Prefs.Snapshot()
		if !settings.AutoUpdateEnabled {
			continue
		}
		info, err := a.Updater.Check(ctx)
		if err != nil {
			a.Logger.Warn("自动检查 mihomo 更新失败", "error", err)
			a.AddEvent("error", "自动检查 mihomo 更新失败："+err.Error())
			continue
		}
		current := settings.ActiveBinary
		if current == "" {
			current = a.Core.BinaryPath()
		}
		if strings.Contains(filepath.Base(current), info.Tag) {
			continue
		}
		staged, binary, err := a.Updater.Stage(ctx)
		if err != nil {
			a.Logger.Warn("自动下载 mihomo 更新失败", "error", err)
			a.AddEvent("error", "自动下载 mihomo 更新失败："+err.Error())
			continue
		}
		previous := a.Core.BinaryPath()
		_, err = a.Prefs.Update(func(current *config.Settings) error {
			current.PreviousBinary = previous
			current.ActiveBinary = binary
			return nil
		})
		if err != nil {
			a.Logger.Error("保存已下载的 mihomo 更新失败", "error", err)
			a.AddEvent("error", "新内核已下载，但保存切换状态失败："+err.Error())
			continue
		}
		a.AddEvent("info", fmt.Sprintf("mihomo %s 已静默下载；下次内核重启时切换", staged.Tag))
	}
}
