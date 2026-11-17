package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reia-clash/internal/coreservice"
	platform "reia-clash/internal/platform/coreservice"
	"runtime"
)

func main() {
	dataDir := flag.String("data-dir", "", "ReiaClash 用户数据目录")
	uid := flag.String("service-uid", "", "服务所属用户编号")
	install := flag.Bool("install-core-service", false, "安装系统服务")
	uninstall := flag.Bool("uninstall-core-service", false, "卸载系统服务")
	flag.Bool("core-service", true, "运行独立内核服务")
	flag.Parse()
	logName := "core-service-startup.log"
	if *install || *uninstall {
		logName = "core-service-operation.log"
	}
	// Linux polkit operations report via stderr; the user-session caller
	// writes the diagnostic so a failed install never leaves a root-owned log.
	writeDiagnostics := runtime.GOOS != "linux" || !(*install || *uninstall)
	if *dataDir != "" && writeDiagnostics {
		_ = os.Remove(filepath.Join(*dataDir, logName))
	}
	var err error
	switch {
	case *install:
		err = platform.InstallPrivileged(*dataDir, *uid)
	case *uninstall:
		err = platform.UninstallPrivileged(*dataDir, *uid)
	case *dataDir == "":
		err = fmt.Errorf("缺少 --data-dir，内核服务请通过客户端设置启动")
	default:
		err = platform.RunService(func(ctx context.Context) error {
			runErr := coreservice.RunContext(ctx, *dataDir)
			if runErr != nil {
				_ = os.WriteFile(filepath.Join(*dataDir, "core-service-startup.log"), []byte(runErr.Error()), 0600)
			}
			return runErr
		})
	}
	if err != nil {
		if *dataDir != "" && writeDiagnostics {
			_ = os.WriteFile(filepath.Join(*dataDir, logName), []byte(err.Error()), 0600)
		}
		fmt.Fprintln(os.Stderr, "ReiaClash 服务:", err)
		os.Exit(1)
	}
}
