package update

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClearCachePreservesCompletedBinaries(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(root)
	removed := []string{".mihomo-download.tmp", "mihomo-v1.tmp", "mihomo-v2.tmp.exe"}
	kept := []string{"mihomo-active.exe", "mihomo-staged", "mihomo-rollback", "unrelated.tmp", "settings.json"}
	for _, name := range append(removed, kept...) {
		if err := os.WriteFile(filepath.Join(root, name), []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// A similarly named directory must never be traversed or removed.
	directory := filepath.Join(root, "mihomo-folder.tmp")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	count, err := manager.ClearCache()
	if err != nil || count != len(removed) {
		t.Fatalf("%d, %v", count, err)
	}
	for _, name := range removed {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatal(name, err)
		}
	}
	for _, name := range append(kept, "mihomo-folder.tmp") {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatal(name, err)
		}
	}
	manager.stageMu.Lock()
	if _, err := manager.ClearCache(); err == nil {
		t.Error("cleanup must not race a download/extraction")
	}
	manager.stageMu.Unlock()
}

func TestClearCacheDoesNotFollowExternalSymlinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	target := filepath.Join(outside, "protected")
	if err := os.WriteFile(target, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, ".mihomo-download.tmp")); err != nil {
		t.Skip("symlink unavailable:", err)
	}
	if _, err := NewManager(root).ClearCache(); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(target); string(data) != "keep" {
		t.Fatal("external file changed")
	}
}
