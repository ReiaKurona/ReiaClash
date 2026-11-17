package instance

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestSingleInstanceAcrossProcesses(t *testing.T) {
	role := os.Getenv("REIA_LOCK_TEST_ROLE")
	if role != "" {
		lock, err := Acquire(role)
		if errors.Is(err, ErrAlreadyRunning) {
			return
		}
		if lock != nil {
			lock.Close()
		}
		t.Fatalf("child acquired an existing lock: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	role = fmt.Sprintf("test-%d-%d", os.Getpid(), time.Now().UnixNano())
	lock, err := Acquire(role)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	command := exec.Command(os.Args[0], "-test.run=^TestSingleInstanceAcrossProcesses$")
	command.Env = append(os.Environ(), "REIA_LOCK_TEST_ROLE="+role)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := Acquire(role)
	if err != nil {
		t.Fatal(err)
	}
	next.Close()
}

func TestKernelLeaseAcrossProcesses(t *testing.T) {
	directory := os.Getenv("REIA_KERNEL_LEASE_TEST_DIR")
	if directory != "" {
		lock, err := AcquireIn(directory, "kernel")
		if errors.Is(err, ErrAlreadyRunning) {
			return
		}
		if lock != nil {
			lock.Close()
		}
		t.Fatalf("second process acquired kernel lease: %v", err)
	}
	directory = t.TempDir()
	lock, err := AcquireIn(directory, "kernel")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	command := exec.Command(os.Args[0], "-test.run=^TestKernelLeaseAcrossProcesses$")
	command.Env = append(os.Environ(), "REIA_KERNEL_LEASE_TEST_DIR="+directory)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, output)
	}
}
