#[cfg(windows)]
use winreg::enums::*;
#[cfg(windows)]
use winreg::RegKey;

const APP_NAME: &str = "ReiaClash";

pub struct AutoStart;

impl AutoStart {
    /// 注册开机自启
    #[cfg(windows)]
    pub fn enable() -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        let current_exe = std::env::current_exe()?;
        let exe_path_str = current_exe.to_str().ok_or("获取当前程序路径失败")?;

        let hkcu = RegKey::predef(HKEY_CURRENT_USER);
        let run_path = r"Software\Microsoft\Windows\CurrentVersion\Run";
        let (key, _) = hkcu.create_subkey(run_path)?;

        // Windows 规范：带空格路径需包裹在双引号中
        let quoted_exe = format!("\"{}\"", exe_path_str);
        key.set_value(APP_NAME, &quoted_exe)?;

        tracing::info!("⏰ 已开启开机自启: {}", quoted_exe);
        Ok(())
    }

    /// 注销开机自启
    #[cfg(windows)]
    pub fn disable() -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        let hkcu = RegKey::predef(HKEY_CURRENT_USER);
        let run_path = r"Software\Microsoft\Windows\CurrentVersion\Run";
        if let Ok(key) = hkcu.open_subkey_with_flags(run_path, KEY_WRITE) {
            let _ = key.delete_value(APP_NAME);
            tracing::info!("🔕 已关闭开机自启");
        }
        Ok(())
    }

    /// 查询当前是否已设置开机自启
    #[cfg(windows)]
    pub fn is_enabled() -> bool {
        let hkcu = RegKey::predef(HKEY_CURRENT_USER);
        let run_path = r"Software\Microsoft\Windows\CurrentVersion\Run";
        if let Ok(key) = hkcu.open_subkey_with_flags(run_path, KEY_READ) {
            let val: Result<String, _> = key.get_value(APP_NAME);
            return val.is_ok();
        }
        false
    }

    // 非 Windows 平台保底实现
    #[cfg(not(windows))]
    pub fn enable() -> Result<(), Box<dyn std::error::Error + Send + Sync>> { Ok(()) }
    #[cfg(not(windows))]
    pub fn disable() -> Result<(), Box<dyn std::error::Error + Send + Sync>> { Ok(()) }
    #[cfg(not(windows))]
    pub fn is_enabled() -> bool { false }
}