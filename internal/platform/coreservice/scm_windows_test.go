//go:build windows

package coreservice

import (
	"context"
	"errors"
	"golang.org/x/sys/windows/svc"
	"testing"
	"time"
)

func TestSCMStopCancelsAndWaitsForCore(t *testing.T) {
	requests := make(chan svc.ChangeRequest, 1)
	statuses := make(chan svc.Status, 8)
	stopped := make(chan struct{})
	handler := &serviceHandler{run: func(ctx context.Context) error { NotifyReady(ctx); <-ctx.Done(); close(stopped); return nil }}
	result := make(chan uint32, 1)
	go func() { _, code := handler.Execute(nil, requests, statuses); result <- code }()
	if status := <-statuses; status.State != svc.StartPending {
		t.Fatal(status)
	}
	if status := <-statuses; status.State != svc.Running {
		t.Fatal(status)
	}
	requests <- svc.ChangeRequest{Cmd: svc.Stop}
	if status := <-statuses; status.State != svc.StopPending {
		t.Fatal(status)
	}
	select {
	case code := <-result:
		if code != 0 {
			t.Fatal(code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SCM stop timed out")
	}
	select {
	case <-stopped:
	default:
		t.Fatal("service reported stopped before cleanup")
	}
}

func TestSCMReportsCoreFailure(t *testing.T) {
	handler := &serviceHandler{run: func(context.Context) error { return errors.New("core failed") }}
	specific, code := handler.Execute(nil, make(chan svc.ChangeRequest), make(chan svc.Status, 8))
	if !specific || code == 0 {
		t.Fatalf("service failure was hidden: %v %d", specific, code)
	}
}

func TestSCMStaysPendingAndCleansUpBeforeReadiness(t *testing.T) {
	requests := make(chan svc.ChangeRequest, 1)
	statuses := make(chan svc.Status, 8)
	handler := &serviceHandler{run: func(ctx context.Context) error { <-ctx.Done(); return nil }}
	done := make(chan uint32, 1)
	go func() { _, code := handler.Execute(nil, requests, statuses); done <- code }()
	if status := <-statuses; status.State != svc.StartPending {
		t.Fatal(status)
	}
	select {
	case status := <-statuses:
		t.Fatalf("SCM announced readiness before kernel: %v", status)
	default:
	}
	requests <- svc.ChangeRequest{Cmd: svc.Stop}
	if status := <-statuses; status.State != svc.StopPending {
		t.Fatal(status)
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Fatal(code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pending service did not clean up")
	}
}

func TestServiceEntryCannotBecomeUnmanagedAgent(t *testing.T) {
	called := false
	err := RunService(func(context.Context) error { called = true; return nil })
	if err == nil || called {
		t.Fatal("interactive --core-service fallback started an unmanaged service process")
	}
}
