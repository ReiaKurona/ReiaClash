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

func TestSubscriptionSourceIsLocalEscapedAndUncached(t *testing.T) {
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
	if err := os.WriteFile(paths.Profile, []byte("# <script>test-only</script>\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s := New(&agent.App{Core: core}, logger, "")
	handler := s.withCORS(s.routes())
	for _, headers := range []map[string]string{
		{"Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document"},
		{"Origin": "https://example.test", "Sec-Fetch-Mode": "cors"},
		{"Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "iframe"},
	} {
		req := httptest.NewRequest("GET", "http://127.0.0.1:25500"+subscriptionSourcePath, nil)
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Access-Control-Allow-Origin") != "" ||
			w.Header().Get("X-Frame-Options") != "DENY" {
			t.Fatal("missing subscription source privacy headers")
		}
		if headers["Sec-Fetch-Dest"] == "document" {
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "&lt;script&gt;test-only&lt;/script&gt;") ||
				!strings.Contains(w.Body.String(), "?view=runtime") {
				t.Fatal("local subscription source is missing or unescaped")
			}
		} else if w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), "test-only") {
			t.Fatal("non-document request obtained subscription source")
		}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:25500"+subscriptionSourcePath+"?view=runtime", nil))
	if w.Code != http.StatusServiceUnavailable || strings.Contains(w.Body.String(), "test-only") {
		t.Fatal("runtime view must report no active core")
	}
}

func TestSubscriptionStatusDoesNotExposeSavedURL(t *testing.T) {
	paths, err := config.PathsAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	prefs, err := config.OpenStore(paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	_, err = prefs.Update(func(s *config.Settings) error {
		s.SubscriptionURL = "https://subscription.example/profile?token=private-value"
		s.SubscriptionUpdatedAt = 123
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	s := New(&agent.App{Prefs: prefs}, slog.New(slog.NewTextHandler(io.Discard, nil)), "")
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:25500/api/subscription/status", nil))
	var status map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || status["has_url"] != true || status["refresh_interval_hours"] != float64(12) || strings.Contains(w.Body.String(), "private-value") {
		t.Fatal(w.Code, w.Body.String())
	}
}
