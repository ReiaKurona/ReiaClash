#[cfg(windows)]
use winreg::enums::*;
#[cfg(windows)]
use winreg::RegKey;

// 引入系统底层 wininet 动态库，用于即时通知 Windows 刷新代理，无需重启电脑或浏览器
#[cfg(windows)]
#[link(name = "wininet")]
unsafe extern "system" {
    fn InternetSetOptionW(
        h_internet: *mut std::ffi::c_void,
        dw_option: u32,
        lp_buffer: *mut std::ffi::c_void,
        dw_buffer_length: u32,
    ) -> i32;
}

/// 通知操作系统代理设置已发生变更
#[cfg(windows)]
fn notify_system_proxy_changed() {
    unsafe {
        const INTERNET_OPTION_SETTINGS_CHANGED: u32 = 39;
        const INTERNET_OPTION_REFRESH: u32 = 37;
        InternetSetOptionW(std::ptr::null_mut(), INTERNET_OPTION_SETTINGS_CHANGED, std::ptr::null_mut(), 0);
        InternetSetOptionW(std::ptr::null_mut(), INTERNET_OPTION_REFRESH, std::ptr::null_mut(), 0);
    }
}

pub struct SystemProxy;

impl SystemProxy {
    /// 开启 Windows 系统代理
    #[cfg(windows)]
    pub fn enable(port: u16) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        let hkcu = RegKey::predef(HKEY_CURRENT_USER);
        let settings_path = r"Software\Microsoft\Windows\CurrentVersion\Internet Settings";
        let (key, _) = hkcu.create_subkey(settings_path)?;

        // 1. 设置 ProxyEnable = 1 (开启)
        key.set_value("ProxyEnable", &1u32)?;

        // 2. 设置代理服务器地址 (例如: 127.0.0.1:7890)
        let proxy_server = format!("127.0.0.1:{}", port);
        key.set_value("ProxyServer", &proxy_server)?;

        // 3. 设置常用内网绕过白名单 (ProxyOverride)
        let bypass_list = "<local>;localhost;127.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;192.168.*";
        key.set_value("ProxyOverride", &bypass_list)?;

        // 4. 强制通知系统生效
        notify_system_proxy_changed();
        tracing::info!("🌐 已开启系统代理 -> {}", proxy_server);
        Ok(())
    }

    /// 关闭 Windows 系统代理
    #[cfg(windows)]
    pub fn disable() -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        let hkcu = RegKey::predef(HKEY_CURRENT_USER);
        let settings_path = r"Software\Microsoft\Windows\CurrentVersion\Internet Settings";
        let key = hkcu.open_subkey_with_flags(settings_path, KEY_WRITE)?;

        // 设置 ProxyEnable = 0 (关闭)
        key.set_value("ProxyEnable", &0u32)?;

        notify_system_proxy_changed();
        tracing::info!("🚫 已关闭系统代理");
        Ok(())
    }

    /// 查询当前系统代理是否处于开启状态
    #[cfg(windows)]
    pub fn is_enabled() -> bool {
        let hkcu = RegKey::predef(HKEY_CURRENT_USER);
        let settings_path = r"Software\Microsoft\Windows\CurrentVersion\Internet Settings";
        if let Ok(key) = hkcu.open_subkey_with_flags(settings_path, KEY_READ) {
            let enabled: Result<u32, _> = key.get_value("ProxyEnable");
            return enabled.unwrap_or(0) == 1;
        }
        false
    }

    // 非 Windows 平台的保底空实现
    #[cfg(not(windows))]
    pub fn enable(_port: u16) -> Result<(), Box<dyn std::error::Error + Send + Sync>> { Ok(()) }
    #[cfg(not(windows))]
    pub fn disable() -> Result<(), Box<dyn std::error::Error + Send + Sync>> { Ok(()) }
    #[cfg(not(windows))]
    pub fn is_enabled() -> bool { false }
}