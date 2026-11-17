package mihomo

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestDNSLogStreamUsesDebugAndAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("level") != "debug" || r.Header.Get("Authorization") != "Bearer test" {
			t.Error("incorrect log subscription")
		}
		fmt.Fprintln(w, `{"type":"debug","payload":"[DNS] example.com --> 1.2.3.4 A"}`)
	}))
	defer server.Close()
	client := NewClient("test")
	client.BaseURL = server.URL
	var payloads []string
	if err := client.StreamLogs(context.Background(), func(payload string) { payloads = append(payloads, payload) }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(payloads, []string{"[DNS] example.com --> 1.2.3.4 A"}) {
		t.Fatal(payloads)
	}
}
