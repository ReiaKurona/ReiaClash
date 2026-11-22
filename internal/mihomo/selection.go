package mihomo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"time"
)

func (m *Manager) readSelections() (map[string]string, error) {
	path := filepath.Join(m.dataDir, "node-selections.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		data, err = os.ReadFile(path + ".previous")
	}
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	selections := map[string]string{}
	if err := json.Unmarshal(data, &selections); err != nil {
		return nil, err
	}
	if selections == nil {
		selections = map[string]string{}
	}
	return selections, nil
}

func (m *Manager) SelectProxy(ctx context.Context, group, node string) error {
	m.profileMu.Lock()
	defer m.profileMu.Unlock()
	selections, err := m.readSelections()
	if err != nil {
		return fmt.Errorf("读取已记忆的节点失败: %w", err)
	}
	client := m.Client()
	if err := putSelectedProxy(ctx, client, group, node); err != nil {
		return err
	}
	_ = client.CloseConnections(ctx)
	if selections[group] == node {
		return nil
	}
	selections[group] = node
	data, err := json.Marshal(selections)
	if err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(m.dataDir, "node-selections.json"), data); err != nil {
		return fmt.Errorf("节点已切换，但保存节点记忆失败: %w", err)
	}
	return nil
}

func putSelectedProxy(ctx context.Context, client *Client, group, node string) error {
	_, _, err := client.Do(ctx, http.MethodPut, "/proxies/"+url.PathEscape(group), map[string]string{"name": node})
	return err
}

func (m *Manager) restoreSelections(ctx context.Context) {
	selections, err := m.readSelections()
	if err != nil {
		m.logger.Warn("读取节点记忆失败", "error", err)
		return
	}
	if len(selections) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	client := m.Client()
	var current struct {
		Proxies map[string]struct {
			All []string `json:"all"`
			Now string   `json:"now"`
		} `json:"proxies"`
	}
	if err := client.GetJSON(ctx, "/proxies", &current); err != nil {
		m.logger.Warn("读取节点列表失败，未恢复节点记忆", "error", err)
		return
	}
	changed := false
	for group, node := range selections {
		proxy := current.Proxies[group]
		if proxy.Now == node || !slices.Contains(proxy.All, node) {
			continue
		}
		if err := putSelectedProxy(ctx, client, group, node); err != nil {
			m.logger.Warn("恢复代理组的节点记忆失败", "error", err)
		} else {
			changed = true
		}
	}
	if changed {
		_ = client.CloseConnections(ctx)
	}
}

// RememberedProxy returns the user selection independently of a kernel's cache.
func (m *Manager) RememberedProxy(group string) string {
	selections, _ := m.readSelections()
	return selections[group]
}
