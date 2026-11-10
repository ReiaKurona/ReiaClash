use std::path::PathBuf;
use std::process::Stdio;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::{Child, Command};
use tokio::sync::Mutex;
use crate::state::{CoreStatus, SharedState};
use crate::core::yaml_patcher::YamlPatcher;
use crate::system::Notifier;

#[cfg(windows)]
const CREATE_NO_WINDOW: u32 = 0x08000000;

pub struct CoreManager {
    child_process: Arc<Mutex<Option<Child>>>,
    pub state: SharedState,
    pub core_binary_path: PathBuf,
    pub work_dir: PathBuf,
    pub config_path: PathBuf,
    /// 标记是否为用户主动关闭（主动关闭时不触发崩溃守护）
    pub is_intentional_stop: Arc<AtomicBool>,
}

impl CoreManager {
    pub fn new(
        state: SharedState,
        core_binary_path: PathBuf,
        work_dir: PathBuf,
        config_path: PathBuf,
    ) -> Self {
        Self {
            child_process: Arc::new(Mutex::new(None)),
            state,
            core_binary_path,
            work_dir,
            config_path,
            is_intentional_stop: Arc::new(AtomicBool::new(false)),
        }
    }

    pub async fn start(&self) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        let mut child_lock = self.child_process.lock().await;

        if child_lock.is_some() {
            self.stop_internal(&mut child_lock).await?;
        }

        self.is_intentional_stop.store(false, Ordering::SeqCst);

        if !self.core_binary_path.exists() {
            let err_msg = format!("未找到 Mihomo 二进制文件: {:?}", self.core_binary_path);
            let mut s = self.state.write().await;
            s.last_error = Some(err_msg.clone());
            return Err(err_msg.into());
        }

        let (api_port, secret) = {
            let s = self.state.read().await;
            (s.mihomo_api_port, s.mihomo_secret.clone())
        };
        YamlPatcher::patch_and_save(&self.config_path, api_port, &secret)?;

        tracing::info!("🚀 正在启动 Mihomo 内核进程...");

        let mut cmd = Command::new(&self.core_binary_path);
        cmd.arg("-d")
            .arg(&self.work_dir)
            .arg("-f")
            .arg(&self.config_path)
            .stderr(Stdio::piped());

        #[cfg(windows)]
        cmd.creation_flags(CREATE_NO_WINDOW);

        match cmd.spawn() {
            Ok(mut child) => {
                // 1. 接管 stderr 捕获错误日志
                if let Some(stderr) = child.stderr.take() {
                    let state_clone = self.state.clone();
                    tokio::spawn(async move {
                        let mut reader = BufReader::new(stderr).lines();
                        while let Ok(Some(line)) = reader.next_line().await {
                            tracing::warn!("⚠️ [Mihomo]: {}", line);
                            let mut s = state_clone.write().await;
                            s.last_error = Some(line);
                        }
                    });
                }

                // 2. 守护协程：监听内核是否意外崩溃
                let intentional = self.is_intentional_stop.clone();
                let state_for_watcher = self.state.clone();
                let core_path = self.core_binary_path.clone();
                let work = self.work_dir.clone();
                let conf = self.config_path.clone();
                let child_holder = self.child_process.clone();

                tokio::spawn(async move {
                    let status = child.wait().await;
                    // 如果不是用户故意停的，说明是崩溃退出！
                    if !intentional.load(Ordering::SeqCst) {
                        tracing::error!("🚨 警报：Mihomo 内核意外退出！原因: {:?}", status);
                        {
                            let mut s = state_for_watcher.write().await;
                            s.core_status = CoreStatus::Stopped;
                            s.last_error = Some(format!("内核异常退出 ({:?})，守护进程正在尝试拉起...", status));
                        }
                        Notifier::send("ReiaClash 守护提醒", "检测到 Mihomo 内核异常退出，正在尝试自动唤醒...");

                        tokio::time::sleep(tokio::time::Duration::from_secs(1)).await;

                        // 自动唤醒拉起
                        let mut cmd = Command::new(&core_path);
                        cmd.arg("-d").arg(&work).arg("-f").arg(&conf);
                        #[cfg(windows)]
                        cmd.creation_flags(CREATE_NO_WINDOW);

                        if let Ok(new_child) = cmd.spawn() {
                            let mut lock = child_holder.lock().await;
                            *lock = Some(new_child);
                            let mut s = state_for_watcher.write().await;
                            s.core_status = CoreStatus::Running;
                            s.last_error = None;
                            tracing::info!("♻️ 守护进程已成功自愈并拉起 Mihomo！");
                        }
                    }
                });

                *child_lock = None; // 由 watcher 跟踪句柄
                let mut state = self.state.write().await;
                state.core_status = CoreStatus::Running;
                state.last_error = None;
                tracing::info!("🎉 Mihomo 内核已就绪！");
                Ok(())
            }
            Err(e) => {
                let err_str = format!("拉起进程失败: {}", e);
                let mut state = self.state.write().await;
                state.core_status = CoreStatus::Stopped;
                state.last_error = Some(err_str.clone());
                Err(e.into())
            }
        }
    }

    pub async fn stop(&self) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        self.is_intentional_stop.store(true, Ordering::SeqCst);
        let mut child_lock = self.child_process.lock().await;
        self.stop_internal(&mut child_lock).await
    }

    async fn stop_internal(
        &self,
        child_lock: &mut Option<Child>,
    ) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        if let Some(mut child) = child_lock.take() {
            let _ = child.kill().await;
        }
        #[cfg(windows)]
        {
            use std::os::windows::process::CommandExt;
            let _ = std::process::Command::new("taskkill")
                .args(["/F", "/IM", "mihomo.exe"])
                .creation_flags(CREATE_NO_WINDOW)
                .output();
        }
        let mut state = self.state.write().await;
        state.core_status = CoreStatus::Stopped;
        Ok(())
    }

    pub async fn restart(&self) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        self.stop().await?;
        self.start().await?;
        Ok(())
    }

    pub async fn is_tun_active(&self) -> bool {
        if !self.config_path.exists() { return false; }
        if let Ok(content) = std::fs::read_to_string(&self.config_path) {
            if let Ok(root) = serde_yaml::from_str::<serde_yaml::Value>(&content) {
                if let Some(tun) = root.get("tun") {
                    if let Some(enable) = tun.get("enable") {
                        return enable.as_bool().unwrap_or(false);
                    }
                }
            }
        }
        false
    }
}