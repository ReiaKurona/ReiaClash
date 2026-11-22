//go:build darwin

package notification

import (
	"fmt"
	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
	branding "reia-clash/ico"
	"runtime"
	"unsafe"
)

// Install a delegate on the notification center already used by systray.
// Initialization and cleanup run on the tray's Cocoa/main thread.
type Notifier struct {
	show             func(string, string)
	center, delegate uintptr
	objc             unsafe.Pointer
}

func (n *Notifier) call(name string, args ...uintptr) (uintptr, error) {
	symbol, err := ffi.GetSymbol(n.objc, name)
	if err != nil {
		return 0, err
	}
	descriptors := make([]*types.TypeDescriptor, len(args))
	pointers := make([]unsafe.Pointer, len(args))
	for index := range args {
		descriptors[index] = types.PointerTypeDescriptor
		pointers[index] = unsafe.Pointer(&args[index])
	}
	cif := &types.CallInterface{}
	if err := ffi.PrepareCallInterface(cif, types.DefaultCall, types.PointerTypeDescriptor, descriptors); err != nil {
		return 0, err
	}
	var result uintptr
	_, err = ffi.CallFunction(cif, symbol, unsafe.Pointer(&result), pointers)
	runtime.KeepAlive(args)
	return result, err
}
func (n *Notifier) textCall(symbol, value string) (uintptr, error) {
	bytes := append([]byte(value), 0)
	result, err := n.call(symbol, uintptr(unsafe.Pointer(&bytes[0])))
	runtime.KeepAlive(bytes)
	return result, err
}
func (n *Notifier) send(object uintptr, selector string, args ...uintptr) (uintptr, error) {
	command, err := n.textCall("sel_registerName", selector)
	if err != nil {
		return 0, err
	}
	return n.call("objc_msgSend", append([]uintptr{object, command}, args...)...)
}

func New(show func(string, string), open func()) (*Notifier, error) {
	notifier := &Notifier{show: show}
	library, err := ffi.LoadLibrary("/usr/lib/libobjc.A.dylib")
	if err != nil {
		return notifier, err
	}
	notifier.objc = library
	_ = notifier.setApplicationIcon()
	if processClass, err := notifier.textCall("objc_getClass", "NSProcessInfo"); err == nil {
		if process, err := notifier.send(processClass, "processInfo"); err == nil {
			if name, err := notifier.textCall("sel_registerName", "stringWithUTF8String:"); err == nil {
				strClass, _ := notifier.textCall("objc_getClass", "NSString")
				data := append([]byte(branding.AppName), 0)
				value, err := notifier.call("objc_msgSend", strClass, name, uintptr(unsafe.Pointer(&data[0])))
				runtime.KeepAlive(data)
				if err == nil && value != 0 {
					_, _ = notifier.send(process, "setProcessName:", value)
				}
			}
		}
	}
	base, err := notifier.textCall("objc_getClass", "NSObject")
	if err != nil {
		return notifier, err
	}
	className := append([]byte("ReiaClashNotificationDelegate"), 0)
	class, err := notifier.call("objc_allocateClassPair", base, uintptr(unsafe.Pointer(&className[0])), 0)
	runtime.KeepAlive(className)
	if err != nil || class == 0 {
		return notifier, fmt.Errorf("创建通知代理失败: %v", err)
	}
	activate := ffi.NewCallback(func(self, command, center, notification uintptr) uintptr { go open(); return 0 })
	present := ffi.NewCallback(func(self, command, center, notification uintptr) uintptr { return 1 })
	for _, method := range []struct {
		name, encoding string
		callback       uintptr
	}{
		{"userNotificationCenter:didActivateNotification:", "v@:@@", activate},
		{"userNotificationCenter:shouldPresentNotification:", "B@:@@", present},
	} {
		selector, err := notifier.textCall("sel_registerName", method.name)
		if err != nil {
			return notifier, err
		}
		encoding := append([]byte(method.encoding), 0)
		_, err = notifier.call("class_addMethod", class, selector, method.callback, uintptr(unsafe.Pointer(&encoding[0])))
		runtime.KeepAlive(encoding)
		if err != nil {
			return notifier, err
		}
	}
	if _, err := notifier.call("objc_registerClassPair", class); err != nil {
		return notifier, err
	}
	object, err := notifier.send(class, "alloc")
	if err != nil {
		return notifier, err
	}
	notifier.delegate, err = notifier.send(object, "init")
	if err != nil {
		return notifier, err
	}
	centerClass, err := notifier.textCall("objc_getClass", "NSUserNotificationCenter")
	if err != nil {
		return notifier, err
	}
	notifier.center, err = notifier.send(centerClass, "defaultUserNotificationCenter")
	if err != nil || notifier.center == 0 {
		return notifier, fmt.Errorf("系统通知中心不可用: %v", err)
	}
	_, err = notifier.send(notifier.center, "setDelegate:", notifier.delegate)
	return notifier, err
}

func (n *Notifier) setApplicationIcon() error {
	dataClass, err := n.textCall("objc_getClass", "NSData")
	if err != nil {
		return err
	}
	data, err := n.send(dataClass, "dataWithBytes:length:", uintptr(unsafe.Pointer(&branding.PNG[0])), uintptr(len(branding.PNG)))
	runtime.KeepAlive(branding.PNG)
	if err != nil {
		return err
	}
	imageClass, err := n.textCall("objc_getClass", "NSImage")
	if err != nil {
		return err
	}
	object, err := n.send(imageClass, "alloc")
	if err != nil {
		return err
	}
	icon, err := n.send(object, "initWithData:", data)
	if err != nil || icon == 0 {
		return fmt.Errorf("读取应用图标失败: %v", err)
	}
	defer n.send(icon, "release")
	appClass, err := n.textCall("objc_getClass", "NSApplication")
	if err != nil {
		return err
	}
	app, err := n.send(appClass, "sharedApplication")
	if err != nil {
		return err
	}
	_, err = n.send(app, "setApplicationIconImage:", icon)
	return err
}
func (n *Notifier) Show(title, message string) error { n.show(title, message); return nil }
func (n *Notifier) Close() {
	if n.center != 0 {
		n.send(n.center, "setDelegate:", 0)
	}
	if n.delegate != 0 {
		n.send(n.delegate, "release")
	}
}

func RegisterApp() error { return nil }
