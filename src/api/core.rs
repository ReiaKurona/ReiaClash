use axum::{extract::State, Json};
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use crate::core::CoreManager;

#[derive(Serialize)]
pub struct SimpleStatusResponse {
    pub success: bool,
    pub message: String,
}

#[derive(Deserialize)]
pub struct SelectProxyRequest {
    pub group: String,
    pub node: String,
}

#[derive(Deserialize)]
pub struct GroupDelayRequest {
    pub group: String,
}

pub async fn restart_core(State(core_mgr): State<Arc<CoreManager>>) -> Json<SimpleStatusResponse> {
    match core_mgr.restart().await {
        Ok(_) => Json(SimpleStatusResponse { success: true, message: "内核重启成功".into() }),
        Err(e) => Json(SimpleStatusResponse { success: false, message: format!("重启失败: {}", e) }),
    }
}

pub async fn stop_core(State(core_mgr): State<Arc<CoreManager>>) -> Json<SimpleStatusResponse> {
    match core_mgr.stop().await {
        Ok(_) => Json(SimpleStatusResponse { success: true, message: "内核已停止".into() }),
        Err(e) => Json(SimpleStatusResponse { success: false, message: format!("停止失败: {}", e) }),
    }
}

/// 中继拉取全部节点列表 (GET /api/core/proxies)
pub async fn relay_get_proxies(State(core_mgr): State<Arc<CoreManager>>) -> axum::response::Response {
    let (api_port, secret) = {
        let s = core_mgr.state.read().await;
        (s.mihomo_api_port, s.mihomo_secret.clone())
    };
    let client = reqwest::Client::new();
    let res = client
        .get(format!("http://127.0.0.1:{}/proxies", api_port))
        .header("Authorization", format!("Bearer {}", secret))
        .send()
        .await;

    if let Ok(resp) = res {
        let body = resp.text().await.unwrap_or_else(|_| "{}".into());
        axum::response::Response::builder()
            .header("content-type", "application/json")
            .body(axum::body::Body::from(body))
            .unwrap()
    } else {
        axum::response::Response::builder()
            .status(500)
            .body(axum::body::Body::from("{\"error\": \"获取原生节点失败\"}"))
            .unwrap()
    }
}

/// 核心：切换节点后自动 DELETE /connections 强制掐断旧连接！(POST /api/core/select)
pub async fn relay_select_proxy(
    State(core_mgr): State<Arc<CoreManager>>,
    Json(payload): Json<SelectProxyRequest>,
) -> Json<SimpleStatusResponse> {
    let (api_port, secret) = {
        let s = core_mgr.state.read().await;
        (s.mihomo_api_port, s.mihomo_secret.clone())
    };

    let client = reqwest::Client::new();
    let target_url = format!(
        "http://127.0.0.1:{}/proxies/{}",
        api_port,
        urlencoding::encode(&payload.group)
    );

    // 1. 切换节点
    let res = client
        .put(&target_url)
        .header("Authorization", format!("Bearer {}", secret))
        .json(&serde_json::json!({ "name": payload.node }))
        .send()
        .await;

    match res {
        Ok(r) if r.status().is_success() => {
            tracing::info!("🎯 节点已切换至 [{}]，正在强制掐断所有旧连接...", payload.node);
            
            // 2. 关键点：强制关闭所有连接，确保新请求立即走新节点！
            let _ = client
                .delete(format!("http://127.0.0.1:{}/connections", api_port))
                .header("Authorization", format!("Bearer {}", secret))
                .send()
                .await;

            Json(SimpleStatusResponse {
                success: true,
                message: format!("已成功切换至 {}，并已强制刷新连接", payload.node),
            })
        }
        Ok(r) => Json(SimpleStatusResponse { success: false, message: format!("内核拒绝: HTTP {}", r.status()) }),
        Err(e) => Json(SimpleStatusResponse { success: false, message: format!("中继失败: {}", e) }),
    }
}

/// 核心：使用 Mihomo 原生整组并发测速 (POST /api/core/group-delay)
pub async fn relay_group_delay(
    State(core_mgr): State<Arc<CoreManager>>,
    Json(payload): Json<GroupDelayRequest>,
) -> axum::response::Response {
    let (api_port, secret) = {
        let s = core_mgr.state.read().await;
        (s.mihomo_api_port, s.mihomo_secret.clone())
    };

    let target_url = format!(
        "http://127.0.0.1:{}/group/{}/delay?url=http%3A%2F%2Fwww.gstatic.com%2Fgenerate_204&timeout=5000",
        api_port,
        urlencoding::encode(&payload.group)
    );

    let client = reqwest::Client::new();
    if let Ok(resp) = client.get(&target_url).header("Authorization", format!("Bearer {}", secret)).send().await {
        let body = resp.text().await.unwrap_or_else(|_| "{}".into());
        axum::response::Response::builder()
            .header("content-type", "application/json")
            .body(axum::body::Body::from(body))
            .unwrap()
    } else {
        axum::response::Response::builder()
            .status(500)
            .body(axum::body::Body::from("{}"))
            .unwrap()
    }


}

#[derive(Deserialize)]
pub struct DelayTestRequest {
    pub node: String,
}

#[derive(Serialize)]
pub struct DelayTestResponse {
    pub delay: u16,
}

/// 核心：通过 Agent 中继单节点测速 (POST /api/core/delay)
pub async fn relay_test_delay(
    State(core_mgr): State<Arc<CoreManager>>,
    Json(payload): Json<DelayTestRequest>,
) -> Json<DelayTestResponse> {
    let (api_port, secret) = {
        let s = core_mgr.state.read().await;
        (s.mihomo_api_port, s.mihomo_secret.clone())
    };

    let target_url = format!(
        "http://127.0.0.1:{}/proxies/{}/delay?timeout=5000&url=http%3A%2F%2Fwww.gstatic.com%2Fgenerate_204",
        api_port,
        urlencoding::encode(&payload.node)
    );

    let client = reqwest::Client::new();
    if let Ok(res) = client.get(&target_url).header("Authorization", format!("Bearer {}", secret)).send().await {
        if let Ok(data) = res.json::<serde_json::Value>().await {
            if let Some(delay) = data.get("delay").and_then(|d| d.as_u64()) {
                return Json(DelayTestResponse { delay: delay as u16 });
            }
        }
    }
    Json(DelayTestResponse { delay: 0 })
}