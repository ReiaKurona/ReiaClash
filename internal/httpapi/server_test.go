package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORS_PreflightAndPNA(t *testing.T) {
	s := &Server{}
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	handler := s.withCORS(testHandler)

	// 1. 测试浏览器最新的 Private Network Access (PNA) OPTIONS 预检请求
	req := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1:25500/api/agent/real-status", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "authorization, content-type")
	req.Header.Set("Access-Control-Request-Private-Network", "true")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("预检请求预期状态码 204，实际为 %d", resp.StatusCode)
	}
	if pna := resp.Header.Get("Access-Control-Allow-Private-Network"); pna != "true" {
		t.Fatalf("PNA 响应头 Access-Control-Allow-Private-Network 应为 true，实际为 %q", pna)
	}
	if allowOrigin := resp.Header.Get("Access-Control-Allow-Origin"); allowOrigin != "https://example.com" {
		t.Fatalf("Access-Control-Allow-Origin 应回显 origin，实际为 %q", allowOrigin)
	}
	if allowHeaders := resp.Header.Get("Access-Control-Allow-Headers"); allowHeaders != "authorization, content-type" {
		t.Fatalf("Access-Control-Allow-Headers 应匹配请求头，实际为 %q", allowHeaders)
	}

	// 2. 测试本地文件 (file://) 打开 WebUI 时发送的 Origin: null
	reqNull := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:25500/api/agent/real-status", nil)
	reqNull.Header.Set("Origin", "null")
	wNull := httptest.NewRecorder()
	handler.ServeHTTP(wNull, reqNull)

	respNull := wNull.Result()
	if respNull.StatusCode != http.StatusOK {
		t.Fatalf("Origin: null 预期通过验证并返回 200，实际为 %d", respNull.StatusCode)
	}
	if allowOrigin := respNull.Header.Get("Access-Control-Allow-Origin"); allowOrigin != "*" {
		t.Fatalf("Origin: null 时 Access-Control-Allow-Origin 应为 *，实际为 %q", allowOrigin)
	}
	if pna := respNull.Header.Get("Access-Control-Allow-Private-Network"); pna != "true" {
		t.Fatalf("常规请求响应头 Access-Control-Allow-Private-Network 应为 true，实际为 %q", pna)
	}
}

func TestCleanPanelOrigin(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"null", "null"},
		{"file:///C:/Users/test/test-webui.html", "null"},
		{"http://localhost:25500", "http://localhost:25500"},
		{"http://127.0.0.1:25500/test-webui.html", "http://127.0.0.1:25500"},
		{"https://panel.example.com", "https://panel.example.com"},
	}

	for _, c := range cases {
		out, err := cleanPanelOrigin(c.input)
		if err != nil {
			t.Fatalf("cleanPanelOrigin(%q) 出错: %v", c.input, err)
		}
		if out != c.expected {
			t.Errorf("cleanPanelOrigin(%q) = %q; 期望 %q", c.input, out, c.expected)
		}
	}
}

func TestRequestHostIsLoopback(t *testing.T) {
	ctx := context.Background()
	validHosts := []string{"localhost", "localhost:25500", "127.0.0.1", "127.0.0.1:25500", "[::1]", "[::1]:25500", "sub.localhost"}
	for _, h := range validHosts {
		if !requestHostIsLoopback(ctx, h) {
			t.Errorf("host %s 应当被判定为 loopback", h)
		}
	}

	invalidHosts := []string{"192.168.1.1", "10.0.0.1", "google.com"}
	for _, h := range invalidHosts {
		if requestHostIsLoopback(ctx, h) {
			t.Errorf("host %s 不应当被判定为 loopback", h)
		}
	}
}
