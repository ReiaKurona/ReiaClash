package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
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

type networkTransport func(*http.Request) (*http.Response, error)

func (f networkTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestConnectionDNSAndGroupOrderAPIs(t *testing.T) {
	paths, err := config.PathsAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store, err := config.OpenStore(paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	core, err := mihomo.NewManager(paths.Root, filepath.Join(paths.Root, "mihomo.yaml"), "", store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Profile, []byte("proxy-groups: [{name: Z}, {name: A}]"), 0600); err != nil {
		t.Fatal(err)
	}
	app := &agent.App{Paths: paths, Prefs: store, Core: core}
	server := New(app, slog.Default(), "")
	handler := server.routes()
	original := http.DefaultTransport
	defer func() { http.DefaultTransport = original }()
	var forwarded []string
	http.DefaultTransport = networkTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "127.0.0.1:9090" {
			return nil, fmt.Errorf("unexpected address %s", r.URL.Host)
		}
		if r.Header.Get("Authorization") != "Bearer "+core.Secret() {
			t.Fatal("missing kernel authentication")
		}
		forwarded = append(forwarded, r.Method+" "+r.URL.Path)
		data := `{}`
		status := 200
		switch r.URL.Path {
		case "/connections":
			data = `{"connections":[{"id":"connection-1","metadata":{"host":"example.com"}}]}`
		case "/connections/connection-1":
			if r.Method != "DELETE" {
				t.Fatal(r.Method)
			}
			status = 204
		case "/dns/query":
			if r.URL.Query().Get("name") != "example.com" || r.URL.Query().Get("type") != "A" {
				t.Fatal(r.URL)
			}
			data = `{"Status":0,"Answer":[{"name":"example.com","data":"1.2.3.4"}]}`
		case "/proxies":
			data = `{"proxies":{"A":{"type":"Selector"},"Z":{"type":"Selector"}}}`
		default:
			t.Fatalf("unexpected kernel path %s", r.URL.Path)
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(data))}, nil
	})
	request := func(method, path string, code int) map[string]any {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, "http://127.0.0.1:25500"+path, nil))
		if response.Code != code {
			t.Fatalf("%s: %d %s", path, response.Code, response.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if got := request("GET", "/api/core/proxies", 200)["group_order"].([]any); got[0] != "Z" || got[1] != "A" {
		t.Fatal(got)
	}
	request("GET", "/api/core/connections", 200)
	request("DELETE", "/api/core/connections/connection-1", 200)
	request("DELETE", "/api/core/connections", 200)
	request("GET", "/api/core/dns/query?name=example.com&type=A", 200)
	if len(appDNSRecords(app)) != 1 {
		t.Fatal("DNS query was not recorded")
	}
	request("GET", "/api/core/dns/query?name=a%26b&type=INVALID", 400)
	request("GET", "/api/core/dns/query?name=https://example.com&type=A", 400)
	request("GET", "/api/core/dns/records", 200)
	request("DELETE", "/api/core/dns/records", 200)
	if len(appDNSRecords(app)) != 0 {
		t.Fatal("DNS history not cleared")
	}
	if len(forwarded) != 5 {
		t.Fatal(forwarded)
	}
	lifecycle := &restartTestLifecycle{}
	core.SetLifecycle(lifecycle)
	request("POST", "/api/core/restart", 202)
	request("POST", "/api/agent/restart", 202)
	if lifecycle.count != 2 {
		t.Fatal("restart aliases did not reach lifecycle")
	}
}

type restartTestLifecycle struct{ count int }

func (l *restartTestLifecycle) Start(context.Context) error { return nil }
func (l *restartTestLifecycle) Status() (bool, string)      { return true, "" }
func (l *restartTestLifecycle) Shutdown() error             { return nil }
func (l *restartTestLifecycle) Restart() error              { l.count++; return nil }

func appDNSRecords(app *agent.App) []agent.DNSRecord { records, _ := app.DNSHistory(); return records }
