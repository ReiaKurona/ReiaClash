//go:build windows

package launch

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os/exec"
	"unsafe"
)

// The service owns this Job handle. Windows closes it on a forced service
// termination too, so its Mihomo child cannot outlive the supervisor.
func TrackChildProcess(command *exec.Cmd) (func(), error) {
	return trackChildProcess(command, false)
}

// TrackDesktopChildProcess permits an already job-contained child on systems
// where the enclosing desktop job forbids creating a nested job.
func TrackDesktopChildProcess(command *exec.Cmd) (func(), error) {
	return trackChildProcess(command, true)
}

var isProcessInJob = windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")

func inWindowsJob(process windows.Handle) bool {
	var assigned uint32
	ok, _, _ := isProcessInJob.Call(uintptr(process), 0, uintptr(unsafe.Pointer(&assigned)))
	return ok != 0 && assigned != 0
}

func trackChildProcess(command *exec.Cmd, allowInheritedJob bool) (func(), error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	closeJob := func() { windows.CloseHandle(job) }
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		closeJob()
		return nil, err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(command.Process.Pid))
	if err != nil {
		closeJob()
		return nil, err
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		closeJob()
		if allowInheritedJob && errors.Is(err, windows.ERROR_ACCESS_DENIED) &&
			inWindowsJob(windows.CurrentProcess()) && inWindowsJob(process) {
			return func() {}, nil
		}
		return nil, fmt.Errorf("绑定内核进程生命周期失败: %w", err)
	}
	return closeJob, nil
}
