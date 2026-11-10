use serde::{Deserialize, Serialize};
use std::path::Path;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AgentSettings {
    pub master_active: bool,
    pub use_system_proxy: bool,
    pub use_tun: bool,
    pub tun_stack: String,
    pub outbound_mode: String,
    pub secret: String,
    pub auto_update_enabled: bool,
    pub update_interval_hours: u64,
    /// 允許局域網連線 (Allow LAN)
    pub allow_lan: bool,
}

impl Default for AgentSettings {
    fn default() -> Self {
        Self {
            master_active: false,
            use_system_proxy: true,
            use_tun: false,
            tun_stack: "mixed".to_string(),
            outbound_mode: "rule".to_string(),
            secret: uuid::Uuid::new_v4().to_string(),
            auto_update_enabled: false,
            update_interval_hours: 24,
            allow_lan: false, // 默認關閉局域網共享
        }
    }
}

impl AgentSettings {
    pub fn load(path: &Path) -> Self {
        if let Ok(content) = std::fs::read_to_string(path) {
            let mut s: Self = serde_json::from_str(&content).unwrap_or_default();
            if s.secret.is_empty() {
                s.secret = uuid::Uuid::new_v4().to_string();
                s.save(path);
            }
            s
        } else {
            let s = Self::default();
            s.save(path);
            s
        }
    }

    pub fn save(&self, path: &Path) {
        if let Some(parent) = path.parent() {
            let _ = std::fs::create_dir_all(parent);
        }
        if let Ok(content) = serde_json::to_string_pretty(self) {
            let _ = std::fs::write(path, content);
        }
    }
}