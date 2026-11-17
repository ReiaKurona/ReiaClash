package launch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// OpenDirectory runs in the Agent's desktop session, never in the core service.
func OpenDirectory(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("目标不是目录")
	}
	name := "xdg-open"
	if runtime.GOOS == "windows" {
		name = "explorer.exe"
	} else if runtime.GOOS == "darwin" {
		name = "open"
	}
	command := exec.Command(name, path)
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}
