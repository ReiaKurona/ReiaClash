package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"reia-clash/internal/agent"
	"reia-clash/internal/config"
	"reia-clash/internal/mihomo"
	"testing"
	"time"
)

type retainedService struct{ stops int }

func (s *retainedService) Start(context.Context) error { return nil }
func (s *retainedService) Status() (bool, string)      { return true, "" }
func (s *retainedService) Restart() error              { return nil }
func (s *retainedService) Shutdown() error             { s.stops++; return nil }

func TestExitAPIDoesNotStopService(t *testing.T) {
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
	service := &retainedService{}
	core.SetLifecycle(service)
	app := &agent.App{Paths: paths, Prefs: prefs, Core: core, Logger: logger, Exit: make(chan struct{})}
	response := httptest.NewRecorder()
	New(app, logger, "").routes().ServeHTTP(response, httptest.NewRequest("POST", "http://127.0.0.1:25500/api/agent/exit", nil))
	if response.Code != 202 {
		t.Fatal(response.Code, response.Body.String())
	}
	select {
	case <-app.Exit:
		t.Fatal("service mode disconnected the WebUI API")
	case <-time.After(250 * time.Millisecond):
	}
	app.Close()
	if service.stops != 0 {
		t.Fatal("exit API/cleanup shut down persistent service")
	}
}
