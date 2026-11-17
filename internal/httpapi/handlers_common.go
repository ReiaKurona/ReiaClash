package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

func decodeBody(r *http.Request, target any) error {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(body) > 1<<20 {
		return fmt.Errorf("请求内容超过 1 MiB 限制")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("请求包含多余 JSON 内容")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"success": false, "code": code, "message": message})
}

func writeAppError(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "operation_failed"
	var netErr net.Error
	if errors.As(err, &netErr) || errors.Is(err, context.DeadlineExceeded) {
		status, code = http.StatusServiceUnavailable, "mihomo_unavailable"
	}
	message := err.Error()
	lower := strings.ToLower(message)
	if strings.Contains(lower, "管理员") || strings.Contains(lower, "系统网络权限") || strings.Contains(lower, "权限") || strings.Contains(lower, "cap_net_admin") {
		status, code = http.StatusPreconditionFailed, "permission_required"
	}
	if strings.Contains(message, "局域网共享") {
		status, code = http.StatusForbidden, "loopback_only"
	}
	if strings.Contains(message, "mihomo API 返回 HTTP 404") {
		status, code = http.StatusBadGateway, "mihomo_route_unavailable"
	}
	writeError(w, status, code, message)
}
