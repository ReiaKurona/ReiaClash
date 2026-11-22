package httpapi

import (
	_ "embed"
	"html/template"
	"net/http"
)

const subscriptionSourcePath = "/subscription/source"

//go:embed subscription_source.html
var subscriptionSourceHTML string

var subscriptionSourceView = template.Must(template.New("subscription-source").Parse(subscriptionSourceHTML))

func (s *Server) subscriptionSource(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	mode := r.Header.Get("Sec-Fetch-Mode")
	dest := r.Header.Get("Sec-Fetch-Dest")
	if r.Header.Get("Origin") != "" || (mode != "" && mode != "navigate") || (dest != "" && dest != "document") {
		http.Error(w, "请在本机浏览器标签页打开订阅原始配置", http.StatusForbidden)
		return
	}
	display := struct {
		Runtime  bool
		Filename string
		Content  string
		Error    string
	}{Runtime: r.URL.Query().Get("view") == "runtime"}
	var source []byte
	var err error
	if display.Runtime {
		display.Filename, source, err = s.app.Core.RunningConfigSource()
		if err != nil {
			display.Error = "暂时无法读取正在运行的内核配置。"
		}
	} else {
		display.Filename = "profile.yaml"
		source, err = s.app.Core.SubscriptionSource()
		if err != nil {
			display.Error = "本机暂时无法读取订阅缓存。"
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	display.Content = string(source)
	if err := subscriptionSourceView.Execute(w, display); err != nil {
		s.logger.Warn("写入本机订阅源文件查看页失败")
	}
}

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
