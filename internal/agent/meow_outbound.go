package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"reia-clash/internal/config"
	"reia-clash/internal/kernelbinary"
	"reia-clash/internal/mihomo"
)

func canVerifyMeowEgress(settings config.Settings) bool {
	return kernelbinary.IsMeow(settings.ActiveBinary) && settings.OutboundMode != "direct"
}

type meowHandoffBaseline struct {
	mixed    bool
	ipv6     bool
	tunIPv6  bool
	dns      bool
	dnsIPv6  bool
	selected string
}

// Compare the reachable mixed port and any known working manual choice.
// Each check is enforced only if it worked through the old kernel.
func (a *App) measureMeowBaseline(ctx context.Context, candidate string) meowHandoffBaseline {
	settings := a.Prefs.Snapshot()
	if !kernelbinary.IsMeow(candidate) || a.Core.RunningBinary() == "" {
		return meowHandoffBaseline{}
	}
	var result meowHandoffBaseline
	var probes sync.WaitGroup
	if tunEnabled(settings) {
		probes.Go(func() {
			system := measureMeowSystemBaseline(ctx)
			result.dns, result.dnsIPv6, result.tunIPv6 = system.dns, system.dnsIPv6, system.tunIPv6
		})
	}
	if settings.OutboundMode == "direct" {
		probes.Wait()
		return result
	}
	probes.Go(func() { result.mixed = probeMixedOutbound(ctx) == nil })
	if settings.Enabled {
		probes.Go(func() { result.ipv6 = probeMixedIPv6(ctx) == nil })
	}
	var groups []string
	if settings.OutboundMode == "global" {
		groups = []string{"GLOBAL"}
	} else {
		if profile, err := os.ReadFile(a.Paths.Profile); err == nil {
			groups, _ = mihomo.ProfileGroupOrder(profile)
		}
	}
	for _, group := range groups {
		node := a.Core.RememberedProxy(group)
		if node == "" {
			continue
		}
		if a.probeSelectedNode(ctx, node) == nil {
			result.selected = node
		}
		break
	}
	probes.Wait()
	return result
}

func (a *App) verifyMeowBaseline(ctx context.Context, baseline meowHandoffBaseline) error {
	var mixedErr, ipv6Err, tunIPv6Err, dnsErr, dnsIPv6Err, selectionErr error
	var probes sync.WaitGroup
	if baseline.mixed {
		probes.Go(func() { mixedErr = probeMixedOutbound(ctx) })
	}
	if baseline.ipv6 {
		probes.Go(func() { ipv6Err = probeMixedIPv6(ctx) })
	}
	if baseline.dns {
		probes.Go(func() { dnsErr = probeSystemDNS(ctx, "ip4") })
	}
	if baseline.dnsIPv6 {
		probes.Go(func() { dnsIPv6Err = probeSystemDNS(ctx, "ip6") })
	}
	if baseline.tunIPv6 {
		probes.Go(func() { tunIPv6Err = probeSystemIPv6(ctx) })
	}
	if baseline.selected != "" {
		if a.probeSelectedNode(ctx, baseline.selected) != nil {
			selectionErr = fmt.Errorf("原来可用的手动节点在 meow-rs 上未通过连通性检查，已保留该选择")
		}
	}
	probes.Wait()
	return errors.Join(dnsErr, dnsIPv6Err, tunIPv6Err, ipv6Err, mixedErr, selectionErr)
}

func measureMeowSystemBaseline(ctx context.Context) meowHandoffBaseline {
	var baseline meowHandoffBaseline
	var probes sync.WaitGroup
	probes.Go(func() { baseline.dns = probeSystemDNS(ctx, "ip4") == nil })
	probes.Go(func() { baseline.dnsIPv6 = probeSystemDNS(ctx, "ip6") == nil })
	probes.Go(func() { baseline.tunIPv6 = probeSystemIPv6(ctx) == nil })
	probes.Wait()
	return baseline
}

func probeSystemDNS(parent context.Context, family string) error {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupIP(ctx, family, "www.cloudflare.com")
	if err != nil || len(addresses) == 0 {
		if family == "ip6" {
			return fmt.Errorf("meow-rs TUN 下系统 DNS 未能解析 AAAA")
		}
		return fmt.Errorf("meow-rs TUN 下系统 DNS 未能解析 A")
	}
	return nil
}

func probeSystemIPv6(parent context.Context) error {
	// Bypass the loopback HTTP proxy, so this probes the host routing path.
	transport := &http.Transport{DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	defer cancel()
	if _, err := lookupIPv6Outbound(ctx, client, outboundTraceIPv6URL); err != nil {
		return fmt.Errorf("meow-rs TUN 下系统 IPv6 出站未通过: %w", err)
	}
	return nil
}

func (a *App) probeSelectedNode(parent context.Context, node string) error {
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	defer cancel()
	query := url.Values{"url": {"https://www.gstatic.com/generate_204"}, "timeout": {"3500"}}
	var result struct {
		Delay int `json:"delay"`
	}
	if err := a.Core.Client().GetJSON(ctx, "/proxies/"+url.PathEscape(node)+"/delay?"+query.Encode(), &result); err != nil {
		return err
	}
	if result.Delay <= 0 {
		return fmt.Errorf("节点没有返回有效延迟")
	}
	return nil
}

func probeMixedOutbound(parent context.Context) error {
	return probeViaMixed(parent, []string{outboundTraceURL, "https://www.gstatic.com/generate_204"})
}

func probeMixedIPv6(parent context.Context) error {
	client, transport := mixedOutboundClient()
	defer transport.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	if _, err := lookupIPv6Outbound(ctx, client, outboundTraceIPv6URL); err != nil {
		return fmt.Errorf("meow-rs 混合端口 IPv6 出站未通过: %w", err)
	}
	return nil
}

func probeViaMixed(parent context.Context, endpoints []string) error {
	client, transport := mixedOutboundClient()
	defer transport.CloseIdleConnections()
	return probeOutboundURLs(parent, client, endpoints)
}

func mixedOutboundClient() (*http.Client, *http.Transport) {
	transport := &http.Transport{Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: "127.0.0.1:7890"}), DisableKeepAlives: true}
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return client, transport
}

func probeOutboundURLs(parent context.Context, client *http.Client, endpoints []string) error {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	results := make(chan error, len(endpoints))
	for _, endpoint := range endpoints {
		go func() {
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
			if err != nil {
				results <- err
				return
			}
			response, err := client.Do(request)
			if err == nil {
				response.Body.Close()
				if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNoContent {
					err = fmt.Errorf("HTTP %d", response.StatusCode)
				}
			}
			results <- err
		}()
	}
	var failure error
	for range endpoints {
		select {
		case err := <-results:
			if err == nil {
				return nil
			}
			failure = errors.Join(failure, err)
		case <-ctx.Done():
			failure = errors.Join(failure, ctx.Err())
		}
	}
	return fmt.Errorf("meow-rs 出站不可用，请检查当前策略或选择兼容节点：%w", failure)
}
