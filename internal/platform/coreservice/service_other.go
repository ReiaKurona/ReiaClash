//go:build (!linux && !windows && !darwin) || android

package coreservice

import (
	"context"
	"errors"
)

var errUnsupported = errors.New("当前平台尚未提供特权内核服务安装器")

func Supported() bool { return false }

func RequestInstall(context.Context, string) error   { return errUnsupported }
func RequestUninstall(context.Context, string) error { return errUnsupported }

// These commands are unavailable outside Linux desktop builds.
func InstallPrivileged(string, string) error   { return errUnsupported }
func UninstallPrivileged(string, string) error { return errUnsupported }
