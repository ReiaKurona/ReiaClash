package trayui

import (
	"context"
	"errors"
	"testing"
)

func TestExitBlocksRecoveryLaunch(t *testing.T) {
	gate := &agentLaunchGate{}
	if !gate.beginExit() {
		t.Fatal("initial exit rejected")
	}
	launched := false
	err := gate.launch(func() error { launched = true; return nil })
	if !errors.Is(err, context.Canceled) || launched {
		t.Fatal("watchdog relaunched agent during exit")
	}
	if gate.beginExit() {
		t.Fatal("duplicate exit started")
	}
	gate.cancelExit()
	if err := gate.launch(func() error { launched = true; return nil }); err != nil || !launched {
		t.Fatal("failed exit did not restore recovery")
	}
}
