package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reia-clash/internal/kernelbinary"
	"runtime"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

// Test the full update pipeline with a real, renamed/nested kernel, without network.
func TestStageVersionedReleaseBinary(t *testing.T) {
	source := os.Getenv("REIA_TEST_MIHOMO")
	if source == "" {
		t.Skip("set REIA_TEST_MIHOMO for a real update test")
	}
	payload, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("package/usr/bin/mihomo-" + runtime.GOOS + "-" + runtime.GOARCH + "-test" + binarySuffix())
	if err != nil {
		t.Fatal(err)
	}
	entry.Write(payload)
	unit, _ := writer.Create("package/lib/systemd/system/mihomo.service")
	unit.Write([]byte("[Service]"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(buffer.Bytes())
	assetName := "mihomo-" + runtime.GOOS + "-" + runtime.GOARCH + "-test.zip"
	publication := release{TagName: "test-release", Assets: []asset{{Name: assetName, URL: "https://github.com/MetaCubeX/mihomo/releases/download/test/" + assetName, Digest: "sha256:" + hex.EncodeToString(digest[:])}}}
	releaseBody, _ := json.Marshal(publication)
	manager := NewManager(t.TempDir())
	manager.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		content := buffer.Bytes()
		if request.URL.String() == releaseAPI {
			content = releaseBody
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(content))}, nil
	})}
	_, path, err := manager.Stage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != manager.directory || !strings.Contains(filepath.Base(path), "test-release") {
		t.Fatal(path)
	}
	extracted, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(extracted, payload) {
		t.Fatal("published binary differs from verified package payload")
	}
	if err := kernelbinary.Validate(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(manager.directory, "package")); !os.IsNotExist(err) {
		t.Fatal("package service files were installed")
	}
	if _, err := manager.Queue(t.Context(), manager); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ClearCache(); err != nil {
		t.Fatal(err)
	}
	pending, meow, err := manager.Pending()
	if err != nil || meow || pending.Tag != "test-release" {
		t.Fatalf("invalid pending release: %+v %t %v", pending, meow, err)
	}
	_, staged, err := manager.UnpackPending()
	if err != nil {
		t.Fatal(err)
	}
	if staged != path {
		t.Fatalf("unexpected staged file: %s", staged)
	}
	if err := manager.DiscardPending(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Pending(); !os.IsNotExist(err) {
		t.Fatalf("unexpected pending file: %v", err)
	}

}
