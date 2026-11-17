package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reia-clash/internal/mihomo"
	"reia-clash/internal/platform/coreservice"
	"testing"
)

type startupLifecycle struct{ reachable bool }

func (l startupLifecycle) Start(context.Context) error { return nil }
func (l startupLifecycle) Status() (bool, string)      { return false, "" }
func (l startupLifecycle) Restart() error              { return nil }
func (l startupLifecycle) Shutdown() error             { return nil }
func (l startupLifecycle) Probe(context.Context) error {
	if l.reachable {
		return nil
	}
	return errors.New("unreachable")
}

func TestRegisteredServiceNeverFallsBackToLocalSupervisor(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		root := t.TempDir()
		if corrupt {
			if err := os.WriteFile(filepath.Join(root, "core-service.token"), []byte("invalid"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		lifecycle, enabled, _ := startupService(root, coreservice.Registration{Installed: true})
		if !enabled || lifecycle == nil {
			t.Fatal("registered service allowed a second local kernel")
		}
		if !corrupt {
			if _, marked, err := mihomo.LoadServiceLifecycle(root); err != nil || !marked {
				t.Fatalf("missing service marker was not recovered: %v", err)
			}
		}
	}
}

func TestStartupServiceSelection(t *testing.T) {
	for _, tt := range []struct {
		name                        string
		registered, reachable, want bool
	}{
		{"stale marker falls back", false, false, false},
		{"reachable legacy helper retained", false, true, true},
		{"registered stopped helper remains repairable", true, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := usableStartupService(startupLifecycle{tt.reachable}, tt.registered); got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}
