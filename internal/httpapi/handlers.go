package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"reia-clash/internal/agent"
	"reia-clash/internal/mihomo"
	"reia-clash/internal/platform/autostart"
	"reia-clash/internal/platform/coreservice"
	"reia-clash/internal/version"
)

func (s *Server) handshake(w http.ResponseWriter, r *http.Request) {
	var request struct {
		PanelOrigin string `json:"panel_origin"`
		PanelURL    string `json:"panel_url"`
	}
	if err := decodeBody(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "null" {
		requestOrigin, err := cleanPanelOrigin(request.PanelOrigin)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_panel_origin", err.Error())
			return
		}
		headerOrigin, err := cleanPanelOrigin(origin)
		if err != nil || (requestOrigin != headerOrigin && requestOrigin != "null") {
			writeError(w, http.StatusForbidden, "origin_mismatch", "面板地址与浏览器 origin 不一致")
			return
		}
	}
	rawURL := request.PanelURL
	if rawURL == "" {
		rawURL = request.PanelOrigin
	}
	panelURL, err := agent.NormalizePanelURL(rawURL)
	if err != nil {
		writeError(w, 400, "invalid_panel_url", err.Error())
		return
	}
	if request.PanelURL != "" {
		origin := r.Header.Get("Origin")
		if origin != "" && origin != "null" {
			panelOrigin, _ := cleanPanelOrigin(request.PanelURL)
			if panelOrigin != origin {
				writeError(w, 403, "origin_mismatch", "面板 URL 与浏览器 origin 不一致")
				return
			}
		} else if origin == "null" && !strings.HasPrefix(request.PanelURL, "file:") && panelURL != agent.BuiltinPanelURL {
			writeError(w, 403, "origin_mismatch", "本地文件面板必须上报本地 file:/// 地址")
			return
		}
	}
	s.app.PairPanel(panelURL)
	writeJSON(w, http.StatusOK, map[string]any{
		"agent_version": version.Agent,
		"panel":         s.app.PanelState(),
		"mihomo_controller": map[string]string{
			"api_base": s.app.Core.APIBase(),
			"secret":   s.app.Core.Secret(),
		},
		"features": []string{"rest", "websocket", "subscription", "subscription_refresh", "autostart", "panel_pairing", "agent_maintenance"},
	})
}

func (s *Server) realStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.app.RealStatus(r.Context()))
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	since, _ := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64)
	writeJSON(w, http.StatusOK, map[string]any{"events": s.app.EventsAfter(since)})
}

func (s *Server) systemStatus(w http.ResponseWriter, r *http.Request) {
	registration := coreservice.RegistrationStatus()
	running, serviceError := false, ""
	if s.app.Core.UsesExternalLifecycle() {
		running, serviceError = s.app.Core.Status()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"core_service_registration": registration,
		"autostart_enabled":         autostart.Enabled(""),
		"core_service_supported":    coreservice.Supported(),
		"core_service_enabled":      registration.Installed && s.app.Core.UsesExternalLifecycle(),
		"core_service_ready":        registration.Running && running,
		"core_service_error":        serviceError,
		"settings":                  s.app.Prefs.Snapshot(),
	})
}

func (s *Server) setAutoStart(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Enable bool `json:"enable"`
	}
	if err := decodeBody(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := s.app.SetAutoStart(request.Enable); err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "autostart_enabled": request.Enable})
}

func (s *Server) setMaster(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Enable bool `json:"enable"`
	}
	if err := decodeBody(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := s.app.SetEnabled(r.Context(), request.Enable); err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request) {
	var patch agent.SettingsPatch
	if err := decodeBody(r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := s.app.UpdateSettings(r.Context(), patch); err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "settings": s.app.Prefs.Snapshot()})
}

func (s *Server) proxies(w http.ResponseWriter, r *http.Request) {
	data, _, err := s.app.Core.Client().Do(r.Context(), http.MethodGet, "/proxies", nil)
	if err != nil {
		writeAppError(w, err)
		return
	}
	var response map[string]any
	if err := json.Unmarshal(data, &response); err != nil {
		writeAppError(w, err)
		return
	}
	profile, err := os.ReadFile(s.app.Paths.Profile)
	if err != nil {
		writeAppError(w, err)
		return
	}
	order, err := mihomo.ProfileGroupOrder(profile)
	if err != nil {
		writeAppError(w, err)
		return
	}
	response["group_order"] = order
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) selectProxy(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Group string `json:"group"`
		Node  string `json:"node"`
	}
	if err := decodeBody(r, &request); err != nil || request.Group == "" || request.Node == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "group 和 node 都是必填项")
		return
	}
	path := "/proxies/" + url.PathEscape(request.Group)
	_, _, err := s.app.Core.Client().Do(r.Context(), http.MethodPut, path, map[string]string{"name": request.Node})
	if err != nil {
		writeAppError(w, err)
		return
	}
	// 切换节点后强制掐断现有连接，使所有后续请求立即通过新节点
	_ = s.app.Core.Client().CloseConnections(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (s *Server) groupDelay(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Group string `json:"group"`
	}
	if err := decodeBody(r, &request); err != nil || request.Group == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "group 是必填项")
		return
	}
	query := url.Values{}
	query.Set("url", "https://www.gstatic.com/generate_204")
	query.Set("timeout", "5000")
	path := "/group/" + url.PathEscape(request.Group) + "/delay?" + query.Encode()
	data, _, err := s.app.Core.Client().Do(r.Context(), http.MethodGet, path, nil)
	if err != nil {
		writeAppError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(data)
}
