package update

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"reia-clash/internal/kernelbinary"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

var releaseVersion = regexp.MustCompile(`[vV]?[0-9]+\.[0-9]+\.[0-9]+[A-Za-z0-9.+_-]*`)

var unsafeVersion = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func SafeVersion(version string) string {
	version = strings.Trim(unsafeVersion.ReplaceAllString(version, "_"), ".")
	if version == "" {
		return "unknown"
	}
	return version[:min(len(version), 60)]
}

// Installation stages files on the same filesystem as the final executable.
type Installation struct {
	Target, BackupBinary                               string
	directory, family, live, backup, temporary, staged string
	movedOld, movedNew                                 bool
}

func PrepareInstall(directory, candidate, previousTag string, meow bool) (*Installation, error) {
	family := "mihomo"
	if meow {
		family = "meow"
	}
	name := family + binarySuffix()
	live := filepath.Join(directory, name)
	bundled := meow && runtime.GOOS == "windows"
	if bundled {
		live = filepath.Join(directory, family)
	}
	if previousTag == "" {
		old := live
		if bundled {
			old = filepath.Join(live, name)
		}
		if output, err := kernelbinary.VersionOutput(old); err == nil {
			previousTag = releaseVersion.FindString(output)
		}
	}
	backup := filepath.Join(directory, fmt.Sprintf("%s-%s-ota-%d.bak", family, SafeVersion(previousTag), time.Now().UnixNano()))
	temporary, err := os.MkdirTemp(directory, ".reia-install-")
	if err != nil {
		return nil, err
	}
	install := &Installation{Target: live, BackupBinary: backup, directory: directory, family: family, live: live, backup: backup, temporary: temporary, staged: filepath.Join(temporary, name)}
	if bundled {
		install.Target = filepath.Join(live, name)
		install.BackupBinary = filepath.Join(backup, name)
		install.staged = temporary
	}
	if err := copyInstallFile(candidate, filepath.Join(temporary, name), 0700); err != nil {
		install.Close()
		return nil, err
	}
	if bundled {
		if err := copyInstallFile(filepath.Join(filepath.Dir(candidate), "wintun.dll"), filepath.Join(temporary, "wintun.dll"), 0600); err != nil {
			install.Close()
			return nil, err
		}
	}
	if info, err := os.Lstat(live); err == nil {
		if (bundled && !info.IsDir()) || (!bundled && !info.Mode().IsRegular()) {
			install.Close()
			return nil, fmt.Errorf("内核安装位置已有不兼容的文件: %s", live)
		}
	} else if os.IsNotExist(err) {
		install.BackupBinary = ""
	} else {
		install.Close()
		return nil, err
	}
	return install, nil
}

func copyInstallFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	return errors.Join(copyErr, out.Sync(), out.Close())
}

func (i *Installation) Replace() error {
	if i.BackupBinary != "" {
		if err := os.Rename(i.live, i.backup); err != nil {
			return err
		}
		i.movedOld = true
	}
	if err := os.Rename(i.staged, i.live); err != nil {
		return err
	}
	i.movedNew = true
	// A bundled candidate moved the temporary directory itself.
	if i.staged == i.temporary {
		i.temporary = ""
	}
	return nil
}

func (i *Installation) Rollback() error {
	if i.movedNew {
		rejected, err := os.MkdirTemp(i.directory, ".reia-install-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(rejected)
		if err := os.Rename(i.live, filepath.Join(rejected, "rejected")); err != nil {
			return err
		}
	}
	if i.movedOld {
		return os.Rename(i.backup, i.live)
	}
	return nil
}

func (i *Installation) Close() {
	if i.temporary != "" {
		_ = os.RemoveAll(i.temporary)
	}
}

func (i *Installation) Prune() error {
	entries, err := os.ReadDir(i.directory)
	if err != nil {
		return err
	}
	pattern := regexp.MustCompile(`^` + i.family + `-[A-Za-z0-9._-]+-ota-([0-9]+)\.bak$`)
	type backup struct {
		name      string
		timestamp int64
	}
	var backups []backup
	for _, entry := range entries {
		match := pattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() || (info.IsDir() && i.family == "meow" && runtime.GOOS == "windows") {
			if timestamp, err := strconv.ParseInt(match[1], 10, 64); err == nil {
				backups = append(backups, backup{entry.Name(), timestamp})
			}
		}
	}
	sort.Slice(backups, func(a, b int) bool { return backups[a].timestamp > backups[b].timestamp })
	for _, old := range backups[min(3, len(backups)):] {
		path := filepath.Join(i.directory, old.name)
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}
