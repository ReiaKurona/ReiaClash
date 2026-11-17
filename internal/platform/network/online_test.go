package network

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestOnlineProbeFallbackAndOffline(t *testing.T) {
	dial := func(_ context.Context, address string) (net.Conn, error) {
		if address == "working" {
			connection, peer := net.Pipe()
			peer.Close()
			return connection, nil
		}
		return nil, errors.New("unreachable")
	}
	if !reachable(context.Background(), []string{"down", "working"}, dial) {
		t.Fatal("one failed website must not force offline")
	}
	if reachable(context.Background(), []string{"down", "down2"}, dial) {
		t.Fatal("offline reported online")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if reachable(ctx, []string{"pending"}, func(ctx context.Context, _ string) (net.Conn, error) { <-ctx.Done(); return nil, ctx.Err() }) {
		t.Fatal("timeout reported online")
	}
}
