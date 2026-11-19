package mihomo

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"reia-clash/internal/config"
)

func TestBackgroundProfileAndRuntimeUpdatesPreserveLatestSubscription(t *testing.T) {
	root := t.TempDir()
	store, err := config.OpenStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	old := []byte("proxies: []\nproxy-groups: []\nrules: [MATCH,DIRECT]\n")
	latest := []byte("proxies: []\nproxy-groups: []\nrules:\n  - DOMAIN,latest.example,DIRECT\n  - MATCH,DIRECT\n")
	if err := os.WriteFile(filepath.Join(root, "profile.yaml"), old, 0600); err != nil {
		t.Fatal(err)
	}
	m := &Manager{dataDir: root, configPath: filepath.Join(root, "mihomo.yaml"), settings: store, secret: "test-secret"}
	errors := make(chan error, 40)
	var group sync.WaitGroup
	for i := 0; i < 20; i++ {
		group.Add(2)
		go func() { defer group.Done(); errors <- m.PersistProfile(latest) }()
		go func() { defer group.Done(); errors <- m.PersistCurrentRuntimeConfig() }()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"profile.yaml", "mihomo.yaml"} {
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || !strings.Contains(string(content), "latest.example") {
			t.Fatalf("latest profile lost in %s: %v", name, err)
		}
	}
}

func TestSubscriptionHotReloadRestoresFilesWhenRejected(t *testing.T) {
	root := t.TempDir()
	store, err := config.OpenStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(root, "profile.yaml")
	runtimePath := filepath.Join(root, "mihomo.yaml")
	m := &Manager{dataDir: root, configPath: runtimePath, settings: store, secret: "test-secret", binary: "unused-on-hot-reload"}
	const oldProfile = "rules:\n  - MATCH,DIRECT\n"
	if err := os.WriteFile(profilePath, []byte(oldProfile), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.PersistCurrentRuntimeConfig(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/version":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut && r.URL.Path == "/configs" && r.URL.Query().Get("force") == "true":
			content, err := os.ReadFile(runtimePath)
			if err != nil || strings.Contains(string(content), "INVALID") {
				http.Error(w, "bad config", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	previousTransport := http.DefaultTransport
	transport := previousTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr != "127.0.0.1:9090" {
			return nil, &net.AddrError{Err: "unexpected controller", Addr: addr}
		}
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	http.DefaultTransport = transport
	defer func() { http.DefaultTransport = previousTransport; transport.CloseIdleConnections() }()

	const validProfile = "rules:\n  - MATCH,REJECT\n"
	if err := m.ImportProfile(t.Context(), []byte(validProfile)); err != nil {
		t.Fatal(err)
	}
	if err := m.ImportProfile(t.Context(), []byte("rules:\n  - INVALID\n")); err == nil {
		t.Fatal("accepted a rejected subscription")
	}
	for _, file := range []string{profilePath, runtimePath} {
		data, err := os.ReadFile(file)
		if err != nil || !strings.Contains(string(data), validProfile) {
			t.Fatalf("failed to restore last good profile in %s: %v", file, err)
		}
	}
}
