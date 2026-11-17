package network

import (
	"context"
	"net"
	"time"
)

// Probe multiple destinations without relying on the user's HTTP proxy.
func Online(parent context.Context) bool {
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	return reachable(parent, []string{"next.rka.jp:443", "1.1.1.1:443", "223.5.5.5:443"}, func(ctx context.Context, address string) (net.Conn, error) {
		return dialer.DialContext(ctx, "tcp", address)
	})
}

func reachable(parent context.Context, addresses []string, dial func(context.Context, string) (net.Conn, error)) bool {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	results := make(chan bool, len(addresses))
	for _, address := range addresses {
		go func(address string) {
			connection, err := dial(ctx, address)
			if err == nil {
				connection.Close()
			}
			results <- err == nil
		}(address)
	}
	for range addresses {
		select {
		case online := <-results:
			if online {
				return true
			}
		case <-ctx.Done():
			return false
		}
	}
	return false
}
