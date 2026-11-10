use axum::{extract::State, Json};
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use crate::core::{yaml_patcher::YamlPatcher, CoreManager, CoreUpdater};
use crate::settings::AgentSettings;
use crate::state::{CoreStatus, SharedState};
use crate::system::SystemProxy;

#[derive(Clone)]
pub struct AgentContext {
    pub state: SharedState,
    pub core_mgr: Arc<CoreManager>,
}

#[derive(Deserialize)]
pub struct MasterToggleRequest {
    pub enable: bool,
}

#[derive(Deserialize)]
pub struct UpdateSettingsRequest {
    pub use_system_proxy: Option<bool>,
    pub use_tun: Option<bool>,
    pub tun_stack: Option<String>,
    pub outbound_mode: Option<String>,
    pub auto_update_enabled: Option<bool>,
    pub update_interval_hours: Option<u64>,
    pub allow_lan: Option<bool>,
}

#[derive(Serialize)]
pub struct SettingsResponse {
    pub success: bool,
    pub settings: AgentSettings,
}

#[derive(Serialize)]
pub struct CheckUpdateResponse {
    pub success: bool,
    pub tag: String,
    pub download_url: String,
    pub os: String,
    pub arch: String,
}

#[derive(Deserialize)]
pub struct UpgradeRequest {
    pub download_url: Option<String>,
    pub is_silent: bool,
}

#[derive(Serialize)]
pub struct ApiResponse {
    pub success: bool,
    pub message: String,
}

#[derive(Serialize)]
pub struct GroundTruthStatus {
    pub core_running: bool,
    pub real_system_proxy: bool,
    pub real_tun: bool,
    pub is_actually_intercepting: bool,
    pub last_error: Option<String>,
    pub settings: AgentSettings,
}

/// 核心：零停機原地熱重載狀態機
pub async fn apply_interception_state(
    settings: &AgentSettings,
    core_mgr: &Arc<CoreManager>,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let (api_port, secret) = {
        let s = core_mgr.state.read().await;
        (s.mihomo_api_port, s.mihomo_secret.clone())
    };

    let client = reqwest::Client::new();
    let base_api = format!("http://127.0.0.1:{}", api_port);

    if settings.master_active {
        // 系統代理切換（操作註冊表，內核零停機）
        if settings.use_system_proxy {
            let _ = SystemProxy::enable(7890);
        } else {
            let _ = SystemProxy::disable();
        }

        // TUN 與局域網設置落盤
        let _ = YamlPatcher::update_tun_enable(&core_mgr.config_path, settings.use_tun);
        let _ = YamlPatcher::update_tun_stack(&core_mgr.config_path, &settings.tun_stack);
        let _ = YamlPatcher::update_allow_lan(&core_mgr.config_path, settings.allow_lan);

        // 原地熱更新 Mihomo 內核（模式、TUN、局域網連線）
        let _ = client
            .patch(format!("{}/configs", base_api))
            .header("Authorization", format!("Bearer {}", secret))
            .json(&serde_json::json!({
                "mode": settings.outbound_mode,
                "allow-lan": settings.allow_lan,
                "tun": {
                    "enable": settings.use_tun,
                    "stack": settings.tun_stack
                }
            }))
            .send()
            .await;
    } else {
        // 總開關關閉：釋放代理與 TUN，內核保持靜默待機
        let _ = SystemProxy::disable();
        let _ = YamlPatcher::update_tun_enable(&core_mgr.config_path, false);
        let _ = YamlPatcher::update_allow_lan(&core_mgr.config_path, settings.allow_lan);
        let _ = client
            .patch(format!("{}/configs", base_api))
            .header("Authorization", format!("Bearer {}", secret))
            .json(&serde_json::json!({
                "allow-lan": settings.allow_lan,
                "tun": { "enable": false }
            }))
            .send()
            .await;
    }

    Ok(())
}

/// 獲取當前記憶設置 (GET /api/agent/settings) —— 補回缺失的接口！
pub async fn get_settings(State(ctx): State<AgentContext>) -> Json<SettingsResponse> {
    let s = ctx.state.read().await;
    Json(SettingsResponse {
        success: true,
        settings: s.settings.clone(),
    })
}

