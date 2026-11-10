use std::sync::Arc;
use muda::{Menu, MenuEvent, MenuItem, PredefinedMenuItem};
use tray_icon::{MouseButton, MouseButtonState, TrayIcon, TrayIconBuilder, TrayIconEvent};
use crate::core::CoreManager;
use crate::state::SharedState;

fn create_default_icon() -> tray_icon::Icon {
    let width = 32u32;
    let height = 32u32;
    let mut rgba = Vec::with_capacity((width * height * 4) as usize);

    for y in 0..height {
        for x in 0..width {
            let dx = x as f32 - 15.5;
            let dy = y as f32 - 15.5;
            let dist = (dx * dx + dy * dy).sqrt();

            if dist <= 12.0 && dist >= 6.0 {
                rgba.extend_from_slice(&[37, 99, 235, 255]); // 藍色外環
            } else if dist < 6.0 {
                rgba.extend_from_slice(&[255, 255, 255, 255]); // 白色內核
            } else {
                rgba.extend_from_slice(&[0, 0, 0, 0]);
            }
        }
    }
    tray_icon::Icon::from_rgba(rgba, width, height).expect("生成默認圖標失敗")
}

fn open_in_browser(url: &str) {
    #[cfg(windows)]
    {
        let _ = std::process::Command::new("cmd")
            .args(["/c", "start", url])
            .spawn();
    }
}

pub struct AppTray {
    _tray_icon: TrayIcon,
}

impl AppTray {
    pub fn init(
        state: SharedState,
        core_mgr: Arc<CoreManager>,
    ) -> Result<Self, Box<dyn std::error::Error>> {
        let menu = Menu::new();

        let open_panel_item = MenuItem::new("🌐 打開網頁控制面板", true, None);
        let sep1 = PredefinedMenuItem::separator();
        
        let toggle_proxy_item = MenuItem::new("⚡ 切換系統代理", true, None);
        let toggle_tun_item = MenuItem::new("🛡️ 切換虛擬網卡 (TUN)", true, None);
        let sep2 = PredefinedMenuItem::separator();
        let restart_core_item = MenuItem::new("🔄 重啟內核 (Mihomo)", true, None);

        menu.append_items(&[
            &open_panel_item,
            &sep1,
            &toggle_proxy_item,
            &toggle_tun_item,
            &sep2,
            &restart_core_item,
        ])?;

        let icon = create_default_icon();
        let tray_icon = TrayIconBuilder::new()
            .with_menu(Box::new(menu))
            .with_tooltip("ReiaClash - 無頭守護中心")
            .with_icon(icon)
            .build()?;

        let open_panel_id = open_panel_item.id().clone();
        let toggle_proxy_id = toggle_proxy_item.id().clone();
        let toggle_tun_id = toggle_tun_item.id().clone();
        let restart_core_id = restart_core_item.id().clone();

        let state_menu = state.clone();
        let core_mgr_menu = core_mgr.clone();

        // 托盤菜單點擊事件：直接調用狀態機，與 Web 面板保持 100% 邏輯一致！
        MenuEvent::set_event_handler(Some(move |event: MenuEvent| {
            let id = &event.id;

            if id == &open_panel_id {
                let s = state_menu.clone();
                tokio::spawn(async move {
                    let target_url = {
                        let guard = s.read().await;
                        guard.active_panel_url.clone().unwrap_or_else(|| "http://localhost:5173".into())
                    };
                    open_in_browser(&target_url);
                });
            } else if id == &toggle_proxy_id {
                let s = state_menu.clone();
                let cm = core_mgr_menu.clone();
                tokio::spawn(async move {
                    let mut state_guard = s.write().await;
                    let target = !state_guard.settings.use_system_proxy;
                    state_guard.settings.use_system_proxy = target;
                    state_guard.settings.master_active = true; // 托盤點擊直接激活總開關
                    state_guard.settings.save(&state_guard.settings_path);
                    let settings = state_guard.settings.clone();
                    drop(state_guard);
                    let _ = crate::api::agent::apply_interception_state(&settings, &cm).await;
                });
            } else if id == &toggle_tun_id {
                let s = state_menu.clone();
                let cm = core_mgr_menu.clone();
                tokio::spawn(async move {
                    let mut state_guard = s.write().await;
                    let target = !state_guard.settings.use_tun;
                    state_guard.settings.use_tun = target;
                    state_guard.settings.master_active = true; // 托盤點擊直接激活總開關
                    state_guard.settings.save(&state_guard.settings_path);
                    let settings = state_guard.settings.clone();
                    drop(state_guard);
                    let _ = crate::api::agent::apply_interception_state(&settings, &cm).await;
                });
            } else if id == &restart_core_id {
                let cm = core_mgr_menu.clone();
                tokio::spawn(async move {
                    let _ = cm.restart().await;
                });
            }
        }));

        let state_tray = state.clone();
        TrayIconEvent::set_event_handler(Some(move |event: TrayIconEvent| {
            if let TrayIconEvent::Click {
                button: MouseButton::Left,
                button_state: MouseButtonState::Up,
                ..
            } = event
            {
                let s = state_tray.clone();
                tokio::spawn(async move {
                    let target_url = {
                        let guard = s.read().await;
                        guard.active_panel_url.clone().unwrap_or_else(|| "http://localhost:5173".into())
                    };
                    open_in_browser(&target_url);
                });
            }
        }));

        Ok(Self { _tray_icon: tray_icon })
    }
}