package mihomo

import (
	"reia-clash/internal/config"
	"strings"
	"testing"
)

func TestManagedGeoLoaderOverridesSubscription(t *testing.T) {
	profile := []byte("geodata-loader: standard\n'geodata-loader': memconservative\ngeodata-mode: false\nproxies: []\n")
	for _, enabled := range []bool{true, false} {
		settings := config.Defaults()
		settings.GeoLowMemory = enabled
		rendered, err := BuildConfig(profile, settings, "test")
		if err != nil {
			t.Fatal(err)
		}
		text := string(rendered)
		if strings.Count(text, "geodata-loader:") != 1 || !strings.Contains(text, "geodata-loader: "+config.GeodataLoader(enabled)) {
			t.Fatal(text)
		}
		if !strings.Contains(text, "geodata-mode: false") {
			t.Fatal("low memory setting changed GeoIP data format")
		}
	}
}
