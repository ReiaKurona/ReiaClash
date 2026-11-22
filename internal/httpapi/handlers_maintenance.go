package httpapi

import (
	"net/http"
	"strings"
	"time"
)

func (s *Server) subscription(w http.ResponseWriter, r *http.Request) {
	var request struct {
		URL      string `json:"url"`
		SyncOnly bool   `json:"sync_only"`
	}
	if err := decodeBody(r, &request); err != nil || strings.TrimSpace(request.URL) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "订阅 URL 不能为空")
		return
	}
	apply := s.app.ImportSubscription
	if request.SyncOnly {
		apply = s.app.SyncSubscription
	}
	if err := apply(r.Context(), request.URL); err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "订阅已同步"})
}

func (s *Server) restartCore(w http.ResponseWriter, r *http.Request) {
	if err := s.app.RestartCore(); err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"success": true, "message": "已请求重启内核"})
}

func (s *Server) exitAgent(w http.ResponseWriter, r *http.Request) {
	s.finishExit(w, r, false)
}

func (s *Server) uninstallServiceAndExit(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Mode") != "" {
		writeError(w, http.StatusForbidden, "desktop_only", "请从桌面托盘注销服务并退出")
		return
	}
	s.finishExit(w, r, true)
}

func (s *Server) finishExit(w http.ResponseWriter, r *http.Request, uninstallService bool) {
	background, err := s.app.PrepareDesktopExit(r.Context(), uninstallService)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if background {
		writeJSON(w, http.StatusAccepted, map[string]any{"success": true, "background": true, "message": "已转入后台，面板仍可连接"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"success": true, "message": "正在关闭 ReiaClash"})
	go func() {
		time.Sleep(150 * time.Millisecond)
		s.app.RequestExit()
	}()
}

func (s *Server) checkUpdate(w http.ResponseWriter, r *http.Request) {
	info, err := s.app.CheckUpdate(r.Context())
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) upgrade(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Silent bool `json:"is_silent"`
	}
	if err := decodeBody(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	info, err := s.app.UpgradeCore(r.Context(), request.Silent)
	if err != nil {
		writeAppError(w, err)
		return
	}
	message := "内核已校验并启动"
	if request.Silent {
		message = "当前内核已是最新版本"
		if info.Pending {
			message = "更新包已校验，暂停代理或下次启动、重启内核时自动应用"
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": message, "tag": info.Tag, "pending": info.Pending})
}

func (s *Server) coreType(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var request struct {
			Kind string `json:"kind"`
		}
		if err := decodeBody(r, &request); err != nil || (request.Kind != "mihomo" && request.Kind != "meow-rs") {
			writeError(w, http.StatusBadRequest, "invalid_request", "kind 必须为 mihomo 或 meow-rs")
			return
		}
		if err := s.app.SwitchCore(r.Context(), request.Kind); err != nil {
			writeAppError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "kind": s.app.CoreKind()})
}
