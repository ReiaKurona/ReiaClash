//go:build windows

package coreservice

import (
	"context"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var shellExecuteEx = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// SHELLEXECUTEINFOW. The Go layout follows the pointer width of the target.
type shellExecuteInfo struct {
	Size, Mask                  uint32
	Window                      windows.Handle
	Verb, File, Parameters, Dir *uint16
	Show                        int32
	Instance                    windows.Handle
	IDList                      uintptr
	Class                       *uint16
	ClassKey                    windows.Handle
	HotKey                      uint32
	IconOrMonitor, Process      windows.Handle
}

func startElevated(ctx context.Context, executable string, args []string) (windows.Handle, error) {
	file, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return 0, err
	}
	parameters, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(args))
	if err != nil {
		return 0, err
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	const (
		keepProcessHandle = 0x00000040
		noAsync           = 0x00000100
		noShellErrorUI    = 0x00000400
	)
	info := shellExecuteInfo{Mask: keepProcessHandle | noAsync | noShellErrorUI, Verb: verb, File: file,
		Parameters: parameters, Show: windows.SW_HIDE}
	info.Size = uint32(unsafe.Sizeof(info))

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	initErr := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE)
	switch initErr {
	case nil, syscall.Errno(windows.S_FALSE):
		defer windows.CoUninitialize()
	case syscall.Errno(windows.RPC_E_CHANGED_MODE):
		// The current thread is already initialized by another component.
	default:
		return 0, fmt.Errorf("初始化 Windows 授权组件失败: %w", initErr)
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if ok, _, callErr := shellExecuteEx.Call(uintptr(unsafe.Pointer(&info))); ok == 0 {
		return 0, callErr
	}
	if info.Process == 0 {
		return 0, fmt.Errorf("Windows 未返回授权进程句柄")
	}
	return info.Process, nil
}

func waitElevated(handle windows.Handle) (uint32, error) {
	// Keep lifecycle handover serialized if a browser disconnects after UAC.
	// The helper continues independently until its SCM operation completes.
	state, err := windows.WaitForSingleObject(handle, windows.INFINITE)
	if err != nil {
		return 0, err
	}
	if state != windows.WAIT_OBJECT_0 {
		return 0, fmt.Errorf("等待授权进程返回了意外状态 %d", state)
	}
	var code uint32
	err = windows.GetExitCodeProcess(handle, &code)
	return code, err
}
