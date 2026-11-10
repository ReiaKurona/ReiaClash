use std::fs;
use std::io::{Cursor, Read};
use std::path::{Path, PathBuf};
use std::sync::Arc;
use serde::Deserialize;
use crate::core::CoreManager;
use crate::system::Notifier;

pub struct CoreUpdater;

#[derive(Deserialize)]
struct GitHubRelease {
    tag_name: String,
    assets: Vec<GitHubAsset>,
}

#[derive(Deserialize)]
struct GitHubAsset {
    name: String,
    browser_download_url: String,
}

impl CoreUpdater {
    /// 自动获取匹配当前操作系统和 CPU 架构的关键词
    pub fn get_target_triple() -> (&'static str, &'static str) {
        let os = match std::env::consts::OS {
            "windows" => "windows",
            "linux" => "linux",
            "macos" => "darwin",
            other => other,
        };
        let arch = match std::env::consts::ARCH {
            "x86_64" => "amd64",
            "aarch64" => "arm64",
            "x86" => "386",
            "arm" => "armv7",
            other => other,
        };
        (os, arch)
    }

    /// 冷启动检查：如果有 update.pack，在启动内核前替换
    pub fn check_and_apply_cold_ota(work_dir: &Path, core_binary: &Path) {
        let pack_path = work_dir.join("update.pack");
        if pack_path.exists() {
            Notifier::send("ReiaClash OTA", "检测到本地新内核文件，正在执行冷启动替换...");
            if core_binary.exists() {
                let _ = fs::remove_file(core_binary);
            }
            if fs::rename(&pack_path, core_binary).is_ok() {
                Notifier::send("ReiaClash OTA", "内核已成功更新为最新版本，服务已启动！");
            }
        }
    }

    /// 自动向 GitHub 查询最新版本并获取匹配当前系统的下载链接
    pub async fn fetch_latest_release_url() -> Result<(String, String), Box<dyn std::error::Error + Send + Sync>> {
        let proxy = reqwest::Proxy::all("http://127.0.0.1:7890")?;
        let client = reqwest::Client::builder()
            .proxy(proxy)
            .user_agent("ReiaClash-Updater")
            .build()?;

        let release: GitHubRelease = client
            .get("https://api.github.com/repos/MetaCubeX/mihomo/releases/latest")
            .send()
            .await?
            .json()
            .await?;

        let (target_os, target_arch) = Self::get_target_triple();
        tracing::info!("🔍 正在寻找适用于 [{}-{}] 的内核包...", target_os, target_arch);

        // 在 Release assets 中寻找匹配项
        for asset in release.assets {
            let name_lower = asset.name.to_lowercase();
            if name_lower.contains(target_os) && name_lower.contains(target_arch) {
                // 排除带有 debug 或兼容老旧内核字样的非主包
                if !name_lower.contains("debug") {
                    return Ok((release.tag_name, asset.browser_download_url));
                }
            }
        }

        Err("未在 GitHub Release 中找到匹配当前系统与架构的安装包".into())
    }

    /// 下载并自动解压出干净的可执行二进制数据
    pub async fn download_and_extract(url: &str) -> Result<Vec<u8>, Box<dyn std::error::Error + Send + Sync>> {
        let proxy = reqwest::Proxy::all("http://127.0.0.1:7890")?;
        let client = reqwest::Client::builder()
            .proxy(proxy)
            .user_agent("ReiaClash-Updater")
            .build()?;

        let resp = client.get(url).send().await?;
        let bytes = resp.bytes().await?;

        // 1. 如果是 .zip 文件（借用切片 &bytes[..]，不夺走所有权）
        if url.ends_with(".zip") {
            let reader = Cursor::new(&bytes[..]);
            let mut archive = zip::ZipArchive::new(reader)?;
            for i in 0..archive.len() {
                let mut file = archive.by_index(i)?;
                if file.name().ends_with(".exe") || !file.name().contains('.') {
                    let mut extracted = Vec::new();
                    file.read_to_end(&mut extracted)?;
                    return Ok(extracted);
                }
            }
            return Err("未在 zip 压缩包中找到可执行文件".into());
        } else if url.ends_with(".gz") {
            // 2. 如果是 .gz 文件
            use flate2::read::GzDecoder;
            let mut decoder = GzDecoder::new(&bytes[..]);
            let mut extracted = Vec::new();
            decoder.read_to_end(&mut extracted)?;
            return Ok(extracted);
        }

        // 3. 直接下载的纯二进制文件
        Ok(bytes.to_vec())
    }

    /// 手动在线热升级
    pub async fn manual_upgrade(
        core_mgr: Arc<CoreManager>,
        download_url: &str,
    ) -> Result<String, Box<dyn std::error::Error + Send + Sync>> {
        let binary_bytes = Self::download_and_extract(download_url).await?;

        let temp_path = core_mgr.work_dir.join("temp_mihomo.new");
        fs::write(&temp_path, binary_bytes)?;

        // 安全停止旧进程
        core_mgr.stop().await?;

        if core_mgr.core_binary_path.exists() {
            let _ = fs::remove_file(&core_mgr.core_binary_path);
        }
        fs::rename(&temp_path, &core_mgr.core_binary_path)?;

        // 重新拉起
        core_mgr.start().await?;
        Notifier::send("ReiaClash", "Mihomo 内核手动热更新已完成，服务已重启运行！");
        Ok("内核已更新并重启成功".into())
    }

    /// 静默下载至 update.pack
    pub async fn silent_download(
        work_dir: PathBuf,
        download_url: &str,
    ) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        let binary_bytes = Self::download_and_extract(download_url).await?;
        let pack_path = work_dir.join("update.pack");
        fs::write(&pack_path, binary_bytes)?;
        Notifier::send("ReiaClash", "静默更新已下载至 update.pack，将在下次退出或重启时自动生效！");
        Ok(())
    }

    /// 启动后台定时巡检静默更新任务
    pub fn spawn_auto_updater(core_mgr: Arc<CoreManager>) {
        tokio::spawn(async move {
            loop {
                // 每隔 1 小时检查一次设置
                tokio::time::sleep(tokio::time::Duration::from_secs(3600)).await;

                let (enabled, interval_hours, work_dir) = {
                    let s = core_mgr.state.read().await;
                    (
                        s.settings.auto_update_enabled,
                        s.settings.update_interval_hours.max(1),
                        core_mgr.work_dir.clone(),
                    )
                };

                if enabled {
                    tracing::info!("⏰ 触发静默更新检查...");
                    if let Ok((tag, url)) = Self::fetch_latest_release_url().await {
                        tracing::info!("💡 检测到 GitHub 最新版本: {}, 开始静默打包...", tag);
                        let _ = Self::silent_download(work_dir, &url).await;
                    }
                }

                // 按照设定的周期挂起
                tokio::time::sleep(tokio::time::Duration::from_secs(interval_hours * 3600)).await;
            }
        });
    }
}