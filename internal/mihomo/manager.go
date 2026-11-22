package mihomo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"reia-clash/internal/config"
	"reia-clash/internal/kernelbinary"
	"reia-clash/internal/platform/instance"
)

type Manager struct {
	strictChildJob bool
	operationMu    sync.Mutex
	runningBinary  string
	runningPID     int
	profileMu      sync.Mutex
	mu             sync.RWMutex
	dataDir        string
	kernelDataDir  string
	configPath     string
	binary         string
	secret         string
	settings       *config.Store
	logger         *slog.Logger
	started        bool
	running        bool
	lastError      string
	lastOutput     string
	restart        chan struct{}
	stop           context.CancelFunc
	done           chan struct{}
	notify         func(string, string)
	lifecycle      ProcessLifecycle
}

// ProcessLifecycle 把内核的启动、重启和状态查询交给本地进程监督器，
// 或交给独立特权服务；配置和 Mihomo 原生 API 客户端仍由 Manager 管理。
type ProcessLifecycle interface {
	Start(context.Context) error
	Status() (bool, string)
	Restart() error
	Shutdown() error
}

func NewManager(dataDir, configPath, binary string, settings *config.Store, logger *slog.Logger) (*Manager, error) {
	return newManager(dataDir, configPath, binary, settings, logger, true)
}

// NewDelegatedManager leaves the first runtime configuration to the service.
func NewDelegatedManager(dataDir, configPath, binary string, settings *config.Store, logger *slog.Logger) (*Manager, error) {
	return newManager(dataDir, configPath, binary, settings, logger, false)
}

