// Command reia-agent 是 ReiaClash 本机 agent 的可执行入口。
//
// 入口只解析参数并组装 agent；HTTP、内核管理和平台适配分别放在独立包里。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"reia-clash/internal/agent"
	"reia-clash/internal/coreservice"
	"reia-clash/internal/httpapi"
	platformcoreservice "reia-clash/internal/platform/coreservice"
	"reia-clash/internal/platform/launch"
	"reia-clash/internal/platform/signals"
	"reia-clash/internal/version"
	"runtime"
	"time"
)

func main() {
	showVersion := flag.Bool("version", false, "显示 ReiaClash agent 版本")
	webUIPath := flag.String("webui", "test-webui.html", "本地测试 WebUI 文件路径")
	background := flag.Bool("background", false, "由系统启动项或托盘静默启动 agent")
	login := flag.Bool("login-autostart", false, "由用户登录启动项启动")
	noTray := flag.Bool("no-tray", false, "禁止自动拉起桌面托盘")
	coreService := flag.Bool("core-service", false, "仅运行 Mihomo 特权服务，不启动 WebUI API")
	installCoreService := flag.Bool("install-core-service", false, "由系统授权器安装 Mihomo 特权服务")
	uninstallCoreService := flag.Bool("uninstall-core-service", false, "由系统授权器移除 Mihomo 特权服务")
	dataDir := flag.String("data-dir", "", "core-service 使用的 ReiaClash 数据目录")
	serviceUID := flag.String("service-uid", "", "特权服务所属的登录用户编号")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ReiaClash agent %s\n", version.Agent)
		return
	}
	if *installCoreService {
		if err := platformcoreservice.InstallPrivileged(*dataDir, *serviceUID); err != nil {
			fmt.Fprintln(os.Stderr, "ReiaClash 特权服务安装失败:", err)
			os.Exit(1)
		}
		return
	}
	if *uninstallCoreService {
		if err := platformcoreservice.UninstallPrivileged(*dataDir, *serviceUID); err != nil {
			fmt.Fprintln(os.Stderr, "ReiaClash 特权服务移除失败:", err)
			os.Exit(1)
		}
		return
	}
	if *coreService {
		if *dataDir == "" {
			fmt.Fprintln(os.Stderr, "ReiaClash core-service 启动失败: --data-dir 不能为空")
			os.Exit(2)
		}
		if err := platformcoreservice.RunService(func(ctx context.Context) error { return coreservice.RunContext(ctx, *dataDir) }); err != nil {
			fmt.Fprintln(os.Stderr, "ReiaClash core-service 启动失败:", err)
			os.Exit(1)
		}
		return
	}

	if err := run(*webUIPath, *background, *noTray, *login); err != nil {
		fmt.Fprintln(os.Stderr, "ReiaClash agent 启动失败:", err)
		os.Exit(1)
	}
}

func run(webUIPath string, background, noTray, login bool) error {
	// Reserve the API port before Open writes configuration or starts a kernel.
	// Concurrent tray/agent launches must never create competing supervisors.
	listener, err := net.Listen("tcp4", "127.0.0.1:25500")
	if err != nil {
		response, callErr := (&http.Client{Timeout: 2 * time.Second}).Get("http://127.0.0.1:25500/api/health")
		if callErr == nil {
			defer response.Body.Close()
			var state struct {
				Status  string `json:"status"`
				Version string `json:"agent_version"`
			}
			if json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&state) == nil && response.StatusCode == http.StatusOK && state.Status == "ok" && state.Version != "" {
				if !background && !noTray {
					go ensureTray(slog.Default())
				}
				return nil
			}
		}
		return fmt.Errorf("ReiaClash 已运行或 API 端口 25500 被占用: %w", err)
	}
	defer listener.Close()
	logDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	logDir = filepath.Join(logDir, "ReiaClash")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(logDir, "agent.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	var logOutput io.Writer = logFile
	if !background {
		logOutput = io.MultiWriter(os.Stdout, logFile)
	}
	logger := slog.New(slog.NewTextHandler(logOutput, &slog.HandlerOptions{Level: slog.LevelInfo}))

	app, err := agent.Open(logger)
	if err != nil {
		return err
	}
	ctx, cancel := signals.Context()
	defer cancel()
	// Serve health/status before waiting for any kernel or optional OS helper.
	api := httpapi.New(app, logger, webUIPath)
	serverDone := make(chan error, 1)
	go func() { serverDone <- api.RunWithListener(ctx, listener) }()
	if err := app.Start(ctx); err != nil {
		cancel()
		app.Close()
		return err
	}

	if login {
		go app.StartLoginProxy(ctx)
	}

	// 当 agent 独立前台启动时，自动联动检测并拉起同目录的桌面托盘
	if !background && !noTray {
		go ensureTray(logger)
	}

	select {
	case <-ctx.Done():
	case <-app.Exit:
		cancel()
	case err := <-serverDone:
		cancel()
		app.Close()
		return err
	}
	app.Close()
	return nil
}

func ensureTray(logger *slog.Logger) {
	current, err := os.Executable()
	if err != nil {
		return
	}
	name := "reia-tray"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	trayPath := filepath.Join(filepath.Dir(current), name)
	if _, err := os.Stat(trayPath); err != nil {
		return
	}
	if launch.IsProcessRunning(name) {
		return
	}
	if err := launch.StartBackground(trayPath); err != nil {
		logger.Warn("自动拉起桌面托盘失败", "error", err)
	} else {
		logger.Info("已自动联动启动桌面托盘", "tray", trayPath)
	}
}
