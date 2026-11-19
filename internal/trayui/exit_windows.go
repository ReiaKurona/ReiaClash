//go:build windows

package trayui

import (
	"context"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var exitMessageBox = windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW")

func showExitDialog(ctx context.Context) (exitChoice, error) {
	if err := ctx.Err(); err != nil {
		return cancelExit, err
	}
	message, _ := windows.UTF16PtrFromString(exitMessage + "\n\n是：继续后台运行\n否：注销服务并退出\n取消：返回托盘")
	title, _ := windows.UTF16PtrFromString(exitTitle)
	const (
		yesNoCancel   = 0x00000003
		iconQuestion  = 0x00000020
		defaultCancel = 0x00000200
		setForeground = 0x00010000
	)
	result, _, err := exitMessageBox.Call(0, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), yesNoCancel|iconQuestion|defaultCancel|setForeground)
	switch result {
	case 6: // IDYES
		return keepService, nil
	case 7: // IDNO
		return uninstallService, nil
	case 2: // IDCANCEL
		return cancelExit, nil
	default:
		return cancelExit, fmt.Errorf("无法打开 Windows 退出对话框: %w", err)
	}
}
