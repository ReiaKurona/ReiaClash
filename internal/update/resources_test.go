package update

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestResourceSyncPreservesExistingFileOnInvalidDownload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "GeoSite.dat")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(dir)
	payload := []byte("<html>error</html>")
	manager.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "github.com" || r.URL.Path != "/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat" {
			t.Errorf("unexpected source %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(payload))}, nil
	})}
	resource := GeoResource{"geosite", "GeoSite.dat", "geosite.dat"}
	if err := manager.SyncGeoResource(t.Context(), dir, resource); err == nil {
		t.Fatal("accepted invalid data")
	}
	if data, _ := os.ReadFile(path); string(data) != "old" {
		t.Fatal("replaced existing file")
	}
	payload = append([]byte{10, 128, 8}, bytes.Repeat([]byte{1}, 1024)...)
	if err := manager.SyncGeoResource(t.Context(), dir, resource); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); !bytes.Equal(data, payload) {
		t.Fatal("did not publish validated asset")
	}
}
