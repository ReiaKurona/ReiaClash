package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

type OutboundIP struct {
	IP          string `json:"ip"`
	CountryCode string `json:"country_code,omitempty"`
	Colo        string `json:"colo,omitempty"`
}

const outboundTraceURL = "https://1.1.1.1/cdn-cgi/trace"

func (a *App) OutboundIP(ctx context.Context) (OutboundIP, error) {
	transport := &http.Transport{DisableKeepAlives: true}
	settings := a.Prefs.Snapshot()
	if settings.Enabled && (settings.UseSystemProxy || settings.UseTun) {
		transport.Proxy = http.ProxyURL(&url.URL{Scheme: "http", Host: "127.0.0.1:7890"})
	}
	client := &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	trace, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return lookupOutboundIP(trace, client, outboundTraceURL)
}

func lookupOutboundIP(ctx context.Context, client *http.Client, endpoint string) (OutboundIP, error) {
	var result OutboundIP
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return result, err
	}
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := client.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("IP 查询返回 HTTP %d", resp.StatusCode)
	}
	return parseOutboundTrace(resp.Body)
}

func parseOutboundTrace(body io.Reader) (OutboundIP, error) {
	var result OutboundIP
	scanner := bufio.NewScanner(io.LimitReader(body, (16<<10)+1))
	for scanner.Scan() {
		key, value, _ := strings.Cut(scanner.Text(), "=")
		switch key {
		case "ip":
			result.IP = strings.TrimSpace(value)
		case "loc":
			result.CountryCode = traceCode(value, 2)
		case "colo":
			result.Colo = traceCode(value, 3)
		}
	}
	if err := scanner.Err(); err != nil {
		return OutboundIP{}, err
	}
	ip, err := netip.ParseAddr(result.IP)
	if err != nil || ip.Zone() != "" || !isPublicIP(ip.Unmap().AsSlice()) || netip.MustParsePrefix("100.64.0.0/10").Contains(ip.Unmap()) {
		return OutboundIP{}, fmt.Errorf("IP 查询未返回有效公网地址")
	}
	result.IP = ip.Unmap().String()
	return result, nil
}

func traceCode(value string, length int) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) == length && strings.IndexFunc(value, func(r rune) bool { return r < 'A' || r > 'Z' }) == -1 {
		return value
	}
	return ""
}
