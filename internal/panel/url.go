package panel

import (
	"fmt"
	"net/url"
	"strings"
)

// Preserve file locations and SPA routes. Missing URLs do not imply pairing.
func NormalizeURL(raw string) (string, error) {
	if raw == "" || raw == "null" {
		return "", nil
	}
	if len(raw) > 4096 {
		return "", fmt.Errorf("面板 URL 过长")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("无效的面板 URL")
	}
	if parsed.User != nil || strings.ContainsAny(raw+parsed.Path+parsed.Fragment, "\r\n\x00") {
		return "", fmt.Errorf("面板 URL 不能包含用户名、密码或控制字符")
	}
	if parsed.Scheme == "file" {
		if parsed.Opaque != "" || !strings.HasPrefix(parsed.Path, "/") || parsed.Path == "/" || parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
			return "", fmt.Errorf("本地面板必须使用完整的 file:/// 绝对路径")
		}
		return parsed.String(), nil
	}
	if (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Hostname() == "" {
		return "", fmt.Errorf("面板 URL 必须是 HTTP(S) 或本地 file:/// 地址")
	}
	return parsed.String(), nil
}
