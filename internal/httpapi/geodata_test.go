package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reia-clash/internal/agent"
	"reia-clash/internal/config"
	"reia-clash/internal/mihomo"
	"strings"
	"testing"
)

func TestGeoLowMemoryAPI(t *testing.T) {
	paths, err := config.PathsAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := config.OpenStore(paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	core, err := mihomo.NewManager(paths.Root, filepath.Join(paths.Root, "mihomo.yaml"), "", store, logger)
	if err != nil {
		t.Fatal(err)
	}
	server := New(&agent.App{Paths: paths, Prefs: store, Core: core}, logger, "")
	handler := server.routes()
	request := func(method, body string, code int) map[string]any {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, "http://127.0.0.1:25500/api/agent/geo-low-memory", strings.NewReader(body)))
		if response.Code != code {
			t.Fatalf("got %d: %s", response.Code, response.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if result := request(http.MethodGet, "", 200); result["enabled"] != true || result["geodata_loader"] != "memconservative" {
		t.Fatal(result)
	}
	for _, body := range []string{`{}`, `{"enable":null}`, `{"enable":"false"}`} {
		request(http.MethodPost, body, 400)
	}
	result := request(http.MethodPost, `{"enable":false}`, 200)
	if result["enabled"] != false || result["geodata_loader"] != "standard" || result["restart_requested"] != false {
		t.Fatal(result)
	}
	generated, err := os.ReadFile(filepath.Join(paths.Root, "mihomo.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), "geodata-loader: standard") {
		t.Fatal(string(generated))
	}
	result = request(http.MethodPost, `{"enable":true}`, 200)
	if result["enabled"] != true || result["geodata_loader"] != "memconservative" {
		t.Fatal(result)
	}
}
