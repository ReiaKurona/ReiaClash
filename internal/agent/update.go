package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"reia-clash/internal/config"
	"reia-clash/internal/kernelbinary"
	"reia-clash/internal/mihomo"
	"reia-clash/internal/platform/network"
	"reia-clash/internal/update"
)

func (a *App) CoreKind() string {
	if kernelbinary.IsMeow(a.Core.BinaryPath()) {
		return "meow-rs"
	}
	return "mihomo"
}

func (a *App) CheckUpdate(ctx context.Context) (update.Info, error) {
	return a.Updater.ForCore(a.CoreKind() == "meow-rs", false).Check(ctx)
}

func (a *App) SwitchCore(ctx context.Context, kind string) error {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	if kind == a.CoreKind() {
		return nil
	}
	previous := a.Core.BinaryPath()
	if kind == "mihomo" {
		return a.restoreMihomo(ctx)
	}
	if kind != "meow-rs" {
		return fmt.Errorf("不支持的内核类型")
	}
	if a.Core.RunningBinary() != previous || previous == "" {
		return fmt.Errorf("请先启动 mihomo，以便通过现有代理下载 meow-rs")
	}
	directory, err := config.MihomoDataDir()
	if err != nil {
		return err
	}
	installed := filepath.Join(filepath.Dir(directory), "meow")
	if runtime.GOOS == "windows" {
		installed = filepath.Join(installed, "meow.exe")
	}
	if err := a.checkMeowSubscription(); err != nil {
		return err
	}
	if err := a.prepareMeowGeodata(ctx); err != nil {
		return err
	}
	if kernelbinary.Validate(installed) == nil {
		return a.activateBinary(ctx, previous, installed)
	}
	info, target, err := a.Updater.StageCore(ctx, a.Updater.ForCore(true, true))
	if err != nil {
		return err
	}
	return a.installRelease(ctx, info, target, true)
}

func (a *App) restoreMihomo(ctx context.Context) error {
	previous := a.Core.BinaryPath()
	settings := a.Prefs.Snapshot()
	target := settings.MihomoFallback
	if target == "" || kernelbinary.IsMeow(target) || kernelbinary.Validate(target) != nil {
		settings.ActiveBinary, settings.PreviousBinary = "", ""
		var err error
		target, err = findMihomo(settings, a.Paths.BinaryDir)
		if err != nil {
			info, staged, err := a.Updater.Stage(ctx)
			if err != nil {
				return err
			}
			return a.installRelease(ctx, info, staged, false)
		}
	}
	if kernelbinary.IsManagedUpdate(a.Paths.UpdateDir, target) {
		return a.installRelease(ctx, update.Info{Tag: legacyRelease(target)}, target, false)
	}
	return a.activateBinary(ctx, previous, target)
}

func (a *App) UpgradeCore(ctx context.Context, silent bool) (update.Info, error) {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	meow := a.CoreKind() == "meow-rs"
	source := a.Updater.ForCore(meow, false)
	if silent {
		latest, err := source.Check(ctx)
		if err != nil {
			return update.Info{}, err
		}
		if latest.Tag == installedRelease(a.Prefs.Snapshot(), meow) {
			return latest, nil
		}
		pending, kind, err := a.Updater.Pending()
		if err != nil || kind != meow || pending.SHA256 != latest.SHA256 {
			pending, err = a.Updater.Queue(ctx, source)
			if err != nil {
				return update.Info{}, err
			}
		}
		pending.Pending = true
		if !a.Prefs.Snapshot().Enabled {
			go a.applyIdleUpdate(a.context())
		}
		return pending, nil
	}
	info, target, err := a.Updater.StageCore(ctx, source)
	if err != nil {
		return update.Info{}, err
	}
	if err := a.installRelease(ctx, info, target, meow); err != nil {
		return update.Info{}, err
	}
	if _, kind, err := a.Updater.Pending(); err == nil && kind == meow {
		if err := a.Updater.DiscardPending(); err != nil {
			a.AddEvent("error", err.Error())
		}
	}
	return info, nil
}

func installedRelease(s config.Settings, meow bool) string {
	if meow {
		return s.MeowRelease
	}
	return s.MihomoRelease
}

func (a *App) applyIdleUpdate(ctx context.Context) {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	if a.exiting || a.Prefs.Snapshot().Enabled {
		return
	}
	if _, err := a.applyPending(ctx); err != nil {
		a.AddEvent("error", "静默更新未应用："+err.Error())
	}
}

