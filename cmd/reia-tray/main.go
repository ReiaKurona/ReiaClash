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
	flag.Bool("background", false, "由系统开机启动项静默启动托盘")
	flag.Parse()
	ctx, cancel := signals.Context()
	defer cancel()
	if err := trayui.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "ReiaClash 托盘启动失败:", err)
		os.Exit(1)
	}
}
