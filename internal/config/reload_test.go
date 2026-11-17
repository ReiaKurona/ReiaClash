package config

import (
	"path/filepath"
	"testing"
)

func TestServiceReloadPreservesAuthorityAndReadsAgentSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	agent, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	service, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	service.SetRuntimeTunAuthorization(true)
	_, err = agent.Update(func(s *Settings) error {
		s.UseTun = true
		s.Enabled = true
		s.AllowLAN = true
		s.ActiveBinary = "new-kernel"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Reload(); err != nil {
		t.Fatal(err)
	}
	s := service.Snapshot()
	if !s.TunAuthorized || !s.UseTun || !s.Enabled || !s.AllowLAN || s.ActiveBinary != "new-kernel" {
		t.Fatalf("stale service settings: %+v", s)
	}
}
