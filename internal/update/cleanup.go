package update

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
		if name != ".mihomo-download.tmp" && !(strings.HasPrefix(name, "mihomo-") && (strings.HasSuffix(name, ".tmp") || strings.HasSuffix(name, ".tmp.exe"))) {
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
