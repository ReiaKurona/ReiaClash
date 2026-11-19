package agent

import (
	"strings"
	"testing"
)

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
