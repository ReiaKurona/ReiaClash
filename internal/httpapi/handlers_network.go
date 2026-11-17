package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reia-clash/internal/agent"
	"strings"
	"time"
)

func (s *Server) connections(w http.ResponseWriter, r *http.Request) {
	path := "/connections"
	if id := r.PathValue("id"); id != "" {
		if len(id) > 128 || strings.ContainsAny(id, "/\\?#") {
			writeError(w, 400, "invalid_id", "无效连接 ID")
			return
		}
		path += "/" + url.PathEscape(id)
	}
	data, _, err := s.app.Core.Client().Do(r.Context(), r.Method, path, nil)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if r.Method == http.MethodDelete {
		writeJSON(w, 200, map[string]any{"success": true})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(data)
}

func (s *Server) dnsQuery(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	kind := strings.ToUpper(r.URL.Query().Get("type"))
	if kind == "" {
		kind = "A"
	}
	if name == "" || len(name) > 253 || strings.ContainsAny(name, "/\\?# \t\r\n") {
		writeError(w, 400, "invalid_name", "请输入有效的 DNS 查询名称")
		return
	}
	allowed := map[string]bool{"A": true, "AAAA": true, "CNAME": true, "MX": true, "TXT": true, "NS": true, "SOA": true, "PTR": true, "SRV": true, "HTTPS": true, "SVCB": true}
	if !allowed[kind] {
		writeError(w, 400, "invalid_type", "不支持的 DNS 查询类型")
		return
	}
	start := time.Now()
	data, _, err := s.app.Core.Client().Do(r.Context(), http.MethodGet, "/dns/query?"+url.Values{"name": {name}, "type": {kind}}.Encode(), nil)
	record := agent.DNSRecord{Source: "manual", Name: name, Type: kind, DurationMS: time.Since(start).Milliseconds()}
	if err != nil {
		record.Error = err.Error()
		s.app.RecordDNS(record)
		writeAppError(w, err)
		return
	}
	if !json.Valid(data) {
		writeError(w, 502, "invalid_dns_response", "内核返回了无效 DNS 响应")
		return
	}
	record.Result = data
	s.app.RecordDNS(record)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(data)
}

func (s *Server) dnsRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		s.app.ClearDNSHistory()
		writeJSON(w, 200, map[string]any{"success": true})
		return
	}
	records, captureError := s.app.DNSHistory()
	writeJSON(w, 200, map[string]any{"records": records, "capture_error": captureError, "limit": 200, "scope": "本次 Agent 运行期间收集的 DNS 日志及手动查询，非完整 DNS 抓包"})
}
