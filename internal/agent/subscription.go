package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"reia-clash/internal/config"
)

func (a *App) ImportSubscription(ctx context.Context, rawURL string) error {
	parsed, err := parseSubscriptionURL(rawURL)
	if err != nil {
		return err
	}
	defer func() {
		select {
		case a.subscriptionWake <- struct{}{}:
		default:
		}
	}()
	return a.updateSubscription(ctx, parsed.String(), true)
}

const SubscriptionRefreshInterval = 12 * time.Hour

type SubscriptionStatus struct {
	HasURL               bool   `json:"has_url"`
	Refreshing           bool   `json:"refreshing"`
	LastSuccessAt        int64  `json:"last_success_at"`
	LastAttemptAt        int64  `json:"last_attempt_at"`
	NextRefreshAt        int64  `json:"next_refresh_at"`
	RefreshIntervalHours int    `json:"refresh_interval_hours"`
	LastError            string `json:"last_error,omitempty"`
}

func (a *App) SubscriptionStatus() SubscriptionStatus {
	s := a.Prefs.Snapshot()
	a.subscriptionStatusMu.Lock()
	defer a.subscriptionStatusMu.Unlock()
	status := SubscriptionStatus{HasURL: s.SubscriptionURL != "", Refreshing: a.subscriptionBusy, LastSuccessAt: s.SubscriptionUpdatedAt, LastAttemptAt: a.subscriptionAttempt, LastError: a.subscriptionError, RefreshIntervalHours: 12}
	if s.SubscriptionUpdatedAt > 0 {
		status.NextRefreshAt = s.SubscriptionUpdatedAt + int64(SubscriptionRefreshInterval/time.Second)
	}
	return status
}

func (a *App) RefreshSubscription(ctx context.Context) error {
	return a.updateSubscription(ctx, "", false)
}

func (a *App) updateSubscription(ctx context.Context, rawURL string, replace bool) (err error) {
	a.subscriptionMu.Lock()
	defer a.subscriptionMu.Unlock()
	if err = ctx.Err(); err != nil {
		return err
	}
	if replace {
		a.serviceMu.Lock()
		_, err = a.Prefs.Update(func(s *config.Settings) error {
			if s.SubscriptionURL != rawURL {
				s.SubscriptionUpdatedAt = 0
			}
			s.SubscriptionURL = rawURL
			return nil
		})
		a.serviceMu.Unlock()
		if err != nil {
			return err
		}
	}
	saved := a.Prefs.Snapshot()
	if saved.SubscriptionURL == "" {
		return fmt.Errorf("尚未保存订阅链接，请先手动安装订阅或打开站点同步")
	}
	a.subscriptionStatusMu.Lock()
	a.subscriptionBusy = true
	a.subscriptionAttempt = time.Now().Unix()
	a.subscriptionStatusMu.Unlock()
	defer func() {
		a.subscriptionStatusMu.Lock()
		a.subscriptionBusy = false
		a.subscriptionError = ""
		if err != nil {
			a.subscriptionError = "订阅刷新失败，客户端将自动重试；请检查网络、订阅有效性和内核状态"
		}
		a.subscriptionStatusMu.Unlock()
	}()
	fetch := a.subscriptionFetch
	if fetch == nil {
		fetch = downloadProfile
	}
	profile, err := fetch(ctx, saved.SubscriptionURL)
	if err != nil {
		return err
	}
	previous, _ := os.ReadFile(a.Paths.Profile)
	if saved.SubscriptionUpdatedAt == 0 || !bytes.Equal(previous, profile) {
		// Service switching and background refresh must not overlap the handover.
		a.serviceMu.Lock()
		apply := a.subscriptionApply
		if apply != nil {
			err = apply(profile)
		} else {
			err = a.Core.ImportProfile(ctx, profile)
		}
		a.serviceMu.Unlock()
		if err != nil {
			return err
		}
	}
	_, err = a.Prefs.Update(func(s *config.Settings) error { s.SubscriptionUpdatedAt = time.Now().Unix(); return nil })
	if err == nil {
		a.AddEvent("info", "订阅已刷新，客户端将每 12 小时自动检查")
	}
	return err
}

