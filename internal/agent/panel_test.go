package agent

import "testing"

func TestPanelRuntimeTargets(t *testing.T) {
	app := &App{}
	if got := app.PanelState(); got.OpenURL != DefaultPanelURL || got.Paired {
		t.Fatal(got)
	}
	app.panelNetwork = "online"
	if got := app.PanelState(); got.OpenURL != DefaultPanelURL || got.Source != "default" {
		t.Fatal(got)
	}
	url := "https://next.rka.jp/theme/panel?tab=nodes#/dashboard"
	app.PairPanel(url)
	app.PairPanel(url)
	if got := app.PanelState(); got.OpenURL != url || !got.Paired {
		t.Fatal(got)
	}
	if len(app.EventsAfter(0)) != 1 {
		t.Fatal("periodic scans must not repeat pairing events")
	}
	app.panelNetwork = "offline"
	if got := app.PanelState(); got.OpenURL != url || !got.Paired {
		t.Fatal(got)
	}
	if (&App{}).PanelState().Paired {
		t.Fatal("pairing leaked across agent sessions")
	}
}

func TestExplicitLocalPanelURLsAreRetained(t *testing.T) {
	app := &App{panelNetwork: "online"}
	locals := []string{BuiltinPanelURL, "http://localhost:25500/test-webui.html#/nodes", "http://[::1]:25500/", "file:///C:/Users/tcs15/Desktop/ReiaClash/test-webui.html"}
	for _, local := range locals {
		app.PairPanel(local)
		if got := app.PanelState(); !got.Paired || got.PanelURL != local || got.OpenURL != local || got.Source != "paired" {
			t.Fatal("actual local page was replaced", got)
		}
	}
	fileURL := locals[len(locals)-1]
	app.PairPanel("")
	app.panelNetwork = "checking"
	if got := app.PanelState(); got.OpenURL != fileURL {
		t.Fatal("missing payload or pending probe overwrote paired file URL", got)
	}
	fresh := &App{panelNetwork: "offline"}
	if got := fresh.PanelState(); got.OpenURL != BuiltinPanelURL || got.Paired {
		t.Fatal(got)
	}
}

func TestPanelURLValidation(t *testing.T) {
	for _, raw := range []string{"javascript:alert(1)", "https://user:password@example.com", ":bad", "https:///missing-host"} {
		if _, err := NormalizePanelURL(raw); err == nil {
			t.Fatal("accepted " + raw)
		}
	}
	for _, raw := range []string{"null", ""} {
		if got, err := NormalizePanelURL(raw); err != nil || got != "" {
			t.Fatalf("%s: %s %v", raw, got, err)
		}
	}
	fileURL := "file:///C:/Users/tcs15/Desktop/ReiaClash/test-webui.html#/nodes"
	if got, err := NormalizePanelURL(fileURL); err != nil || got != fileURL {
		t.Fatalf("file URL was not preserved: %s %v", got, err)
	}
	raw := "https://next.rka.jp/panel?theme=x#/nodes"
	if got, err := NormalizePanelURL(raw); err != nil || got != raw {
		t.Fatalf("lost panel path/query/hash: %s %v", got, err)
	}
}
