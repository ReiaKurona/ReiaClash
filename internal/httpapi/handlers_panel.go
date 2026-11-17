package httpapi

import "net/http"

func (s *Server) panelStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.app.PanelState())
}
func (s *Server) openPanel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, s.app.PanelState().OpenURL, http.StatusFound)
}
