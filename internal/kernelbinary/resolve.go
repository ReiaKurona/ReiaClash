package kernelbinary

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reia-clash/internal/config"
	"reia-clash/internal/platform/launch"
	"runtime"
	"strings"
	"time"
)

func Name(goos string) string {
	if goos == "windows" {
		return "mihomo.exe"
	}
	return "mihomo"
}

func Resolve(settings config.Settings, managedDir string) (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	workingDir, _ := os.Getwd()
	return resolveAt(settings, managedDir, filepath.Dir(executable), workingDir, Validate)
}

func resolveAt(settings config.Settings, managedDir, appDir, workingDir string, validate func(string) error) (string, error) {
	name := Name(runtime.GOOS)
	var candidates []string
	// Explicitly downloaded updates retain priority; legacy cache does not mask bundled kernels.
	if settings.ActiveBinary != "" && filepath.Clean(filepath.Dir(settings.ActiveBinary)) == filepath.Join(filepath.Dir(managedDir), "updates") {
		candidates = append(candidates, settings.ActiveBinary)
	}
	candidates = append(candidates, directoryCandidates(appDir, name)...)
	if workingDir != "" {
		candidates = append(candidates, directoryCandidates(workingDir, name)...)
	}
	candidates = append(candidates, settings.ActiveBinary, filepath.Join(managedDir, name), settings.PreviousBinary)
	seen := map[string]bool{}
	var problems []string
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		absolute, err := filepath.Abs(candidate)
		if err != nil || seen[absolute] {
			continue
		}
		seen[absolute] = true
		if err := validate(absolute); err == nil {
			return absolute, nil
		} else {
			problems = append(problems, absolute+": "+err.Error())
		}
	}
	return "", fmt.Errorf("找不到可运行的 %s/%s 内核（文件名 %s）：%s", runtime.GOOS, runtime.GOARCH, name, strings.Join(problems, "; "))
}

func directoryCandidates(directory, name string) []string {
	paths := []string{filepath.Join(directory, name)}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return paths
	}
	for _, entry := range entries {
		lower := strings.ToLower(entry.Name())
		if entry.IsDir() || !(strings.HasPrefix(lower, "mihomo-") || strings.HasPrefix(lower, "mihomo_")) {
			continue
		}
		// Do not execute archives, checksum files, or service/config files.
		if strings.HasSuffix(lower, ".gz") || strings.HasSuffix(lower, ".zip") || strings.HasSuffix(lower, ".tar") || strings.HasSuffix(lower, ".tgz") || strings.HasSuffix(lower, ".sha256") || strings.HasSuffix(lower, ".txt") || strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".service") {
			continue
		}
		if runtime.GOOS == "windows" && !strings.HasSuffix(lower, ".exe") || runtime.GOOS != "windows" && strings.HasSuffix(lower, ".exe") {
			continue
		}
		paths = append(paths, filepath.Join(directory, entry.Name()))
	}
	return paths
}

// Probe executable permissions, OS/architecture support and actual version output.
func Validate(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("不是普通文件")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0 {
		if err := prepareExecutable(path); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := launch.CommandContext(ctx, path, "-v")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("版本探测失败，检查平台/架构/权限: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	if !strings.Contains(strings.ToLower(string(output)), "mihomo") {
		return fmt.Errorf("版本输出无法识别为 mihomo")
	}
	return nil
}
