package httpapi

import (
	"net/http"
	"reia-clash/internal/config"
)

func (s *Server) geoLowMemory(w http.ResponseWriter, r *http.Request) {
	restartRequested := false
	if r.Method == http.MethodPost {
		var request struct {
			Enable *bool `json:"enable"`
		}
		if err := decodeBody(r, &request); err != nil || request.Enable == nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "enable 必须为布尔值")
			return
		}
		var err error
		restartRequested, err = s.app.SetGeoLowMemory(r.Context(), *request.Enable)
		if err != nil {
			writeAppError(w, err)
			return
		}
	}
	enabled := s.app.Prefs.Snapshot().GeoLowMemory
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "enabled": enabled, "geodata_loader": config.GeodataLoader(enabled), "restart_requested": restartRequested})
}
