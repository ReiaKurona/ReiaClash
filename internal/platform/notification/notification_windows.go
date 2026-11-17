//go:build windows

package notification

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"syscall"
	"unsafe"
)

// Bridge balloon activation for the pinned systray v0.3 backend, which exposes
// tray clicks but currently drops NIN_BALLOONUSERCLICK. No global mouse hooks.
type Notifier struct {
	window, callback uintptr
	show             func(string, string)
}

var user32 = windows.NewLazySystemDLL("user32.dll")
var findWindow = user32.NewProc("FindWindowExW")
var windowPID = user32.NewProc("GetWindowThreadProcessId")
var comctl32 = windows.NewLazySystemDLL("comctl32.dll")
var subclass = comctl32.NewProc("SetWindowSubclass")
var removeSubclass = comctl32.NewProc("RemoveWindowSubclass")
var defaultSubclass = comctl32.NewProc("DefSubclassProc")

func New(show func(string, string), open func()) (*Notifier, error) {
	notifier := &Notifier{show: show}
	class, _ := windows.UTF16PtrFromString("GoGPUSystrayMsg")
	var previous uintptr
	for {
		window, _, _ := findWindow.Call(^uintptr(2), previous, uintptr(unsafe.Pointer(class)), 0)
		if window == 0 {
			return notifier, fmt.Errorf("未找到 ReiaClash 托盘通知窗口")
		}
		previous = window
		var pid uint32
		windowPID.Call(window, uintptr(unsafe.Pointer(&pid)))
		if pid == uint32(os.Getpid()) {
			notifier.window = window
			break
		}
	}
	notifier.callback = syscall.NewCallback(func(window, msg, wp, lp, id, data uintptr) uintptr {
		if msg == 0x0400+100 && lp&0xffff == 0x0400+5 {
			go open()
			return 0
		}
		result, _, _ := defaultSubclass.Call(window, msg, wp, lp)
		return result
	})
	if result, _, err := subclass.Call(notifier.window, notifier.callback, 1, 0); result == 0 {
		return notifier, fmt.Errorf("启用通知点击失败: %v", err)
	}
	return notifier, nil
}
func (n *Notifier) Show(title, message string) error { n.show(title, message); return nil }
func (n *Notifier) Close() {
	if n.window != 0 && n.callback != 0 {
		removeSubclass.Call(n.window, n.callback, 1)
	}
}
