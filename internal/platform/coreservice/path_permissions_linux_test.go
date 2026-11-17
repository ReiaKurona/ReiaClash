//go:build linux && !android

package coreservice

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareServicePaths(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "reia-service")
	dataDir := filepath.Join(root, "config")
	if err := os.Mkdir(dataDir, 0775); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dataDir, 0775); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("test executable"), 0775); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(executable, 0775); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "core-service.token"), []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(root, "unrelated")
	if err := os.WriteFile(unrelated, nil, 0666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unrelated, 0666); err != nil {
		t.Fatal(err)
	}
	if err := prepareServicePaths(executable, dataDir); err != nil {
		t.Fatal(err)
	}
	for path, mode := range map[string]os.FileMode{executable: 0755, dataDir: 0755, unrelated: 0666} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("%s: %v %v", path, info, err)
		}
	}
	if err := checkOwnedPath(executable, os.Getuid(), false, true); err != nil {
		t.Fatal(err)
	}
	if err := checkOwnedPath(dataDir, os.Getuid(), true, false); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".service-access-") {
			t.Fatal("probe file leaked")
		}
	}
	// A missing token is rejected before any authentication prompt or unit write.
	if err := os.Remove(filepath.Join(dataDir, "core-service.token")); err != nil {
		t.Fatal(err)
	}
	if err := prepareServicePaths(executable, dataDir); err == nil || !strings.Contains(err.Error(), "core-service.token") {
		t.Fatalf("missing token: %v", err)
	}
}

func TestOperationDiagnosticReplacesLinkWithoutTouchingTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "preserved")
	if err := os.WriteFile(target, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "core-service-operation.log")
	if err := os.Symlink(target, log); err != nil {
		t.Fatal(err)
	}
	if err := writeOperationDiagnostic(root, "permission details"); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(target)
	if err != nil || string(content) != "preserve" {
		t.Fatalf("target changed: %q %v", content, err)
	}
	info, err := os.Lstat(log)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		t.Fatalf("unsafe diagnostic: %v %v", info, err)
	}
	if err := writeOperationDiagnostic(root, ""); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(log)
	if err != nil || len(content) != 0 {
		t.Fatalf("stale error: %q %v", content, err)
	}
}
