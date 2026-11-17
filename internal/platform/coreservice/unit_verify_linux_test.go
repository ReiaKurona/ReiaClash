//go:build linux && !android

package coreservice

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Runs without root and never registers or starts the generated unit.
func TestSystemdAnalyzeGeneratedUnit(t *testing.T) {
	analyze, err := exec.LookPath("systemd-analyze")
	if err != nil {
		t.Skip("systemd-analyze unavailable")
	}
	executable, err := exec.LookPath("true")
	if err != nil {
		t.Skip("true unavailable")
	}
	root := filepath.Join(t.TempDir(), "配置 a $d %u")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	unit := filepath.Join(t.TempDir(), unitName)
	if err := os.WriteFile(unit, []byte(managedUnitMarker+renderUnit(executable, root, os.Getuid(), os.Getgid())), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(analyze, "verify", "--man=no", unit).CombinedOutput(); err != nil {
		t.Fatalf("systemd-analyze verify: %v\n%s", err, output)
	}
}
