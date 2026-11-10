use axum::Json;
use serde::{Deserialize, Serialize};
use crate::system::{AutoStart, SystemProxy};

/// 当前系统状态概览数据
#[derive(Serialize)]
pub struct SystemStatusResponse {
    pub system_proxy_enabled: bool,
    pub autostart_enabled: bool,
}

/// 切换系统代理请求
#[derive(Deserialize)]
pub struct ToggleProxyRequest {
    pub enable: bool,
    /// 代理端口，如果不填默认为 7890
    pub port: Option<u16>,
}

/// 切换自启动请求
#[derive(Deserialize)]
pub struct ToggleAutostartRequest {
    pub enable: bool,
}

#[derive(Serialize)]
pub struct SimpleResponse {
    pub success: bool,
    pub message: String,
}

/// 获取系统代理与开机自启状态 (GET /api/system/status)
pub async fn get_system_status() -> Json<SystemStatusResponse> {
    Json(SystemStatusResponse {
        system_proxy_enabled: SystemProxy::is_enabled(),
        autostart_enabled: AutoStart::is_enabled(),
    })
}

/// 设置系统代理 (POST /api/system/proxy)
pub async fn set_system_proxy(
    Json(payload): Json<ToggleProxyRequest>,
) -> Json<SimpleResponse> {
    let result = if payload.enable {
        let port = payload.port.unwrap_or(7890);
        SystemProxy::enable(port)
    } else {
        SystemProxy::disable()
    };

    match result {
        Ok(_) => Json(SimpleResponse {
            success: true,
            message: if payload.enable { "系统代理已开启".into() } else { "系统代理已关闭".into() },
        }),
        Err(e) => Json(SimpleResponse {
            success: false,
            message: format!("设置系统代理失败: {}", e),
        }),
    }
}

/// 设置开机自启 (POST /api/system/autostart)
pub async fn set_autostart(
    Json(payload): Json<ToggleAutostartRequest>,
) -> Json<SimpleResponse> {
    let result = if payload.enable {
        AutoStart::enable()
    } else {
        AutoStart::disable()
    };

    match result {
        Ok(_) => Json(SimpleResponse {
            success: true,
            message: if payload.enable { "已开启开机自启".into() } else { "已关闭开机自启".into() },
        }),
        Err(e) => Json(SimpleResponse {
            success: false,
            message: format!("设置开机自启失败: {}", e),
        }),
    }
}