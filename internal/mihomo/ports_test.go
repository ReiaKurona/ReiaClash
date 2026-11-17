package mihomo

import (
	"net"
	"strings"
	"testing"
)

func TestPortConflictDiagnosticAndRecovery(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	address := listener.Addr().String()
	if err := checkPorts(address); err == nil || !strings.Contains(err.Error(), address) {
		t.Fatalf("missing port diagnostic: %v", err)
	}
	listener.Close()
	if err := checkPorts(address); err != nil {
		t.Fatal(err)
	}
}
