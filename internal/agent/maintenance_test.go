package agent

import (
	"os"
	"path/filepath"
	"testing"

	"reia-clash/internal/config"
	"reia-clash/internal/update"
)

func TestAgentMaintenancePreservesConfigurationAndLogWriter(t *testing.T) {
	paths, err := config.PathsAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := &App{Paths: paths, Updater: update.NewManager(paths.UpdateDir)}
	for _, name := range []string{"settings.json", "profile.yaml", "mihomo.yaml", "mihomo.secret", "core-service.token", "core-service.json", "system-proxy-backup.json", "core-service.log", "agent-startup.log"} {
		if err := os.WriteFile(filepath.Join(paths.Root, name), []byte("preserve"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writer, err := os.OpenFile(filepath.Join(paths.Root, "agent.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if _, err := writer.WriteString("before"); err != nil {
		t.Fatal(err)
	}
	app.AddEvent("info", "before")
	app.RecordDNS(DNSRecord{Name: "example.com"})
	app.PairPanel("https://example.com/panel")
	panel := app.PanelState()
	if err := os.WriteFile(filepath.Join(paths.UpdateDir, ".mihomo-download.tmp"), []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	cache, err := app.ClearAgentCache()
	if err != nil || cache.Files != 1 || cache.DNSRecords != 1 {
		t.Fatal(cache, err)
	}
	app.RecordDNS(DNSRecord{Name: "after.example"})
	if history, _ := app.DNSHistory(); len(history) != 1 || history[0].ID != 2 {
		t.Fatal(history)
	}
	logs, err := app.ClearAgentLogs()
	if err != nil || logs.Files != 3 || logs.Events != 2 {
		t.Fatal(logs, err)
	}
	if len(app.EventsAfter(0)) != 0 {
		t.Fatal("events not cleared")
	}
	app.AddEvent("info", "after")
	if events := app.EventsAfter(2); len(events) != 1 || events[0].ID != 3 {
		t.Fatal(events)
	}
	if _, err := writer.WriteString("after"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(paths.Root, "agent.log")); string(data) != "after" {
		t.Fatalf("existing log handle failed: %q", data)
	}
	for _, name := range []string{"settings.json", "profile.yaml", "mihomo.yaml", "mihomo.secret", "core-service.token", "core-service.json", "system-proxy-backup.json"} {
		if data, _ := os.ReadFile(filepath.Join(paths.Root, name)); string(data) != "preserve" {
			t.Fatal("changed", name)
		}
	}
	if data, err := os.ReadFile(filepath.Join(paths.Root, "core-service.log")); err != nil || len(data) != 0 {
		t.Fatalf("service log was not cleared: %v", err)
	}
	if app.PanelState() != panel {
		t.Fatal("pairing changed")
	}
}

func TestLogCleanupRejectsSymlink(t *testing.T) {
	paths, err := config.PathsAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "protected")
	if err := os.WriteFile(target, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(paths.Root, "agent.log")); err != nil {
		t.Skip("symlink unavailable:", err)
	}
	if _, err := (&App{Paths: paths}).ClearAgentLogs(); err == nil {
		t.Fatal("expected refusal")
	}
	if data, _ := os.ReadFile(target); string(data) != "preserve" {
		t.Fatal("symlink target changed")
	}
}
