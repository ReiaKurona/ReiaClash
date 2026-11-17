package mihomo

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"reia-clash/internal/config"
	"reia-clash/internal/platform/instance"
	"testing"
)

func TestSupervisorHandoverRequiresExclusiveOwnership(t *testing.T) {
	root := t.TempDir()
	prefs, err := config.OpenStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	makeManager := func() *Manager {
		m, err := NewManager(root, filepath.Join(root, "mihomo.yaml"), "", prefs, slog.New(slog.NewTextHandler(io.Discard, nil)))
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	first, second := makeManager(), makeManager()
	if err := first.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer first.Shutdown()
	if err := second.Start(context.Background()); !errors.Is(err, instance.ErrAlreadyRunning) {
		t.Fatalf("second supervisor acquired same kernel: %v", err)
	}
	if second.SupervisorStarted() {
		t.Fatal("failed ownership attempt started a supervisor")
	}
	if err := first.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if err := second.Start(context.Background()); err != nil {
		t.Fatalf("shutdown did not release ownership: %v", err)
	}
	defer second.Shutdown()
}
