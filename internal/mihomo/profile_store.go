package mihomo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reia-clash/internal/platform/launch"
	"strings"
	"time"
)

func (m *Manager) ImportProfile(profile []byte) error {
	if m.BinaryPath() == "" {
		return fmt.Errorf("尚未安装 mihomo 内核，暂时不能校验并载入订阅")
	}
	if err := m.PersistProfile(profile); err != nil {
		return err
	}
	return m.Restart()
}

// PersistRuntimeConfig only rewrites agent-owned settings. Existing profiles
// were validated on import; a mode/LAN change must not download Geo assets.
func (m *Manager) PersistRuntimeConfig(profile []byte) error {
	m.profileMu.Lock()
	defer m.profileMu.Unlock()
	return m.persistRuntimeConfig(profile)
}

func (m *Manager) persistRuntimeConfig(profile []byte) error {
	rendered, err := BuildConfig(profile, m.settings.Snapshot(), m.secret)
	if err != nil {
		return err
	}
	return atomicWrite(m.configPath, rendered)
}

// Read inside the transaction, so a mode/Geo update cannot restore an old
// profile that was read before a concurrent background subscription refresh.
func (m *Manager) PersistCurrentRuntimeConfig() error {
	m.profileMu.Lock()
	defer m.profileMu.Unlock()
	profile, err := os.ReadFile(filepath.Join(m.dataDir, "profile.yaml"))
	if err != nil {
		return err
	}
	return m.persistRuntimeConfig(profile)
}

func (m *Manager) ReloadProfile() error {
	if m.BinaryPath() == "" {
		return fmt.Errorf("尚未安装 mihomo 内核，暂时不能校验并载入订阅")
	}
	m.profileMu.Lock()
	profile, err := os.ReadFile(filepath.Join(m.dataDir, "profile.yaml"))
	if err == nil {
		err = m.persistProfile(profile)
	}
	m.profileMu.Unlock()
	if err != nil {
		return err
	}
	return m.Restart()
}

// PersistProfile 校验并保存配置，但不重启 mihomo。
// 出站模式等热设置会同时更新配置文件，保证下次启动仍使用当前模式。
func (m *Manager) PersistProfile(profile []byte) error {
	m.profileMu.Lock()
	defer m.profileMu.Unlock()
	return m.persistProfile(profile)
}

func (m *Manager) persistProfile(profile []byte) error {
	profilePath := filepath.Join(m.dataDir, "profile.yaml")
	previousProfile, err := os.ReadFile(profilePath)
	if err != nil {
		return err
	}
	settings := m.settings.Snapshot()
	rendered, err := BuildConfig(profile, settings, m.secret)
	if err != nil {
		return err
	}
	temp := m.configPath + ".candidate"
	profileTemp := profilePath + ".candidate"
	if err := atomicWrite(temp, rendered); err != nil {
		return err
	}
	if binary := m.BinaryPath(); binary != "" {
		if err := m.validateWith(binary, temp); err != nil {
			_ = os.Remove(temp)
			return err
		}
	}
	if err := os.WriteFile(profileTemp, profile, 0600); err != nil {
		_ = os.Remove(temp)
		return err
	}
	if err := replaceFile(profileTemp, profilePath); err != nil {
		_ = os.Remove(temp)
		_ = os.Remove(profileTemp)
		return err
	}
	if err := replaceFile(temp, m.configPath); err != nil {
		_ = os.Remove(temp)
		_ = os.Remove(profileTemp)
		if rollbackErr := atomicWrite(profilePath, previousProfile); rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("恢复旧订阅文件失败: %w", rollbackErr))
		}
		return err
	}
	_ = os.Remove(profileTemp)
	return nil
}

// WaitReady 等待带密钥的控制器能够响应，或等到调用方上下文超时。
// 接管系统流量前先确认 mihomo 已就绪，避免代理指向尚未启动的端口。
func (m *Manager) WaitReady(ctx context.Context) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		var version map[string]any
		if err := m.Client().GetJSON(ctx, "/version", &version); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (m *Manager) validateWith(binary, path string) error {
	if err := os.MkdirAll(m.kernelDataDir, 0700); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-t", "-d", m.kernelDataDir, "-f", path)
	launch.HideCommand(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mihomo 配置校验失败: %s", strings.TrimSpace(string(output)))
	}
	return nil
}
