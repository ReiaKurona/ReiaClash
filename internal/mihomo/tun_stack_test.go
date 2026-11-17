package mihomo

import (
	"os"
	"os/exec"
	"path/filepath"
	"reia-clash/internal/config"
	"strings"
	"testing"
)

func TestMipsTunConfig(t *testing.T) {
	settings := config.Defaults()
	settings.TunStack, settings.Enabled, settings.UseTun, settings.TunAuthorized = "mips", true, true, true
	rendered, err := BuildConfig([]byte("tun:\n  stack: mixed\nproxies: []\nproxy-groups: []\nrules:\n  - MATCH,DIRECT\n"), settings, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rendered), "stack: mips") || strings.Count(string(rendered), "stack:") != 1 {
		t.Fatalf("incorrect mips configuration: %s", rendered)
	}
	binary := os.Getenv("REIA_TEST_MIHOMO")
	if binary == "" {
		return
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "config.yaml")
	if err := os.WriteFile(path, rendered, 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(binary, "-t", "-d", directory, "-f", path).CombinedOutput(); err != nil {
		t.Fatalf("real kernel rejected mips: %v: %s", err, output)
	}
}
