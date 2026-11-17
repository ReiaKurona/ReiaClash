package coreservice

import "context"

type readinessKey struct{}

// NotifyReady publishes SCM Running only after our own kernel is healthy.
func NotifyReady(ctx context.Context) {
	if ready, ok := ctx.Value(readinessKey{}).(chan struct{}); ok {
		select {
		case ready <- struct{}{}:
		default:
		}
	}
}
