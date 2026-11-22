package update

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reia-clash/internal/kernelbinary"
	"strings"
)

// ClearCache removes only interrupted download/extraction files. Completed
// binaries may be active, staged for restart or needed for rollback.
func (m *Manager) ClearCache() (int, error) {
	if !m.stageMu.TryLock() {
		return 0, fmt.Errorf("内核更新正在下载或校验，请完成后再清理缓存")
	}
	defer m.stageMu.Unlock()
	parent, err := os.OpenRoot(filepath.Dir(m.directory))
	if err != nil {
		return 0, err
	}
	defer parent.Close()
	root, err := parent.OpenRoot(filepath.Base(m.directory))
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer root.Close()
	directory, err := root.Open(".")
	if err != nil {
		return 0, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return 0, err
	}
	count := 0
	var failures error
	for _, entry := range entries {
		name := entry.Name()
		download := name == ".mihomo-download.tmp" || (strings.HasPrefix(name, ".mihomo-download-") && strings.HasSuffix(name, ".tmp"))
		pack := strings.HasPrefix(name, ".stage-") && (strings.HasSuffix(name, "-update.pack") || strings.HasSuffix(name, ".tmp"))
		extraction := (strings.HasPrefix(name, "mihomo-") || strings.HasPrefix(name, "meow-")) && (strings.HasSuffix(name, ".tmp") || strings.HasSuffix(name, ".tmp.exe"))
		if !download && !pack && !extraction {
			continue
		}
		info, err := root.Lstat(name)
		if err != nil {
			failures = errors.Join(failures, err)
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if err := root.Remove(name); err != nil {
			failures = errors.Join(failures, err)
		} else {
			count++
		}
	}
	return count, failures
}

// RemoveSuperseded removes the previous application-managed release after handoff.
func (m *Manager) RemoveSuperseded(previous string) error {
	if !kernelbinary.IsManagedUpdate(m.directory, previous) {
		return nil
	}
	relative, err := filepath.Rel(m.directory, previous)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(m.directory)
	if err != nil {
		return err
	}
	defer root.Close()
	name := filepath.Base(relative)
	parent := filepath.Dir(relative)
	if parent == "." && !strings.HasPrefix(name, "mihomo-") && !strings.HasPrefix(name, "meow-") {
		return nil
	}
	remove := func(path string) error {
		err := root.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := remove(relative); err != nil {
		return err
	}
	if parent != "." {
		return errors.Join(remove(filepath.Join(parent, "wintun.dll")), remove(parent))
	}
	return nil
}
