package mihomo

import (
	"os"
	"path/filepath"
	"reia-clash/internal/config"
	"runtime"
	"strings"
	"testing"
)

func TestManagedMeowTUNCompatibility(t *testing.T) {
	settings := config.Defaults()
	settings.ActiveBinary = "meow.exe"
	settings.Enabled, settings.TunAuthorized = true, true
	rendered, err := BuildConfig([]byte("rules: [MATCH,DIRECT]"), settings, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rendered), "tun:\n  enable: true\n") {
		t.Fatal("TUN should remain available")
	}
	if got := strings.Contains(string(rendered), "dns-hijack: []"); got != (runtime.GOOS == "windows" || runtime.GOOS == "linux") {
		t.Fatal("unexpected DNS redirect policy")
	}
	if strings.Contains(string(rendered), "stack: mips") || !strings.Contains(string(rendered), "auto-route: true") {
		t.Fatal("unexpected meow routing configuration")
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "linux" {
		if !strings.Contains(string(rendered), "sniffer:") {
			t.Fatal("default hostname sniffing unavailable")
		}
		custom, err := BuildConfig([]byte("sniffer: {enable: false}"), settings, "test")
		if err != nil || strings.Count(string(custom), "sniffer:") != 1 || !strings.Contains(string(custom), "{enable: false}") {
			t.Fatal("subscription sniffer configuration was changed")
		}
	}
	settings.ActiveBinary = "mihomo.exe"
	rendered, err = BuildConfig([]byte("rules: [MATCH,DIRECT]"), settings, "test")
	if err != nil || strings.Contains(string(rendered), "dns-hijack: []") {
		t.Fatalf("mihomo policy changed: %v", err)
	}
}

func TestMeowTunIPv6UsesSubscriptionPolicy(t *testing.T) {
	for _, test := range []struct{ profile, want string }{
		{"dns: {ipv6: true}", "fdfe:dcba:9876::1/126"},
		{"ipv6: true\ntun: {inet6-address: ['fd00::5/126']}", "fd00::5/126"},
		{"ipv6: false\ndns: {ipv6: true}", ""},
		{"ipv6: true\ntun: {inet6-address: []}", ""},
		{"dns: {nameserver: ['2606:4700:4700::1111']}", ""},
	} {
		got, err := MeowTunOptions([]byte(test.profile))
		if err != nil || got.IPv6Address != test.want {
			t.Fatalf("TUN IPv6 address for %q = %q, %v; want %q", test.profile, got.IPv6Address, err, test.want)
		}
	}
}

func TestMeowDNSHijackFollowsExplicitSubscriptionPolicy(t *testing.T) {
	for _, test := range []struct {
		dns, entries string
		want         bool
	}{
		{"true", "['any:53', 'tcp://any:53']", true},
		{"true", "['udp://[::]:53']", true},
		{"true", "['tcp://any:53']", false},
		{"true", "['192.0.2.10:53']", false},
		{"true", "[]", false},
		{"false", "['any:53']", false},
	} {
		profile := []byte("dns: {enable: " + test.dns + ", enhanced-mode: fake-ip}\ntun: {dns-hijack: " + test.entries + "}\n")
		settings := config.Defaults()
		settings.ActiveBinary = "meow.exe"
		rendered, err := BuildConfig(profile, settings, "test")
		want := test.want && preserveMeowSystemDNS()
		if err != nil || strings.Contains(string(rendered), `dns-hijack: ["any:53"]`) != want {
			t.Fatalf("unexpected meow DNS policy with enable=%s, entries=%s: %v", test.dns, test.entries, err)
		}
		settings.ActiveBinary = "mihomo.exe"
		rendered, err = BuildConfig(profile, settings, "test")
		if err != nil || strings.Contains(string(rendered), "dns-hijack:") {
			t.Fatalf("subscription DNS interception changed the mihomo config: %v", err)
		}
	}
}

func TestTargetKernelValidatesCachedSubscriptionInEitherDirection(t *testing.T) {
	meow, mihomo := os.Getenv("REIA_TEST_MEOW"), os.Getenv("REIA_TEST_MIHOMO")
	if meow == "" || mihomo == "" {
		t.Skip("set REIA_TEST_MEOW and REIA_TEST_MIHOMO for the installed binaries")
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "mihomo.yaml")
	store, err := config.OpenStore(filepath.Join(directory, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{dataDir: directory, configPath: path, settings: store, kernelDataDir: directory, secret: "test-config"}
	profile := []byte("dns:\n  ipv6: true\n  enable: true\n  enhanced-mode: fake-ip\n  nameserver: ['https://1.1.1.1/dns-query']\ntun:\n  dns-hijack: ['any:53', 'tcp://any:53']\nrules:\n  - MATCH,DIRECT\n")
	profilePath := filepath.Join(directory, "profile.yaml")
	if err := os.WriteFile(profilePath, profile, 0600); err != nil {
		t.Fatal(err)
	}
	// Simulate the legacy shared file that produced the reported type error.
	stale := "tun: {auto-route: global}\n"
	if err := os.WriteFile(path, []byte(stale), 0600); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{meow, mihomo}, {mihomo, meow}} {
		if _, err := store.Update(func(s *config.Settings) error { s.ActiveBinary = pair[0]; return nil }); err != nil {
			t.Fatal(err)
		}
		if err := m.ValidateBinaryConfig(pair[1]); err != nil {
			t.Fatal(err)
		}
	}
	if content, err := os.ReadFile(path); err != nil || string(content) != stale {
		t.Fatalf("preflight modified live config: %v", err)
	}
	if err := m.PersistCurrentRuntimeConfig(); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(path); err != nil || !strings.Contains(string(content), "stack: mips") {
		t.Fatalf("mihomo config was not regenerated: %v", err)
	}
	if err := m.persistRuntimeForBinary(meow); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(m.runtimePath(meow)); err != nil || !strings.Contains(string(content), "inet6-address:") {
		t.Fatalf("meow config lost TUN IPv6: %v", err)
	}
	if content, err := os.ReadFile(profilePath); err != nil || string(content) != string(profile) {
		t.Fatalf("cached subscription was changed: %v", err)
	}
}
