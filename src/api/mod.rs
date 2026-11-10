pub mod agent;
pub mod core;
pub mod handshake;
pub mod subscription;
pub mod system;

use std::sync::Arc;
use axum::{
    routing::{get, post},
    Router,
};
use agent::AgentContext;
use crate::core::CoreManager;
use crate::state::SharedState;
use subscription::SubContext;

pub fn create_api_router(state: SharedState, core_mgr: Arc<CoreManager>) -> Router {
    // 1. 信标握手
    let handshake_router = Router::new()
        .route("/api/handshake", post(handshake::handle_handshake))
        .with_state(state.clone());

    // 2. 内核控制路由（原生透明中继）
    let core_router = Router::new()
        .route("/api/core/restart", post(core::restart_core))
        .route("/api/core/stop", post(core::stop_core))
        .route("/api/core/proxies", get(core::relay_get_proxies))
        .route("/api/core/select", post(core::relay_select_proxy))
        .route("/api/core/delay", post(core::relay_test_delay))
        .route("/api/core/group-delay", post(core::relay_group_delay))
        .with_state(core_mgr.clone());

    // 3. 订阅管理
    let sub_ctx = SubContext {
        state: state.clone(),
        core_mgr: core_mgr.clone(),
    };
    let sub_router = Router::new()
        .route("/api/subscription/url", post(subscription::handle_push_url))
        .route("/api/subscription/yaml", post(subscription::handle_push_yaml))
        .with_state(sub_ctx);

    // 4. 系统功能
    let system_router = Router::new()
        .route("/api/system/status", get(system::get_system_status))
        .route("/api/system/proxy", post(system::set_system_proxy))
        .route("/api/system/autostart", post(system::set_autostart));

    // 5. Agent 记忆状态机与升级
    let agent_ctx = AgentContext {
        state: state.clone(),
        core_mgr: core_mgr.clone(),
    };
    let agent_router = Router::new()
        .route("/api/agent/settings", get(agent::get_settings).post(agent::update_settings))
        .route("/api/agent/master", post(agent::toggle_master))
        .route("/api/agent/exit", post(agent::handle_exit))
        .route("/api/agent/upgrade", post(agent::handle_upgrade))
        // 在 agent_router 中加上：
        .route("/api/agent/real-status", get(agent::get_real_status))
        .route("/api/agent/check-update", get(agent::check_update))
        .with_state(agent_ctx);

    Router::new()
        .merge(handshake_router)
        .merge(core_router)
        .merge(sub_router)
        .merge(system_router)
        .merge(agent_router)
}