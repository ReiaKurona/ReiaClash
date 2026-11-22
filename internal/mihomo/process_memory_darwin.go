//go:build darwin

package mihomo

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func processMemory(pid int) (uint64, bool) {
	if pid <= 0 {
		return 0, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	data, err := exec.CommandContext(ctx, "/bin/ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, false
	}
	kib, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	return kib * 1024, err == nil
}
