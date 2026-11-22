package mihomo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"reia-clash/internal/config"
	"reia-clash/internal/kernelbinary"
	"reia-clash/internal/platform/launch"
	"reia-clash/internal/platform/network"
)

func StartupTimeout(binary string) time.Duration {
	if kernelbinary.IsMeow(binary) {
		return 40 * time.Second
	}
	return 15 * time.Second
}

func (m *Manager) supervise(ctx context.Context) {
	backoff := time.Second
	shortFailures := 0
	for ctx.Err() == nil {
		if m.strictChildJob {
			if err := m.settings.Reload(); err != nil {
				m.logger.Warn("重新读取内核设置失败", "error", err)
			}
		}
		binary := m.BinaryPath()
		proxyAddress := "127.0.0.1:7890"
		if m.settings.Snapshot().AllowLAN {
			proxyAddress = "0.0.0.0:7890"
		}
		if err := checkPorts("127.0.0.1:9090", proxyAddress); err != nil {
			_, previousError := m.Status()
			message := err.Error()
			m.setRunning(false, message)
			if previousError != message {
				m.logger.Error("mihomo 启动受阻", "binary", binary, "error", err)
				m.emit("error", message)
			}
			select {
			case <-time.After(2 * time.Second):
				continue
			case <-m.restart:
				continue
			case <-ctx.Done():
				return
			}
		}
		if err := network.RestoreMeowDNS(m.dataDir); err != nil {
			m.setRunning(false, fmt.Sprintf("恢复系统 DNS 后才能启动内核: %v", err))
			select {
			case <-time.After(2 * time.Second):
				continue
			case <-ctx.Done():
				return
			}
		}
		if binary == "" {
			m.setRunning(false, "mihomo 内核尚未安装")
			select {
			case <-m.restart:
				continue
			case <-ctx.Done():
				return
			}
		}
		if err := os.MkdirAll(m.kernelDataDir, 0700); err != nil {
			m.setRunning(false, fmt.Sprintf("无法创建 mihomo data 目录 %s: %v", m.kernelDataDir, err))
			select {
			case <-m.restart:
				continue
			case <-ctx.Done():
				return
			}
		}
		if err := m.persistRuntimeForBinary(binary); err != nil {
			m.setRunning(false, fmt.Sprintf("生成内核运行配置失败: %v", err))
			select {
			case <-m.restart:
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
				return
			}
			continue
		}
		if err := m.captureManagedDNS(binary); err != nil {
			m.setRunning(false, fmt.Sprintf("无法备份 meow-rs 接管前的系统 DNS: %v", err))
			select {
			case <-time.After(2 * time.Second):
				continue
			case <-ctx.Done():
				return
			}
		}
		args := []string{"-d", m.kernelDataDir, "-f", m.runtimePath(binary)}
		cmd := exec.CommandContext(ctx, binary, args...)
		if kernelbinary.IsMeow(binary) {
			// Keep meow alive long enough for an explicit TUN cleanup request.
			cmd = exec.Command(binary, args...)
		}
		cmd.Dir = m.kernelDataDir
		m.mu.Lock()
		m.lastOutput = ""
		m.mu.Unlock()
		launch.HideCommand(cmd)
		stdout, err := cmd.StdoutPipe()
		requestedRestart := false
		startedProcess := false
		if err == nil {
			var stderr io.ReadCloser
			stderr, err = cmd.StderrPipe()
			if err == nil {
				err = cmd.Start()
			}
			var releaseChild func()
			if err == nil {
				track := launch.TrackDesktopChildProcess
				if m.strictChildJob {
					track = launch.TrackChildProcess
				}
				releaseChild, err = track(cmd)
				if err != nil {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
					m.restoreManagedDNS()
				}
			}
			if err == nil {
				startedProcess = true
				startedAt := time.Now()
				m.logger.Info("mihomo 内核已启动", "pid", cmd.Process.Pid, "binary", binary, "data", m.kernelDataDir)
				go m.copyLogs(stdout, "stdout")
				go m.copyLogs(stderr, "stderr")
				wait := make(chan error, 1)
				go func() { wait <- cmd.Wait() }()
				readyCtx, cancelReady := context.WithTimeout(ctx, StartupTimeout(binary))
				ready := make(chan error, 1)
				go func() { ready <- m.WaitReady(readyCtx) }()
				var readyErr error
				select {
				case readyErr = <-ready:
					if readyErr != nil {
						_ = m.stopKernelProcess(binary, cmd, wait)
					}
				case exitErr := <-wait:
					readyErr = fmt.Errorf("内核提前退出: %v", exitErr)
				}
				cancelReady()
				if readyErr != nil {
					startedProcess = false
					err = fmt.Errorf("mihomo 外部控制器未就绪: %w", readyErr)
				} else {
					m.observeManagedDNS(binary)
					m.profileMu.Lock()
					m.restoreSelections(ctx)
					m.profileMu.Unlock()
					m.mu.Lock()
					m.runningBinary, m.runningPID, m.running, m.lastError = binary, cmd.Process.Pid, true, ""
					m.mu.Unlock()
					select {
					case err = <-wait:
					case <-m.restart:
						requestedRestart = true
						err = m.stopKernelProcess(binary, cmd, wait)
					case <-ctx.Done():
						_ = m.stopKernelProcess(binary, cmd, wait)
						m.setRunning(false, "")
						releaseChild()
						m.restoreManagedDNS()
						return
					}
				}
				releaseChild()
				m.restoreManagedDNS()
				if requestedRestart {
					m.setRunning(false, "")
					backoff, shortFailures = time.Second, 0
					continue
				}
				if readyErr == nil {
					if time.Since(startedAt) > 30*time.Second {
						shortFailures = 0
					} else {
						shortFailures++
					}
				}
			}
		}
		if !startedProcess {
			shortFailures++
		}
		m.mu.RLock()
		output := m.lastOutput
		m.mu.RUnlock()
		message := fmt.Sprintf("mihomo 启动/运行失败 [%s]: %v", binary, err)
		if output != "" {
			message += "；内核输出：" + output
		}
		m.setRunning(false, message)
		if ctx.Err() != nil {
			return
		}
		if (shortFailures >= 3 || (kernelbinary.IsMeow(binary) && !startedProcess)) && m.rollbackPendingBinary(binary) {
			shortFailures = 0
			backoff = time.Second
			continue
		}
		m.logger.Error("mihomo 进程已退出，将自动重启", "error", err, "delay", backoff)
		m.emit("error", message+"；agent 将自动重试")
		select {
		case <-time.After(backoff):
		case <-m.restart:
		case <-ctx.Done():
			return
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (m *Manager) captureManagedDNS(binary string) error {
	settings := m.settings.Snapshot()
	if !kernelbinary.IsMeow(binary) || !settings.Enabled || !settings.UseTun || !settings.TunAuthorized {
		return nil
	}
	profile, err := os.ReadFile(filepath.Join(m.dataDir, "profile.yaml"))
	if err != nil {
		return err
	}
	options, err := MeowTunOptions(profile)
	if err != nil || len(options.DNSHijack) == 0 {
		return err
	}
	return network.SaveMeowDNS(m.dataDir)
}

func (m *Manager) restoreManagedDNS() {
	if err := network.RestoreMeowDNS(m.dataDir); err != nil {
		m.logger.Warn("恢复系统 DNS 失败", "error", err)
		m.emit("error", "系统 DNS 恢复失败，已保留恢复记录")
	}
}

func (m *Manager) stopKernelProcess(binary string, cmd *exec.Cmd, wait <-chan error) error {
	m.observeManagedDNS(binary)
	m.prepareKernelStop(binary)
	if kernelbinary.IsMeow(binary) && cmd.Process.Signal(syscall.SIGTERM) == nil {
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case err := <-wait:
			return err
		case <-timer.C:
		}
	}
	_ = cmd.Process.Kill()
	return <-wait
}

func (m *Manager) observeManagedDNS(binary string) {
	if kernelbinary.IsMeow(binary) {
		if err := network.ObserveMeowDNS(m.dataDir); err != nil {
			m.logger.Warn("确认 meow-rs DNS 接管记录失败", "error", err)
		}
	}
}

func (m *Manager) prepareKernelStop(binary string) {
	if !kernelbinary.IsMeow(binary) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _, err := m.Client().Do(ctx, http.MethodPatch, "/configs", map[string]any{"tun": map[string]bool{"enable": false}})
	if err != nil {
		m.logger.Warn("meow-rs 停止前关闭 TUN 失败", "error", err)
	}
}

func (m *Manager) rollbackPendingBinary(failed string) bool {
	// The service may still have the pending snapshot after the Agent committed it.
	if m.settings.Reload() != nil {
		return false
	}
	settings := m.settings.Snapshot()
	if settings.ActiveBinary != failed {
		return false
	}
	if settings.ActiveBinary == "" || settings.PreviousBinary == "" || settings.ActiveBinary == settings.PreviousBinary {
		return false
	}
	previous := settings.PreviousBinary
	if _, err := m.settings.Update(func(current *config.Settings) error {
		current.ActiveBinary = previous
		current.PreviousBinary = ""
		if !kernelbinary.IsMeow(previous) {
			current.MihomoFallback = ""
		}
		return nil
	}); err != nil {
		m.logger.Error("写入内核回滚状态失败", "error", err)
		m.emit("error", "保存 mihomo 回滚状态失败："+err.Error())
		return false
	}
	m.mu.Lock()
	m.binary = previous
	m.mu.Unlock()
	if err := m.PersistCurrentRuntimeConfig(); err != nil {
		m.logger.Warn("恢复旧内核运行配置失败", "error", err)
	}
	m.logger.Error("新内核连续启动失败，已回滚到上一版本", "binary", previous)
	m.emit("error", "新内核连续启动失败，已回滚到上一版本")
	return true
}

func (m *Manager) copyLogs(reader io.ReadCloser, stream string) {
	defer reader.Close()
	buf := make([]byte, 16*1024)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			m.mu.Lock()
			m.lastOutput += string(buf[:n])
			if len(m.lastOutput) > 3000 {
				m.lastOutput = m.lastOutput[len(m.lastOutput)-3000:]
			}
			m.mu.Unlock()
			m.logger.Info("mihomo", "stream", stream, "line", strings.TrimSpace(string(buf[:n])))
		}
		if err != nil {
			return
		}
	}
}

func (m *Manager) setRunning(running bool, lastError string) {
	m.mu.Lock()
	m.running, m.lastError = running, lastError
	if !running {
		m.runningPID = 0
	}
	m.mu.Unlock()
}
