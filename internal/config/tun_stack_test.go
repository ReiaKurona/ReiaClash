package config

import (
	"path/filepath"
	"testing"
)

func TestMipsTunStackSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(func(s *Settings) error { s.TunStack = "mips"; return nil }); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Snapshot().TunStack; got != "mips" {
		t.Fatalf("mips was reset to %q", got)
	}
}
