//go:build linux

package mihomo

import (
	"os"
	"strconv"
	"strings"
)

func processMemory(pid int) (uint64, bool) {
	if pid <= 0 {
		return 0, false
	}
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/statm")
	fields := strings.Fields(string(data))
	if err != nil || len(fields) < 2 {
		return 0, false
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	return pages * uint64(os.Getpagesize()), err == nil
}
