//go:build windows

package coreservice

import (
	"context"
	"errors"
	"testing"
	"unsafe"
)

func TestShellExecuteInfoLayoutAndCancelledAuthorization(t *testing.T) {
	wantSize, wantProcessOffset := uintptr(112), uintptr(104)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		wantSize, wantProcessOffset = 60, 56
	}
	var info shellExecuteInfo
	if unsafe.Sizeof(info) != wantSize || unsafe.Offsetof(info.Process) != wantProcessOffset {
		t.Fatalf("invalid SHELLEXECUTEINFOW: size=%d hProcess offset=%d", unsafe.Sizeof(info), unsafe.Offsetof(info.Process))
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if handle, err := startElevated(ctx, "not-started.exe", nil); handle != 0 || !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancelled request: handle=%d error=%v", handle, err)
	}
}
