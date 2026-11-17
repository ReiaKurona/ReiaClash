package mihomo

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

func (c *Client) StreamLogs(ctx context.Context, receive func(string)) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/logs?level=debug", nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.Secret)
	client := &http.Client{Transport: c.HTTP.Transport}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("DNS 日志订阅返回 HTTP %d", response.StatusCode)
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		var entry struct {
			Payload string `json:"payload"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) == nil {
			receive(entry.Payload)
		}
	}
	return scanner.Err()
}
