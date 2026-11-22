package mihomo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	serviceControlAddress = "127.0.0.1:25501"
	serviceTokenFile      = "core-service.token"
	serviceConfigFile     = "core-service.json"
)

type serviceDescriptor struct {
	Endpoint string `json:"endpoint"`
}

// EnsureServiceCredentials 准备内核服务和用户 agent 共享的本机控制密钥。
// 服务端口固定为 loopback，密钥不返回给 WebUI。
func EnsureServiceCredentials(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return err
	}
	tokenPath := filepath.Join(dataDir, serviceTokenFile)
	if info, err := os.Lstat(tokenPath); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Mihomo 服务认证密钥必须是普通文件")
		}
		if err := os.Chmod(tokenPath, 0600); err != nil {
			return fmt.Errorf("无法保护 Mihomo 服务认证密钥：%w", err)
		}
		if _, err := readServiceToken(tokenPath); err != nil {
			return err
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return err
	}
	file, err := os.OpenFile(tokenPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if os.IsExist(err) {
			return EnsureServiceCredentials(dataDir)
		}
		return err
	}
	if _, err := file.WriteString(hex.EncodeToString(secret)); err != nil {
		_ = file.Close()
		_ = os.Remove(tokenPath)
		return err
	}
	return file.Close()
}

// EnableServiceLifecycle 记录操作系统服务安装器已注册 helper。
// 它只写入 agent 使用的标记，不负责安装操作系统服务。
func EnableServiceLifecycle(dataDir string) error {
	if err := EnsureServiceCredentials(dataDir); err != nil {
		return err
	}
	data, err := json.Marshal(serviceDescriptor{Endpoint: "http://" + serviceControlAddress})
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dataDir, serviceConfigFile), data)
}

// LoadServiceToken 读取 agent 和操作系统 helper 共用的控制密钥。
func LoadServiceToken(dataDir string) (string, error) {
	return readServiceToken(filepath.Join(dataDir, serviceTokenFile))
}

// DisableServiceLifecycle 移除服务标记，让 agent 回到内置 supervisor。
func DisableServiceLifecycle(dataDir string) error {
	if err := os.Remove(filepath.Join(dataDir, serviceConfigFile)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// LoadServiceLifecycle 在没有启用操作系统内核服务时返回 nil。
func LoadServiceLifecycle(dataDir string) (ProcessLifecycle, bool, error) {
	configPath := filepath.Join(dataDir, serviceConfigFile)
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var descriptor serviceDescriptor
	if err := json.Unmarshal(data, &descriptor); err != nil {
		return nil, false, fmt.Errorf("读取 Mihomo 服务配置失败：%w", err)
	}
	if descriptor.Endpoint != "http://"+serviceControlAddress {
		return nil, false, fmt.Errorf("Mihomo 服务地址必须固定为：%s", serviceControlAddress)
	}
	token, err := LoadServiceToken(dataDir)
	if err != nil {
		return nil, false, err
	}
	return &remoteLifecycle{
		endpoint: descriptor.Endpoint,
		token:    token,
		client:   &http.Client{Timeout: 5 * time.Second},
	}, true, nil
}

func readServiceToken(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("读取 Mihomo 服务密钥失败: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("Mihomo 服务密钥必须是普通文件")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return "", fmt.Errorf("Mihomo 服务认证密钥必须限制为当前用户读取")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(data))
	if len(token) != 64 {
		return "", fmt.Errorf("Mihomo 服务密钥格式无效")
	}
	if _, err := hex.DecodeString(token); err != nil {
		return "", fmt.Errorf("Mihomo 服务密钥格式无效")
	}
	return token, nil
}

type remoteLifecycle struct {
	endpoint string
	token    string
	client   *http.Client
}

type serviceStatus struct {
	Binary      string  `json:"binary,omitempty"`
	MemoryBytes *uint64 `json:"memory_bytes,omitempty"`
	Started     bool    `json:"started"`
	Running     bool    `json:"running"`
	LastError   string  `json:"last_error,omitempty"`
}

func (l *remoteLifecycle) Start(ctx context.Context) error {
	readyCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	lastError := ""
	for {
		request, err := http.NewRequestWithContext(readyCtx, http.MethodGet, l.endpoint+"/status", nil)
		if err == nil {
			response, callErr := l.do(request)
			if callErr == nil {
				if response.StatusCode != http.StatusOK {
					response.Body.Close()
					return fmt.Errorf("Mihomo 服务控制认证失败或端口被其他程序占用: HTTP %d", response.StatusCode)
				}
				var status serviceStatus
				decodeErr := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&status)
				response.Body.Close()
				if decodeErr == nil && status.Running {
					return nil
				}
				if decodeErr == nil && !status.Started {
					if startErr := l.post(readyCtx, "/start"); startErr != nil {
						return startErr
					}
				}
				if status.LastError != "" {
					lastError = status.LastError
				}
			} else {
				lastError = callErr.Error()
			}
		}
		select {
		case <-readyCtx.Done():
			if lastError != "" {
				return fmt.Errorf("Mihomo 特权服务尚未就绪：%s", lastError)
			}
			return fmt.Errorf("等待 Mihomo 特权服务启动失败：%w", readyCtx.Err())
		case <-ticker.C:
		}
	}
}

func (l *remoteLifecycle) Status() (bool, string) {
	status, err := l.status()
	if err != nil {
		return false, err.Error()
	}
	return status.Running, status.LastError
}

func (l *remoteLifecycle) status() (serviceStatus, error) {
	return l.getStatus("/status")
}

func (l *remoteLifecycle) statusWithMetrics() (serviceStatus, error) {
	return l.getStatus("/status?metrics=1")
}

func (l *remoteLifecycle) getStatus(path string) (serviceStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, l.endpoint+path, nil)
	if err != nil {
		return serviceStatus{}, err
	}
	response, err := l.do(request)
	if err != nil {
		return serviceStatus{}, err
	}
	defer response.Body.Close()
	var status serviceStatus
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&status); err != nil {
		return serviceStatus{}, err
	}
	return status, nil
}

// Probe checks the authenticated helper itself, even if its kernel is stopped.
func (l *remoteLifecycle) Probe(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, l.endpoint+"/status", nil)
	if err != nil {
		return err
	}
	response, err := l.do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("内核服务返回 HTTP %d", response.StatusCode)
	}
	var status serviceStatus
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&status)
}

func (l *remoteLifecycle) Restart() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return l.post(ctx, "/restart")
}

func (l *remoteLifecycle) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 22*time.Second)
	defer cancel()
	return l.post(ctx, "/stop")
}

func (l *remoteLifecycle) post(ctx context.Context, path string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, l.endpoint+path, nil)
	if err != nil {
		return err
	}
	response, err := l.do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Mihomo 特权服务返回 HTTP %d", response.StatusCode)
	}
	return nil
}

func (l *remoteLifecycle) do(request *http.Request) (*http.Response, error) {
	request.Header.Set("Authorization", "Bearer "+l.token)
	response, err := l.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("无法连接 Mihomo 特权服务：%w", err)
	}
	return response, nil
}
