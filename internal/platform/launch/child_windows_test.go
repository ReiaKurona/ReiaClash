//go:build windows

package launch

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestJobClosesOwnedChild(t *testing.T) {
	if os.Getenv("REIA_JOB_TEST_CHILD") == "1" {
		time.Sleep(30 * time.Second)
		return
	}
	command := exec.Command(os.Args[0], "-test.run=^TestJobClosesOwnedChild$")
	command.Env = append(os.Environ(), "REIA_JOB_TEST_CHILD=1")
	HideCommand(command)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Process.Kill()
	cleanup, err := TrackChildProcess(command)
	if err != nil {
		command.Process.Kill()
		command.Wait()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	cleanup()
	select {
	case <-done:
		// Windows may use exit code zero when a Job is closed. Exiting
		// before the child's 30-second sleep completes proves ownership.
	case <-time.After(3 * time.Second):
		t.Fatal("owned child survived service Job closure")
	}
}
