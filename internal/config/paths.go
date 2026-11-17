package config

import (
	"os"
	"path/filepath"
)

// Paths 把运行文件放到用户配置目录，避免将配置或密钥写回安装目录。
type Paths struct {
	Root       string
	ConfigFile string
	Profile    string
	BinaryDir  string
	UpdateDir  string
}

// MihomoDataDir is stable across kernel upgrades and shared by agent and service.
// Keep GeoIP/GeoSite assets in data beside the application binaries.
func MihomoDataDir() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(executable), "data"), nil
}

func UserPaths() (Paths, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, err
	}
	return PathsAt(filepath.Join(base, "ReiaClash"))
}

// PathsAt 为独立的内核服务打开用户 agent 已有的数据目录，
// 确保设置、订阅、mihomo secret 和更新文件共用同一位置。
func PathsAt(root string) (Paths, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Paths{}, err
	}
	paths := Paths{
		Root:       root,
		ConfigFile: filepath.Join(root, "settings.json"),
		Profile:    filepath.Join(root, "profile.yaml"),
		BinaryDir:  filepath.Join(root, "bin"),
		UpdateDir:  filepath.Join(root, "updates"),
	}
	for _, dir := range []string{root, paths.BinaryDir, paths.UpdateDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return Paths{}, err
		}
	}
	return paths, nil
}
