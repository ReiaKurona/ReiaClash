package mihomo

import (
	"context"
	"encoding/base64"
	"encoding/json"
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

func TestRunningConfigSourceFollowsLiveBinary(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "mihomo.yaml")
	m := &Manager{configPath: configPath, running: true, runningBinary: "meow.exe"}
	for name, content := range map[string]string{"mihomo.yaml": "# mihomo", "meow.yaml": "# meow-rs"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	name, content, err := m.RunningConfigSource()
	if err != nil || name != "meow.yaml" || string(content) != "# meow-rs" {
		t.Fatalf("runtime source did not follow the running binary: %q, %q, %v", name, content, err)
	}
	m.runningBinary = "mihomo.exe"
	name, content, err = m.RunningConfigSource()
	if err != nil || name != "mihomo.yaml" || string(content) != "# mihomo" {
		t.Fatalf("runtime source did not follow the running binary: %q, %q, %v", name, content, err)
	}
}

func TestSubscriptionHotReloadRestoresFilesWhenRejected(t *testing.T) {
	t.Run("mihomo", func(t *testing.T) { testSubscriptionHotReload(t, "mihomo.exe", false) })
	t.Run("meow-rs", func(t *testing.T) { testSubscriptionHotReload(t, "meow.exe", true) })
}

func testSubscriptionHotReload(t *testing.T, binary string, meow bool) {
	t.Helper()
	root := t.TempDir()
	store, err := config.OpenStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(root, "profile.yaml")
	m := &Manager{dataDir: root, configPath: filepath.Join(root, "mihomo.yaml"), settings: store, secret: "test-secret", binary: binary}
	runtimePath := m.runtimePath(binary)
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
			var payload struct {
				Path    string `json:"path"`
				Payload string `json:"payload"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Payload == "" {
				http.Error(w, "request body must contain path or payload", http.StatusBadRequest)
				return
			}
			if payload.Path != "" {
				http.Error(w, "path is not subpath of home directory or SAFE_PATHS", http.StatusBadRequest)
				return
			}
			decoded := []byte(payload.Payload)
			if meow {
				var err error
				decoded, err = base64.StdEncoding.DecodeString(payload.Payload)
				if err != nil {
					http.Error(w, "payload is not valid base64", http.StatusBadRequest)
					return
				}
			}
			content, err := os.ReadFile(runtimePath)
			if err != nil || string(content) != string(decoded) || strings.Contains(string(decoded), "INVALID") {
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
	if err := m.ReloadRuntime(t.Context()); err != nil {
		t.Fatalf("reload must also use inline configuration: %v", err)
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
