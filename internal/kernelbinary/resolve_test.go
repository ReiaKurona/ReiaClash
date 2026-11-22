package kernelbinary

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reia-clash/internal/config"
	"runtime"
	"strings"
	"testing"
)

func TestAcceptedMeowWinsColdBootFallback(t *testing.T) {
	root := t.TempDir()
	app, managed := filepath.Join(root, "app"), filepath.Join(root, "bin")
	saved := filepath.Join(root, "old-install", "meow.exe")
	bundled := filepath.Join(app, Name(runtime.GOOS))
	settings := config.Settings{ActiveBinary: saved, MihomoFallback: bundled}
	for _, probe := range []error{nil, context.DeadlineExceeded} {
		got, err := resolveAt(settings, managed, app, "", func(path string) error {
			if path == saved {
				return probe
			}
			return nil
		})
		if err != nil || got != saved {
			t.Fatalf("accepted meow was replaced at startup: %s, %v", got, err)
		}
	}
}

func TestPlatformNames(t *testing.T) {
	for platform, want := range map[string]string{"windows": "mihomo.exe", "linux": "mihomo", "darwin": "mihomo"} {
		if got := Name(platform); got != want {
			t.Fatalf("%s: got %s want %s", platform, got, want)
		}
	}
}

func TestDiscoveryOrderAndFallback(t *testing.T) {
	root := t.TempDir()
	managed, app, working := filepath.Join(root, "user", "bin"), filepath.Join(root, "app"), filepath.Join(root, "cwd")
	bundled := filepath.Join(app, Name(runtime.GOOS))
	cached := filepath.Join(managed, Name(runtime.GOOS))
	update := filepath.Join(root, "user", "updates", "mihomo-new")
	for _, tt := range []struct {
		name, active, want string
		missingLocal       bool
	}{
		{"local beats old cache", cached, bundled, false},
		{"explicit update remains selected", update, update, false},
		{"missing local falls back to cache", cached, cached, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			check := func(path string) error {
				if path == cached || path == update || path == bundled && !tt.missingLocal {
					return nil
				}
				return os.ErrNotExist
			}
			got, err := resolveAt(config.Settings{ActiveBinary: tt.active}, managed, app, working, check)
			if err != nil || got != tt.want {
				t.Fatalf("got %q %v want %q", got, err, tt.want)
			}
		})
	}
	_, err := resolveAt(config.Settings{}, managed, app, working, func(string) error { return fmt.Errorf("wrong architecture") })
	if err == nil || !strings.Contains(err.Error(), "wrong architecture") {
		t.Fatalf("lost diagnostic: %v", err)
	}
}

func TestValidationRejectsNonKernel(t *testing.T) {
	path := filepath.Join(t.TempDir(), Name(runtime.GOOS))
	if err := os.WriteFile(path, []byte("not an executable"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := Validate(path); err == nil {
		t.Fatal("filename alone must not qualify a kernel")
	}
}

func TestVersionedBundledKernelDoesNotRequireDownloadedUpdate(t *testing.T) {
	root := t.TempDir()
	app, working, managed := filepath.Join(root, "app"), filepath.Join(root, "cwd"), filepath.Join(root, "config", "bin")
	if err := os.MkdirAll(app, 0700); err != nil {
		t.Fatal(err)
	}
	versioned := "mihomo-" + runtime.GOOS + "-" + runtime.GOARCH + "-v1.0"
	if runtime.GOOS == "windows" {
		versioned += ".exe"
	}
	for _, name := range []string{versioned, "mihomo-download.gz", "mihomo-checksums.txt", "mihomo-config.yaml"} {
		if err := os.WriteFile(filepath.Join(app, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	var attempts []string
	want := filepath.Join(app, versioned)
	got, err := resolveAt(config.Settings{}, managed, app, working, func(path string) error {
		attempts = append(attempts, path)
		if path == want {
			return nil
		}
		return os.ErrNotExist
	})
	if err != nil || got != want {
		t.Fatalf("got %s %v want %s", got, err, want)
	}
	if len(attempts) != 2 || attempts[0] != filepath.Join(app, Name(runtime.GOOS)) {
		t.Fatal(attempts)
	}
}

func TestRealKernelVersionProbe(t *testing.T) {
	binary := os.Getenv("REIA_TEST_MIHOMO")
	if binary == "" {
		t.Skip("set REIA_TEST_MIHOMO to check the real kernel")
	}
	if err := Validate(binary); err != nil {
		t.Fatal(err)
	}
}
