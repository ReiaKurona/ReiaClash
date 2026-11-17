package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reia-clash/internal/agent"
	"reia-clash/internal/config"
	"reia-clash/internal/mihomo"
	"strings"
	"testing"
)

func TestHandshakePairsFullURLAndRedirects(t *testing.T) {
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
	app := &agent.App{Paths: paths, Prefs: prefs, Core: core}
	handler := New(app, logger, "").withCORS(New(app, logger, "").routes())
	pair := func(origin string, payload map[string]string, code int) {
		t.Helper()
		body, _ := json.Marshal(payload)
		request := httptest.NewRequest("POST", "http://127.0.0.1:25500/api/handshake", strings.NewReader(string(body)))
		request.Header.Set("Origin", origin)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != code {
			t.Fatalf("%d: %s", response.Code, response.Body.String())
		}
	}
	full := "https://next.rka.jp/theme#/nodes"
	fileURL := "file:///C:/Users/tcs15/Desktop/ReiaClash/test-webui.html"
	pair("null", map[string]string{"panel_origin": "null"}, 200)
	if state := app.PanelState(); state.Paired || state.OpenURL != agent.DefaultPanelURL {
		t.Fatal("missing file URL invented pairing", state)
	}
	pair("null", map[string]string{"panel_origin": "null", "panel_url": fileURL}, 200)
	if state := app.PanelState(); !state.Paired || state.PanelURL != fileURL || state.OpenURL != fileURL {
		t.Fatal("file pairing was replaced by a fallback URL", state)
	}
	fileStatus := httptest.NewRecorder()
	handler.ServeHTTP(fileStatus, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:25500/api/agent/panel", nil))
	var fileState agent.PanelState
	if err := json.Unmarshal(fileStatus.Body.Bytes(), &fileState); err != nil {
		t.Fatal(err)
	}
	if fileState.PanelURL != fileURL || fileState.OpenURL != fileURL || !fileState.Paired {
		t.Fatal("status API did not return actual file location", fileState)
	}
	fileRedirect := httptest.NewRecorder()
	handler.ServeHTTP(fileRedirect, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:25500/panel", nil))
	if fileRedirect.Code != 302 || fileRedirect.Header().Get("Location") != fileURL {
		t.Fatal("redirect lost file URL", fileRedirect.Header())
	}
	pair("https://next.rka.jp", map[string]string{"panel_origin": "https://next.rka.jp", "panel_url": full}, 200)
	if app.PanelState().PanelURL != full {
		t.Fatal(app.PanelState())
	}
	pairedRedirect := httptest.NewRecorder()
	handler.ServeHTTP(pairedRedirect, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:25500/panel", nil))
	if pairedRedirect.Code != 302 || pairedRedirect.Header().Get("Location") != full {
		t.Fatal("redirect lost real panel URL", pairedRedirect.Header())
	}
	pair("https://next.rka.jp", map[string]string{"panel_origin": "https://next.rka.jp", "panel_url": "https://other.example/panel"}, 403)
	if app.PanelState().PanelURL != full {
		t.Fatal("rejected handshake changed pairing")
	}
	pair("null", map[string]string{"panel_origin": "null", "panel_url": fileURL}, 200)
	if app.PanelState().PanelURL != fileURL {
		t.Fatal(app.PanelState())
	}
	pair("https://next.rka.jp", map[string]string{"panel_origin": "https://next.rka.jp"}, 200)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:25500/panel", nil))
	if response.Code != 302 || response.Header().Get("Location") != "https://next.rka.jp" {
		t.Fatal(response.Code, response.Header())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("runtime redirect may be cached")
	}
}