/// 更新偏好設置 (POST /api/agent/settings)
pub async fn update_settings(
    State(ctx): State<AgentContext>,
    Json(payload): Json<UpdateSettingsRequest>,
) -> Json<SettingsResponse> {
    let mut state_guard = ctx.state.write().await;
    if let Some(sp) = payload.use_system_proxy { state_guard.settings.use_system_proxy = sp; }
    if let Some(tun) = payload.use_tun { state_guard.settings.use_tun = tun; }
    if let Some(stack) = payload.tun_stack { state_guard.settings.tun_stack = stack; }
    if let Some(mode) = payload.outbound_mode { state_guard.settings.outbound_mode = mode; }
    if let Some(auto) = payload.auto_update_enabled { state_guard.settings.auto_update_enabled = auto; }
    if let Some(hrs) = payload.update_interval_hours { state_guard.settings.update_interval_hours = hrs; }
    if let Some(lan) = payload.allow_lan { state_guard.settings.allow_lan = lan; }

    state_guard.settings.save(&state_guard.settings_path);
    let settings = state_guard.settings.clone();
    drop(state_guard);

    if settings.master_active {
        let _ = apply_interception_state(&settings, &ctx.core_mgr).await;
    }

    Json(SettingsResponse { success: true, settings })
}

/// 切換總開關 (POST /api/agent/master)
pub async fn toggle_master(
    State(ctx): State<AgentContext>,
    Json(payload): Json<MasterToggleRequest>,
) -> Json<SettingsResponse> {
    let mut state_guard = ctx.state.write().await;
    state_guard.settings.master_active = payload.enable;
    state_guard.settings.save(&state_guard.settings_path);
    let settings = state_guard.settings.clone();
    drop(state_guard);

    let _ = apply_interception_state(&settings, &ctx.core_mgr).await;
    Json(SettingsResponse { success: true, settings })
}

/// 獲取真實物理運行狀態 (GET /api/agent/real-status)
pub async fn get_real_status(State(ctx): State<AgentContext>) -> Json<GroundTruthStatus> {
    let s = ctx.state.read().await;
    let real_sp = SystemProxy::is_enabled();
    let real_tun = ctx.core_mgr.is_tun_active().await;
    let core_running = matches!(s.core_status, CoreStatus::Running);
    let is_actually_intercepting = (real_sp || real_tun) && core_running;

    Json(GroundTruthStatus {
        core_running,
        real_system_proxy: real_sp,
        real_tun,
        is_actually_intercepting,
        last_error: s.last_error.clone(),
        settings: s.settings.clone(),
    })
}

/// 自動檢測架構並查詢 GitHub 最新發布 (GET /api/agent/check-update)
pub async fn check_update() -> Json<CheckUpdateResponse> {
    let (os, arch) = CoreUpdater::get_target_triple();
    match CoreUpdater::fetch_latest_release_url().await {
        Ok((tag, url)) => Json(CheckUpdateResponse {
            success: true,
            tag,
            download_url: url,
            os: os.into(),
            arch: arch.into(),
        }),
        Err(_) => Json(CheckUpdateResponse {
            success: false,
            tag: "檢測失敗".into(),
            download_url: String::new(),
            os: os.into(),
            arch: arch.into(),
        }),
    }
}

/// 觸發升級 (POST /api/agent/upgrade)
pub async fn handle_upgrade(
    State(ctx): State<AgentContext>,
    Json(payload): Json<UpgradeRequest>,
) -> Json<ApiResponse> {
    let url = if let Some(u) = payload.download_url {
        if !u.is_empty() { u } else {
            match CoreUpdater::fetch_latest_release_url().await {
                Ok((_, auto_url)) => auto_url,
                Err(e) => return Json(ApiResponse { success: false, message: format!("自動匹配包失敗: {}", e) }),
            }
        }
    } else {
        match CoreUpdater::fetch_latest_release_url().await {
            Ok((_, auto_url)) => auto_url,
            Err(e) => return Json(ApiResponse { success: false, message: format!("自動匹配包失敗: {}", e) }),
        }
    };

    if payload.is_silent {
        let work_dir = ctx.core_mgr.work_dir.clone();
        tokio::spawn(async move {
            let _ = CoreUpdater::silent_download(work_dir, &url).await;
        });
        Json(ApiResponse { success: true, message: "已觸發自動靜默下載至 update.pack，下次冷啟動或退出時生效".into() })
    } else {
        match CoreUpdater::manual_upgrade(ctx.core_mgr.clone(), &url).await {
            Ok(msg) => Json(ApiResponse { success: true, message: msg }),
            Err(e) => Json(ApiResponse { success: false, message: format!("升級失敗: {}", e) }),
        }
    }
}

/// 安全退出客戶端 (POST /api/agent/exit)
pub async fn handle_exit(State(ctx): State<AgentContext>) -> Json<ApiResponse> {
    tokio::spawn(async move {
        let _ = SystemProxy::disable();
        let _ = ctx.core_mgr.stop().await;
        CoreUpdater::check_and_apply_cold_ota(&ctx.core_mgr.work_dir, &ctx.core_mgr.core_binary_path);
        tokio::time::sleep(tokio::time::Duration::from_millis(300)).await;
        std::process::exit(0);
    });
    Json(ApiResponse { success: true, message: "正在安全退出並清理網絡...".into() })
}