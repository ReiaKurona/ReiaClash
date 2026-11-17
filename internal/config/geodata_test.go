package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGeoLowMemoryMigrationAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"tun_stack":"mips"}`), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if !store.Snapshot().GeoLowMemory {
		t.Fatal("old settings must default to low memory")
	}
	if _, err := store.Update(func(s *Settings) error { s.GeoLowMemory = false; return nil }); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Snapshot().GeoLowMemory {
		t.Fatal("explicit false was reset")
	}
	if err := store.Reload(); err != nil {
		t.Fatal(err)
	}
	if store.Snapshot().GeoLowMemory {
		t.Fatal("helper reload lost explicit false")
	}
}
