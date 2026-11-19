package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaults_DisabledByDefault(t *testing.T) {
	d := Defaults()
	if d.Enabled {
		t.Fatalf("首页开关 Enabled 默认必须为 false，防止首次启动即开启代理")
	}
	if d.UseSystemProxy || !d.UseTun || d.TunStack != "mips" {
		t.Fatalf("首次启动的接管偏好应为 TUN/mips: %+v", d)
	}
}

func TestSavedTunPreferencesRemainCompatible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"use_system_proxy":true,"use_tun":false,"tun_stack":"mixed"}`), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	s := store.Snapshot()
	if !s.UseSystemProxy || s.UseTun || s.TunStack != "mixed" || s.TunDevice != "" {
		t.Fatalf("existing settings changed: %+v", s)
	}
	valid := "ReiaWork"
	if runtime.GOOS == "darwin" {
		valid = "utun12"
	}
	if !ValidTunDevice(valid) || !ValidTunDevice("") || ValidTunDevice("eth0") || ValidTunDevice("Reia../other") {
		t.Fatal("TUN interface name validation failed")
	}
}
