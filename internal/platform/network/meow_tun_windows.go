//go:build windows

package network

import (
	"net"
	"strings"

	"golang.org/x/sys/windows"
)

// ActiveMeowTun finds the running Windows adapter through both split IPv4
// routes. Merely finding an adapter does not establish that it has routes.
func ActiveMeowTun(configuredDevice string) string {
	var lower, upper uint32
	if err := windows.GetBestInterfaceEx(&windows.SockaddrInet4{Addr: [4]byte{1, 1, 1, 1}}, &lower); err != nil {
		return ""
	}
	if err := windows.GetBestInterfaceEx(&windows.SockaddrInet4{Addr: [4]byte{208, 67, 222, 222}}, &upper); err != nil || lower == 0 || lower != upper {
		return ""
	}
	iface, err := net.InterfaceByIndex(int(lower))
	if err != nil || iface.Flags&net.FlagUp == 0 {
		return ""
	}
	if strings.EqualFold(iface.Name, "meow-tun") || (configuredDevice != "" && strings.EqualFold(iface.Name, configuredDevice)) {
		return iface.Name
	}
	return ""
}
