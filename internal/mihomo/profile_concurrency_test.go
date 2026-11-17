package mihomo

import (
	"os"
	"path/filepath"
	"reia-clash/internal/config"
	"strings"
	"sync"
	"testing"
)

func TestBackgroundProfileAndRuntimeUpdatesPreserveLatestSubscription(t *testing.T) {
	root := t.TempDir()
	store, err := config.OpenStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	old := []byte("proxies: []\nproxy-groups: []\nrules: [MATCH,DIRECT]\n")
	latest := []byte("proxies: []\nproxy-groups: []\nrules:\n  - DOMAIN,latest.example,DIRECT\n  - MATCH,DIRECT\n")
	if err := os.WriteFile(filepath.Join(root, "profile.yaml"), old, 0600); err != nil {
		t.Fatal(err)
	}
	m := &Manager{dataDir: root, configPath: filepath.Join(root, "mihomo.yaml"), settings: store, secret: "test-secret"}
	errors := make(chan error, 40)
	var group sync.WaitGroup
	for i := 0; i < 20; i++ {
		group.Add(2)
		go func() { defer group.Done(); errors <- m.PersistProfile(latest) }()
		go func() { defer group.Done(); errors <- m.PersistCurrentRuntimeConfig() }()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"profile.yaml", "mihomo.yaml"} {
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || !strings.Contains(string(content), "latest.example") {
			t.Fatalf("latest profile lost in %s: %v", name, err)
		}
	}
}
