package mihomo

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPersistAndRestoreProxySelection(t *testing.T) {
	requests := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			io.WriteString(w, `{"proxies":{"Main":{"all":["A","B"],"now":"A"},"Removed":{"all":["A"],"now":"A"}}}`)
		case "PUT":
			var selection struct{ Name string }
			_ = json.NewDecoder(r.Body).Decode(&selection)
			requests <- r.URL.Path + ":" + selection.Name
			w.WriteHeader(204)
		case "DELETE":
			w.WriteHeader(204)
		}
	}))
	defer server.Close()
	original := http.DefaultTransport
	transport := original.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	http.DefaultTransport = transport
	defer func() { http.DefaultTransport = original; transport.CloseIdleConnections() }()
	directory := t.TempDir()
	m := &Manager{dataDir: directory}
	if err := m.SelectProxy(t.Context(), "Main", "B"); err != nil {
		t.Fatal(err)
	}
	if err := m.SelectProxy(t.Context(), "Removed", "C"); err != nil {
		t.Fatal(err)
	}
	<-requests
	<-requests
	restarted := &Manager{dataDir: directory, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	restarted.restoreSelections(t.Context())
	select {
	case got := <-requests:
		if got != "/proxies/Main:B" {
			t.Fatal(got)
		}
	default:
		t.Fatal("user selection was not restored")
	}
	select {
	case got := <-requests:
		t.Fatalf("unavailable node was sent: %s", got)
	default:
	}
	if restarted.RememberedProxy("Removed") != "C" {
		t.Fatal("temporarily unavailable selection was erased")
	}
}
