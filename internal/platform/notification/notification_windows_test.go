//go:build windows

package notification

import (
	"golang.org/x/sys/windows"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// Verify the native balloon-click bridge with a hidden test message window.
// This never creates a tray icon or displays a desktop notification.
func TestWindowsBalloonClickBridge(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	type windowClass struct {
		Size, Style                        uint32
		Procedure                          uintptr
		ClassExtra, WindowExtra            int32
		Instance, Icon, Cursor, Background uintptr
		MenuName, ClassName                *uint16
		SmallIcon                          uintptr
	}
	className, _ := windows.UTF16PtrFromString("GoGPUSystrayMsg")
	defaultWindow := user32.NewProc("DefWindowProcW")
	callback := syscall.NewCallback(func(window, msg, wp, lp uintptr) uintptr {
		result, _, _ := defaultWindow.Call(window, msg, wp, lp)
		return result
	})
	instance, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
	definition := windowClass{Procedure: callback, Instance: instance, ClassName: className}
	definition.Size = uint32(unsafe.Sizeof(definition))
	if result, _, err := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&definition))); result == 0 {
		t.Fatalf("register test class: %v", err)
	}
	defer user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(className)), instance)
	window, _, err := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, ^uintptr(2), 0, instance, 0)
	if window == 0 {
		t.Fatalf("create hidden test window: %v", err)
	}
	defer user32.NewProc("DestroyWindow").Call(window)
	clicked := make(chan struct{}, 1)
	notifier, err := New(func(string, string) {}, func() { clicked <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	defer notifier.Close()
	user32.NewProc("SendMessageW").Call(window, 0x0400+100, 0, 0x0400+5)
	select {
	case <-clicked:
	case <-time.After(time.Second):
		t.Fatal("notification activation did not reach opener")
	}
}
