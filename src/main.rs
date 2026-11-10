mod api;
mod core;
mod pna_cors;
mod settings;
mod state;
mod system;
mod tray;

use std::path::PathBuf;
use std::sync::Arc;
use tokio::sync::RwLock;

// Windows 原生消息队列 FFI（用于托盘菜单事件循环）
#[cfg(windows)]
#[link(name = "user32")]
unsafe extern "system" {
    fn GetMessageW(msg: *mut std::ffi::c_void, hwnd: *mut std::ffi::c_void, min: u32, max: u32) -> i32;
    fn TranslateMessage(msg: *const std::ffi::c_void) -> i32;
    fn DispatchMessageW(msg: *const std::ffi::c_void) -> isize;
}

#[cfg(windows)]
#[link(name = "kernel32")]
unsafe extern "system" {
    fn SetConsoleOutputCP(wCodePageID: u32) -> i32;
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    #[cfg(windows)]
    unsafe {
        SetConsoleOutputCP(65001); // 启用终端 UTF-8
    }

    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| "info".into()),
        )
        .init();

    tracing::info!("🚀 ReiaClash Agent 正在启动...");

    // 1. 启动前清场：强制终止可能占着 9090 端口的旧 mihomo 孤儿进程
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        let _ = std::process::Command::new("taskkill")
            .args(["/F", "/IM", "mihomo.exe"])
            .creation_flags(0x08000000) // 静默无窗口
            .output();
    }

    let work_dir = PathBuf::from("./data");
    let config_path = work_dir.join("config.yaml");
    let settings_path = work_dir.join("agent_settings.json");

    #[cfg(windows)]
    let core_binary_path = PathBuf::from("./mihomo.exe");
    #[cfg(not(windows))]
    let core_binary_path = PathBuf::from("./mihomo");

    // 2. 冷启动 OTA 检查
    core::updater::CoreUpdater::check_and_apply_cold_ota(&work_dir, &core_binary_path);
    

    // 3. 初始化持久化全局状态
    let state = Arc::new(RwLock::new(state::AppStateInner::new(config_path.clone(), settings_path)));

    let core_mgr = Arc::new(core::CoreManager::new(
        state.clone(),
        core_binary_path,
        work_dir,
        config_path,
    ));

    // 启动后台自动静默巡检任务
    core::updater::CoreUpdater::spawn_auto_updater(core_mgr.clone());
    
    // 4. 托盘初始化
    let _tray = tray::AppTray::init(state.clone(), core_mgr.clone())?;

    // 5. REST 路由
    let app = api::create_api_router(state.clone(), core_mgr.clone())
        .layer(pna_cors::PnaCorsLayer);

    let listen_addr = "127.0.0.1:25500";
    let listener = tokio::net::TcpListener::bind(listen_addr).await?;
    tracing::info!("📡 Agent REST 服务已就绪: http://{}", listen_addr);

    tokio::spawn(async move {
        if let Err(e) = axum::serve(listener, app).await {
            tracing::error!("REST 服务异常: {}", e);
        }
    });

    // 6. 拉起新内核
    let auto_start_mgr = core_mgr.clone();
    tokio::spawn(async move {
        if let Err(e) = auto_start_mgr.start().await {
            tracing::warn!("⚠️ 启动 Mihomo 失败: {}", e);
        }
    });

    // 7. 维持 Windows 原生消息队列（消除 GetMessageW 警告，托盘稳定运行）
    #[cfg(windows)]
    {
        tracing::info!("💡 Windows 消息循环已就绪，托盘已入驻任务栏");
        unsafe {
            let mut msg = [0u8; 48];
            while GetMessageW(msg.as_mut_ptr() as _, std::ptr::null_mut(), 0, 0) > 0 {
                TranslateMessage(msg.as_ptr() as _);
                DispatchMessageW(msg.as_ptr() as _);
            }
        }
    }

    #[cfg(not(windows))]
    tokio::signal::ctrl_c().await?;

    Ok(())
}