use notify_rust::Notification;

pub struct Notifier;

impl Notifier {
    /// 发送跨平台系统桌面通知（支持 Windows 11 通知中心、Linux、macOS）
    pub fn send(title: &str, body: &str) {
        let _ = Notification::new()
            .summary(title)
            .body(body)
            .appname("ReiaClash")
            .show();
    }
}