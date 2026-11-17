package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reia-clash/internal/config"
	"reia-clash/internal/mihomo"
)

// Status may still report false while the real controller is already ready.
// Hot settings must reach that controller and must not invoke -t/downloads.
func TestHotSettingsReachStartingController(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:9090")
	if err != nil {
		t.Skip("controller port already in use")
	}
	var mode string
	var lan bool
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/version":
			io.WriteString(w, `{}`)
		case r.Method == "PATCH" && r.URL.Path == "/configs":
			var patch struct {
				Mode     string `json:"mode"`
				AllowLAN bool   `json:"allow-lan"`
			}
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			mode, lan = patch.Mode, patch.AllowLAN
			w.WriteHeader(204)
		case r.Method == "DELETE" && r.URL.Path == "/connections":
			w.WriteHeader(204)
		default:
			http.Error(w, "unexpected", 400)
		}
	})}
	go server.Serve(listener)
	defer server.Close()
	paths, err := config.PathsAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	prefs, err := config.OpenStore(paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	core, err := mihomo.NewManager(paths.Root, filepath.Join(paths.Root, "mihomo.yaml"), "", prefs, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	app := &App{Paths: paths, Prefs: prefs, Core: core}
	direct := "direct"
	if err := app.UpdateSettings(context.Background(), SettingsPatch{OutboundMode: &direct}); err != nil {
		t.Fatal(err)
	}
	if mode != direct {
		t.Fatalf("mode was not applied: %q", mode)
	}
	enabled := true
	if err := app.UpdateSettings(context.Background(), SettingsPatch{AllowLAN: &enabled}); err != nil {
		t.Fatal(err)
	}
	if !lan {
		t.Fatal("LAN setting was not applied")
	}
	data, err := os.ReadFile(filepath.Join(paths.Root, "mihomo.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "mode: direct") || !strings.Contains(string(data), "allow-lan: true") {
		t.Fatalf("runtime settings were not persisted: %s", data)
	}
}
