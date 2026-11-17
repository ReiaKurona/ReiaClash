package mihomo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"reia-clash/internal/config"
)

func TestBuildConfig_CORSAndPNA(t *testing.T) {
	settings := config.Defaults()
	settings.Enabled = true
	settings.UseTun = true
	settings.TunAuthorized = true

	cfg, err := BuildConfig([]byte("proxies: []\nrules:\n  - MATCH,DIRECT\n"), settings, "mysecret")
	if err != nil {
		t.Fatalf("BuildConfig 失败: %v", err)
	}

	content := string(cfg)
	if !strings.Contains(content, "external-controller-cors:") {
		t.Errorf("配置缺少 external-controller-cors 配置")
	}
	if !strings.Contains(content, "allow-private-network: true") {
		t.Errorf("配置缺少 allow-private-network: true")
	}
	if !strings.Contains(content, "tun:") || !strings.Contains(content, "enable: true") {
		t.Errorf("tun 应当在授权且开启时为 true")
	}
}

func TestClient_CloseConnections(t *testing.T) {
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/connections" {
			deleted = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := &Client{
		BaseURL: "http://127.0.0.1" + server.URL[len("http://127.0.0.1"):], // keep 127.0.0.1
		Secret:  "test-secret",
		HTTP:    server.Client(),
	}

	err := client.CloseConnections(context.Background())
	if err != nil {
		t.Fatalf("CloseConnections 失败: %v", err)
	}
	if !deleted {
		t.Errorf("应当向 /connections 发送 DELETE 请求")
	}
}
