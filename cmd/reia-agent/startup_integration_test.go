package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reia-clash/internal/config"
	"reia-clash/internal/mihomo"
	"reia-clash/internal/platform/coreservice"
	"runtime"
	"testing"
	"time"
)

// Reproduce a stale service descriptor plus a busy controller port, using
// isolated preferences and no system proxy/TUN changes or other process kills.
func TestStartupAPIWithStaleServiceAndBusyKernelPort(t *testing.T) {
	binary := os.Getenv("REIA_TEST_AGENT")
	if binary == "" {
		t.Skip("set REIA_TEST_AGENT to a freshly built agent")
	}
	if coreservice.RegistrationStatus().Installed {
		t.Skip("a real OS service is registered")
	}
	for _, address := range []string{"127.0.0.1:25500", "127.0.0.1:25501"} {
		listener, err := net.Listen("tcp4", address)
		if err != nil {
			t.Skip("live agent/helper owns " + address)
		}
		listener.Close()
	}
	// Keep the kernel from starting even when no other client owns the port.
	occupied, err := net.Listen("tcp4", "127.0.0.1:9090")
	if err == nil {
		defer occupied.Close()
	}
	base := t.TempDir()
	configBase := base
	if runtime.GOOS == "darwin" {
		configBase = filepath.Join(base, "Library", "Application Support")
	}
	paths, err := config.PathsAt(filepath.Join(configBase, "ReiaClash"))
	if err != nil {
		t.Fatal(err)
	}
	prefs, err := config.OpenStore(paths.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prefs.Update(func(s *config.Settings) error { *s = config.Defaults(); s.Enabled = false; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := mihomo.EnableServiceLifecycle(paths.Root); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "--background", "--no-tray")
	command.Env = append(os.Environ(), "APPDATA="+base, "XDG_CONFIG_HOME="+base, "HOME="+base)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = command.Process.Kill(); _ = command.Wait() }()
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(5 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		response, err := client.Get("http://127.0.0.1:25500/api/health")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		t.Fatal("API was blocked by stale service or busy kernel port")
	}
	response, err := client.Get("http://127.0.0.1:25500/api/system/status")
	if err != nil {
		t.Fatal(err)
	}
	var status struct {
		External bool `json:"core_service_enabled"`
	}
	err = json.NewDecoder(response.Body).Decode(&status)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if status.External {
		t.Fatal("stale marker still selects an absent helper")
	}
	response, err = client.Post("http://127.0.0.1:25500/api/agent/exit", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
}
