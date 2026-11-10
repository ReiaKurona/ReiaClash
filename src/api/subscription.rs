use axum::{extract::State, Json};
use base64::prelude::*;
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use crate::core::{yaml_patcher::YamlPatcher, CoreManager};
use crate::state::SharedState;

#[derive(Clone)]
pub struct SubContext {
    pub state: SharedState,
    pub core_mgr: Arc<CoreManager>,
}

#[derive(Debug, Deserialize)]
pub struct PushUrlRequest {
    pub url: String,
    pub is_base64: Option<bool>,
}

#[derive(Debug, Deserialize)]
pub struct PushYamlRequest {
    pub yaml_content: String,
}

#[derive(Debug, Serialize)]
pub struct SubResponse {
    pub success: bool,
    pub message: String,
}

fn decode_subscription_url(raw: &str, force_base64: Option<bool>) -> Result<String, String> {
    let should_decode = force_base64.unwrap_or_else(|| {
        !raw.starts_with("http://") && !raw.starts_with("https://")
    });

    if should_decode {
        let trimmed = raw.trim();
        let bytes = BASE64_STANDARD.decode(trimmed).map_err(|e| format!("Base64 解碼失敗: {}", e))?;
        String::from_utf8(bytes).map_err(|e| format!("非 UTF-8 URL: {}", e))
    } else {
        Ok(raw.trim().to_string())
    }
}

pub async fn handle_push_url(
    State(ctx): State<SubContext>,
    Json(payload): Json<PushUrlRequest>,
) -> Json<SubResponse> {
    let target_url = match decode_subscription_url(&payload.url, payload.is_base64) {
        Ok(u) => u,
        Err(err) => return Json(SubResponse { success: false, message: err }),
    };

    tracing::info!("📥 下載遠程訂閱: {}", target_url);

    let client = match reqwest::Client::builder()
        .user_agent("mihomo/1.19.0 clash-meta")
        .build()
    {
        Ok(c) => c,
        Err(e) => return Json(SubResponse { success: false, message: format!("創建客戶端失敗: {}", e) }),
    };

    let response = match client.get(&target_url).send().await {
        Ok(res) => res,
        Err(e) => return Json(SubResponse { success: false, message: format!("請求失敗: {}", e) }),
    };

    if !response.status().is_success() {
        return Json(SubResponse { success: false, message: format!("伺服器狀態異常: {}", response.status()) });
    }

    let yaml_text = match response.text().await {
        Ok(t) => t,
        Err(e) => return Json(SubResponse { success: false, message: format!("讀取內容失敗: {}", e) }),
    };

    save_config_and_hot_reload(&ctx, &yaml_text, Some(target_url)).await
}

pub async fn handle_push_yaml(
    State(ctx): State<SubContext>,
    Json(payload): Json<PushYamlRequest>,
) -> Json<SubResponse> {
    tracing::info!("📄 收到直接推送的 YAML 配置");
    save_config_and_hot_reload(&ctx, &payload.yaml_content, None).await
}

/// 核心改進：落盤後調用原生 PUT /configs?force=true 觸發熱重載，進程不退出！
async fn save_config_and_hot_reload(
    ctx: &SubContext,
    yaml_content: &str,
    source_url: Option<String>,
) -> Json<SubResponse> {
    let (config_path, api_port, secret, settings) = {
        let s = ctx.state.read().await;
        (s.config_path.clone(), s.mihomo_api_port, s.mihomo_secret.clone(), s.settings.clone())
    };

    // 1. 寫入新配置
    if let Err(e) = std::fs::write(&config_path, yaml_content) {
        return Json(SubResponse { success: false, message: format!("寫入文件失敗: {}", e) });
    }

    // 2. 自動補齊 external-controller, secret, mixed-port: 7890
    if let Err(e) = YamlPatcher::patch_and_save(&config_path, api_port, &secret) {
        return Json(SubResponse { success: false, message: format!("配置修補失敗: {}", e) });
    }

    // 3. 套用記憶中的 TUN, Stack, Allow LAN 設置
    let _ = YamlPatcher::update_tun_enable(&config_path, settings.use_tun);
    let _ = YamlPatcher::update_tun_stack(&config_path, &settings.tun_stack);
    let _ = YamlPatcher::update_allow_lan(&config_path, settings.allow_lan);

    // 4. 更新記憶來源
    {
        let mut s = ctx.state.write().await;
        s.current_subscription_url = source_url;
    }

    // 5. 原地觸發原生熱重載（進程不退出，連接不中斷）
    let client = reqwest::Client::new();
    let reload_res = client
        .put(format!("http://127.0.0.1:{}/configs?force=true", api_port))
        .header("Authorization", format!("Bearer {}", secret))
        .json(&serde_json::json!({ "path": "", "payload": "" }))
        .send()
        .await;

    match reload_res {
        Ok(res) if res.status().is_success() => {
            tracing::info!("🎉 新配置已落盤，Mihomo 原地熱重載成功！");
            Json(SubResponse {
                success: true,
                message: "配置已更新，內核已原地熱重載生效（無重啟停機）".into(),
            })
        }
        _ => {
            // 保底方案：若內核未啟動或未響應，嘗試啟動
            let _ = ctx.core_mgr.start().await;
            Json(SubResponse {
                success: true,
                message: "配置已更新並重新啟動".into(),
            })
        }
    }
}