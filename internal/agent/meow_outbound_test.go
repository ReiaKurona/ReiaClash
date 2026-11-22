package agent

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOutboundProbeRequiresSuccessfulHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/working" {
			w.WriteHeader(204)
			return
		}
		w.WriteHeader(502)
	}))
	defer server.Close()
	if err := probeOutboundURLs(t.Context(), server.Client(), []string{server.URL + "/failed"}); err == nil {
		t.Fatal("proxy listener alone was accepted")
	}
	if err := probeOutboundURLs(t.Context(), server.Client(), []string{server.URL + "/working", server.URL + "/failed"}); err != nil {
		t.Fatal(err)
	}
}
