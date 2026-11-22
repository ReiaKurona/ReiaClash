package update

import (
	"archive/zip"
	"bytes"
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
	meow      bool
	directory string
	client    *http.Client
}

type Info struct {
	Pending     bool   `json:"pending,omitempty"`
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
			Timeout: 4 * time.Minute,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) > 5 || req.URL.Scheme != "https" || !trustedHost(req.URL.Hostname()) {
					return fmt.Errorf("mihomo 发布文件重定向到了不受信任的地址")
				}
				return nil
			},
		},
	}
}

// ForCore selects the release source and optionally uses the running mixed proxy.
func (m *Manager) ForCore(meow bool, throughProxy bool) *Manager {
	variant := NewManager(m.directory)
	variant.meow = meow
	if throughProxy {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = http.ProxyURL(&url.URL{Scheme: "http", Host: "127.0.0.1:7890"})
		variant.client.Transport = transport
	}
	return variant
}

func (m *Manager) Check(ctx context.Context) (Info, error) {
	latest, err := m.latest(ctx)
	if err != nil {
		return Info{}, err
	}
	return latest.info, nil
}

func (m *Manager) Stage(ctx context.Context) (Info, string, error) {
	return m.StageCore(ctx, m)
}

func (m *Manager) StageCore(ctx context.Context, source *Manager) (Info, string, error) {
	m.stageMu.Lock()
	defer m.stageMu.Unlock()
	return source.stage(ctx)
}

func (m *Manager) stage(ctx context.Context) (Info, string, error) {
	latest, archive, err := m.download(ctx)
	if err != nil {
		return Info{}, "", err
	}
	defer os.Remove(archive)
	return m.extract(latest, archive)
}

func (m *Manager) download(ctx context.Context) (selectedRelease, string, error) {
	latest, err := m.latest(ctx)
	if err != nil {
		return selectedRelease{}, "", err
	}
	if latest.asset.Digest == "" || !strings.HasPrefix(latest.asset.Digest, "sha256:") {
		return selectedRelease{}, "", fmt.Errorf("GitHub 发布信息没有提供 SHA-256，拒绝安装未经校验的内核")
	}
	if err := os.MkdirAll(m.directory, 0700); err != nil {
		return selectedRelease{}, "", err
	}
	response, err := m.get(ctx, latest.asset.URL)
	if err != nil {
		return selectedRelease{}, "", err
	}
	defer response.Body.Close()
	archive, err := os.CreateTemp(m.directory, ".mihomo-download-*.tmp")
	if err != nil {
		return selectedRelease{}, "", err
	}
	archivePath := archive.Name()
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(archive, hasher), io.LimitReader(response.Body, 200<<20+1))
	closeErr := archive.Close()
	if copyErr != nil {
		_ = os.Remove(archivePath)
		return selectedRelease{}, "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(archivePath)
		return selectedRelease{}, "", closeErr
	}
	if written > 200<<20 {
		_ = os.Remove(archivePath)
		return selectedRelease{}, "", fmt.Errorf("mihomo 发布包超过 200 MiB 限制")
	}
	expected := strings.TrimPrefix(latest.asset.Digest, "sha256:")
	if len(expected) != sha256.Size*2 || !strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), expected) {
		_ = os.Remove(archivePath)
		return selectedRelease{}, "", fmt.Errorf("mihomo 发布包 SHA-256 校验失败")
	}
	return latest, archivePath, nil
}

func (m *Manager) extract(latest selectedRelease, archivePath string) (Info, string, error) {
	expected := latest.info.SHA256

	family := "mihomo"
	if m.meow {
		family = "meow"
	}
	binaryName := family
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	versionPart := SafeVersion(latest.info.Tag)
	destination := filepath.Join(m.directory, family+"-"+versionPart+"-"+strings.ToLower(expected[:12])+binarySuffix())
	if m.meow && runtime.GOOS == "windows" {
		path, err := stageMeowWindows(archivePath, latest.asset.Name, strings.TrimSuffix(destination, ".exe"))
		if err != nil {
			return Info{}, "", err
		}
		return latest.info, path, nil
	}
	temporary := destination + ".tmp" + binarySuffix()
	if err := extractBinary(archivePath, latest.asset.Name, binaryName, temporary); err != nil {
		_ = os.Remove(temporary)
		return Info{}, "", err
	}
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
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	endpoint := releaseAPI
	if m.meow {
		endpoint = "https://api.github.com/repos/meow-rs/meow-rs/releases/latest"
	}
	response, err := m.get(ctx, endpoint)
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
	if m.meow {
		selected, err = chooseMeowAsset(latest.TagName, latest.Assets)
	}
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

func chooseMeowAsset(tag string, assets []asset) (asset, error) {
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64", "386": "i686", "arm": "armv7", "riscv64": "riscv64gc"}[runtime.GOARCH]
	var target string
	switch runtime.GOOS {
	case "windows":
		target = "-pc-windows-msvc.zip"
	case "darwin":
		target = "-apple-darwin.tar.gz"
	case "linux":
		target = "-unknown-linux-musl.tar.gz"
		if arch == "armv7" {
			target = "-unknown-linux-musleabihf.tar.gz"
		}
	}
	for _, item := range assets {
		if arch != "" && target != "" && item.Name == "meow-"+tag+"-"+arch+target {
			return item, nil
		}
	}
	return asset{}, fmt.Errorf("meow-rs has no release asset for %s/%s", runtime.GOOS, runtime.GOARCH)
}

func stageMeowWindows(archivePath, assetName, directory string) (string, error) {
	temporary, err := os.MkdirTemp(filepath.Dir(directory), ".meow-stage-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temporary)
	candidate := filepath.Join(temporary, "meow.exe")
	if err := extractBinary(archivePath, assetName, "meow.exe", candidate); err != nil {
		return "", err
	}
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer archive.Close()
	var library *zip.File
	for _, entry := range archive.File {
		if strings.EqualFold(filepath.Base(entry.Name), "wintun.dll") && entry.Mode().IsRegular() {
			if library != nil {
				return "", fmt.Errorf("meow-rs 发行包包含多份 wintun.dll")
			}
			library = entry
		}
	}
	if library == nil || library.UncompressedSize64 > 16<<20 {
		return "", fmt.Errorf("meow-rs 发行包缺少有效的 wintun.dll")
	}
	reader, err := library.Open()
	if err != nil {
		return "", err
	}
	payload, err := io.ReadAll(io.LimitReader(reader, 16<<20+1))
	reader.Close()
	if err != nil {
		return "", err
	}
	if len(payload) > 16<<20 || !bytes.HasPrefix(payload, []byte("MZ")) {
		return "", fmt.Errorf("meow-rs 发行包中的 wintun.dll 无效")
	}
	if err := os.WriteFile(filepath.Join(temporary, "wintun.dll"), payload, 0600); err != nil {
		return "", err
	}
	if err := kernelbinary.Validate(candidate); err != nil {
		return "", err
	}
	destination := filepath.Join(directory, "meow.exe")
	if err := os.Rename(temporary, directory); err != nil {
		// An already installed copy is reusable only if both files still match.
		for _, name := range []string{"meow.exe", "wintun.dll"} {
			fresh, freshErr := os.ReadFile(filepath.Join(temporary, name))
			existing, existingErr := os.ReadFile(filepath.Join(directory, name))
			if freshErr != nil || existingErr != nil || !bytes.Equal(fresh, existing) {
				return "", err
			}
		}
	}
	return destination, nil
}
