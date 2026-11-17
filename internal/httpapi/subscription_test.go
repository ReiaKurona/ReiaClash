package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"reia-clash/internal/agent"
	"reia-clash/internal/config"
	"strings"
	"testing"
)

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
