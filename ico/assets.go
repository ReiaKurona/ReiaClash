// Package branding contains the shared ReiaClash artwork. Keep the source
// JPEG and the generated platform assets together in this directory.
package branding

import (
	"bytes"
	_ "embed"
	"os"
	"path/filepath"
)

const AppName = "ReiaClash"

//go:generate go run ../scripts/gen-icons -windows-resources

//go:embed reia-clash-64.png
var TrayPNG []byte

//go:embed reia-clash.png
var PNG []byte

// DesktopIconPath materializes the embedded icon for notification/desktop
// APIs that accept a filename rather than PNG bytes.
func DesktopIconPath() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(cache, "ReiaClash", "icons", "reia-clash.png")
	return path, writeAsset(path, PNG)
}

func writeAsset(path string, content []byte) error {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, content) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".reia-icon-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(content); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
