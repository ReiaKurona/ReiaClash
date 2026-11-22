package httpapi

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reia-clash/internal/agent"
	"reia-clash/internal/config"
	"reia-clash/internal/update"
)

func TestAgentMaintenanceAPI(t *testing.T) {
	paths, err := config.PathsAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := &agent.App{Paths: paths, Updater: update.NewManager(paths.UpdateDir)}
	server := New(app, nil, "")
	var opened []string
	server.openDirectory = func(path string) error { opened = append(opened, path); return nil }
	handler := server.withCORS(server.routes())
	request := func(method, path, body string, code int) string {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, "http://127.0.0.1:25500"+path, strings.NewReader(body)))
		if response.Code != code {
			t.Fatalf("%s %s: %d %s", method, path, response.Code, response.Body.String())
		}
		return response.Body.String()
	}
	for _, kind := range []string{"cache", "config"} {
		request("POST", "/api/agent/directory/open", `{"directory":"`+kind+`"}`, 202)
	}
	if len(opened) != 2 || opened[0] != paths.UpdateDir || opened[1] != paths.Root {
		t.Fatal(opened)
	}
	for _, body := range []string{`{}`, `{"directory":"../../"}`, `{"directory":"logs"}`, `{"directory":"config","path":"/tmp"}`, `{"directory":"config"} {}`, `{"directory":true}`} {
		request("POST", "/api/agent/directory/open", body, 400)
	}
	if len(opened) != 2 {
		t.Fatal("invalid directory launched")
	}
	server.openDirectory = func(string) error { return errors.New("no desktop opener") }
	request("POST", "/api/agent/directory/open", `{"directory":"config"}`, 500)
	if err := os.WriteFile(filepath.Join(paths.Root, "agent.log"), []byte("log"), 0600); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(request("POST", "/api/agent/logs/clear", "", 200), `"log_files":1`) {
		t.Fatal("missing cleanup counts")
	}
	if err := os.WriteFile(filepath.Join(paths.UpdateDir, "update.pack"), []byte("pack"), 0600); err != nil {
		t.Fatal(err)
	}
	var stats struct {
		Directories map[string]agent.DirectorySize `json:"directories"`
	}
	if err := json.Unmarshal([]byte(request("GET", "/api/agent/directory/sizes", "", 200)), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.Directories["cache"].Bytes != 4 || stats.Directories["config"].Bytes != 0 || len(stats.Directories) != 4 {
		t.Fatalf("invalid size totals: %+v", stats)
	}
	request("POST", "/api/agent/config/subscription/clear", `{"confirm":false}`, 400)
	app.RecordDNS(agent.DNSRecord{Name: "example.com"})
	if !strings.Contains(request("POST", "/api/agent/cache/clear", "", 200), `"dns_records":1`) {
		t.Fatal("missing cache counts")
	}
	for _, path := range []string{"/api/agent/cache/clear", "/api/agent/logs/clear", "/api/agent/directory/open"} {
		request("GET", path, "", 404)
	}
}
