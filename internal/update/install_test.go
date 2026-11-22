package update

import (
	"fmt"
	"os"
	"path/filepath"
	"reia-clash/internal/kernelbinary"
	"runtime"
	"testing"
	"time"
)

func TestInstallRollbackAndHistory(t *testing.T) {
	directory := t.TempDir()
	live := filepath.Join(directory, kernelbinary.Name(runtime.GOOS))
	candidate := filepath.Join(t.TempDir(), "candidate")
	write := func(path, value string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(value), 0700); err != nil {
			t.Fatal(err)
		}
	}
	read := func(path string) string {
		t.Helper()
		value, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(value)
	}
	write(live, "old")
	write(candidate, "new")
	installation, err := PrepareInstall(directory, candidate, "v1.0.0", false)
	if err != nil {
		t.Fatal(err)
	}
	defer installation.Close()
	if err := installation.Replace(); err != nil {
		t.Fatal(err)
	}
	if read(live) != "new" || read(installation.BackupBinary) != "old" {
		t.Fatal("replacement or backup invalid")
	}
	if err := installation.Rollback(); err != nil {
		t.Fatal(err)
	}
	if read(live) != "old" {
		t.Fatal("rollback did not restore old file")
	}
	for n := 0; n < 5; n++ {
		path := filepath.Join(directory, fmt.Sprintf("mihomo-v1.0.%d-ota-%d.bak", n, n))
		write(path, "backup")
		date := time.Now().Add(time.Duration(n-10) * time.Hour)
		if err := os.Chtimes(path, date, date); err != nil {
			t.Fatal(err)
		}
	}
	userBackup := filepath.Join(directory, "mihomo-manual.bak")
	write(userBackup, "user")
	if err := installation.Prune(); err != nil {
		t.Fatal(err)
	}
	entries, _ := filepath.Glob(filepath.Join(directory, "mihomo-*-ota-*.bak"))
	if len(entries) != 3 || read(userBackup) != "user" {
		t.Fatal("unexpected backup retention")
	}
}
