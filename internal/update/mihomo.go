package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reia-clash/internal/kernelbinary"
	"runtime"
	"strings"
	"sync"
	"time"
)

const releaseAPI = "https://api.github.com/repos/MetaCubeX/mihomo/releases/latest"

type Manager struct {
	stageMu   sync.Mutex
	directory string
	client    *http.Client
}

type Info struct {
	Success     bool   `json:"success"`
	Tag         string `json:"tag"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	DownloadURL string `json:"download_url"`
	AssetName   string `json:"asset_name"`
	SHA256      string `json:"sha256"`
	PublishedAt string `json:"published_at,omitempty"`
}

type release struct {
	TagName     string  `json:"tag_name"`
	Draft       bool    `json:"draft"`
	Prerelease  bool    `json:"prerelease"`
	PublishedAt string  `json:"published_at"`
	Assets      []asset `json:"assets"`
}

type asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
}

func NewManager(directory string) *Manager {
	return &Manager{
		directory: directory,
		client: &http.Client{
			Timeout: 45 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) > 5 || req.URL.Scheme != "https" || !trustedHost(req.URL.Hostname()) {
					return fmt.Errorf("mihomo 发布文件重定向到了不受信任的地址")
				}
				return nil
			},
		},
	}
}

func (m *Manager) Check(ctx context.Context) (Info, error) {
	latest, err := m.latest(ctx)
	if err != nil {
		return Info{}, err
	}
	return latest.info, nil
}

func (m *Manager) Stage(ctx context.Context) (Info, string, error) {
	m.stageMu.Lock()
	defer m.stageMu.Unlock()
	latest, err := m.latest(ctx)
	if err != nil {
		return Info{}, "", err
	}
	if latest.asset.Digest == "" || !strings.HasPrefix(latest.asset.Digest, "sha256:") {
		return Info{}, "", fmt.Errorf("GitHub 发布信息没有提供 SHA-256，拒绝安装未经校验的内核")
	}
	if err := os.MkdirAll(m.directory, 0700); err != nil {
		return Info{}, "", err
	}
	response, err := m.get(ctx, latest.asset.URL)
	if err != nil {
		return Info{}, "", err
	}
	defer response.Body.Close()
	archivePath := filepath.Join(m.directory, ".mihomo-download.tmp")
	archive, err := os.OpenFile(archivePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return Info{}, "", err
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(archive, hasher), io.LimitReader(response.Body, 200<<20+1))
	closeErr := archive.Close()
	if copyErr != nil {
		_ = os.Remove(archivePath)
		return Info{}, "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(archivePath)
		return Info{}, "", closeErr
	}
	if written > 200<<20 {
		_ = os.Remove(archivePath)
		return Info{}, "", fmt.Errorf("mihomo 发布包超过 200 MiB 限制")
	}
	expected := strings.TrimPrefix(latest.asset.Digest, "sha256:")
	if len(expected) != sha256.Size*2 || !strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), expected) {
		_ = os.Remove(archivePath)
		return Info{}, "", fmt.Errorf("mihomo 发布包 SHA-256 校验失败")
	}
	binaryName := "mihomo"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	versionPart := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_").Replace(latest.info.Tag)
	destination := filepath.Join(m.directory, "mihomo-"+versionPart+"-"+strings.ToLower(expected[:12])+binarySuffix())
	temporary := destination + ".tmp" + binarySuffix()
	if err := extractBinary(archivePath, latest.asset.Name, binaryName, temporary); err != nil {
		_ = os.Remove(archivePath)
		_ = os.Remove(temporary)
		return Info{}, "", err
	}
	_ = os.Remove(archivePath)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(temporary, 0700); err != nil {
			_ = os.Remove(temporary)
			return Info{}, "", err
		}
	}
	if err := kernelbinary.Validate(temporary); err != nil {
		_ = os.Remove(temporary)
		return Info{}, "", fmt.Errorf("更新包内核运行验证失败: %w", err)
	}
	if existing, err := os.Open(destination); err == nil {
		existingHash := sha256.New()
		_, copyErr := io.Copy(existingHash, existing)
		closeErr := existing.Close()
		newFile, openErr := os.Open(temporary)
		if copyErr == nil && closeErr == nil && openErr == nil {
			newHash := sha256.New()
			_, newCopyErr := io.Copy(newHash, newFile)
			newCloseErr := newFile.Close()
			if newCopyErr == nil && newCloseErr == nil && strings.EqualFold(hex.EncodeToString(existingHash.Sum(nil)), hex.EncodeToString(newHash.Sum(nil))) {
				_ = os.Remove(temporary)
				return latest.info, destination, nil
			}
		} else if openErr == nil {
			_ = newFile.Close()
		}
		if err := os.Remove(destination); err != nil {
			_ = os.Remove(temporary)
			return Info{}, "", fmt.Errorf("移除旧版 mihomo 文件失败: %w", err)
		}
	}
	if err := os.Rename(temporary, destination); err != nil {
		_ = os.Remove(temporary)
		return Info{}, "", err
	}
	return latest.info, destination, nil
}

type selectedRelease struct {
	info  Info
	asset asset
}

func (m *Manager) latest(ctx context.Context) (selectedRelease, error) {
	response, err := m.get(ctx, releaseAPI)
	if err != nil {
		return selectedRelease{}, fmt.Errorf("查询 mihomo 官方发布失败: %w", err)
	}
	defer response.Body.Close()
	var latest release
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&latest); err != nil {
		return selectedRelease{}, err
	}
	if latest.Draft || latest.Prerelease || latest.TagName == "" {
		return selectedRelease{}, fmt.Errorf("mihomo 官方 latest 发布信息不完整")
	}
	selected, err := chooseAsset(latest.Assets)
	if err != nil {
		return selectedRelease{}, err
	}
	return selectedRelease{
		info: Info{
			Success: true, Tag: latest.TagName, OS: runtime.GOOS, Arch: runtime.GOARCH,
			DownloadURL: selected.URL, AssetName: selected.Name,
			SHA256: strings.TrimPrefix(selected.Digest, "sha256:"), PublishedAt: latest.PublishedAt,
		},
		asset: selected,
	}, nil
}

func (m *Manager) get(ctx context.Context, rawURL string) (*http.Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || !trustedHost(u.Hostname()) {
		return nil, fmt.Errorf("拒绝访问非 mihomo 官方 HTTPS 地址")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ReiaClash")
	req.Header.Set("Accept", "application/vnd.github+json")
	response, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		return nil, fmt.Errorf("GitHub 返回 HTTP %d", response.StatusCode)
	}
	return response, nil
}

func chooseAsset(assets []asset) (asset, error) {
	osName := runtime.GOOS
	arch := runtime.GOARCH
	archNames := []string{arch}
	if arch == "arm64" {
		archNames = []string{"arm64-v8", "arm64"}
	} else if arch == "arm" {
		archNames = []string{"armv7", "arm"}
	} else if arch == "386" {
		archNames = []string{"386"}
	}
	for _, candidateArch := range archNames {
		prefix := "mihomo-" + osName + "-" + candidateArch + "-"
		var fallback *asset
		for i := range assets {
			item := &assets[i]
			name := strings.ToLower(item.Name)
			if !strings.HasPrefix(name, prefix) || !(strings.HasSuffix(name, ".zip") || strings.HasSuffix(name, ".gz") || strings.HasSuffix(name, ".tgz") || strings.HasSuffix(name, ".tar")) {
				continue
			}
			if strings.Contains(name, "-go1") || strings.Contains(name, "-v2-") || strings.Contains(name, "-v3-") {
				continue
			}
			if fallback == nil {
				fallback = item
			}
			if strings.Contains(name, "-compatible-") {
				return *item, nil
			}
		}
		if fallback != nil {
			return *fallback, nil
		}
	}
	return asset{}, fmt.Errorf("mihomo 暂无适配 %s/%s 的官方发布包", runtime.GOOS, runtime.GOARCH)
}
