package update

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type GeoResource struct{ ID, Name, Asset string }

var geoResources = []GeoResource{
	{"country", "Country.mmdb", "country.mmdb"},
	{"geoip", "GeoIP.dat", "geoip.dat"},
	{"metadb", "geoip.metadb", "geoip.metadb"},
	{"geosite", "GeoSite.dat", "geosite.dat"},
}

func GeoResources() []GeoResource { return append([]GeoResource(nil), geoResources...) }

func (m *Manager) SyncGeoResource(ctx context.Context, directory string, resource GeoResource) error {
	allowed := false
	for _, candidate := range geoResources {
		if candidate == resource {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("不支持的 Geo 资源")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	name := resource.Name
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), name) {
			if !entry.Type().IsRegular() {
				return fmt.Errorf("Geo 资源路径不是普通文件")
			}
			name = entry.Name()
			break
		}
	}
	response, err := m.get(ctx, "https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/"+resource.Asset)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (64<<20)+1))
	if err != nil {
		return err
	}
	valid := false
	switch resource.ID {
	case "country", "metadb":
		valid = validCountryDatabase(data)
	case "geoip", "geosite":
		valid = validGeoDAT(data)
	}
	if !valid {
		return fmt.Errorf("资源文件格式无效或超过大小限制")
	}
	file, err := os.CreateTemp(directory, ".reia-resource-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), filepath.Join(directory, name))
}

// GeoIPList and GeoSiteList both contain field 1 as repeated length-delimited entries.
func validGeoDAT(data []byte) bool {
	if len(data) < 1024 || len(data) > 64<<20 {
		return false
	}
	for len(data) > 0 {
		if data[0] != 0x0a {
			return false
		}
		length, width := binary.Uvarint(data[1:])
		if width <= 0 || length == 0 || length > uint64(len(data)-1-width) {
			return false
		}
		data = data[1+width+int(length):]
	}
	return true
}
