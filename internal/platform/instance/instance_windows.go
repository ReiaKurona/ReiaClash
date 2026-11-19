//go:build windows

package instance

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"path/filepath"
	"strings"
	"unsafe"
)

type Lock struct{ handle windows.Handle }

func Acquire(role string) (*Lock, error) {
	return acquire(role, nil)
}

// AcquireIn is shared between a user's agent and its privileged OS service.
func AcquireIn(directory, role string) (*Lock, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;AU)")
	if err != nil {
		return nil, err
	}
	attributes := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: descriptor}
	key := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(absolute))))
	return acquire(fmt.Sprintf("%s.%x", role, key[:16]), attributes)
}

func acquire(role string, attributes *windows.SecurityAttributes) (*Lock, error) {
	name, err := windows.UTF16PtrFromString(`Global\ReiaClash.` + role)
	if err != nil {
		return nil, err
	}
	// The lease is represented by an open handle. Requesting full mutex
	// access would require rights the Agent does not use during a handover.
	handle, err := windows.CreateMutexEx(attributes, name, 0, windows.SYNCHRONIZE)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
		return nil, ErrAlreadyRunning
	}
	if err != nil {
		return nil, err
	}
	return &Lock{handle: handle}, nil
}
func (l *Lock) Close() error { return windows.CloseHandle(l.handle) }
