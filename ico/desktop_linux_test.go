//go:build linux && !android

package branding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopAssetsUseUserDirectories(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "share"))
	if err := InstallDesktopEntries(); err != nil {
		t.Fatal(err)
	}
	icon, err := DesktopIconPath()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(icon, filepath.Join(root, "cache")) {
		t.Fatal(icon)
	}
	content, err := os.ReadFile(icon)
	if err != nil || string(content) != string(PNG) {
		t.Fatalf("bad exported icon: %v", err)
	}
	// Registration runs alongside the current test executable; no application
	// binaries exist there, so it must not create bogus launcher entries.
	if entries, _ := os.ReadDir(filepath.Join(root, "share", "applications")); len(entries) != 0 {
		t.Fatal(entries)
	}
}

func TestDesktopEntryEscapesFieldCodesAndKeepsHelpersHidden(t *testing.T) {
	entry := desktopEntry("reia-tray", `/a b/100%/$HOME/"reia-tray"`, "/icon with space.png")
	if !strings.Contains(entry, "NoDisplay=true\n") || !strings.Contains(entry, "Icon=/icon with space.png\n") || !strings.Contains(entry, "100%%") {
		t.Fatal(entry)
	}
	if strings.Contains(desktopEntry("reia", "/app/reia", "/icon.png"), "NoDisplay=true") {
		t.Fatal("main launcher is hidden")
	}
}
