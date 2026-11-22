package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDualStackTraceKeepsIPv4WhenIPv6Fails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v6":
			_, _ = w.Write([]byte("ip=2606:4700::1111\n"))
		default:
			_, _ = w.Write([]byte("ip=1.1.1.1\n"))
		}
	}))
	defer server.Close()
	for _, probe := range []struct{ path, want string }{{"/v6", "2606:4700::1111"}, {"/v4", ""}} {
		got, err := lookupDualStackOutboundIP(t.Context(), server.Client(), server.URL+"/v4", server.URL+probe.path)
		if err != nil || got.IP != "1.1.1.1" || got.IPv6 == nil || *got.IPv6 != probe.want {
			t.Fatalf("unexpected dual-stack trace for %s: %+v, %v", probe.path, got, err)
		}
	}
}

func TestParseCloudflareOutboundTrace(t *testing.T) {
	trace := "fl=42\r\nip=2606:4700::1111\r\nloc=jp\r\ncolo=nrt\r\nuag=ignored=value\r\n"
	data, err := parseOutboundTrace(strings.NewReader(trace))
	if err != nil || data.IP != "2606:4700::1111" || data.CountryCode != "JP" || data.Colo != "NRT" {
		t.Fatalf("invalid trace result %v, %v", data, err)
	}
	for _, trace := range []string{"ip=127.0.0.1", "ip=100.64.0.10", "ip=::1", "ip=fd00::123", "<html>Blocked</html>"} {
		if _, err := parseOutboundTrace(strings.NewReader(trace)); err == nil {
			t.Fatalf("accepted malformed trace %q", trace)
		}
	}
}
