//go:build windows

package coreservice

import (
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreationTimeUsesCIMPrecision(t *testing.T) {
	if !sameCreationTime(134392002655370568, 134392002655370560) {
		t.Fatal("CIM truncation rejected the actual orphan")
	}
	if sameCreationTime(134392002655370578, 134392002655370560) || sameCreationTime(0, 0) {
		t.Fatal("PID reuse/unknown creation time was accepted")
	}
}

func TestTerminateVerifiedOwnedChild(t *testing.T) {
	if os.Getenv("REIA_CLEANUP_TEST_CHILD") == "1" {
		time.Sleep(30 * time.Second)
		return
	}
	command := exec.Command(os.Args[0], "-test.run=^TestTerminateVerifiedOwnedChild$")
	command.Env = append(os.Environ(), "REIA_CLEANUP_TEST_CHILD=1")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Process.Kill()
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(command.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	var created, exited, kernel, user windows.Filetime
	err = windows.GetProcessTimes(handle, &created, &exited, &kernel, &user)
	windows.CloseHandle(handle)
	if err != nil {
		t.Fatal(err)
	}
	native := uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := terminateVerifiedProcess(processRecord{PID: uint32(command.Process.Pid), Path: executable, Created: native / 10 * 10}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("verified child survived cleanup")
	}
}

func TestCleanupRequiresExactProjectIdentity(t *testing.T) {
	installation := `C:\ReiaClash`
	root := `C:\Users\demo\AppData\Roaming\ReiaClash`
	binary := filepath.Join(installation, "mihomo.exe")
	args := []string{binary, "-d", filepath.Join(installation, "data"), "-f", filepath.Join(root, "mihomo.yaml")}
	valid := processRecord{Path: binary, CommandLine: windows.ComposeCommandLine(args)}
	if !ownedProcess(valid, root, installation, binary) {
		t.Fatal("owned orphan was not recognized")
	}
	for _, changed := range []processRecord{
		{Path: binary, CommandLine: windows.ComposeCommandLine([]string{binary, "-f", `C:\OtherClash\mihomo.yaml`, "-d", filepath.Join(installation, "data")})},
		{Path: binary, CommandLine: windows.ComposeCommandLine([]string{binary, "-f", filepath.Join(root, "mihomo.yaml"), "-d", `C:\OtherClash\data`})},
		{Path: `C:\OtherClash\mihomo.exe`, CommandLine: strings.ReplaceAll(valid.CommandLine, binary, `C:\OtherClash\mihomo.exe`)},
		{Path: filepath.Join(installation, "reia-agent.exe"), CommandLine: windows.ComposeCommandLine([]string{filepath.Join(installation, "reia-agent.exe"), "--background"})},
		{Path: filepath.Join(installation, "reia-agent.exe"), CommandLine: windows.ComposeCommandLine([]string{filepath.Join(installation, "reia-agent.exe"), "--core-service", "--data-dir", `C:\OtherClash`})},
	} {
		if ownedProcess(changed, root, installation, binary) {
			t.Fatalf("cleanup accepted unrelated process: %+v", changed)
		}
	}
	helper := filepath.Join(installation, "reia-agent.exe")
	if !ownedProcess(processRecord{Path: helper, CommandLine: windows.ComposeCommandLine([]string{helper, "--core-service", "--data-dir", root})}, root, installation, binary) {
		t.Fatal("legacy service helper was not recognized")
	}
}
