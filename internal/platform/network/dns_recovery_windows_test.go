//go:build windows

package network

import "testing"

func TestMeowDNSRecoveryChangesOnlyOwnedLoopback(t *testing.T) {
	for _, tc := range []struct {
		state   dnsAdapterState
		current []string
		want    bool
	}{
		{dnsAdapterState{}, []string{"127.0.0.1"}, true},
		{dnsAdapterState{IPv6: true, Servers: []string{"2001:db8::1"}}, []string{"0:0:0:0:0:0:0:1"}, true},
		{dnsAdapterState{Servers: []string{"127.0.0.1"}}, []string{"127.0.0.1"}, false},
		{dnsAdapterState{}, []string{"127.0.0.1", "192.0.2.53"}, false},
		{dnsAdapterState{}, []string{"192.0.2.53"}, false},
		{dnsAdapterState{Servers: []string{"192.0.2.53"}, RedirectSeen: true}, nil, true},
		{dnsAdapterState{Servers: []string{"192.0.2.53"}}, nil, false},
	} {
		if got := shouldRestoreMeowDNS(tc.state, tc.current); got != tc.want {
			t.Fatalf("recovery decision for %v: got %v, want %v", tc.current, got, tc.want)
		}
	}
}
