//go:build windows

package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"reia-clash/internal/platform/launch"
)

type dnsAdapterState struct {
	GUID         string   `json:"guid"`
	IPv6         bool     `json:"ipv6"`
	Servers      []string `json:"servers"`
	RedirectSeen bool     `json:"redirect_seen,omitempty"`
}

const dnsRecoveryFile = "meow-dns-backup.json"

var (
	ipHelperDNS    = windows.NewLazySystemDLL("iphlpapi.dll")
	dnsGUIDToLUID  = ipHelperDNS.NewProc("ConvertInterfaceGuidToLuid")
	dnsLUIDToIndex = ipHelperDNS.NewProc("ConvertInterfaceLuidToIndex")
)

func dnsRegistryPath(ipv6 bool) string {
	service := "Tcpip"
	if ipv6 {
		service = "Tcpip6"
	}
	return `SYSTEM\CurrentControlSet\Services\` + service + `\Parameters\Interfaces`
}

func dnsRegistryServers(root registry.Key, guid string) ([]string, error) {
	key, err := registry.OpenKey(root, guid, registry.QUERY_VALUE)
	if err != nil {
		return nil, err
	}
	defer key.Close()
	value, _, err := key.GetStringValue("NameServer")
	if errors.Is(err, registry.ErrUnexpectedType) {
		values, _, err := key.GetStringsValue("NameServer")
		return values, err
	}
	if errors.Is(err, registry.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }), nil
}

// Capture the per-family static settings. An empty list means use DHCP or the
// other Windows automatic resolver settings for that family.
func SaveMeowDNS(root string) error {
	path := filepath.Join(root, dnsRecoveryFile)
	if _, err := os.Stat(path); err == nil {
		return errors.New("上次 meow-rs 的 DNS 尚未恢复")
	} else if !os.IsNotExist(err) {
		return err
	}
	var outbound uint32
	if err := windows.GetBestInterfaceEx(&windows.SockaddrInet4{Addr: [4]byte{208, 67, 222, 222}}, &outbound); err != nil {
		return fmt.Errorf("查找 meow-rs 出站网卡失败: %w", err)
	}
	states := make([]dnsAdapterState, 0, 2)
	for _, ipv6 := range []bool{false, true} {
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, dnsRegistryPath(ipv6), registry.ENUMERATE_SUB_KEYS)
		if errors.Is(err, registry.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		names, err := key.ReadSubKeyNames(-1)
		if err != nil {
			key.Close()
			return err
		}
		for _, name := range names {
			index, err := dnsInterfaceIndex(name)
			if err != nil || index != outbound {
				continue
			}
			addresses, err := dnsRegistryServers(key, name)
			if errors.Is(err, registry.ErrNotExist) {
				continue
			}
			if err != nil {
				key.Close()
				return err
			}
			states = append(states, dnsAdapterState{GUID: name, IPv6: ipv6, Servers: addresses})
		}
		key.Close()
	}
	if len(states) == 0 {
		return errors.New("找不到出站网卡的 DNS 设置")
	}
	return writeDNSRecovery(path, states)
}

func writeDNSRecovery(path string, states []dnsAdapterState) error {
	data, err := json.Marshal(states)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".meow-dns-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	_, writeErr := temp.Write(data)
	if err := errors.Join(writeErr, temp.Close()); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}

func readDNSRecovery(path string) ([]dnsAdapterState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var states []dnsAdapterState
	if err := json.Unmarshal(data, &states); err != nil {
		return nil, fmt.Errorf("读取 DNS 恢复记录失败: %w", err)
	}
	return states, nil
}

// ObserveMeowDNS records that the managed child really replaced this
// adapter's settings. This also lets us restore a static list if the child
// subsequently removes its loopback setting by switching back to automatic DNS.
func ObserveMeowDNS(root string) error {
	path := filepath.Join(root, dnsRecoveryFile)
	states, err := readDNSRecovery(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	changed := false
	for index, state := range states {
		if state.RedirectSeen {
			continue
		}
		key, err := registry.OpenKey(registry.LOCAL_MACHINE, dnsRegistryPath(state.IPv6), registry.ENUMERATE_SUB_KEYS)
		if err != nil {
			return err
		}
		current, err := dnsRegistryServers(key, state.GUID)
		key.Close()
		if err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		if err == nil && shouldRestoreMeowDNS(state, current) {
			states[index].RedirectSeen = true
			changed = true
		}
	}
	if changed {
		return writeDNSRecovery(path, states)
	}
	return nil
}

func RestoreMeowDNS(root string) error {
	path := filepath.Join(root, dnsRecoveryFile)
	states, err := readDNSRecovery(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	var failures error
	for _, state := range states {
		failures = errors.Join(failures, restoreDNSAdapter(ctx, state))
	}
	if failures != nil {
		return failures
	}
	if err := launch.CommandContext(ctx, "ipconfig.exe", "/flushdns").Run(); err != nil {
		return fmt.Errorf("刷新 Windows DNS 缓存失败: %w", err)
	}
	return os.Remove(path)
}

func restoreDNSAdapter(ctx context.Context, state dnsAdapterState) error {
	root, err := registry.OpenKey(registry.LOCAL_MACHINE, dnsRegistryPath(state.IPv6), registry.ENUMERATE_SUB_KEYS)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	current, err := dnsRegistryServers(root, state.GUID)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil || !shouldRestoreMeowDNS(state, current) {
		return err
	}
	for _, server := range state.Servers {
		addr, err := netip.ParseAddr(server)
		if err != nil || addr.Is6() != state.IPv6 {
			return errors.New("DNS 恢复记录含无效的服务器地址")
		}
	}
	index, err := dnsInterfaceIndex(state.GUID)
	if err != nil {
		return err
	}
	family := "IPv4"
	if state.IPv6 {
		family = "IPv6"
	}
	input := "Get-DnsClientServerAddress -InterfaceIndex " + strconv.FormatUint(uint64(index), 10) + " -AddressFamily " + family
	action := " -ResetServerAddresses"
	script := "$ErrorActionPreference='Stop'; "
	if len(state.Servers) != 0 {
		script += "$servers = [Console]::In.ReadToEnd().Split([char]10); "
		action = " -ServerAddresses $servers"
	}
	cmd := launch.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script+input+" | Set-DnsClientServerAddress"+action)
	cmd.Stdin = strings.NewReader(strings.Join(state.Servers, "\n"))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("恢复网卡 %d 的 %s DNS 失败: %w", index, family, err)
	}
	current, err = dnsRegistryServers(root, state.GUID)
	if err != nil {
		return err
	}
	if !equalDNSServers(current, state.Servers) {
		return fmt.Errorf("网卡 %d 的 %s DNS 恢复后验证失败", index, family)
	}
	return nil
}

func dnsInterfaceIndex(name string) (uint32, error) {
	guid, err := windows.GUIDFromString(name)
	if err != nil {
		return 0, err
	}
	var luid uint64
	var index uint32
	if result, _, _ := dnsGUIDToLUID.Call(uintptr(unsafe.Pointer(&guid)), uintptr(unsafe.Pointer(&luid))); result != 0 {
		return 0, fmt.Errorf("查找 DNS 网卡失败: %w", windows.Errno(result))
	}
	if result, _, _ := dnsLUIDToIndex.Call(uintptr(unsafe.Pointer(&luid)), uintptr(unsafe.Pointer(&index))); result != 0 {
		return 0, fmt.Errorf("查找 DNS 网卡编号失败: %w", windows.Errno(result))
	}
	return index, nil
}

func equalDNSServers(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		x, xErr := netip.ParseAddr(a[index])
		y, yErr := netip.ParseAddr(b[index])
		if !strings.EqualFold(a[index], b[index]) && (xErr != nil || yErr != nil || x != y) {
			return false
		}
	}
	return true
}

func shouldRestoreMeowDNS(state dnsAdapterState, current []string) bool {
	if state.RedirectSeen && len(current) == 0 && len(state.Servers) != 0 {
		return true
	}
	if len(current) != 1 || equalDNSServers(state.Servers, current) {
		return false
	}
	loopback := "127.0.0.1"
	if state.IPv6 {
		loopback = "::1"
	}
	return equalDNSServers(current, []string{loopback})
}
