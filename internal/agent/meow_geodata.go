package agent

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
	"reia-clash/internal/config"
)

func (a *App) prepareMeowGeodata(ctx context.Context) error {
	data, err := os.ReadFile(a.Paths.Profile)
	if err != nil {
		return err
	}
	if !needsCountryDatabase(data) {
		return nil
	}
	directory, err := config.MihomoDataDir()
	if err != nil {
		return err
	}
	if err := a.Updater.ForCore(true, true).PrepareCountryDatabase(ctx, directory); err != nil {
		return fmt.Errorf("无法通过当前 mihomo 代理准备 meow-rs 所需的 GeoIP 数据：%w", err)
	}
	return nil
}

func needsCountryDatabase(data []byte) bool {
	var profile struct {
		Rules []string `yaml:"rules"`
		DNS   struct {
			Enable         bool     `yaml:"enable"`
			Fallback       []string `yaml:"fallback"`
			FallbackFilter struct {
				GeoIP *bool `yaml:"geoip"`
			} `yaml:"fallback-filter"`
		} `yaml:"dns"`
	}
	if yaml.Unmarshal(data, &profile) != nil {
		return false
	}
	for _, rule := range profile.Rules {
		upper := strings.ToUpper(rule)
		if strings.Contains(upper, "GEOIP,") {
			return true
		}
	}
	return profile.DNS.Enable && len(profile.DNS.Fallback) > 0 &&
		(profile.DNS.FallbackFilter.GeoIP == nil || *profile.DNS.FallbackFilter.GeoIP)
}

// meow-rs v0.29 skips TUIC entries. Keep the original YAML unchanged.
func (a *App) checkMeowSubscription() error {
	data, err := os.ReadFile(a.Paths.Profile)
	if err != nil {
		return err
	}
	var profile struct {
		Proxies []struct {
			Type string `yaml:"type"`
		} `yaml:"proxies"`
		Providers map[string]yaml.Node `yaml:"proxy-providers"`
	}
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return fmt.Errorf("读取订阅节点类型失败：%w", err)
	}
	tuic, other := 0, 0
	for _, proxy := range profile.Proxies {
		switch strings.ToLower(proxy.Type) {
		case "tuic":
			tuic++
		case "", "direct", "reject", "reject-drop", "pass":
		default:
			other++
		}
	}
	if tuic == 0 {
		return nil
	}
	if other == 0 && len(profile.Providers) == 0 {
		return fmt.Errorf("当前订阅的代理节点都是 TUIC，meow-rs v0.29.0 无法使用；请继续使用 mihomo")
	}
	a.AddEvent("info", fmt.Sprintf("meow-rs v0.29.0 会跳过 %d 个 TUIC 节点", tuic))
	return nil
}
