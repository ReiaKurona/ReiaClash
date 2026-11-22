//go:build !windows && !linux && !darwin

package mihomo

func processMemory(int) (uint64, bool) { return 0, false }
