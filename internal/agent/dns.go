package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

type DNSRecord struct {
	ID         uint64          `json:"id"`
	At         time.Time       `json:"at"`
	Source     string          `json:"source"`
	Name       string          `json:"name,omitempty"`
	Type       string          `json:"type,omitempty"`
	Payload    string          `json:"payload,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
	DurationMS int64           `json:"duration_ms,omitempty"`
}

func (a *App) RecordDNS(record DNSRecord) {
	a.dnsMu.Lock()
	defer a.dnsMu.Unlock()
	a.dnsID++
	record.ID = a.dnsID
	record.At = time.Now().UTC()
	if len(record.Payload) > 4096 {
		record.Payload = strings.Clone(record.Payload[:4096])
	}
	if len(record.Error) > 4096 {
		record.Error = strings.Clone(record.Error[:4096])
	}
	if len(record.Result) > 32768 {
		record.Result = nil
		record.Payload = "查询响应过大，请查看本次查询结果"
	}
	a.dnsRecords = append(a.dnsRecords, record)
	if len(a.dnsRecords) > 200 {
		a.dnsRecords = append([]DNSRecord(nil), a.dnsRecords[len(a.dnsRecords)-200:]...)
	}
}

func (a *App) DNSHistory() ([]DNSRecord, string) {
	a.dnsMu.Lock()
	defer a.dnsMu.Unlock()
	return append([]DNSRecord{}, a.dnsRecords...), a.dnsCaptureError
}

func (a *App) ClearDNSHistory() { a.dnsMu.Lock(); a.dnsRecords = nil; a.dnsMu.Unlock() }

func (a *App) captureDNS(ctx context.Context) {
	for ctx.Err() == nil {
		session, cancel := context.WithTimeout(ctx, 2*time.Minute)
		err := a.Core.Client().StreamLogs(session, func(payload string) {
			if strings.Contains(strings.ToUpper(payload), "[DNS]") {
				a.RecordDNS(DNSRecord{Source: "kernel", Payload: payload})
			}
			a.dnsMu.Lock()
			a.dnsCaptureError = ""
			a.dnsMu.Unlock()
		})
		cancel()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			a.dnsMu.Lock()
			a.dnsCaptureError = "DNS 日志订阅正在重连"
			a.dnsMu.Unlock()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}
