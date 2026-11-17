package coreservice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"time"

	"reia-clash/internal/config"
	"reia-clash/internal/kernelbinary"
	"reia-clash/internal/mihomo"
	platformservice "reia-clash/internal/platform/coreservice"
	"reia-clash/internal/platform/privilege"
	"reia-clash/internal/platform/signals"
)

// Run starts the kernel supervisor and its authenticated loopback control
// endpoint inside an OS-managed process. The user agent remains responsible
// for the browser API and the logged-in desktop's system proxy.
func Run(dataDir string) error {
	ctx, cancel := signals.Context()
	defer cancel()
	return RunContext(ctx, dataDir)
}

func RunContext(ctx context.Context, dataDir string) error {
	listener, err := net.Listen("tcp4", "127.0.0.1:25501")
	if err != nil {
		return fmt.Errorf("内核服务已运行或 25501 端口已占用: %w", err)
	}
	defer listener.Close()
	paths, err := config.PathsAt(dataDir)
	if err != nil {
		return err
	}
	if err := privilege.RequireTunPrivilege(); err != nil {
		return fmt.Errorf("Mihomo 服务进程没有获得 TUN 权限: %w", err)
	}
	settings, err := config.OpenStore(paths.ConfigFile)
	if err != nil {
		return fmt.Errorf("读取 ReiaClash 设置失败: %w", err)
	}
	settings.SetRuntimeTunAuthorization(true)
	binary, err := kernelbinary.Resolve(settings.Snapshot(), paths.BinaryDir)
	if err != nil {
		return err
	}
	if _, err := settings.Update(func(s *config.Settings) error { s.ActiveBinary = binary; return nil }); err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(paths.Root, "core-service.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	logger := slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelInfo}))
	manager, err := mihomo.NewManager(paths.Root, filepath.Join(paths.Root, "mihomo.yaml"), binary, settings, logger)
	if err != nil {
		return err
	}
	token, err := mihomo.LoadServiceToken(paths.Root)
	if err != nil {
		return err
	}
	if err := manager.Start(ctx); err != nil {
		return err
	}
	// Status is set by our supervisor, unlike a controller probe which could
	// accidentally reach an orphan from a legacy service.
	deadline := time.NewTimer(40 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if running, _ := manager.Status(); running {
			break
		}
		select {
		case <-ctx.Done():
			return manager.Shutdown()
		case <-deadline.C:
			_, detail := manager.Status()
			return errors.Join(fmt.Errorf("内核服务启动失败: %s", detail), manager.Shutdown())
		case <-ticker.C:
		}
	}
	platformservice.NotifyReady(ctx)
	serveErr := mihomo.ServeServiceLifecycleWithListener(ctx, manager, token, listener)
	return errors.Join(serveErr, manager.Shutdown())
}
