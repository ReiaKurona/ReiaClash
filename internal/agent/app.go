package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"reia-clash/internal/config"
	"reia-clash/internal/mihomo"
	"reia-clash/internal/platform/coreservice"
	"reia-clash/internal/platform/privilege"
	"reia-clash/internal/platform/systemproxy"
	"reia-clash/internal/update"
)

type App struct {
	Paths                config.Paths
	Prefs                *config.Store
	Core                 *mihomo.Manager
	Updater              *update.Manager
	Logger               *slog.Logger
	Exit                 chan struct{}
	exitOnce             sync.Once
	serviceMu            sync.Mutex
	exiting              bool
	subscriptionMu       sync.Mutex
	subscriptionStatusMu sync.Mutex
	subscriptionBusy     bool
	subscriptionAttempt  int64
	subscriptionError    string
	subscriptionWake     chan struct{}
	subscriptionFetch    func(context.Context, string) ([]byte, error)
	subscriptionApply    func([]byte) error
	services             servicePlatform
	dnsMu                sync.Mutex
	dnsRecords           []DNSRecord
	dnsID                uint64
	dnsCaptureError      string
	panelMu              sync.RWMutex
	panelURL             string
	panelNetwork         string
	runCtxMu             sync.RWMutex
	runCtx               context.Context
	eventsMu             sync.Mutex
	events               []Event
	eventID              uint64
}

func Open(logger *slog.Logger) (*App, error) {
	paths, err := config.UserPaths()
	if err != nil {
		return nil, err
	}
	prefs, err := config.OpenStore(paths.ConfigFile)
	if err != nil {
		return nil, fmt.Errorf("读取 ReiaClash 设置失败: %w", err)
	}
	lifecycle, serviceEnabled, serviceNotice := startupService(paths.Root, coreservice.RegistrationStatus())
	tunAuthorized := privilege.RequireTunPrivilege() == nil || serviceEnabled
	prefs.SetRuntimeTunAuthorization(tunAuthorized)
	profilePath := filepath.Join(paths.Root, "profile.yaml")
	if _, err := os.Stat(profilePath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(profilePath, []byte("proxies: []\nproxy-groups: []\nrules:\n  - MATCH,DIRECT\n"), 0600); err != nil {
			return nil, err
		}
	}
	binary, binaryErr := findMihomo(prefs.Snapshot(), paths.BinaryDir)
	if binaryErr != nil {
		logger.Warn("mihomo 内核尚未就绪", "error", binaryErr)
		binary = ""
	}
	if prefs.Snapshot().ActiveBinary != binary {
		if _, err := prefs.Update(func(s *config.Settings) error { s.ActiveBinary = binary; return nil }); err != nil {
			return nil, err
		}
	}
	logger.Info("已选择 mihomo 内核", "binary", binary)
	core, err := mihomo.NewManager(paths.Root, filepath.Join(paths.Root, "mihomo.yaml"), binary, prefs, logger)
	if err != nil {
		return nil, err
	}
	if serviceEnabled {
		core.SetLifecycle(lifecycle)
	}
	app := &App{Paths: paths, Prefs: prefs, Core: core, Updater: update.NewManager(paths.UpdateDir), Logger: logger, Exit: make(chan struct{})}
	app.subscriptionWake = make(chan struct{}, 1)
	core.SetNotifier(app.AddEvent)
	if serviceNotice != "" {
		app.AddEvent("error", serviceNotice)
		logger.Warn(serviceNotice)
	}
	if binary == "" {
		app.AddEvent("error", "未找到可运行的 mihomo 内核；已检查程序目录、工作目录和用户缓存："+binaryErr.Error())
	}
	if prefs.Snapshot().Enabled && prefs.Snapshot().UseTun && !tunAuthorized {
		app.AddEvent("error", "TUN 偏好已保存，但系统授权当前不可用；暂未启用 TUN，请在面板重新启用代理或服务以申请系统授权")
	}
	return app, nil
}

