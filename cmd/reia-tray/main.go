//go:build (windows || linux || darwin) && !android

// Command reia-tray 只管理桌面托盘菜单，不创建普通应用窗口。
package main

import (
	//"context"
	"flag"
	"fmt"
	"os"

	"reia-clash/internal/platform/signals"
	"reia-clash/internal/trayui"
)

func main() {
	background := flag.Bool("background", false, "由旧版系统登录启动项启动")
	login := flag.Bool("login-autostart", false, "由系统登录启动项启动")
	flag.Parse()
	ctx, cancel := signals.Context()
	defer cancel()
	if err := trayui.Run(ctx, *login || *background); err != nil {
		fmt.Fprintln(os.Stderr, "ReiaClash 托盘启动失败:", err)
		os.Exit(1)
	}
}