func (a *App) applyPending(ctx context.Context) (bool, error) {
	if a.Updater == nil {
		return false, nil
	}
	info, meow, err := a.Updater.Pending()
	if os.IsNotExist(err) || (err == nil && meow != (a.CoreKind() == "meow-rs")) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Tag == installedRelease(a.Prefs.Snapshot(), meow) {
		return false, a.Updater.DiscardPending()
	}
	info, candidate, err := a.Updater.UnpackPending()
	if err != nil {
		return false, err
	}
	if err := a.installRelease(ctx, info, candidate, meow); err != nil {
		return false, err
	}
	return true, a.Updater.DiscardPending()
}

func (a *App) installRelease(ctx context.Context, info update.Info, candidate string, meow bool) error {
	if err := a.Core.ValidateBinaryConfig(candidate); err != nil {
		return err
	}
	data, err := config.MihomoDataDir()
	if err != nil {
		return err
	}
	old := a.Prefs.Snapshot()
	previous := a.Core.BinaryPath()
	install, err := update.PrepareInstall(filepath.Dir(data), candidate, installedRelease(old, meow), meow)
	if err != nil {
		return err
	}
	defer install.Close()
	if err := ctx.Err(); err != nil {
		return err
	}
	baseline := a.measureMeowBaseline(ctx, candidate)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := a.Core.Shutdown(); err != nil {
		return err
	}
	recovery := previous
	if previous == install.Target && install.BackupBinary != "" {
		recovery = install.BackupBinary
	}
	save := func(active, backup, fallback string) error {
		_, err := a.Prefs.Update(func(s *config.Settings) error {
			s.ActiveBinary, s.PreviousBinary, s.MihomoFallback = active, backup, fallback
			return nil
		})
		if err != nil {
			return err
		}
		return a.Core.PersistCurrentRuntimeConfig()
	}
	start := func(path string) error {
		if err := a.Core.Start(a.context()); err != nil {
			return err
		}
		if path == "" {
			return nil
		}
		return a.waitForBinary(path)
	}
	rollback := func(cause error) error {
		if err := a.Core.Shutdown(); err != nil {
			return errors.Join(cause, err)
		}
		if err := install.Rollback(); err != nil {
			return errors.Join(cause, err)
		}
		if err := save(previous, "", old.MihomoFallback); err != nil {
			return errors.Join(cause, err)
		}
		return errors.Join(cause, start(previous))
	}
	fallback := old.MihomoFallback
	if meow && previous != "" && !kernelbinary.IsMeow(previous) {
		fallback = previous
	}
	if err := save(install.Target, recovery, fallback); err != nil {
		return rollback(err)
	}
	if err := install.Replace(); err != nil {
		return rollback(err)
	}
	if err := start(install.Target); err != nil {
		return rollback(err)
	}
	if err := a.verifyMeowBaseline(context.WithoutCancel(ctx), baseline); err != nil {
		return rollback(err)
	}
	if _, err := a.Prefs.Update(func(s *config.Settings) error {
		s.PreviousBinary = ""
		if meow {
			s.MeowRelease = info.Tag
		} else {
			s.MihomoRelease = info.Tag
			s.MihomoFallback = ""
		}
		return nil
	}); err != nil {
		return rollback(err)
	}
	if err := install.Prune(); err != nil {
		a.AddEvent("error", "历史内核清理失败："+err.Error())
	}
	_ = a.Updater.RemoveSuperseded(candidate)
	a.AddEvent("info", fmt.Sprintf("%s %s 已在应用目录启动", a.CoreKind(), info.Tag))
	return nil
}

func legacyRelease(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), ".exe")
	if name == "meow" {
		name = filepath.Base(filepath.Dir(path))
	}
	_, tag, ok := strings.Cut(name, "-")
	if ok {
		if index := strings.LastIndex(tag, "-"); index > 0 && len(tag)-index == 13 {
			return tag[:index]
		}
	}
	return "legacy"
}

func (a *App) migrateActiveKernel(ctx context.Context) error {
	if a.Updater == nil {
		return nil
	}
	active := a.Core.BinaryPath()
	if !kernelbinary.IsManagedUpdate(a.Paths.UpdateDir, active) {
		return nil
	}
	return a.installRelease(ctx, update.Info{Tag: legacyRelease(active)}, active, kernelbinary.IsMeow(active))
}

func (a *App) CoreOperationBusy() bool {
	if !a.serviceMu.TryLock() {
		return true
	}
	a.serviceMu.Unlock()
	return false
}

func (a *App) RestartCore() error {
	a.serviceMu.Lock()
	defer a.serviceMu.Unlock()
	if applied, err := a.applyPending(a.context()); applied || err != nil {
		return err
	}
	return a.Core.Restart()
}

