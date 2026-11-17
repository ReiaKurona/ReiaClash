package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"reia-clash/internal/agent"
	"reia-clash/internal/version"
)

type Server struct {
	app           *agent.App
	logger        *slog.Logger
	webUIPath     string
	openDirectory func(string) error
}

func New(app *agent.App, logger *slog.Logger, webUIPath string) *Server {
	return &Server{app: app, logger: logger, webUIPath: webUIPath}
}

// Run 在 IPv4 和 IPv6 loopback 上分别监听，避免 localhost 解析到 ::1 时
// 浏览器访问失败。它不会使用 0.0.0.0 或 [::] 作为监听地址。
func (s *Server) Run(ctx context.Context) error {
	return s.RunWithListener(ctx, nil)
}

// RunWithListener reuses the IPv4 listener reserved before starting the kernel.
func (s *Server) RunWithListener(ctx context.Context, primary net.Listener) error {
	handler := s.withCORS(s.routes())
	addresses := []struct{ network, address string }{
		{"tcp4", "127.0.0.1:25500"},
		{"tcp6", "[::1]:25500"},
	}
	var listeners []net.Listener
	if primary != nil {
		listeners = append(listeners, primary)
	}
	for _, item := range addresses {
		if primary != nil && item.network == "tcp4" {
			continue
		}
		listener, err := net.Listen(item.network, item.address)
		if err != nil {
			s.logger.Warn("loopback 地址不可监听", "address", item.address, "error", err)
			continue
		}
		listeners = append(listeners, listener)
	}
	if len(listeners) == 0 {
		return fmt.Errorf("无法监听 127.0.0.1:25500 或 [::1]:25500")
	}
	servers := make([]*http.Server, 0, len(listeners))
	errCh := make(chan error, len(listeners))
	for _, listener := range listeners {
		server := &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
			IdleTimeout:       60 * time.Second,
		}
		servers = append(servers, server)
		go func(server *http.Server, listener net.Listener) {
			if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			}
		}(server, listener)
	}
	s.logger.Info("ReiaClash API 已监听 loopback", "addresses", []string{"127.0.0.1:25500", "[::1]:25500"})
	select {
	case <-ctx.Done():
	case err := <-errCh:
		return err
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var closeErr error
	for _, server := range servers {
		if err := server.Shutdown(shutdownCtx); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
	}
	return closeErr
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		s.serveWebUI(w, r)
	})
	mux.HandleFunc("GET /test-webui.html", s.serveWebUI)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "agent_version": version.Agent, "agent_pid": os.Getpid()})
	})
	mux.HandleFunc("POST /api/handshake", s.handshake)
	mux.HandleFunc("GET /api/agent/panel", s.panelStatus)
	mux.HandleFunc("GET /panel", s.openPanel)
	mux.HandleFunc("GET /api/panel/open", s.openPanel)
	mux.HandleFunc("GET /api/agent/real-status", s.realStatus)
	mux.HandleFunc("GET /api/agent/events", s.events)
	mux.HandleFunc("GET /api/system/status", s.systemStatus)
	mux.HandleFunc("POST /api/system/autostart", s.setAutoStart)
	mux.HandleFunc("POST /api/system/core-service", s.setCoreService)
	mux.HandleFunc("POST /api/agent/master", s.setMaster)
	mux.HandleFunc("POST /api/agent/settings", s.updateSettings)
	mux.HandleFunc("GET /api/agent/geo-low-memory", s.geoLowMemory)
	mux.HandleFunc("POST /api/agent/geo-low-memory", s.geoLowMemory)
	mux.HandleFunc("GET /api/core/proxies", s.proxies)
	mux.HandleFunc("POST /api/core/select", s.selectProxy)
	mux.HandleFunc("POST /api/core/group-delay", s.groupDelay)
	mux.HandleFunc("POST /api/subscription/url", s.subscription)
	mux.HandleFunc("GET /api/subscription/status", s.subscriptionStatus)
	mux.HandleFunc("POST /api/subscription/refresh", s.refreshSubscription)
	mux.HandleFunc("POST /api/agent/restart", s.restartCore)
	mux.HandleFunc("POST /api/core/restart", s.restartCore)
	mux.HandleFunc("GET /api/core/connections", s.connections)
	mux.HandleFunc("DELETE /api/core/connections", s.connections)
	mux.HandleFunc("DELETE /api/core/connections/{id}", s.connections)
	mux.HandleFunc("GET /api/core/dns/query", s.dnsQuery)
	mux.HandleFunc("GET /api/core/dns/records", s.dnsRecords)
	mux.HandleFunc("DELETE /api/core/dns/records", s.dnsRecords)
	mux.HandleFunc("POST /api/agent/exit", s.exitAgent)
	mux.HandleFunc("POST /api/agent/cache/clear", s.clearAgentCache)
	mux.HandleFunc("POST /api/agent/logs/clear", s.clearAgentLogs)
	mux.HandleFunc("POST /api/agent/directory/open", s.openAgentDirectory)
	mux.HandleFunc("GET /api/agent/check-update", s.checkUpdate)
	mux.HandleFunc("POST /api/agent/upgrade", s.upgrade)
	return mux
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requestHostIsLoopback(r.Context(), r.Host) {
			writeError(w, http.StatusForbidden, "loopback_only", "ReiaClash API 只接受指向本机 loopback 地址的请求")
			return
		}

		origin := r.Header.Get("Origin")
		if origin == "null" {
			// 本地文件 (file://) 打开测试 WebUI 时浏览器发送 Origin: null
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
		}

		// 响应最新的 Chrome / 现代浏览器私有网络访问 (Private Network Access, PNA) 规范
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
		w.Header().Add("Vary", "Access-Control-Request-Private-Network")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")

		if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
			w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
			w.Header().Add("Vary", "Access-Control-Request-Headers")
		} else {
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Access-Control-Request-Private-Network, X-Requested-With, Accept")
		}
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requestHostIsLoopback 拒绝 DNS 重绑定的 Host；本地别名必须解析出的地址全部是 loopback。
func requestHostIsLoopback(ctx context.Context, hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = strings.Trim(hostport, "[]")
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "127.0.0.1" || host == "::1" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return isApprovedLoopbackIP(ip)
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupIPAddr(lookupCtx, host)
	if err != nil || len(addresses) == 0 {
		return false
	}
	for _, address := range addresses {
		if !isApprovedLoopbackIP(address.IP) {
			return false
		}
	}
	return true
}

// isApprovedLoopbackIP 只允许 agent 实际监听的两个地址，避免把整个
// 127.0.0.0/8 地址段都当作可接受的 API Host。
func isApprovedLoopbackIP(ip net.IP) bool {
	return ip.Equal(net.IPv4(127, 0, 0, 1)) || ip.Equal(net.IPv6loopback)
}

func (s *Server) serveWebUI(w http.ResponseWriter, r *http.Request) {
	path := s.webUIPath
	if path == "" {
		path = "test-webui.html"
	}
	if !filepath.IsAbs(path) {
		if executable, err := os.Executable(); err == nil {
			candidate := filepath.Join(filepath.Dir(executable), path)
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
			}
		}
	}
	if _, err := os.Stat(path); err != nil {
		http.Error(w, "测试 WebUI 文件不存在；可通过 --webui 指定路径", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, path)
}

func cleanPanelOrigin(value string) (string, error) {
	if value == "" || value == "null" || strings.HasPrefix(value, "file://") {
		return "null", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		return "null", nil
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "null", nil
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}
