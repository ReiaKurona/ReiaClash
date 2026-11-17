//go:build windows

package signals

import (
	"context"
	"os"
	"os/signal"
)

func Context() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}