func (a *App) activateBinary(ctx context.Context, previous, target string) error {
	if target == previous && a.Core.RunningBinary() == target {
		return nil
	}
	if err := a.Core.ValidateBinaryConfig(target); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	baseline := a.measureMeowBaseline(ctx, target)
	if err := ctx.Err(); err != nil {
		return err
	}
	old := a.Prefs.Snapshot()
	if _, err := a.Prefs.Update(func(s *config.Settings) error {
		s.ActiveBinary, s.PreviousBinary = target, previous
		if kernelbinary.IsMeow(target) && !kernelbinary.IsMeow(previous) {
			s.MihomoFallback = previous
		}
		return nil
	}); err != nil {
		return err
	}

	// After writing pending state, finish the handoff even if the browser closes.
	err := a.Core.PersistCurrentRuntimeConfig()
	if err == nil {
		err = a.Core.Restart()
	}
	if err == nil {
		err = a.waitForBinary(target)
	}
	if err == nil {
		err = a.verifyMeowBaseline(context.WithoutCancel(ctx), baseline)
	}
	if err != nil {
		if _, saveErr := a.Prefs.Update(func(s *config.Settings) error {
			s.ActiveBinary, s.PreviousBinary, s.MihomoFallback = previous, "", old.MihomoFallback
			return nil
		}); saveErr != nil {
			return errors.Join(err, saveErr)
		}
		if previous == "" {
			return err
		}
		restoreErr := a.Core.PersistCurrentRuntimeConfig()
		if restoreErr == nil {
			restoreErr = a.Core.Restart()
		}
		if restoreErr == nil {
			restoreErr = a.waitForBinary(previous)
		}
		return errors.Join(err, restoreErr)
	}
	if _, err := a.Prefs.Update(func(s *config.Settings) error {
		s.PreviousBinary = ""
		if !kernelbinary.IsMeow(target) {
			s.MihomoFallback = ""
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

func (a *App) waitForBinary(path string) error {
	var meowIPv6 string
	settings := a.Prefs.Snapshot()
	expectTun := tunEnabled(settings)
	isMeow := kernelbinary.IsMeow(path)
	if isMeow && expectTun {
		profile, err := os.ReadFile(a.Paths.Profile)
		if err != nil {
			return err
		}
		options, err := mihomo.MeowTunOptions(profile)
		if err != nil {
			return err
		}
		meowIPv6 = options.IPv6Address
	}
	ctx, cancel := context.WithTimeout(context.Background(), mihomo.StartupTimeout(path)+10*time.Second)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var readySince time.Time
	detail := "等待目标内核及控制器就绪"
	for {
		if a.Core.RunningBinary() == path {
			connection, err := net.DialTimeout("tcp", "127.0.0.1:7890", 250*time.Millisecond)
			if err != nil {
				detail = fmt.Sprintf("混合代理端口未就绪: %v", err)
			} else {
				connection.Close()
			}
			if err == nil && (expectTun || isMeow) {
				var actual controllerConfig
				probe, stop := context.WithTimeout(ctx, 500*time.Millisecond)
				err = a.Core.Client().GetJSON(probe, "/configs", &actual)
				stop()
				osTun := isMeow && network.ActiveMeowTun(settings.TunDevice) != ""
				switch {
				case osTun && expectTun:
					err = nil
				case osTun || (err == nil && actual.Tun.Enable != expectTun):
					if expectTun {
						err = errors.New("TUN 未启用")
					} else {
						err = errors.New("TUN 仍在接管")
					}
				case err == nil:
					err = actual.checkTunIPv6(meowIPv6)
				}
				if err != nil {
					detail = fmt.Sprintf("TUN 验证失败: %v", err)
				}
			}
			if err == nil {
				if readySince.IsZero() {
					readySince = time.Now()
				}
				if time.Since(readySince) >= 2*time.Second {
					return nil
				}
			} else {
				readySince = time.Time{}
			}
		} else {
			readySince = time.Time{}
			if _, lastError := a.Core.Status(); lastError != "" {
				detail = lastError
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("目标内核启动验证失败 [%s]：%s", filepath.Base(path), detail)
		case <-ticker.C:
		}
	}
}

func (a *App) autoUpdateLoop(ctx context.Context) {
	delay := 30 * time.Second
	for {
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return
		}
		settings := a.Prefs.Snapshot()
		delay = time.Duration(settings.UpdateIntervalHours) * time.Hour
		if !settings.AutoUpdateEnabled {
			continue
		}
		if _, err := a.UpgradeCore(ctx, true); err != nil && ctx.Err() == nil {
			a.AddEvent("error", "自动更新内核失败："+err.Error())
		}
	}
}
