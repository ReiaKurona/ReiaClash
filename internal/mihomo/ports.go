package mihomo

import (
	"fmt"
	"net"
)

func checkPorts(addresses ...string) error {
	for _, address := range addresses {
		listener, err := net.Listen("tcp4", address)
		if err != nil {
			return fmt.Errorf("mihomo 端口 %s 已占用或不可绑定，请关闭其他 Clash/mihomo 客户端或实例后重试: %w", address, err)
		}
		listener.Close()
	}
	return nil
}
