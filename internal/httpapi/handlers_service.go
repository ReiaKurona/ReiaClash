package httpapi

import "net/http"

func (s *Server) setCoreService(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Enable bool `json:"enable"`
		Repair bool `json:"repair"`
	}
	if err := decodeBody(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	var err error
	if request.Repair && request.Enable {
		err = s.app.RepairCoreService(r.Context())
	} else {
		err = s.app.SetCoreService(r.Context(), request.Enable)
	}
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":              true,
		"core_service_enabled": s.app.Core.UsesExternalLifecycle(),
	})
}