func (a *App) subscriptionRefreshLoop(ctx context.Context) {
	first := true
	pendingRetry := false
	retry := time.Minute
	for {
		saved := a.Prefs.Snapshot()
		delay := SubscriptionRefreshInterval
		if pendingRetry || subscriptionDue(saved, time.Now(), first) {
			if err := a.RefreshSubscription(ctx); err != nil {
				if ctx.Err() != nil {
					return
				}
				delay = retry
				pendingRetry = true
				retry = min(retry*2, 30*time.Minute)
			} else {
				retry = time.Minute
				pendingRetry = false
			}
		} else if saved.SubscriptionURL != "" && saved.SubscriptionUpdatedAt > 0 {
			delay = max(time.Second, time.Until(time.Unix(saved.SubscriptionUpdatedAt, 0).Add(SubscriptionRefreshInterval)))
		}
		first = false
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-a.subscriptionWake:
			timer.Stop()
			retry = time.Minute
			pendingRetry = a.SubscriptionStatus().LastError != ""
		case <-timer.C:
		}
	}
}

func subscriptionDue(s config.Settings, now time.Time, startup bool) bool {
	return s.SubscriptionURL != "" && (startup || s.SubscriptionUpdatedAt <= 0 || !now.Before(time.Unix(s.SubscriptionUpdatedAt, 0).Add(SubscriptionRefreshInterval)))
}

func parseSubscriptionURL(rawURL string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
		return nil, fmt.Errorf("订阅地址必须是有效的 HTTP 或 HTTPS URL")
	}
	if strings.EqualFold(u.Hostname(), "localhost") || net.ParseIP(u.Hostname()) != nil && !isPublicIP(net.ParseIP(u.Hostname())) {
		return nil, fmt.Errorf("订阅地址不能指向本机或内网 IP")
	}
	u.Fragment = ""
	return u, nil
}

func downloadProfile(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := parseSubscriptionURL(rawURL)
	if err != nil {
		return nil, err
	}
	if err := validatePublicHost(ctx, u.Hostname()); err != nil {
		return nil, err
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			var lastErr error
			for _, candidate := range ips {
				if !isPublicIP(candidate.IP) {
					continue
				}
				conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			if lastErr == nil {
				lastErr = fmt.Errorf("订阅域名没有可用的公网地址")
			}
			return nil, lastErr
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Timeout:   45 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 4 {
				return fmt.Errorf("订阅重定向次数过多")
			}
			return validatePublicHost(req.Context(), req.URL.Hostname())
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "clash.meta ReiaClash/0.1")
	req.Header.Set("Accept", "application/yaml, text/yaml, text/plain;q=0.9, */*;q=0.5")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载订阅失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("订阅服务器返回 HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 16<<20 {
		return nil, fmt.Errorf("订阅文件超过 16 MiB 限制")
	}
	body = bytesTrimUTF8BOM(body)
	if !strings.Contains(string(body), "proxies:") && !strings.Contains(string(body), "proxy-groups:") {
		decoded := make([]byte, base64.StdEncoding.DecodedLen(len(body)))
		n, decodeErr := base64.StdEncoding.Decode(decoded, bytesTrimSpace(body))
		if decodeErr != nil {
			decoded = make([]byte, base64.RawStdEncoding.DecodedLen(len(body)))
			n, decodeErr = base64.RawStdEncoding.Decode(decoded, bytesTrimSpace(body))
		}
		if decodeErr == nil {
			body = bytesTrimUTF8BOM(decoded[:n])
		}
	}
	return body, nil
}

func validatePublicHost(ctx context.Context, host string) error {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("无法解析订阅域名: %w", err)
	}
	for _, candidate := range ips {
		if !isPublicIP(candidate.IP) {
			return fmt.Errorf("订阅地址解析到本机或内网 IP，已阻止访问")
		}
	}
	if len(ips) == 0 {
		return fmt.Errorf("订阅域名没有解析到 IP")
	}
	return nil
}

func isPublicIP(ip net.IP) bool {
	return ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsUnspecified()
}

func bytesTrimSpace(src []byte) []byte { return []byte(strings.TrimSpace(string(src))) }

func bytesTrimUTF8BOM(src []byte) []byte {
	return []byte(strings.TrimPrefix(string(src), "\uFEFF"))
}
