//go:build windows

package coreservice

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"reia-clash/internal/config"
	"reia-clash/internal/platform/launch"
)

type processRecord struct {
	PID         uint32 `json:"pid"`
	Path        string `json:"path"`
	CommandLine string `json:"command_line"`
	Created     uint64 `json:"created,string"`
}

func samePath(a, b string) bool {
	return a != "" && b != "" && strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// Match both the executable and its exact configuration arguments. A process
// name or occupied port alone is never sufficient authority to terminate it.
func ownedProcess(p processRecord, root, installation, binary string) bool {
	args, err := windows.DecomposeCommandLine(p.CommandLine)
	if err != nil || len(args) == 0 || !samePath(args[0], p.Path) {
		return false
	}
	value := func(name string) string {
		for i := 1; i < len(args); i++ {
			if args[i] == name && i+1 < len(args) {
				return args[i+1]
			}
			if strings.HasPrefix(args[i], name+"=") {
				return strings.TrimPrefix(args[i], name+"=")
			}
		}
		return ""
	}
	if samePath(p.Path, filepath.Join(installation, "reia-agent.exe")) || samePath(p.Path, filepath.Join(installation, "reia-service.exe")) {
		for _, arg := range args[1:] {
			if arg == "--core-service" || arg == "--core-service=true" {
				return samePath(value("--data-dir"), root)
			}
		}
		return false
	}
	return (samePath(p.Path, binary) || samePath(p.Path, filepath.Join(installation, "mihomo.exe"))) &&
		samePath(value("-f"), filepath.Join(root, "mihomo.yaml")) &&
		samePath(value("-d"), filepath.Join(installation, "data"))
}

// Called only by an elevated installer, after the registered service was
// stopped. This also removes children left behind by pre-Job-Object versions.
func cleanupManagedProcesses(root string) error {
	if root == "" {
		return fmt.Errorf("清理旧内核需要明确的数据目录")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	executable, err := Executable()
	if err != nil {
		return err
	}
	installation := filepath.Dir(executable)
	var settings config.Settings
	if data, err := os.ReadFile(filepath.Join(root, "settings.json")); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	// Query metadata only. Native handles below recheck identity before killing.
	script := `[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false); $ErrorActionPreference='Stop'; ConvertTo-Json -Compress -InputObject @((Get-CimInstance Win32_Process) | Where-Object { $_.CreationDate -and ($_.Name -like '*mihomo*' -or $_.Name -in @('reia-agent.exe','reia-service.exe') -or $_.CommandLine -like '*mihomo.yaml*') } | ForEach-Object { [pscustomobject]@{ pid=$_.ProcessId; path=$_.ExecutablePath; command_line=$_.CommandLine; created=[string]$_.CreationDate.ToFileTimeUtc() } })`
	snapshot := func() ([]processRecord, error) {
		output, err := launch.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("检查旧服务进程失败: %w: %s", err, strings.TrimSpace(string(output)))
		}
		var processes []processRecord
		if err := json.Unmarshal(output, &processes); err != nil {
			return nil, fmt.Errorf("读取旧服务进程信息失败: %w", err)
		}
		return processes, nil
	}
	// Supervisors first, then kernels, preventing a legacy supervisor's retry.
	for _, helpers := range []bool{true, false} {
		processes, err := snapshot()
		if err != nil {
			return err
		}
		for _, p := range processes {
			if p.PID == uint32(os.Getpid()) || !ownedProcess(p, root, installation, settings.ActiveBinary) {
				continue
			}
			isHelper := strings.HasPrefix(strings.ToLower(filepath.Base(p.Path)), "reia-")
			if isHelper != helpers {
				continue
			}
			if err := terminateVerifiedProcess(p); err != nil {
				return err
			}
		}
	}
	return nil
}

func terminateVerifiedProcess(p processRecord) error {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, p.PID)
	if err == windows.ERROR_INVALID_PARAMETER {
		return nil
	} // Already exited.
	if err != nil {
		return fmt.Errorf("打开旧服务进程 %d 失败: %w", p.PID, err)
	}
	defer windows.CloseHandle(handle)
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &created, &exited, &kernel, &user); err != nil {
		return err
	}
	if !sameCreationTime(uint64(created.HighDateTime)<<32|uint64(created.LowDateTime), p.Created) {
		return nil
	} // PID reused.
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		return err
	}
	if !samePath(windows.UTF16ToString(buffer[:size]), p.Path) {
		return fmt.Errorf("旧进程身份已改变，拒绝清理 PID %d", p.PID)
	}
	if err := windows.TerminateProcess(handle, 0); err != nil {
		if state, _ := windows.WaitForSingleObject(handle, 0); state != windows.WAIT_OBJECT_0 {
			return err
		}
	}
	state, err := windows.WaitForSingleObject(handle, 10000)
	if err != nil {
		return err
	}
	if state != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("等待旧进程 %d 退出超时", p.PID)
	}
	return nil
}

func sameCreationTime(native, cim uint64) bool {
	// CIM_DATETIME carries microseconds; FILETIME carries 100-nanosecond
	// ticks. Compare at CIM precision while still rejecting PID reuse.
	return cim != 0 && native/10 == cim/10
}
