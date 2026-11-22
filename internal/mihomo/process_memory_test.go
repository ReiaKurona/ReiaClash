//go:build windows || linux || darwin

package mihomo

import (
	"os"
	"testing"
)

func TestProcessMemoryReportsWorkingSet(t *testing.T) {
	if size, ok := processMemory(os.Getpid()); !ok || size == 0 {
		t.Fatal("could not read the current process's resident memory")
	}
}
