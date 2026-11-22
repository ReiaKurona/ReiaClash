package mihomo

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reia-clash/internal/kernelbinary"
	"reia-clash/internal/platform/launch"
	"strings"
	"time"
)

func (m *Manager) SubscriptionSource() ([]byte, error) {
	m.profileMu.Lock()
	defer m.profileMu.Unlock()
	return readConfigSource(filepath.Join(m.dataDir, "profile.yaml"))
}

func (m *Manager) RunningConfigSource() (string, []byte, error) {
	m.profileMu.Lock()
	defer m.profileMu.Unlock()
	binary := m.RunningBinary()
	if binary == "" {
		return "", nil, fmt.Errorf("未检测到正在运行的内核")
	}
	path := m.runtimePath(binary)
	content, err := readConfigSource(path)
	return filepath.Base(path), content, err
}

func readConfigSource(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	const limit = 16 << 20
	source, err := io.ReadAll(io.LimitReader(file, limit+1))
	if len(source) > limit {
		return nil, fmt.Errorf("订阅内容超过大小限制")
	}
	return source, err
}

func (m *Manager) ImportProfile(ctx context.Context, profile []byte) error {
	binary := m.BinaryPath()
	if binary == "" {
		return fmt.Errorf("尚未安装 mihomo 内核，暂时不能校验并载入订阅")
	}
	m.profileMu.Lock()
	defer m.profileMu.Unlock()
	profilePath := filepath.Join(m.dataDir, "profile.yaml")
	oldProfile, err := os.ReadFile(profilePath)
	if err != nil {
		return err
	}
	runtimePath := m.runtimePath(binary)
	oldRuntime, err := m.renderForBinary(oldProfile, binary)
	if err != nil {
		return err
	}
	client := m.Client()
	probe, cancel := context.WithTimeout(ctx, 2*time.Second)
	_, status, probeErr := client.Do(probe, http.MethodGet, "/version", nil)
	cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if probeErr != nil && status != 0 {
		return probeErr
	}
	if err := m.persistProfile(profile, binary, probeErr != nil); err != nil {
		return err
	}
	if probeErr != nil {
		err = m.Restart()
	} else {
		var runtimeConfig []byte
		runtimeConfig, err = os.ReadFile(runtimePath)
		if err == nil {
			status, err = m.applyRuntimeConfig(ctx, client, binary, runtimeConfig)
			if err == nil {
				m.restoreSelections(ctx)
			}
		}
	}
	if err == nil {
		return nil
	}
	profileErr := atomicWrite(profilePath, oldProfile)
	runtimeErr := atomicWrite(runtimePath, oldRuntime)
	if probeErr == nil && status == 0 && runtimeErr == nil {
		// The controller may already have applied the new payload before the connection failed.
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 12*time.Second)
		defer cancel()
		_, runtimeErr = m.applyRuntimeConfig(recovery, client, binary, oldRuntime)
		if runtimeErr == nil {
			m.restoreSelections(recovery)
		}
	}
	return errors.Join(err, profileErr, runtimeErr)
}

func (m *Manager) renderForBinary(profile []byte, binary string) ([]byte, error) {
	settings := m.settings.Snapshot()
	settings.ActiveBinary = binary
	return BuildConfig(profile, settings, m.secret)
}

func (m *Manager) persistRuntimeConfig(profile []byte, binary string) error {
	rendered, err := m.renderForBinary(profile, binary)
	if err != nil {
		return err
	}
	return atomicWrite(m.runtimePath(binary), rendered)
}

// Read inside the transaction, so a mode/Geo update cannot restore an old
// profile that was read before a concurrent background subscription refresh.
func (m *Manager) PersistCurrentRuntimeConfig() error {
	return m.persistRuntimeForBinary("")
}

func (m *Manager) persistRuntimeForBinary(binary string) error {
	m.profileMu.Lock()
	defer m.profileMu.Unlock()
	if binary == "" {
		binary = m.settings.Snapshot().ActiveBinary
		if binary == "" {
			binary = m.BinaryPath()
		}
	}
	profile, err := os.ReadFile(filepath.Join(m.dataDir, "profile.yaml"))
	if err != nil {
		return err
	}
	return m.persistRuntimeConfig(profile, binary)
}

// PersistProfile 校验并保存配置，但不重启 mihomo。
// 出站模式等热设置会同时更新配置文件，保证下次启动仍使用当前模式。
func (m *Manager) PersistProfile(profile []byte) error {
	m.profileMu.Lock()
	defer m.profileMu.Unlock()
	return m.persistProfile(profile, m.BinaryPath(), true)
}

func (m *Manager) persistProfile(profile []byte, binary string, validate bool) error {
	profilePath := filepath.Join(m.dataDir, "profile.yaml")
	previousProfile, err := os.ReadFile(profilePath)
	if err != nil {
		return err
	}
	rendered, err := m.renderForBinary(profile, binary)
	if err != nil {
		return err
	}
	runtimePath := m.runtimePath(binary)
	temp := runtimePath + ".candidate"
	profileTemp := profilePath + ".candidate"
	if err := atomicWrite(temp, rendered); err != nil {
		return err
	}
	if validate && binary != "" {
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
	if err := replaceFile(temp, runtimePath); err != nil {
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
	cmd.Dir = m.kernelDataDir
	launch.HideCommand(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mihomo 配置校验失败: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func (m *Manager) ReloadRuntime(ctx context.Context) error {
	m.profileMu.Lock()
	defer m.profileMu.Unlock()
	binary := m.BinaryPath()
	profile, err := os.ReadFile(filepath.Join(m.dataDir, "profile.yaml"))
	if err != nil {
		return err
	}
	runtimeConfig, err := m.renderForBinary(profile, binary)
	if err != nil {
		return err
	}
	_, err = m.applyRuntimeConfig(ctx, m.Client(), binary, runtimeConfig)
	if err == nil {
		m.restoreSelections(ctx)
	}
	return err
}

// Keep the generated configuration outside the core data directory and
// use each controller's inline format. meow-rs expects Base64-encoded YAML.
func (m *Manager) applyRuntimeConfig(ctx context.Context, client *Client, binary string, content []byte) (int, error) {
	payload := string(content)
	if kernelbinary.IsMeow(binary) {
		payload = base64.StdEncoding.EncodeToString(content)
	}
	_, status, err := client.Do(ctx, http.MethodPut, "/configs?force=true", map[string]string{"payload": payload})
	return status, err
}
