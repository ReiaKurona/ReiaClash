package agent

import (
	"context"
	"net/url"
	"reia-clash/internal/panel"
	"reia-clash/internal/platform/network"
	"time"
)

const DefaultPanelURL = panel.DefaultURL
const BuiltinPanelURL = panel.BuiltinURL

type PanelState struct {
	Paired   bool   `json:"paired"`
	PanelURL string `json:"panel_url,omitempty"`
	OpenURL  string `json:"open_url"`
	Source   string `json:"source"`
	Network  string `json:"network"`
}

func NormalizePanelURL(raw string) (string, error) {
	normalized, err := panel.NormalizeURL(raw)
	if err != nil || normalized == "" {
		return normalized, err
	}
	parsed, _ := url.Parse(normalized)
	if (parsed.Host == "127.0.0.1:25500" || parsed.Host == "localhost:25500") && (parsed.Path == "/panel" || parsed.Path == "/api/panel/open") {
		return "", nil
	}
	return normalized, nil
}

func (a *App) PairPanel(panelURL string) {
	if panelURL == "" {
		return
	}
	a.panelMu.Lock()
	changed := a.panelURL != panelURL
	a.panelURL = panelURL
	a.panelMu.Unlock()
	if changed {
		a.AddEvent("info", "面板已连接")
	}
}

func (a *App) PanelState() PanelState {
	a.panelMu.RLock()
	defer a.panelMu.RUnlock()
	paired := a.panelURL != ""
	state := PanelState{Paired: paired, Network: a.panelNetwork}
	if paired {
		state.PanelURL = a.panelURL
	}
	if state.Network == "" {
		state.Network = "checking"
	}
	state.OpenURL, state.Source = DefaultPanelURL, "default"
	if state.Network == "offline" {
		state.OpenURL, state.Source = BuiltinPanelURL, "builtin"
	}
	if state.Paired {
		state.OpenURL, state.Source = state.PanelURL, "paired"
	}
	return state
}

func (a *App) checkPanelNetwork(ctx context.Context) {
	for ctx.Err() == nil {
		online := network.Online(ctx)
		a.panelMu.Lock()
		if online {
			a.panelNetwork = "online"
		} else {
			a.panelNetwork = "offline"
		}
		a.panelMu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-time.After(20 * time.Second):
		}
	}
}
