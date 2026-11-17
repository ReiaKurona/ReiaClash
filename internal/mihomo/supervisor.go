package mihomo

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"reia-clash/internal/config"
	"reia-clash/internal/platform/launch"
)

func (m *Manager) supervise(ctx context.Context) {
	backoff := time.Second
	shortFailures := 0
	for ctx.Err() == nil {
		binary := m.BinaryPath()
		if binary == "" {
			m.setRunning(false, "mihomo 内核尚未安装")
			select {
			case <-m.restart:
				continue
			case <-ctx.Done():
				return
			}
		}
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
		if err := os.MkdirAll(m.kernelDataDir, 0700); err != nil {
			m.setRunning(false, fmt.Sprintf("无法创建 mihomo data 目录 %s: %v", m.kernelDataDir, err))
			select {
			case <-m.restart:
				continue
			case <-ctx.Done():
				return
			}
		}
		cmd := exec.CommandContext(ctx, binary, "-d", m.kernelDataDir, "-f", m.configPath)
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
				releaseChild, err = launch.TrackChildProcess(cmd)
				if err != nil {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
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
				readyCtx, cancelReady := context.WithTimeout(ctx, 15*time.Second)
				readyErr := m.WaitReady(readyCtx)
				cancelReady()
				if readyErr != nil {
					_ = cmd.Process.Kill()
					err = <-wait
					startedProcess = false
					err = fmt.Errorf("mihomo 外部控制器未就绪: %w", readyErr)
				} else {
					m.setRunning(true, "")
					select {
					case err = <-wait:
					case <-m.restart:
						requestedRestart = true
						_ = cmd.Process.Kill()
						err = <-wait
					case <-ctx.Done():
						_ = cmd.Process.Kill()
						<-wait
						m.setRunning(false, "")
						releaseChild()
						return
					}
				}
				releaseChild()
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
		if shortFailures >= 3 && m.rollbackPendingBinary() {
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

func (m *Manager) rollbackPendingBinary() bool {
	settings := m.settings.Snapshot()
	if settings.ActiveBinary == "" || settings.PreviousBinary == "" || settings.ActiveBinary == settings.PreviousBinary {
		return false
	}
	previous := settings.PreviousBinary
	if _, err := m.settings.Update(func(current *config.Settings) error {
		current.ActiveBinary = previous
		current.PreviousBinary = ""
		return nil
	}); err != nil {
		m.logger.Error("写入内核回滚状态失败", "error", err)
		m.emit("error", "保存 mihomo 回滚状态失败："+err.Error())
		return false
	}
	m.mu.Lock()
	m.binary = previous
	m.mu.Unlock()
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
	m.mu.Unlock()
}
