//go:build (windows || linux || darwin) && !android

package trayui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gogpu/systray"
	branding "reia-clash/ico"
	"reia-clash/internal/platform/instance"
	"reia-clash/internal/platform/launch"
	"reia-clash/internal/platform/notification"
)

const (
	apiBase = "http://127.0.0.1:25500"
)

type trayRealStatus struct {
	CoreKind               string  `json:"core_kind"`
	CoreMemoryBytes        *uint64 `json:"core_memory_bytes"`
	IsActuallyIntercepting bool    `json:"is_actually_intercepting"`
	RealSystemProxy        bool    `json:"real_system_proxy"`
	RealTun                bool    `json:"real_tun"`
	MihomoRunning          bool    `json:"mihomo_running"`
	Settings               struct {
		Enabled        bool   `json:"enabled"`
		UseSystemProxy bool   `json:"use_system_proxy"`
		UseTun         bool   `json:"use_tun"`
		TunStack       string `json:"tun_stack"`
		OutboundMode   string `json:"outbound_mode"`
	} `json:"settings"`
}

type eventResponse struct {
	Events []struct {
		ID      uint64 `json:"id"`
		Level   string `json:"level"`
		Message string `json:"message"`
	} `json:"events"`
}

func Run(ctx context.Context, login ...bool) error {
	lock, err := instance.Acquire("tray")
	if errors.Is(err, instance.ErrAlreadyRunning) {
		return nil
	}
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := branding.InstallDesktopEntries(); err != nil {
		slog.Warn("注册桌面图标失败", "error", err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := notification.RegisterApp(); err != nil {
		slog.Warn("注册通知应用标识失败", "error", err)
	}
	tray := systray.New()
	var registered atomic.Pointer[notification.Notifier]
	notify := func(message string) {
		if notifier := registered.Load(); notifier != nil {
			if err := notifier.Show(branding.AppName, message); err != nil {
				slog.Warn("发送系统通知失败", "error", err)
			}
		}
	}
	openPanel := func() {
		go func() {
			if err := openControlPanel(); err != nil {
				slog.Error("打开面板失败", "error", err)
				notify("无法打开面板，请稍后重试。")
			}
		}()
	}
	tray.SetAppName(branding.AppName).OnClick(openPanel)
	menu := systray.NewMenu()

	// 1. 打开控制面板 (WebUI)
	menu.Add("打开控制中心", func() {
		openPanel()
	})
	coreItem := menu.Add("当前运行内核 — · 内存占用 —", nil)
	coreItem.SetDisabled(true)
	menu.AddSeparator()

	// 2. 首页总开关联动：启动/暂停代理
	var masterItem *systray.MenuItem
	masterItem = menu.AddCheckbox("启动代理", false, func() {
		toggleMaster(notify, masterItem)
	})
	menu.AddSeparator()

	// 3. 代理模式子菜单
	modeMenu := systray.NewMenu()
	rule := modeMenu.AddCheckbox("规则模式", false, func() { setMode(notify, "rule") })
	global := modeMenu.AddCheckbox("全局模式", false, func() { setMode(notify, "global") })
	direct := modeMenu.AddCheckbox("直连模式", false, func() { setMode(notify, "direct") })
	menu.AddSubmenu("代理模式", modeMenu)
	menu.AddSeparator()

	// 4. 重启内核
	menu.Add("重启内核", func() {
		if err := postJSON("/api/agent/restart", nil); err != nil {
			slog.Error("内核重启失败", "error", err)
			notify("重启失败，请在面板查看详情。")
		} else {
			notify("正在重启…")
		}
	})
	menu.AddSeparator()

	// 5. 关闭 ReiaClash
	menu.Add("退出客户端", func() {
		if !desktopAgentGate.beginExit() {
			return
		}
		go func() {
			exited, err := requestDesktopExit(runCtx)
			if !exited {
				desktopAgentGate.cancelExit()
				if err != nil && runCtx.Err() == nil {
					slog.Error("退出客户端失败", "error", err)
					notify("退出未完成：" + err.Error())
				}
				return
			}
			cancel()
		}()
	})

	tray.SetIcon(branding.TrayPNG).SetTooltip("ReiaClash [初始化中...]").SetMenu(menu).Show()
	notifier, notificationErr := notification.New(func(title, message string) { tray.ShowNotification(title, message) }, openPanel)
	if notificationErr != nil {
		slog.Warn("通知点击初始化失败", "error", notificationErr)
	}
	registered.Store(notifier)
	defer func() {
		registered.Store(nil)
		notifier.Close()
	}()
	notify("正在启动…")
	go func() {
		pairedInitially := false
		if err := ensureAgent(runCtx, len(login) > 0 && login[0]); err != nil {
			if !errors.Is(err, context.Canceled) {
				slog.Error("Agent 启动失败", "error", err)
				notify("启动失败，请重新打开客户端。")
			}
		} else {
			message := "已启动，点击打开面板。"
			if state, err := fetchPanelStatus(); err == nil && state.Paired {
				message = "面板已连接，点击打开。"
				pairedInitially = true
			}
			notify(message)
		}
		go watchPanel(runCtx, notify, pairedInitially)
		go watchAgent(runCtx, notify)
		go syncStatus(runCtx, tray, coreItem, masterItem, rule, global, direct)
		go watchEvents(runCtx, notify)
	}()

	go func() {
		<-runCtx.Done()
		if desktopAgentGate.beginExit() {
			if err := postJSON("/api/agent/exit", nil); err != nil {
				slog.Warn("托盘结束时退出 Agent 失败", "error", err)
			}
		}
		tray.Remove()
	}()
	return tray.Run()
}

func fetchRealStatus() (*trayRealStatus, error) {
	return fetchStatus("/api/agent/real-status")
}

func fetchStatus(path string) (*trayRealStatus, error) {
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get(apiBase + path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var status trayRealStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}
	return &status, nil
}

func toggleMaster(notify func(string), item *systray.MenuItem) {
	status, err := fetchRealStatus()
	if err != nil {
		notify("暂时无法确认代理状态，请稍后重试。")
		return
	}
	target := !status.IsActuallyIntercepting
	if err := postJSON("/api/agent/master", map[string]bool{"enable": target}); err != nil {
		slog.Error("切换代理失败", "error", err)
		notify("切换失败，请在面板查看详情。")
		return
	}
	item.SetChecked(target)
	if target {
		item.SetLabel("代理运行中 (点击暂停)")
		notify("代理已开启。")
	} else {
		item.SetLabel("启动代理")
		notify("代理已关闭。")
	}
}

func syncStatus(ctx context.Context, tray *systray.SystemTray, coreItem, masterItem, rule, global, direct *systray.MenuItem) {
	lastCoreLabel := ""
	for {
		status, err := fetchStatus("/api/agent/real-status?metrics=1")
		coreLabel := runningCoreLabel(status)
		if coreLabel != lastCoreLabel {
			coreItem.SetLabel(coreLabel)
			lastCoreLabel = coreLabel
		}
		if err == nil {
			masterItem.SetChecked(status.IsActuallyIntercepting)
			if status.IsActuallyIntercepting {
				masterItem.SetLabel("代理运行中 (点击暂停)")
				var modes []string
				if status.RealSystemProxy {
					modes = append(modes, "系统代理")
				}
				if status.RealTun {
					modes = append(modes, "TUN")
				}
				detail := strings.Join(modes, "+")
				if detail == "" {
					detail = "接管中"
				}
				tray.SetTooltip(fmt.Sprintf("ReiaClash: 代理运行中 [%s | %s]", modeName(status.Settings.OutboundMode), detail))
			} else {
				masterItem.SetLabel("启动代理")
				tray.SetTooltip("ReiaClash: 代理已暂停 (直连)")
			}

			rule.SetChecked(status.Settings.OutboundMode == "rule")
			global.SetChecked(status.Settings.OutboundMode == "global")
			direct.SetChecked(status.Settings.OutboundMode == "direct")
		} else {
			masterItem.SetChecked(false)
			masterItem.SetLabel("Agent 未连接")
			tray.SetTooltip("ReiaClash [Agent 未连接]")
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(1500 * time.Millisecond):
		}
	}
}

func runningCoreLabel(status *trayRealStatus) string {
	core, memory := "—", "—"
	if status != nil && status.MihomoRunning && status.CoreKind != "" {
		core = status.CoreKind
		if status.CoreMemoryBytes != nil {
			memory = fmt.Sprintf("%.1f MiB", float64(*status.CoreMemoryBytes)/(1<<20))
		}
	}
	return fmt.Sprintf("当前运行内核 %s · 内存占用 %s", core, memory)
}

func watchAgent(ctx context.Context, notify func(string)) {
	lastFailure := ""
	for {
		if ctx.Err() != nil {
			return
		}
		if desktopAgentGate.isExiting() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}
		if err := healthCheck(); err != nil {
			if retryErr := ensureAgent(ctx); retryErr != nil {
				if errors.Is(retryErr, context.Canceled) {
					continue
				}
				message := retryErr.Error()
				if message != lastFailure {
					slog.Error("客户端自动恢复失败", "error", retryErr)
					notify("连接异常，请重新启动客户端。")
					lastFailure = message
				}
			} else {
				lastFailure = ""
				notify("客户端已恢复。")
			}
		} else {
			lastFailure = ""
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func ensureAgent(ctx context.Context, login ...bool) error {
	if ctx.Err() != nil || desktopAgentGate.isExiting() {
		return context.Canceled
	}
	if healthCheck() == nil {
		return nil
	}
	current, err := os.Executable()
	if err != nil {
		return err
	}
	name := "reia-agent"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	agentPath := filepath.Join(filepath.Dir(current), name)
	if _, err := os.Stat(agentPath); err != nil {
		return fmt.Errorf("找不到同目录的 %s", name)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	logDirectory := filepath.Join(base, "ReiaClash")
	if err := os.MkdirAll(logDirectory, 0700); err != nil {
		return err
	}
	startupLog := filepath.Join(logDirectory, "agent-startup.log")
	var offset int64
	if info, err := os.Stat(startupLog); err == nil {
		offset = info.Size()
	}
	if err := desktopAgentGate.launch(func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		args := []string{"--background"}
		if len(login) > 0 && login[0] {
			args = append(args, "--login-autostart")
		}
		return launch.StartBackgroundLogged(agentPath, startupLog, args...)
	}); err != nil {
		return fmt.Errorf("启动 agent 失败：%w", err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil || desktopAgentGate.isExiting() {
			return context.Canceled
		}
		if healthCheck() == nil {
			return nil
		}
		if message := startupFailure(startupLog, offset); message != "" {
			return fmt.Errorf("agent 启动失败：%s", message)
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("agent 启动后没有在 15 秒内响应；请检查 %s", startupLog)
}

func startupFailure(path string, offset int64) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	if _, err := file.Seek(offset, 0); err != nil {
		return ""
	}
	data, _ := io.ReadAll(io.LimitReader(file, 4096))
	return strings.TrimSpace(string(data))
}

func healthCheck() error {
	client := &http.Client{Timeout: 1 * time.Second}
	response, err := client.Get(apiBase + "/api/health")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("agent 返回 HTTP %d", response.StatusCode)
	}
	return nil
}

func postJSON(path string, value any) error {
	var body *bytes.Reader
	if value == nil {
		body = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(http.MethodPost, apiBase+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	timeout := 5 * time.Minute
	if path == "/api/agent/exit" {
		timeout = 10 * time.Second
	}
	response, err := (&http.Client{Timeout: timeout}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(response.Body).Decode(&result)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if result.Message != "" {
			return fmt.Errorf("%s", result.Message)
		}
		return fmt.Errorf("agent 返回 HTTP %d", response.StatusCode)
	}
	return nil
}

func setMode(notify func(string), mode string) {
	if err := postJSON("/api/agent/settings", map[string]string{"outbound_mode": mode}); err != nil {
		slog.Error("切换代理模式失败", "error", err)
		notify("切换失败，请在面板查看详情。")
		return
	}
	notify("已切换到" + modeName(mode) + "模式")
}

func watchEvents(ctx context.Context, notify func(string)) {
	client := &http.Client{Timeout: 3 * time.Second}
	var lastID uint64
	initialized := false
	var lastErrorNotice time.Time
	for {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/api/agent/events?since="+strconv.FormatUint(lastID, 10), nil)
		response, err := client.Do(request)
		if err == nil {
			var data eventResponse
			if response.StatusCode == http.StatusOK && json.NewDecoder(response.Body).Decode(&data) == nil {
				if !initialized {
					if count := len(data.Events); count > 0 {
						lastID = data.Events[count-1].ID
					}
					initialized = true
				} else {
					for _, event := range data.Events {
						lastID = event.ID
						if event.Level == "error" {
							slog.Error("客户端事件", "message", event.Message)
							if time.Since(lastErrorNotice) > 30*time.Second {
								notify("运行异常，请打开面板查看。")
								lastErrorNotice = time.Now()
							}
						}
					}
				}
			}
			_ = response.Body.Close()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func modeName(mode string) string {
	switch strings.ToLower(mode) {
	case "global":
		return "全局"
	case "direct":
		return "直连"
	default:
		return "规则"
	}
}
