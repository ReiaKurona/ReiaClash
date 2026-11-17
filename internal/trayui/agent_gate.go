package trayui

import (
	"context"
	"sync"
)

// Serialize the decision to exit against the actual launch, not just a scan.
type agentLaunchGate struct {
	mu      sync.Mutex
	exiting bool
}

func (g *agentLaunchGate) beginExit() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.exiting {
		return false
	}
	g.exiting = true
	return true
}
func (g *agentLaunchGate) cancelExit()     { g.mu.Lock(); g.exiting = false; g.mu.Unlock() }
func (g *agentLaunchGate) isExiting() bool { g.mu.Lock(); defer g.mu.Unlock(); return g.exiting }
func (g *agentLaunchGate) launch(start func() error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.exiting {
		return context.Canceled
	}
	return start()
}

var desktopAgentGate agentLaunchGate
