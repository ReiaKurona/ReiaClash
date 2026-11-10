use std::sync::Arc;
use tokio::sync::RwLock;
use crate::settings::AgentSettings;

#[derive(Debug, Clone, serde::Serialize)]
#[serde(rename_all = "lowercase")]
pub enum CoreStatus {
    Running,
    Stopped,
    Restarting,
}

#[derive(Debug, Clone)]
pub struct AppStateInner {
    pub active_panel_url: Option<String>,
    pub mihomo_api_port: u16,
    pub mihomo_secret: String,
    pub core_status: CoreStatus,
    pub current_subscription_url: Option<String>,
    pub config_path: std::path::PathBuf,
    pub settings_path: std::path::PathBuf,
    pub settings: AgentSettings,
    pub last_error: Option<String>,
}

pub type SharedState = Arc<RwLock<AppStateInner>>;

impl AppStateInner {
    pub fn new(config_path: std::path::PathBuf, settings_path: std::path::PathBuf) -> Self {
        let settings = AgentSettings::load(&settings_path);
        // 使用 settings 中持久化保存的唯一密钥
        let fixed_secret = settings.secret.clone();

        Self {
            active_panel_url: None,
            mihomo_api_port: 9090,
            mihomo_secret: fixed_secret,
            core_status: CoreStatus::Stopped,
            current_subscription_url: None,
            config_path,
            settings_path,
            settings,
            last_error: None,
        }
    }

    pub fn has_valid_profile(&self) -> bool {
        if !self.config_path.exists() {
            return false;
        }
        if let Ok(meta) = std::fs::metadata(&self.config_path) {
            meta.len() > 50
        } else {
            false
        }
    }
}