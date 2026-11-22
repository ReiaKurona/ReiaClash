package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"reia-clash/internal/config"
	"reia-clash/internal/mihomo"
)

// Status may still report false while the real controller is already ready.
// Hot settings must reach that controller and must not invoke -t/downloads.
func TestHotSettingsReachStartingController(t *testing.T) {
	var controllerMu sync.Mutex
	var mode string
	var lan bool
	var tun struct {
		Enable bool   `json:"enable"`
		Stack  string `json:"stack"`
		Device string `json:"device"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		controllerMu.Lock()
		defer controllerMu.Unlock()
		switch {
		case r.Method == "GET" && r.URL.Path == "/version":
			io.WriteString(w, `{}`)
		case r.Method == "GET" && r.URL.Path == "/configs":
			_ = json.NewEncoder(w).Encode(map[string]any{"tun": tun})
		case r.Method == "PATCH" && r.URL.Path == "/configs":
			var patch struct {
				Mode     *string         `json:"mode"`
				AllowLAN *bool           `json:"allow-lan"`
				Tun      json.RawMessage `json:"tun"`
			}
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			if patch.Mode != nil {
				mode = *patch.Mode
			}
			if patch.AllowLAN != nil {
				lan = *patch.AllowLAN
			}
			if patch.Tun != nil {
				if err := json.Unmarshal(patch.Tun, &tun); err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
				if tun.Stack == "gvisor" {
					tun.Enable = false // Mihomo can acknowledge PATCH even when listener creation failed.
				}
			}
			w.WriteHeader(204)
		case r.Method == "DELETE" && r.URL.Path == "/connections":
			w.WriteHeader(204)
		default:
			http.Error(w, "unexpected", 400)
		}
	}))
	defer server.Close()
	originalTransport := http.DefaultTransport
	transport := originalTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "127.0.0.1:9090" {
			return nil, &net.AddrError{Err: "unexpected controller", Addr: address}
		}
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	http.DefaultTransport = transport
	defer func() { http.DefaultTransport = originalTransport; transport.CloseIdleConnections() }()
	t.Run("verify tun listener", func(t *testing.T) {
		app := &App{Core: &mihomo.Manager{}}
		settings := config.Defaults()
		settings.Enabled, settings.TunAuthorized = true, true
		if err := app.patchCoreSettings(t.Context(), settings, false, false, true); err != nil {
			t.Fatal(err)
		}
		settings.TunStack = "gvisor"
		if err := app.patchCoreSettings(t.Context(), settings, false, false, true); err == nil {
			t.Fatal("accepted a successful HTTP response for an inactive TUN listener")
		}
	})
	paths, err := config.PathsAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	prefs, err := config.OpenStore(paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	core, err := mihomo.NewManager(paths.Root, filepath.Join(paths.Root, "mihomo.yaml"), "", prefs, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if os.IsPermission(err) {
		t.Skipf("cannot resolve the test executable directory for the settings transaction: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	app := &App{Paths: paths, Prefs: prefs, Core: core}
	direct := "direct"
	if err := app.UpdateSettings(context.Background(), SettingsPatch{OutboundMode: &direct}); err != nil {
		t.Fatal(err)
	}
	controllerMu.Lock()
	appliedMode := mode
	controllerMu.Unlock()
	if appliedMode != direct {
		t.Fatalf("mode was not applied: %q", appliedMode)
	}
	enabled := true
	if err := app.UpdateSettings(context.Background(), SettingsPatch{AllowLAN: &enabled}); err != nil {
		t.Fatal(err)
	}
	controllerMu.Lock()
	appliedLAN := lan
	controllerMu.Unlock()
	if !appliedLAN {
		t.Fatal("LAN setting was not applied")
	}
	data, err := os.ReadFile(filepath.Join(paths.Root, "mihomo.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "mode: direct") || !strings.Contains(string(data), "allow-lan: true") {
		t.Fatalf("runtime settings were not persisted: %s", data)
	}
	prefs.SetRuntimeTunAuthorization(true)
	currentTun := func() (bool, string) {
		controllerMu.Lock()
		defer controllerMu.Unlock()
		return tun.Enable, tun.Stack
	}
	if err := app.SetEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if enabled, stack := currentTun(); !enabled || stack != "mips" {
		t.Fatalf("first enable did not hot apply TUN with mips: enabled=%t stack=%s", enabled, stack)
	}
	controllerMu.Lock()
	tun.Enable = false
	controllerMu.Unlock()
	if err := app.SetEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if enabled, _ := currentTun(); !enabled {
		t.Fatal("repeating enable did not restore a stopped listener with enabled preferences")
	}
	unsupportedStack := "gvisor"
	if err := app.UpdateSettings(context.Background(), SettingsPatch{TunStack: &unsupportedStack}); err == nil {
		t.Fatal("accepted an inactive TUN listener")
	}
	if enabled, stack := currentTun(); !enabled || stack != "mips" || prefs.Snapshot().TunStack != "mips" {
		t.Fatalf("TUN patch was not rolled back: enabled=%t stack=%s", enabled, stack)
	}
	if err := app.SetEnabled(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if enabled, _ := currentTun(); enabled {
		t.Fatal("pause did not hot disable TUN")
	}
	controllerMu.Lock()
	tun.Enable = true
	controllerMu.Unlock()
	if err := app.SetEnabled(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if enabled, _ := currentTun(); enabled {
		t.Fatal("repeating pause did not stop an active listener with paused preferences")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prefs.Update(func(s *config.Settings) error {
		s.ActiveBinary = executable
		s.UseTun, s.UseSystemProxy, s.TunStack = false, false, "gvisor"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.SetEnabled(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	settings := prefs.Snapshot()
	if enabled, stack := currentTun(); !enabled || stack != "mips" || !settings.Enabled || !settings.UseTun || settings.UseSystemProxy {
		t.Fatalf("empty interception config did not use the default TUN settings: enabled=%t stack=%s", enabled, stack)
	}

}
