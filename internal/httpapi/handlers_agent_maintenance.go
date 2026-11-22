package httpapi

import (
	"net/http"
	"reia-clash/internal/platform/launch"
)

func (s *Server) clearAgentCache(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.ClearAgentCache()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "operation_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "cleared": result})
}

func (s *Server) clearAgentLogs(w http.ResponseWriter, r *http.Request) {
	result, err := s.app.ClearAgentLogs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "operation_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "cleared": result})
}

func (s *Server) openAgentDirectory(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Directory string `json:"directory"`
	}
	if err := decodeBody(r, &request); err != nil {
		writeError(w, 400, "invalid_request", err.Error())
		return
	}
	path, err := s.app.AgentDirectory(request.Directory)
	if err != nil {
		writeError(w, 400, "invalid_request", err.Error())
		return
	}
	opener := s.openDirectory
	if opener == nil {
		opener = launch.OpenDirectory
	}
	if err := opener(path); err != nil {
		writeError(w, http.StatusInternalServerError, "operation_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"success": true, "directory": request.Directory, "message": "已请求客户端打开目录"})
}

func (s *Server) agentDirectorySizes(w http.ResponseWriter, r *http.Request) {
	sizes, err := s.app.DirectorySizes(r.Context())
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "directories": sizes})
}

func (s *Server) clearSubscription(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Confirm bool `json:"confirm"`
	}
	if err := decodeBody(r, &request); err != nil || !request.Confirm {
		writeError(w, http.StatusBadRequest, "confirmation_required", "请确认清除订阅与缓存配置")
		return
	}
	if err := s.app.ClearSubscription(r.Context()); err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}
