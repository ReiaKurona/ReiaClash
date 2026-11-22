package mihomo

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// ServeServiceLifecycle 只接受经过认证的本机生命周期命令。
// Mihomo REST API 仍监听单独的认证端口。
func ServeServiceLifecycle(ctx context.Context, manager *Manager, token string) error {
	listener, err := net.Listen("tcp4", serviceControlAddress)
	if err != nil {
		return fmt.Errorf("监听 Mihomo 服务控制端口失败：%w", err)
	}
	return ServeServiceLifecycleWithListener(ctx, manager, token, listener)
}

func ServeServiceLifecycleWithListener(ctx context.Context, manager *Manager, token string, listener net.Listener) error {
	defer listener.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		running, lastError := manager.Status()
		status := serviceStatus{
			Started: manager.SupervisorStarted(), Running: running, LastError: lastError, Binary: manager.RunningBinary(),
		}
		if r.URL.Query().Get("metrics") == "1" && running {
			status.Binary, status.MemoryBytes = manager.RunningMetrics()
		}
		writeServiceJSON(w, http.StatusOK, status)
	})
	mux.HandleFunc("POST /start", func(w http.ResponseWriter, r *http.Request) {
		if err := manager.settings.Reload(); err != nil {
			writeServiceError(w, http.StatusInternalServerError, err)
			return
		}
		if err := manager.Start(ctx); err != nil {
			writeServiceError(w, http.StatusInternalServerError, err)
			return
		}
		writeServiceJSON(w, http.StatusAccepted, map[string]bool{"success": true})
	})
	mux.HandleFunc("POST /restart", func(w http.ResponseWriter, r *http.Request) {
		if err := manager.settings.Reload(); err != nil {
			writeServiceError(w, http.StatusInternalServerError, err)
			return
		}
		if !manager.SupervisorStarted() {
			if err := manager.Start(ctx); err != nil {
				writeServiceError(w, http.StatusInternalServerError, err)
				return
			}
			writeServiceJSON(w, http.StatusAccepted, map[string]bool{"success": true})
			return
		}
		if err := manager.Restart(); err != nil {
			writeServiceError(w, http.StatusInternalServerError, err)
			return
		}
		writeServiceJSON(w, http.StatusAccepted, map[string]bool{"success": true})
	})
	mux.HandleFunc("POST /stop", func(w http.ResponseWriter, r *http.Request) {
		if err := manager.Shutdown(); err != nil {
			writeServiceError(w, http.StatusInternalServerError, err)
			return
		}
		writeServiceJSON(w, http.StatusAccepted, map[string]bool{"success": true})
	})
	server := &http.Server{
		Handler:           serviceAuth(token, mux),
		ReadHeaderTimeout: 3 * time.Second,
		IdleTimeout:       15 * time.Second,
	}
	done := make(chan error, 1)
	go func() {
		done <- server.Serve(listener)
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-done:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func serviceAuth(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
		remote := net.ParseIP(remoteIP)
		if err != nil || remote == nil || !remote.IsLoopback() || r.Host != serviceControlAddress {
			http.Error(w, "loopback only", http.StatusForbidden)
			return
		}
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		authorization := strings.TrimPrefix(header, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(authorization), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeServiceJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeServiceError(w http.ResponseWriter, status int, err error) {
	writeServiceJSON(w, status, map[string]any{"success": false, "message": err.Error()})
}
