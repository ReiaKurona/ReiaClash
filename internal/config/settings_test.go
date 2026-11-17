package config

import (
	"testing"
)

func TestDefaults_DisabledByDefault(t *testing.T) {
	d := Defaults()
	if d.Enabled {
		t.Fatalf("首页开关 Enabled 默认必须为 false，防止首次启动即开启代理")
	}
	if !d.UseSystemProxy {
		t.Fatalf("偏好 UseSystemProxy 应当默认开启，作为开关启动时的策略偏好")
	}
	if d.UseTun {
		t.Fatalf("偏好 UseTun 应当默认关闭")
	}
}
