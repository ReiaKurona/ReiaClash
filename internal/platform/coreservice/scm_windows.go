//go:build windows

package coreservice

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
	"os"
	"path/filepath"
	"reia-clash/internal/platform/launch"
	"reia-clash/internal/platform/network"
	"reia-clash/internal/platform/privilege"
	"strings"
	"time"
)

const serviceName = "ReiaClashCore"

func RegistrationStatus() Registration {
	result := Registration{Backend: "windows-scm", Name: serviceName}
	handle, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer windows.CloseServiceHandle(handle)
	name, _ := windows.UTF16PtrFromString(serviceName)
	serviceHandle, err := windows.OpenService(handle, name, windows.SERVICE_QUERY_STATUS)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return result
	}
	if err != nil {
		result.Installed = errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SERVICE_MARKED_FOR_DELETE)
		result.Error = err.Error()
		return result
	}
	defer windows.CloseServiceHandle(serviceHandle)
	result.Installed = true
	if configurationHandle, err := windows.OpenService(handle, name, windows.SERVICE_QUERY_CONFIG); err == nil {
		defer windows.CloseServiceHandle(configurationHandle)
		service := &mgr.Service{Name: serviceName, Handle: configurationHandle}
		if configuration, err := service.Config(); err == nil {
			if args, err := windows.DecomposeCommandLine(configuration.BinaryPathName); err == nil && len(args) > 0 {
				result.Executable = args[0]
				result.LegacyHelper = !strings.EqualFold(filepath.Base(args[0]), "reia-service.exe")
			}
		}
	}
	status, err := (&mgr.Service{Name: serviceName, Handle: serviceHandle}).Query()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Running = status.State == svc.Running
	return result
}

