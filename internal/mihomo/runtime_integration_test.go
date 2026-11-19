package mihomo

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Uses only isolated loopback listeners; never changes the desktop proxy/TUN.
func TestMihomoRuntimeRouting(t *testing.T) {
	binary := os.Getenv("REIA_TEST_MIHOMO")
	if binary == "" {
		t.Skip("set REIA_TEST_MIHOMO to test a real kernel")
	}
	proxyNode := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			fmt.Fprint(w, "proxied")
			return
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		fmt.Fprint(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
		if _, err := http.ReadRequest(bufio.NewReader(conn)); err != nil {
			return
		}
		fmt.Fprint(conn, "HTTP/1.1 200 OK\r\nContent-Length: 7\r\nConnection: close\r\n\r\nproxied")
	}))
	defer proxyNode.Close()
	direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "direct") }))
	defer direct.Close()
	port := func() int {
		l, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		return l.Addr().(*net.TCPAddr).Port
	}
	apiPort, mixedPort := port(), port()
	dnsServer, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer dnsServer.Close()
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, peer, err := dnsServer.ReadFrom(buffer)
			if err != nil {
				return
			}
			if n < 17 {
				continue
			}
			end := 12
			for end < n && buffer[end] != 0 {
				end += int(buffer[end]) + 1
			}
			end += 5
			if end > n {
				continue
			}
			answer := append([]byte(nil), buffer[:end]...)
			answer[2], answer[3] = 0x81, 0x80
			answer[4], answer[5], answer[6], answer[7] = 0, 1, 0, 1
			answer[8], answer[9], answer[10], answer[11] = 0, 0, 0, 0
			answer = append(answer, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, 192, 0, 2, 1)
			dnsServer.WriteTo(answer, peer)
		}
	}()
	nodeURL, _ := url.Parse(proxyNode.URL)
	dir := t.TempDir()
	configuration := fmt.Sprintf("mixed-port: %d\nallow-lan: false\nbind-address: 127.0.0.1\nexternal-controller: 127.0.0.1:%d\nsecret: test-secret\nmode: rule\nproxies:\n  - name: TestProxy\n    type: http\n    server: 127.0.0.1\n    port: %s\nrules:\n  - MATCH,TestProxy\n", mixedPort, apiPort, nodeURL.Port())
	configuration += fmt.Sprintf("dns:\n  enable: true\n  listen: 127.0.0.1:0\n  nameserver:\n    - %s\n", dnsServer.LocalAddr().String())
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(configuration), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-d", dir, "-f", path)
	logFile, err := os.Create(filepath.Join(dir, "kernel.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	client := NewClient("test-secret")
	client.BaseURL = fmt.Sprintf("http://127.0.0.1:%d", apiPort)
	for {
		if _, _, err := client.Do(ctx, "GET", "/version", nil); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			logs, _ := os.ReadFile(logFile.Name())
			t.Fatalf("kernel not ready: %s", logs)
		case <-time.After(100 * time.Millisecond):
		}
	}
	if _, _, err := client.Do(ctx, "PUT", "/proxies/GLOBAL", map[string]string{"name": "TestProxy"}); err != nil {
		t.Fatal(err)
	}
	dnsResult, _, err := client.Do(ctx, "GET", "/dns/query?name=dns-check.reia.test&type=A", nil)
	if err != nil || !strings.Contains(string(dnsResult), "192.0.2.1") {
		t.Fatalf("real DNS API: %s %v", dnsResult, err)
	}
	connections, _, err := client.Do(ctx, "GET", "/connections", nil)
	if err != nil || !strings.Contains(string(connections), "connections") {
		t.Fatalf("real connections API: %s %v", connections, err)
	}
	if err := client.CloseConnections(ctx); err != nil {
		t.Fatal(err)
	}
	proxyURL, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", mixedPort))
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL), DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	httpClient := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	for _, mode := range []string{"rule", "global", "direct", "rule"} {
		if _, _, err := client.Do(ctx, "PATCH", "/configs", map[string]any{"mode": mode}); err != nil {
			t.Fatal(err)
		}
		_ = client.CloseConnections(ctx)
		response, err := httpClient.Get(direct.URL)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		want := "proxied"
		if mode == "direct" {
			want = "direct"
		}
		if string(body) != want {
			t.Fatalf("mode %s: got %q want %q", mode, body, want)
		}
	}
	for _, enabled := range []bool{true, false} {
		bind := "127.0.0.1"
		if enabled {
			bind = "*"
		}
		if _, _, err := client.Do(ctx, "PATCH", "/configs", map[string]any{"allow-lan": enabled, "bind-address": bind}); err != nil {
			t.Fatal(err)
		}
		var result struct {
			AllowLAN bool   `json:"allow-lan"`
			Mode     string `json:"mode"`
		}
		if err := client.GetJSON(ctx, "/configs", &result); err != nil {
			t.Fatal(err)
		}
		if result.AllowLAN != enabled || !strings.EqualFold(result.Mode, "rule") {
			t.Fatalf("unexpected runtime config: %+v", result)
		}
	}
	if _, _, err := client.Do(ctx, http.MethodPatch, "/configs", map[string]any{
		"tun": map[string]any{"enable": false, "stack": "mips"},
	}); err != nil {
		t.Fatal(err)
	}
	var actual struct {
		Mode string `json:"mode"`
		Tun  struct {
			Enable bool   `json:"enable"`
			Stack  string `json:"stack"`
		} `json:"tun"`
	}
	if err := client.GetJSON(ctx, "/configs", &actual); err != nil || actual.Tun.Enable || !strings.EqualFold(actual.Tun.Stack, "mips") {
		t.Fatalf("TUN PATCH without enabling interception: %+v, %v", actual, err)
	}
	reloaded := strings.Replace(configuration, "mode: rule", "mode: direct", 1)
	if err := os.WriteFile(path, []byte(reloaded), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.Do(ctx, http.MethodPut, "/configs?force=true", map[string]string{}); err != nil {
		t.Fatal(err)
	}
	if err := client.GetJSON(ctx, "/configs", &actual); err != nil || !strings.EqualFold(actual.Mode, "direct") {
		t.Fatalf("same running child did not reload its configured file: %+v, %v", actual, err)
	}
}
