use std::fs;
use std::path::Path;
use serde_yaml::{Mapping, Value};

pub struct YamlPatcher;

impl YamlPatcher {
    pub fn patch_and_save(
        config_path: &Path,
        api_port: u16,
        secret: &str,
    ) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        if !config_path.exists() {
            if let Some(parent) = config_path.parent() {
                fs::create_dir_all(parent)?;
            }
            let default_template = r#"
mixed-port: 7890
allow-lan: true
mode: rule
log-level: info
proxies: []
rules:
  - MATCH,DIRECT
"#;
            fs::write(config_path, default_template.trim_start())?;
        }

        let content = fs::read_to_string(config_path)?;
        let mut root: Value = serde_yaml::from_str(&content)
            .unwrap_or_else(|_| Value::Mapping(Mapping::new()));

        let root_map = root.as_mapping_mut().ok_or("配置文件格式无效")?;

        // 核心锁定：保证 mixed-port 为 7890，确保系统代理精准接入！
        root_map.insert(
            Value::String("mixed-port".to_string()),
            Value::Number(serde_yaml::Number::from(7890)),
        );

        // 锁定 external-controller 与 secret
        root_map.insert(
            Value::String("external-controller".to_string()),
            Value::String(format!("127.0.0.1:{}", api_port)),
        );
        root_map.insert(
            Value::String("secret".to_string()),
            Value::String(secret.to_string()),
        );

        let serialized = serde_yaml::to_string(&root)?;
        fs::write(config_path, serialized)?;
        Ok(())
    }

    pub fn update_tun_enable(
        config_path: &Path,
        enable: bool,
    ) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        if !config_path.exists() { return Ok(()); }
        let content = fs::read_to_string(config_path)?;
        let mut root: Value = serde_yaml::from_str(&content)?;

        if let Some(root_map) = root.as_mapping_mut() {
            let tun_key = Value::String("tun".to_string());
            let tun_map = match root_map.get_mut(&tun_key) {
                Some(Value::Mapping(m)) => m,
                _ => {
                    let mut m = Mapping::new();
                    m.insert(Value::String("stack".to_string()), Value::String("mixed".to_string()));
                    m.insert(Value::String("auto-route".to_string()), Value::Bool(true));
                    m.insert(Value::String("auto-detect-interface".to_string()), Value::Bool(true));
                    root_map.insert(tun_key.clone(), Value::Mapping(m));
                    root_map.get_mut(&tun_key).unwrap().as_mapping_mut().unwrap()
                }
            };
            tun_map.insert(Value::String("enable".to_string()), Value::Bool(enable));
        }

        let serialized = serde_yaml::to_string(&root)?;
        fs::write(config_path, serialized)?;
        Ok(())
    }

    /// 支持 mixed / gvisor / system / mips 协议栈
    pub fn update_tun_stack(
        config_path: &Path,
        stack: &str,
    ) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        if !config_path.exists() { return Ok(()); }
        let content = fs::read_to_string(config_path)?;
        let mut root: Value = serde_yaml::from_str(&content)?;

        if let Some(root_map) = root.as_mapping_mut() {
            let tun_key = Value::String("tun".to_string());
            if let Some(Value::Mapping(tun_map)) = root_map.get_mut(&tun_key) {
                tun_map.insert(Value::String("stack".to_string()), Value::String(stack.to_string()));
            }
        }
        let serialized = serde_yaml::to_string(&root)?;
        fs::write(config_path, serialized)?;
        Ok(())
    }

    /// 動態修改 config.yaml 中的 allow-lan 設置
    pub fn update_allow_lan(
        config_path: &Path,
        allow_lan: bool,
    ) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        if !config_path.exists() { return Ok(()); }
        let content = fs::read_to_string(config_path)?;
        let mut root: Value = serde_yaml::from_str(&content)?;

        if let Some(root_map) = root.as_mapping_mut() {
            root_map.insert(
                Value::String("allow-lan".to_string()),
                Value::Bool(allow_lan),
            );
        }
        let serialized = serde_yaml::to_string(&root)?;
        fs::write(config_path, serialized)?;
        Ok(())
    }
}