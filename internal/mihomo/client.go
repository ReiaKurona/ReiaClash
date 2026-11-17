package mihomo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client 是 agent 调用 mihomo 本机控制器的客户端。
// 密钥由 agent 管理，并按当前连接协议通过握手返回给 WebUI。
type Client struct {
	BaseURL string
	Secret  string
	HTTP    *http.Client
}

func NewClient(secret string) *Client {
	return &Client{
		BaseURL: "http://127.0.0.1:9090",
		Secret:  secret,
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) Do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reader = bytes.NewReader(payload)
	}
	endpoint := strings.TrimRight(c.BaseURL, "/") + "/" + strings.TrimLeft(path, "/")
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() != "127.0.0.1" {
		return nil, 0, fmt.Errorf("mihomo API 地址必须指向 127.0.0.1")
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return data, resp.StatusCode, fmt.Errorf("mihomo API 返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, resp.StatusCode, nil
}

func (c *Client) GetJSON(ctx context.Context, path string, target any) error {
	data, _, err := c.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// CloseConnections 强制掐断当前所有活跃连接，用于切换节点或模式时让流量立即通过新节点
func (c *Client) CloseConnections(ctx context.Context) error {
	_, _, err := c.Do(ctx, http.MethodDelete, "/connections", nil)
	return err
}