func InstallPrivileged(dataDir, requestedUID string) (resultErr error) {
	if err := privilege.RequireTunPrivilege(); err != nil {
		return err
	}
	executable, err := Executable()
	if err != nil {
		return err
	}
	executable, err = launch.ResolveExecutable(executable)
	if err != nil {
		return err
	}
	dataDir, err = filepath.Abs(dataDir)
	if err != nil {
		return err
	}
	// Migrate the old task backend before starting the real SCM service.
	if launch.Command("schtasks.exe", "/query", "/tn", taskName).Run() == nil {
		if err := uninstallLegacyTask(""); err != nil {
			return fmt.Errorf("移除旧计划任务失败: %w", err)
		}
	}
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	configuration := mgr.Config{DisplayName: "ReiaClash Core", Description: "ReiaClash privileged Mihomo supervisor", StartType: mgr.StartAutomatic, ServiceType: windows.SERVICE_WIN32_OWN_PROCESS, ServiceStartName: "LocalSystem"}
	service, err := manager.OpenService(serviceName)
	if err == nil {
		defer service.Close()
		if err := quiesceService(service); err != nil {
			return err
		}
		if err := cleanupManagedProcesses(dataDir); err != nil {
			return err
		}
		configuration.BinaryPathName = windows.ComposeCommandLine([]string{executable, "--core-service", "--data-dir", dataDir})
		if err := service.UpdateConfig(configuration); err != nil {
			return err
		}
	} else if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		if err := cleanupManagedProcesses(dataDir); err != nil {
			return err
		}
		service, err = manager.CreateService(serviceName, executable, configuration, "--core-service", "--data-dir", dataDir)
		if err != nil {
			return err
		}
		defer service.Close()
	} else {
		return err
	}
	defer func() {
		if resultErr != nil {
			// A failed registration must not leave SCM retrying a broken
			// supervisor against the same occupied ports forever.
			resultErr = errors.Join(resultErr, quiesceService(service))
		}
	}()
	if err := service.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 3 * time.Second}}, 86400); err != nil {
		return err
	}
	if err := service.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
		return err
	}
	if err := service.Start(); err != nil {
		return fmt.Errorf("启动 Windows 服务失败: %w", err)
	}
	deadline := time.Now().Add(50 * time.Second)
	for time.Now().Before(deadline) {
		status, err := service.Query()
		if err != nil {
			return err
		}
		if status.State == svc.Running {
			return nil
		}
		if status.State == svc.Stopped {
			if detail, err := os.ReadFile(filepath.Join(dataDir, "core-service-startup.log")); err == nil && len(detail) > 0 {
				return fmt.Errorf("Windows 服务启动失败: %s", strings.TrimSpace(string(detail)))
			}
			return fmt.Errorf("Windows 服务已停止，退出码 %d，请检查 core-service.log", status.Win32ExitCode)
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("等待 Windows 服务启动超时")
}

func quiesceService(service *mgr.Service) error {
	configuration, err := service.Config()
	if err != nil {
		return err
	}
	if configuration.ServiceType != windows.SERVICE_WIN32_OWN_PROCESS {
		return fmt.Errorf("拒绝终止共享宿主服务")
	}
	configuration.StartType = mgr.StartDisabled
	if err := service.UpdateConfig(configuration); err != nil {
		return fmt.Errorf("禁用旧服务自动启动失败: %w", err)
	}
	if err := service.ResetRecoveryActions(); err != nil {
		return fmt.Errorf("停用旧服务失败恢复失败: %w", err)
	}
	if err := service.SetRecoveryActionsOnNonCrashFailures(false); err != nil {
		return fmt.Errorf("停用旧服务错误重启失败: %w", err)
	}
	if err := stopService(service); err != nil {
		return fmt.Errorf("停止旧服务失败: %w", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status, err := service.Query()
		if err != nil {
			return err
		}
		if status.State == svc.Stopped {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("旧服务仍未报告停止，已中止切换")
}

func stopService(service *mgr.Service) error {
	status, err := service.Query()
	if err != nil {
		return err
	}
	if status.State == svc.Stopped {
		return nil
	}
	var process windows.Handle
	if status.ProcessId != 0 {
		process, err = windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, status.ProcessId)
		if err != nil {
			return fmt.Errorf("无法跟踪服务进程退出: %w", err)
		}
		defer windows.CloseHandle(process)
	}
	// SCM rejects STOP during START_PENDING. Recovery is already disabled;
	// stop this exact own-process service through its stable native handle.
	if status.State == svc.StartPending && process != 0 {
		return terminateStuckService(process)
	}
	if status.State != svc.StopPending {
		if _, err = service.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) && !errors.Is(err, windows.ERROR_SERVICE_CANNOT_ACCEPT_CTRL) && !errors.Is(err, windows.ERROR_INVALID_SERVICE_CONTROL) {
			return err
		}
	}
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		status, err = service.Query()
		if err != nil {
			return err
		}
		if status.State == svc.Stopped {
			if process != 0 {
				result, err := windows.WaitForSingleObject(process, 10000)
				if err != nil {
					return err
				}
				if result != windows.WAIT_OBJECT_0 {
					return terminateStuckService(process)
				}
			}
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	if process != 0 {
		return terminateStuckService(process)
	}
	return fmt.Errorf("等待 Windows 服务停止超时，未能确认服务进程身份")
}

// This handle belongs to the own-process SCM service, opened before stopping;
// it cannot target a recycled PID. Recovery was disabled by quiesceService.
func terminateStuckService(process windows.Handle) error {
	if err := windows.TerminateProcess(process, 0); err != nil {
		return err
	}
	result, err := windows.WaitForSingleObject(process, 10000)
	if err != nil {
		return err
	}
	if result != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("旧服务进程未退出，已中止切换")
	}
	return nil
}

func UninstallPrivileged(dataDir, requestedUID string) error {
	if err := privilege.RequireTunPrivilege(); err != nil {
		return err
	}
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(serviceName)
	if err == nil {
		defer service.Close()
		if err := quiesceService(service); err != nil {
			return err
		}
		if err := cleanupManagedProcesses(dataDir); err != nil {
			return err
		}
		if err := service.Delete(); err != nil {
			return err
		}
	} else if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return err
	}
	if launch.Command("schtasks.exe", "/query", "/tn", taskName).Run() == nil {
		if err := uninstallLegacyTask(""); err != nil {
			return err
		}
	}
	if err := cleanupManagedProcesses(dataDir); err != nil {
		return err
	}
	return network.RestoreMeowDNS(dataDir)
}

type serviceHandler struct{ run func(context.Context) error }

func (h *serviceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, statuses chan<- svc.Status) (bool, uint32) {
	statuses <- svc.Status{State: svc.StartPending, WaitHint: 50000}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{}, 1)
	ctx = context.WithValue(ctx, readinessKey{}, ready)
	done := make(chan error, 1)
	go func() { done <- h.run(ctx) }()
	startupTimeout := time.NewTimer(50 * time.Second)
	defer startupTimeout.Stop()
startup:
	for {
		select {
		case <-ready:
			break startup
		case err := <-done:
			if err != nil {
				return true, 1
			}
			return false, 0
		case <-startupTimeout.C:
			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
			}
			return true, 3
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				statuses <- svc.Status{State: svc.StartPending, WaitHint: 50000}
			case svc.Stop, svc.Shutdown:
				statuses <- svc.Status{State: svc.StopPending, WaitHint: 20000}
				cancel()
				select {
				case <-done:
					return false, 0
				case <-time.After(20 * time.Second):
					return true, 2
				}
			}
		}
	}
	status := svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	statuses <- status
	for {
		select {
		case err := <-done:
			if err != nil {
				return true, 1
			}
			return false, 0
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				statuses <- status
			case svc.Stop, svc.Shutdown:
				statuses <- svc.Status{State: svc.StopPending, WaitHint: 20000}
				cancel()
				select {
				case err := <-done:
					if err != nil {
						return true, 1
					}
					return false, 0
				case <-time.After(20 * time.Second):
					return true, 2
				}
			}
		}
	}
}

func RunService(run func(context.Context) error) error {
	service, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if service {
		return svc.Run(serviceName, &serviceHandler{run: run})
	}
	return fmt.Errorf("内核服务必须由 Windows 服务管理器启动，请在客户端设置中启用服务模式")
}
