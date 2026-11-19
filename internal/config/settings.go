package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
)

// Settings 是面板管理的持久化偏好。实际是否接管流量还要结合核心和系统状态判断。
type Settings struct {
	Enabled               bool   `json:"enabled"`
	UseSystemProxy        bool   `json:"use_system_proxy"`
	UseTun                bool   `json:"use_tun"`
	TunAuthorized         bool   `json:"-"`
	TunStack              string `json:"tun_stack"`
	TunDevice             string `json:"tun_device"`
	AllowLAN              bool   `json:"allow_lan"`
	GeoLowMemory          bool   `json:"geo_low_memory"`
	AutoStart             bool   `json:"autostart_enabled"`
	AutoUpdateEnabled     bool   `json:"auto_update_enabled"`
	UpdateIntervalHours   int    `json:"update_interval_hours"`
	OutboundMode          string `json:"outbound_mode"`
	SubscriptionURL       string `json:"subscription_url,omitempty"`
	SubscriptionUpdatedAt int64  `json:"subscription_updated_at,omitempty"`
	ActiveBinary          string `json:"active_binary,omitempty"`
	PreviousBinary        string `json:"previous_binary,omitempty"`
}

func Defaults() Settings {
	return Settings{
		Enabled:             false, // 首页总开关默认关闭；首次启动绝不默认开启系统代理或接管流量
		UseTun:              true,
		TunStack:            "mips",
		TunDevice:           defaultTunDevice(),
		GeoLowMemory:        true,
		UpdateIntervalHours: 24,
		OutboundMode:        "rule",
	}
}

// Store 串行化 settings.json 的读取和原子写入，避免并发请求覆盖彼此更新。
type Store struct {
	mu   sync.Mutex
	path string
	data Settings
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, data: Defaults()}
	bytes, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		// Existing installations may already have a live adapter with the kernel's default name.
		s.data.TunDevice = ""
		if err := json.Unmarshal(bytes, &s.data); err != nil {
			return nil, err
		}
		s.applyDefaults()
	}
	return s, nil
}

func ValidTunStack(stack string) bool {
	return stack == "mixed" || stack == "gvisor" || stack == "system" || stack == "mips"
}

var reiaTunName = regexp.MustCompile(`^Reia[A-Za-z0-9_-]{0,11}$`)
var macTunName = regexp.MustCompile(`^utun(0|[1-9][0-9]{0,3})$`)

func defaultTunDevice() string {
	if runtime.GOOS == "darwin" {
		return ""
	}
	return "ReiaClash"
}

func ValidTunDevice(name string) bool {
	if name == "" {
		return true
	}
	if runtime.GOOS == "darwin" {
		return macTunName.MatchString(name)
	}
	return reiaTunName.MatchString(name)
}

func (s *Store) applyDefaults() {
	defaults := Defaults()
	if !ValidTunStack(s.data.TunStack) {
		s.data.TunStack = defaults.TunStack
	}
	if !ValidTunDevice(s.data.TunDevice) {
		s.data.TunDevice = ""
	}
	if s.data.UpdateIntervalHours < 1 || s.data.UpdateIntervalHours > 168 {
		s.data.UpdateIntervalHours = defaults.UpdateIntervalHours
	}
	if s.data.OutboundMode != "rule" && s.data.OutboundMode != "global" && s.data.OutboundMode != "direct" {
		s.data.OutboundMode = defaults.OutboundMode
	}
}

func (s *Store) Snapshot() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data
}

// Reload refreshes settings changed by the desktop agent in another process.
func (s *Store) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	next := Defaults()
	next.TunDevice = ""
	if err := json.Unmarshal(data, &next); err != nil {
		return err
	}
	next.TunAuthorized = s.data.TunAuthorized
	s.data = next
	s.applyDefaults()
	return nil
}

// SetRuntimeTunAuthorization 记录当前进程是否具备 TUN 权限。
// 该值不写入 settings.json，因为系统权限可能在两次启动之间发生变化。
func (s *Store) SetRuntimeTunAuthorization(authorized bool) {
	s.mu.Lock()
	s.data.TunAuthorized = authorized
	s.mu.Unlock()
}

func (s *Store) Update(change func(*Settings) error) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.data
	if err := change(&next); err != nil {
		return s.data, err
	}
	previous := s.data
	s.data = next
	if err := s.saveLocked(); err != nil {
		s.data = previous
		return s.data, err
	}
	return s.data, nil
}

func (s *Store) saveLocked() error {
	bytes, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, bytes, 0600); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		backup := s.path + ".previous"
		_ = os.Remove(backup)
		if err := os.Rename(s.path, backup); err != nil && !errors.Is(err, os.ErrNotExist) {
			_ = os.Remove(tmp)
			return err
		}
		if err := os.Rename(tmp, s.path); err != nil {
			_ = os.Rename(backup, s.path)
			_ = os.Remove(tmp)
			return err
		}
		_ = os.Remove(backup)
		return nil
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
