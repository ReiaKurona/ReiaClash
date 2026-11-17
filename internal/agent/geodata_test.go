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
	"strings"
	"testing"
)

type geoLifecycle struct {
	restarts int
	fail     bool
}

func (l *geoLifecycle) Start(context.Context) error { return nil }
func (l *geoLifecycle) Status() (bool, string)      { return true, "" }
func (l *geoLifecycle) Shutdown() error             { return nil }
func (l *geoLifecycle) Restart() error {
	l.restarts++
	if l.fail {
		return errors.New("helper unavailable")
	}
	return nil
}

func TestGeoLoaderServiceRestartAndRollback(t *testing.T) {
	paths, err := config.PathsAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := config.OpenStore(paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	core, err := mihomo.NewManager(paths.Root, filepath.Join(paths.Root, "mihomo.yaml"), "", store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	helper := &geoLifecycle{}
	core.SetLifecycle(helper)
	app := &App{Paths: paths, Prefs: store, Core: core}
	restarted, err := app.SetGeoLowMemory(context.Background(), false)
	if err != nil || !restarted || helper.restarts != 1 {
		t.Fatalf("restart=%v count=%d error=%v", restarted, helper.restarts, err)
	}
	helper.fail = true
	if _, err := app.SetGeoLowMemory(context.Background(), true); err == nil {
		t.Fatal("helper failure was hidden")
	}
	if store.Snapshot().GeoLowMemory {
		t.Fatal("failed setting was not rolled back")
	}
	rendered, err := os.ReadFile(filepath.Join(paths.Root, "mihomo.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rendered), "geodata-loader: standard") {
		t.Fatal("generated config was not rolled back")
	}
}
