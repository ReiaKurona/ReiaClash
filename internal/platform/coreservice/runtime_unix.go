//go:build !windows

package coreservice

import (
	"context"
	"reia-clash/internal/platform/signals"
)

func RunService(run func(context.Context) error) error {
	ctx, cancel := signals.Context()
	defer cancel()
	return run(ctx)
}
