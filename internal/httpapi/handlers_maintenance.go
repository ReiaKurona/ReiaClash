package httpapi

import (
	"net/http"
	"strings"
	"time"
)

func (s *Server) subscription(w http.ResponseWriter, r *http.Request) {
	var request struct {
		URL string `json:"url"`
	}
	if err := decodeBody(r, &request); err != nil || strings.TrimSpace(request.URL) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "订阅 URL 不能为空")
		return
	}
	if err := s.app.ImportSubscription(r.Context(), request.URL); err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "message": "订阅已校验并载入 mihomo"})
}

func (s *Server) restartCore(w http.ResponseWriter, r *http.Request) {
	if err := s.app.Core.Restart(); err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"success": true, "message": "已请求重启内核"})
}

func (s *Server) exitAgent(w http.ResponseWriter, r *http.Request) {
	if s.app.Core.UsesExternalLifecycle() {
		// The tray may exit, but the user-session API must remain available
		// for pairing, settings and user-scoped system proxy operations.
		writeJSON(w, http.StatusAccepted, map[string]any{"success": true, "background": true, "message": "已转入后台，面板仍可连接"})
		return
	}
	if err := s.app.PrepareExit(); err != nil {
		writeAppError(w, err)
		return
	}
	if !s.app.Core.UsesExternalLifecycle() {
		if err := s.app.Core.Shutdown(); err != nil {
			writeAppError(w, err)
			return
		}
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
	message := "mihomo 已下载并校验"
	if request.Silent {
		message += "；将在内核下次重启时切换"
	} else {
		message += "，正在重启内核"
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"success": true, "message": message, "tag": info.Tag})
}
