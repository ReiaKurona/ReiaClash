package update

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const countryDatabaseURL = "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/country.mmdb"

func (m *Manager) PrepareCountryDatabase(ctx context.Context, directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	name := "Country.mmdb"
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), "Country.mmdb") && entry.Type().IsRegular() {
			name = entry.Name()
			content, err := os.ReadFile(filepath.Join(directory, name))
			if err != nil {
				return err
			}
			if validCountryDatabase(content) {
				return nil
			}
		}
	}
	response, err := m.get(ctx, countryDatabaseURL)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, (64<<20)+1))
	if err != nil {
		return err
	}
	if !validCountryDatabase(content) {
		return fmt.Errorf("GeoIP 文件不是有效的 MMDB 或超过 64 MiB")
	}
	file, err := os.CreateTemp(directory, ".country-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(content)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), filepath.Join(directory, name))
}

func validCountryDatabase(content []byte) bool {
	if len(content) < 1024 || len(content) > 64<<20 {
		return false
	}
	return bytes.Contains(content[max(0, len(content)-(128<<10)):], []byte("\xab\xcd\xefMaxMind.com"))
}
