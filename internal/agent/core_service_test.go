package agent

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reia-clash/internal/config"
	"reia-clash/internal/mihomo"
	"reia-clash/internal/platform/coreservice"
	"strings"
	"testing"
)

type ownedService struct{ stops int }

func (s *ownedService) Start(context.Context) error { return nil }
func (s *ownedService) Status() (bool, string)      { return true, "" }
func (s *ownedService) Restart() error              { return nil }
func (s *ownedService) Shutdown() error             { s.stops++; return nil }

type servicePlatformFake struct {
	installed     bool
	uninstalls    int
	failure       error
	keepInstalled bool
}

func (s *servicePlatformFake) Supported() bool { return true }
func (s *servicePlatformFake) RegistrationStatus() coreservice.Registration {
	return coreservice.Registration{Installed: s.installed, Running: s.installed}
}
func (s *servicePlatformFake) RequestInstall(context.Context, string) error { return nil }
func (s *servicePlatformFake) RequestUninstall(context.Context, string) error {
	s.uninstalls++
	if s.failure != nil {
		return s.failure
	}
	if !s.keepInstalled {
		s.installed = false
	}
	return nil
}

func TestUninstallCannotAcknowledgeLiveRegistration(t *testing.T) {
	app := serviceTestApp(t)
	app.Core.SetLifecycle(&ownedService{})
	app.services = &servicePlatformFake{installed: true, keepInstalled: true}
	if err := app.SetCoreService(context.Background(), false); err == nil {
		t.Fatal("live OS registration was reported removed")
	}
	if !app.Core.UsesExternalLifecycle() || app.Core.SupervisorStarted() {
		t.Fatal("started a local kernel alongside a live service")
	}
}

func serviceTestApp(t *testing.T) *App {
	t.Helper()
	paths, err := config.PathsAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	prefs, err := config.OpenStore(paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	core, err := mihomo.NewManager(paths.Root, filepath.Join(paths.Root, "mihomo.yaml"), "", prefs, logger)
	if err != nil {
		t.Fatal(err)
	}
	return &App{Paths: paths, Prefs: prefs, Core: core, Logger: logger, Exit: make(chan struct{})}
}

func TestDesktopExitPreservesServiceAndProxyBackup(t *testing.T) {
	app := serviceTestApp(t)
	service := &ownedService{}
	app.Core.SetLifecycle(service)
	backup := filepath.Join(app.Paths.Root, "system-proxy-backup.json")
	if err := os.WriteFile(backup, []byte("must not be consumed during UI exit"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := app.PrepareExit(); err != nil {
		t.Fatal(err)
	}
	app.Close()
	if service.stops != 0 {
		t.Fatal("UI exit stopped OS-managed kernel")
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatal("UI exit restored/removed the running service's proxy backup")
	}
}

func TestUninstallChecksOSRegistrationAndDisablesTun(t *testing.T) {
	app := serviceTestApp(t)
	platform := &servicePlatformFake{installed: true}
	app.services = platform
	if _, err := app.Prefs.Update(func(s *config.Settings) error {
		s.Enabled = true
		s.UseTun = true
		s.UseSystemProxy = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.SetCoreService(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	defer app.Core.Shutdown()
	if platform.uninstalls != 1 {
		t.Fatal("real OS registration was ignored because agent used local mode")
	}
	settings := app.Prefs.Snapshot()
	if settings.UseTun || settings.Enabled || app.Core.UsesExternalLifecycle() {
		t.Fatal("uninstall retained privileged TUN state")
	}
	rendered, err := os.ReadFile(filepath.Join(app.Paths.Root, "mihomo.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rendered), "enable: false") {
		t.Fatal("fallback config still enables TUN")
	}
}

func TestCancelledUninstallKeepsServiceAttached(t *testing.T) {
	app := serviceTestApp(t)
	service := &ownedService{}
	app.Core.SetLifecycle(service)
	app.services = &servicePlatformFake{installed: true, failure: errors.New("authorization cancelled")}
	if _, err := app.Prefs.Update(func(s *config.Settings) error { s.UseTun = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := app.SetCoreService(context.Background(), false); err == nil {
		t.Fatal("uninstall failure was hidden")
	}
	if !app.Core.UsesExternalLifecycle() || !app.Prefs.Snapshot().UseTun || service.stops != 0 {
		t.Fatal("cancelled uninstall detached/stopped the active service")
	}
}

func TestDesktopFullExitUninstallsWithoutStartingLocalKernel(t *testing.T) {
	app := serviceTestApp(t)
	platform := &servicePlatformFake{installed: true}
	service := &ownedService{}
	app.services = platform
	app.Core.SetLifecycle(service)
	if _, err := app.Prefs.Update(func(s *config.Settings) error { s.Enabled, s.UseTun = true, true; return nil }); err != nil {
		t.Fatal(err)
	}
	background, err := app.PrepareDesktopExit(t.Context(), false)
	if err != nil || !background || platform.uninstalls != 0 || service.stops != 0 {
		t.Fatalf("background exit touched the service: background=%t, err=%v, uninstalls=%d, stops=%d", background, err, platform.uninstalls, service.stops)
	}
	background, err = app.PrepareDesktopExit(t.Context(), true)
	if err != nil || background || platform.uninstalls != 1 || app.Core.UsesExternalLifecycle() || app.Core.SupervisorStarted() {
		t.Fatalf("full exit did not end service mode cleanly: background=%t, err=%v, uninstalls=%d", background, err, platform.uninstalls)
	}
	if state := app.Prefs.Snapshot(); state.Enabled || state.UseTun {
		t.Fatal("full exit left privileged TUN configured")
	}
	if err := app.SetCoreService(t.Context(), true); err == nil {
		t.Fatal("accepted a service start after committing to exit")
	}
}

func TestDesktopFullExitLeavesServiceOnCancelOrFailedRemoval(t *testing.T) {
	for _, platform := range []*servicePlatformFake{{installed: true, failure: errors.New("authorization cancelled")}, {installed: true, keepInstalled: true}} {
		app := serviceTestApp(t)
		service := &ownedService{}
		app.services = platform
		app.Core.SetLifecycle(service)
		if _, err := app.PrepareDesktopExit(t.Context(), true); err == nil {
			t.Fatal("failed service removal reported as successful exit")
		}
		if app.exiting || !app.Core.UsesExternalLifecycle() || app.Core.SupervisorStarted() || service.stops != 0 {
			t.Fatal("failed service removal changed lifecycle or quit the Agent")
		}
	}
}
