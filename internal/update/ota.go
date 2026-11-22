package update

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type pendingRelease struct {
	Info Info `json:"release"`
	Meow bool `json:"meow"`
}

func (m *Manager) packPath() string { return filepath.Join(m.directory, "update.pack") }

func (m *Manager) Queue(ctx context.Context, source *Manager) (Info, error) {
	m.stageMu.Lock()
	defer m.stageMu.Unlock()
	latest, raw, err := source.download(ctx)
	if err != nil {
		return Info{}, err
	}
	defer os.Remove(raw)
	if _, _, err := source.extract(latest, raw); err != nil {
		return Info{}, err
	}
	out, err := os.CreateTemp(m.directory, ".stage-*.tmp")
	if err != nil {
		return Info{}, err
	}
	defer out.Close()
	defer os.Remove(out.Name())
	archive := zip.NewWriter(out)
	metadata, err := archive.Create("release.json")
	if err == nil {
		err = json.NewEncoder(metadata).Encode(pendingRelease{Info: latest.info, Meow: source.meow})
	}
	if err != nil {
		return Info{}, err
	}
	input, err := os.Open(raw)
	if err != nil {
		return Info{}, err
	}
	defer input.Close()
	payload, err := archive.CreateHeader(&zip.FileHeader{Name: "payload", Method: zip.Store})
	if err == nil {
		_, err = io.Copy(payload, input)
	}
	if err != nil {
		return Info{}, err
	}
	if err := archive.Close(); err != nil {
		return Info{}, err
	}
	if err := out.Sync(); err != nil {
		return Info{}, err
	}
	if err := out.Close(); err != nil {
		return Info{}, err
	}
	return latest.info, os.Rename(out.Name(), m.packPath())
}

func (m *Manager) Pending() (Info, bool, error) {
	archive, err := zip.OpenReader(m.packPath())
	if err != nil {
		return Info{}, false, err
	}
	defer archive.Close()
	var release pendingRelease
	file, err := archive.Open("release.json")
	if err != nil {
		return Info{}, false, err
	}
	defer file.Close()
	err = json.NewDecoder(io.LimitReader(file, 16<<10)).Decode(&release)
	return release.Info, release.Meow, err
}

func (m *Manager) UnpackPending() (Info, string, error) {
	m.stageMu.Lock()
	defer m.stageMu.Unlock()
	info, meow, err := m.Pending()
	if err != nil {
		return Info{}, "", err
	}
	archive, err := zip.OpenReader(m.packPath())
	if err != nil {
		return Info{}, "", err
	}
	defer archive.Close()
	payload, err := archive.Open("payload")
	if err != nil {
		return Info{}, "", err
	}
	defer payload.Close()
	file, err := os.CreateTemp(m.directory, ".mihomo-download-*.tmp")
	if err != nil {
		return Info{}, "", err
	}
	defer os.Remove(file.Name())
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(payload, 200<<20+1))
	closeErr := file.Close()
	if copyErr != nil {
		return Info{}, "", copyErr
	}
	if closeErr != nil {
		return Info{}, "", closeErr
	}
	if size > 200<<20 || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), info.SHA256) {
		return Info{}, "", fmt.Errorf("update.pack SHA-256 校验失败")
	}
	return m.ForCore(meow, false).extract(selectedRelease{info: info, asset: asset{Name: info.AssetName}}, file.Name())
}

func (m *Manager) DiscardPending() error {
	err := os.Remove(m.packPath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
