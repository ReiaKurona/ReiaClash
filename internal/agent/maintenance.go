package agent

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reia-clash/internal/config"
)

var agentLogFiles = []string{"agent.log", "agent-startup.log", "core-service.log", "core-service-operation.log", "launcher-error.log"}

type CacheCleanup struct {
	Files      int `json:"cache_files"`
	DNSRecords int `json:"dns_records"`
}

type LogCleanup struct {
	Files  int `json:"log_files"`
	Events int `json:"events"`
}

func (a *App) ClearAgentCache() (CacheCleanup, error) {
	files, err := a.Updater.ClearCache()
	result := CacheCleanup{Files: files}
	if err != nil {
		return result, err
	}
	a.dnsMu.Lock()
	result.DNSRecords = len(a.dnsRecords)
	a.dnsRecords = nil
	a.dnsMu.Unlock()
	return result, nil
}

// ClearAgentLogs truncates the same files held by O_APPEND log writers, so
// logging continues without reopening or restarting the Agent.
func (a *App) ClearAgentLogs() (LogCleanup, error) {
	result := LogCleanup{}
	root, err := os.OpenRoot(a.Paths.Root)
	if err != nil {
		return result, err
	}
	defer root.Close()
	var failures error
	for _, name := range agentLogFiles {
		info, err := root.Lstat(name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			failures = errors.Join(failures, err)
			continue
		}
		if !info.Mode().IsRegular() {
			failures = errors.Join(failures, fmt.Errorf("拒绝清理非普通日志文件 %s", name))
			continue
		}
		file, err := root.OpenFile(name, os.O_WRONLY, 0)
		if err != nil {
			failures = errors.Join(failures, err)
			continue
		}
		opened, err := file.Stat()
		if err == nil && !os.SameFile(info, opened) {
			err = fmt.Errorf("日志文件 %s 已变化，请重试", name)
		}
		if err == nil {
			err = file.Truncate(0)
		}
		err = errors.Join(err, file.Close())
		if err != nil {
			failures = errors.Join(failures, err)
		} else {
			result.Files++
		}
	}
	a.eventsMu.Lock()
	result.Events = len(a.events)
	a.events = nil // Preserve eventID: connected panels retain their cursors.
	a.eventsMu.Unlock()
	return result, failures
}

// AgentDirectory exposes fixed locations only; callers cannot submit a path.
func (a *App) AgentDirectory(kind string) (string, error) {
	switch kind {
	case "cache":
		return a.Paths.UpdateDir, nil
	case "config":
		return a.Paths.Root, nil
	case "work":
		return config.MihomoDataDir()
	default:
		return "", fmt.Errorf("目录类型仅支持 cache、work 或 config")
	}
}

type DirectorySize struct {
	Bytes   int64 `json:"bytes"`
	Files   int64 `json:"files"`
	Partial bool  `json:"partial"`
}

func (a *App) DirectorySizes(ctx context.Context) (map[string]DirectorySize, error) {
	result := make(map[string]DirectorySize, 4)
	for _, kind := range []string{"cache", "work", "config"} {
		root, err := a.AgentDirectory(kind)
		if err != nil {
			return nil, err
		}
		var size DirectorySize
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if kind == "config" && path == a.Paths.UpdateDir && entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			if walkErr != nil {
				if !(path == root && os.IsNotExist(walkErr)) {
					size.Partial = true
				}
				return nil
			}
			if entry.Type().IsRegular() {
				if info, err := entry.Info(); err == nil {
					size.Bytes += info.Size()
					size.Files++
				} else {
					size.Partial = true
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		result[kind] = size
	}
	var logs DirectorySize
	for _, name := range agentLogFiles {
		info, err := os.Lstat(filepath.Join(a.Paths.Root, name))
		if err != nil {
			if !os.IsNotExist(err) {
				logs.Partial = true
			}
			continue
		}
		if info.Mode().IsRegular() {
			logs.Files++
			logs.Bytes += info.Size()
		}
	}
	result["logs"] = logs

	return result, nil
}
