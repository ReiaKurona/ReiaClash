package trayui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reia-clash/internal/panel"
	"reia-clash/internal/platform/launch"
	"reia-clash/internal/platform/network"
	"strings"
	"time"
)

type panelStatus struct {
	Paired  bool   `json:"paired"`
	OpenURL string `json:"open_url"`
	Network string `json:"network"`
	Source  string `json:"source"`
}

func fetchPanelStatus() (panelStatus, error) {
	var state panelStatus
	response, err := (&http.Client{Timeout: 2 * time.Second}).Get(apiBase + "/api/agent/panel")
	if err != nil {
		return state, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return state, fmt.Errorf("面板状态返回 HTTP %d", response.StatusCode)
	}
	err = json.NewDecoder(response.Body).Decode(&state)
	return state, err
}

func openControlPanel() error {
	if state, err := fetchPanelStatus(); err == nil && state.OpenURL != "" {
		deadline := time.Now().Add(3 * time.Second)
		for !state.Paired && state.Network == "checking" && time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
			if next, err := fetchPanelStatus(); err == nil {
				state = next
			} else {
				break
			}
		}
		if target, err := panel.NormalizeURL(state.OpenURL); err == nil && target != "" {
			return launch.OpenURL(target)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if network.Online(ctx) {
		return launch.OpenURL(panel.DefaultURL)
	}
	if executable, err := os.Executable(); err == nil {
		path := filepath.Join(filepath.Dir(executable), "test-webui.html")
		if _, err := os.Stat(path); err == nil {
			webPath := filepath.ToSlash(path)
			if !strings.HasPrefix(webPath, "/") {
				webPath = "/" + webPath
			}
			return launch.OpenURL((&url.URL{Scheme: "file", Path: webPath}).String())
		}
	}
	return launch.OpenURL(panel.BuiltinURL)
}

func watchPanel(ctx context.Context, notify func(string, string), paired bool) {
	for {
		if state, err := fetchPanelStatus(); err == nil {
			if state.Paired && !paired {
				notify("ReiaClash", "面板已连接，点击打开。")
			}
			paired = state.Paired
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}
