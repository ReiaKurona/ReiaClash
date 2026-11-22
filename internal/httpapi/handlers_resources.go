package httpapi

import "net/http"

func (s *Server) resources(w http.ResponseWriter, r *http.Request) {
	status, err := s.app.Resources()
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}
func (s *Server) setResourceSchedule(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Enable   bool     `json:"enable"`
		Interval int      `json:"interval_hours"`
		Kinds    []string `json:"kinds"`
	}
	if err := decodeBody(r, &request); err != nil {
		writeError(w, 400, "invalid_request", err.Error())
		return
	}
	if err := s.app.SetResourceSchedule(request.Enable, request.Interval, request.Kinds); err != nil {
		writeAppError(w, err)
		return
	}
	s.resources(w, r)
}
func (s *Server) syncResources(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Kinds []string `json:"kinds"`
	}
	if err := decodeBody(r, &request); err != nil {
		writeError(w, 400, "invalid_request", err.Error())
		return
	}
	if err := s.app.SyncResources(r.Context(), request.Kinds); err != nil {
		writeAppError(w, err)
		return
	}
	s.resources(w, r)
}
