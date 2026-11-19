//go:build (windows || linux || darwin) && !android

package trayui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type exitChoice int

const (
	cancelExit exitChoice = iota
	keepService
	uninstallService
	exitTitle        = "退出 ReiaClash"
	exitMessage      = "内核服务已安装。继续后台运行会保留代理和 WebUI 连接。注销服务并退出会停止代理并关闭客户端，可能需要系统授权。"
	keepServiceLabel = "继续后台运行"
	uninstallLabel   = "注销服务并退出"
	cancelExitLabel  = "取消"
)

type trayServiceStatus struct {
	Delegated bool `json:"core_service_delegated"`
	Service   struct {
		Installed bool `json:"installed"`
		Running   bool `json:"running"`
	} `json:"core_service_registration"`
}

func chooseExitEndpoint(ctx context.Context, status trayServiceStatus, choose func(context.Context) (exitChoice, error)) (string, error) {
	if status.Delegated || status.Service.Installed || status.Service.Running {
		choice, err := choose(ctx)
		if err != nil || choice == cancelExit {
			return "", err
		}
		switch choice {
		case keepService:
			return "/api/agent/exit", nil
		case uninstallService:
			return "/api/agent/exit/uninstall-service", nil
		default:
			return "", fmt.Errorf("无法识别退出选项")
		}
	}
	return "/api/agent/exit", nil
}

func requestDesktopExit(ctx context.Context) (bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/api/system/status", nil)
	if err != nil {
		return false, err
	}
	response, err := (&http.Client{Timeout: 6 * time.Second}).Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false, fmt.Errorf("读取内核服务状态失败，HTTP %d", response.StatusCode)
	}
	var status trayServiceStatus
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		return false, err
	}
	endpoint, err := chooseExitEndpoint(ctx, status, showExitDialog)
	if err != nil || endpoint == "" {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := postJSON(endpoint, nil); err != nil {
		return false, err
	}
	return true, nil
}