func newManager(dataDir, configPath, binary string, settings *config.Store, logger *slog.Logger, writeRuntime bool) (*Manager, error) {
	kernelDataDir, err := config.MihomoDataDir()
	if err != nil {
		return nil, err
	}
	secretPath := filepath.Join(dataDir, "mihomo.secret")
	secretBytes, err := os.ReadFile(secretPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if errors.Is(err, os.ErrNotExist) {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		secretBytes = []byte(hex.EncodeToString(raw))
		if err := os.WriteFile(secretPath, secretBytes, 0600); err != nil {
			return nil, err
		}
	}
	if err := os.Chmod(secretPath, 0600); err != nil {
		return nil, err
	}
	secret := strings.TrimSpace(string(secretBytes))
	if secret == "" {
		return nil, fmt.Errorf("mihomo secret 文件为空")
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return nil, err
	}
	profilePath := filepath.Join(dataDir, "profile.yaml")
	profile, err := os.ReadFile(profilePath)
	if errors.Is(err, os.ErrNotExist) {
		profile = []byte("proxies: []\nproxy-groups: []\nrules:\n  - MATCH,DIRECT\n")
		if err := atomicWrite(profilePath, profile); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if binary != "" {
		if _, err := os.Stat(binary); err != nil {
			return nil, fmt.Errorf("mihomo 内核路径不可用 %q: %w", binary, err)
		}
	}
	manager := &Manager{
		dataDir: dataDir, kernelDataDir: kernelDataDir, configPath: configPath, binary: binary,
		secret: secret, settings: settings, logger: logger,
		restart: make(chan struct{}, 1),
	}
	if writeRuntime {
		if err := manager.persistRuntimeConfig(profile, manager.BinaryPath()); err != nil {
			return nil, err
		}
	}
	return manager, nil
}

func (m *Manager) runtimePath(binary string) string {
	if kernelbinary.IsMeow(binary) {
		return filepath.Join(filepath.Dir(m.configPath), "meow.yaml")
	}
	return m.configPath
}

func (m *Manager) RequireServiceChildJob() { m.strictChildJob = true }

func (m *Manager) Secret() string  { return m.secret }
func (m *Manager) APIBase() string { return "http://127.0.0.1:9090" }
func (m *Manager) Client() *Client { return NewClient(m.secret) }
func (m *Manager) BinaryPath() string {
	if active := m.settings.Snapshot().ActiveBinary; active != "" {
		if info, err := os.Stat(active); err == nil && !info.IsDir() {
			return active
		}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.binary
}

func (m *Manager) SetNotifier(notify func(string, string)) {
	m.mu.Lock()
	m.notify = notify
	m.mu.Unlock()
}

// SetLifecycle 在启动前替换进程生命周期控制器。没有安装特权服务时，
// Manager 保持使用内置 supervisor。
func (m *Manager) SetLifecycle(lifecycle ProcessLifecycle) {
	m.mu.Lock()
	m.lifecycle = lifecycle
	m.mu.Unlock()
}

func (m *Manager) UsesExternalLifecycle() bool {
	return m.delegatedLifecycle() != nil
}

// SupervisorStarted 返回本地 supervisor 是否正在管理内核生命周期，供已认证的服务接口查询。
func (m *Manager) SupervisorStarted() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.started
}

func (m *Manager) delegatedLifecycle() ProcessLifecycle {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lifecycle
}

func (m *Manager) emit(level, message string) {
	m.mu.RLock()
	notify := m.notify
	m.mu.RUnlock()
	if notify != nil {
		notify(level, message)
	}
}

func (m *Manager) Start(parent context.Context) error {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	if err := parent.Err(); err != nil {
		return err
	}
	if lifecycle := m.delegatedLifecycle(); lifecycle != nil {
		return lifecycle.Start(parent)
	}
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return nil
	}
	lease, err := instance.AcquireIn(m.dataDir, "kernel")
	if err != nil {
		m.mu.Unlock()
		return fmt.Errorf("内核已由其他 ReiaClash 进程管理，未启动重复实例: %w", err)
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	m.stop = cancel
	m.done = done
	m.started = true
	m.mu.Unlock()
	go func() {
		// Release the lease before closing done, so Shutdown is a complete
		// handover barrier for another supervisor.
		defer close(done)
		defer lease.Close()
		m.supervise(ctx)
	}()
	return nil
}

func (m *Manager) Status() (bool, string) {
	if lifecycle := m.delegatedLifecycle(); lifecycle != nil {
		return lifecycle.Status()
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running, m.lastError
}

func (m *Manager) RunningBinary() string {
	if lifecycle := m.delegatedLifecycle(); lifecycle != nil {
		if remote, ok := lifecycle.(*remoteLifecycle); ok {
			status, _ := remote.status()
			if status.Running {
				return status.Binary
			}
		}
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.running {
		return m.runningBinary
	}
	return ""
}

func (m *Manager) RunningMetrics() (string, *uint64) {
	if lifecycle := m.delegatedLifecycle(); lifecycle != nil {
		if remote, ok := lifecycle.(*remoteLifecycle); ok {
			status, _ := remote.statusWithMetrics()
			if status.Running {
				return status.Binary, status.MemoryBytes
			}
		}
		return "", nil
	}
	m.mu.RLock()
	binary, pid, running := m.runningBinary, m.runningPID, m.running
	m.mu.RUnlock()
	if !running {
		return "", nil
	}
	memory, ok := processMemory(pid)
	if !ok {
		return binary, nil
	}
	return binary, &memory
}

func (m *Manager) ValidateBinaryConfig(binary string) error {
	m.profileMu.Lock()
	defer m.profileMu.Unlock()
	profile, err := os.ReadFile(filepath.Join(m.dataDir, "profile.yaml"))
	if err != nil {
		return err
	}
	rendered, err := m.renderForBinary(profile, binary)
	if err != nil {
		return err
	}
	candidate, err := os.CreateTemp(filepath.Dir(m.configPath), "kernel-check-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(candidate.Name())
	_, writeErr := candidate.Write(rendered)
	if err := errors.Join(writeErr, candidate.Close()); err != nil {
		return err
	}
	return m.validateWith(binary, candidate.Name())
}

func (m *Manager) Restart() error {
	if lifecycle := m.delegatedLifecycle(); lifecycle != nil {
		return lifecycle.Restart()
	}
	m.mu.RLock()
	started := m.started
	m.mu.RUnlock()
	if !started {
		return fmt.Errorf("mihomo supervisor 尚未启动")
	}
	select {
	case m.restart <- struct{}{}:
	default:
	}
	return nil
}

func (m *Manager) Shutdown() error {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	if lifecycle := m.delegatedLifecycle(); lifecycle != nil {
		return lifecycle.Shutdown()
	}
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return nil
	}
	cancel := m.stop
	done := m.done
	m.started = false
	m.mu.Unlock()
	cancel()
	<-done
	return nil
}