func (a *App) Start(ctx context.Context) error {
	a.runCtxMu.Lock()
	a.runCtx = ctx
	a.runCtxMu.Unlock()
	go a.checkPanelNetwork(ctx)
	if a.Core.UsesExternalLifecycle() {
		// Service readiness can take seconds. Keep health/API available throughout.
		go func() {
			if err := a.Core.Start(ctx); err != nil && ctx.Err() == nil {
				a.AddEvent("error", "Mihomo 特权服务暂时无法连接，请在服务开关修复："+err.Error())
			}
		}()
	} else if err := a.Core.Start(ctx); err != nil {
		return err
	}
	settings := a.Prefs.Snapshot()
	if settings.OutboundMode == "global" {
		go func() {
			readyCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			if a.Core.WaitReady(readyCtx) == nil && a.Prefs.Snapshot().OutboundMode == "global" {
				if err := a.ensureGlobalProxy(readyCtx); err != nil {
					a.AddEvent("error", "恢复全局代理选择失败："+err.Error())
				}
			}
		}()
	}
	if settings.Enabled && settings.UseSystemProxy {
		go a.restoreStartupProxy(ctx)
	} else if a.hasSystemProxyBackup() {
		// Restore only a proxy change owned by this client.
		if err := systemproxy.Set(false, filepath.Join(a.Paths.Root, "system-proxy-backup.json")); err != nil {
			a.AddEvent("error", "启动时检查并关闭残留系统代理失败："+err.Error())
		}
	}
	go a.autoUpdateLoop(ctx)
	go a.subscriptionRefreshLoop(ctx)
	go a.captureDNS(ctx)
	return nil
}

func (a *App) restoreStartupProxy(ctx context.Context) {
	readyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := a.Core.WaitReady(readyCtx); err != nil {
		if ctx.Err() == nil {
			a.AddEvent("error", "mihomo 尚未就绪，未启用系统代理："+err.Error())
		}
		return
	}
	if ctx.Err() != nil {
		return
	}
	if systemproxy.Enabled() {
		return
	}
	if err := systemproxy.Set(true, filepath.Join(a.Paths.Root, "system-proxy-backup.json")); err != nil {
		a.Logger.Error("恢复系统代理状态失败", "error", err)
		a.AddEvent("error", "恢复系统代理状态失败："+err.Error())
	}
}

func (a *App) Close() {
	if a.Core.UsesExternalLifecycle() {
		return
	}
	if err := a.PrepareExit(); err != nil {
		a.Logger.Error("恢复系统代理失败", "error", err)
	}
	if err := a.Core.Shutdown(); err != nil {
		a.Logger.Error("关闭 Mihomo 内核失败", "error", err)
		a.AddEvent("error", "关闭 Mihomo 内核失败："+err.Error())
	}
}

// PrepareExit 在普通模式退出前恢复系统代理；服务模式保留后台代理状态。
func (a *App) PrepareExit() error {
	// Exiting the desktop is not a request to stop an OS-managed proxy service.
	if a.Core.UsesExternalLifecycle() {
		return nil
	}
	if !a.hasSystemProxyBackup() {
		return nil
	}
	return systemproxy.Set(false, filepath.Join(a.Paths.Root, "system-proxy-backup.json"))
}

func (a *App) hasSystemProxyBackup() bool {
	_, err := os.Stat(filepath.Join(a.Paths.Root, "system-proxy-backup.json"))
	return err == nil
}

func (a *App) RealStatus(ctx context.Context) Status {
	settings := a.Prefs.Snapshot()
	running, processError := a.Core.Status()
	status := Status{
		Settings:        settings,
		MihomoRunning:   running,
		LastError:       processError,
		RealSystemProxy: systemproxy.Enabled(),
	}
	if running {
		var coreConfig struct {
			Mode     string `json:"mode"`
			AllowLAN bool   `json:"allow-lan"`
			Tun      struct {
				Enable bool   `json:"enable"`
				Device string `json:"device"`
			} `json:"tun"`
		}
		if err := a.Core.Client().GetJSON(ctx, "/configs", &coreConfig); err == nil {
			status.ActualMode = coreConfig.Mode
			status.RealAllowLAN = coreConfig.AllowLAN
			status.RealTun = settings.Enabled && coreConfig.Tun.Enable
			if status.RealTun {
				status.RealTunDevice = coreConfig.Tun.Device
			}
		}
	}
	status.IsActuallyIntercepting = running && (status.RealSystemProxy || status.RealTun)
	return status
}
