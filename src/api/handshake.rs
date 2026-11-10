use axum::{extract::State, http::HeaderMap, Json};
use serde::{Deserialize, Serialize};
use crate::state::{CoreStatus, SharedState};

#[derive(Debug, Deserialize, Default)]
pub struct HandshakeRequest {
    pub panel_origin: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct HandshakeResponse {
    pub connected: bool,
    pub agent_version: String,
    pub core_status: CoreStatus,
    pub need_profile: bool,
    pub has_profile: bool,
    pub current_subscription: Option<String>,
    pub last_error: Option<String>,
    pub mihomo_controller: MihomoConnectionInfo,
}

#[derive(Debug, Serialize)]
pub struct MihomoConnectionInfo {
    pub api_base: String,
    pub secret: String,
}

pub async fn handle_handshake(
    State(state): State<SharedState>,
    headers: HeaderMap,
    payload: Option<Json<HandshakeRequest>>,
) -> Json<HandshakeResponse> {
    let mut state_guard = state.write().await;

    let detected_origin = payload
        .and_then(|p| p.0.panel_origin)
        .or_else(|| {
            headers
                .get("panel_origin")
                .or_else(|| headers.get("panel-origin"))
                .and_then(|v| v.to_str().ok().map(|s| s.to_string()))
        })
        .or_else(|| {
            headers.get("origin").and_then(|v| v.to_str().ok().map(|s| s.to_string()))
        })
        .unwrap_or_else(|| "http://localhost:5173".to_string());

    tracing::info!("📡 捕获到前端信标，绑定来源面板: {}", detected_origin);
    state_guard.active_panel_url = Some(detected_origin);

    let has_profile = state_guard.has_valid_profile();

    let response = HandshakeResponse {
        connected: true,
        agent_version: env!("CARGO_PKG_VERSION").to_string(),
        core_status: state_guard.core_status.clone(),
        need_profile: !has_profile,
        has_profile,
        current_subscription: state_guard.current_subscription_url.clone(),
        last_error: state_guard.last_error.clone(),
        mihomo_controller: MihomoConnectionInfo {
            api_base: format!("http://127.0.0.1:{}", state_guard.mihomo_api_port),
            secret: state_guard.mihomo_secret.clone(), // 永远固定的密钥
        },
    };

    Json(response)
}