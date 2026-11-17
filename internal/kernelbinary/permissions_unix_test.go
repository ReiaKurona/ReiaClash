//go:build (linux || darwin) && !android

package kernelbinary

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUnpackedNativeKernelGetsOnlyOwnerExecute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mihomo")
	content := []byte{0x7f, 'E', 'L', 'F', 0, 0, 0, 0}
	if runtime.GOOS == "darwin" {
		content = []byte{0xcf, 0xfa, 0xed, 0xfe, 0, 0, 0, 0}
	}
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err := prepareExecutable(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0744 {
		t.Fatalf("%v %v", info, err)
	}
	// A script with a convenient filename must not be chmodded automatically.
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho mihomo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err := prepareExecutable(path); err == nil {
		t.Fatal("script was granted execution")
	}
	info, err = os.Stat(path)
	if err != nil || info.Mode().Perm() != 0644 {
		t.Fatalf("%v %v", info, err)
	}
}
