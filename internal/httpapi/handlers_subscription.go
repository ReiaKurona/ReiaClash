package httpapi

import "net/http"

func (s *Server) subscriptionStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.app.SubscriptionStatus())
}

func (s *Server) refreshSubscription(w http.ResponseWriter, r *http.Request) {
	if err := s.app.RefreshSubscription(r.Context()); err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "subscription": s.app.SubscriptionStatus()})
}
