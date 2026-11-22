//go:build (linux || darwin) && !android

package launch

import "os/exec"

// systemd/launchd own the service process group on Unix desktop platforms.
func TrackChildProcess(command *exec.Cmd) (func(), error) { return func() {}, nil }

func TrackDesktopChildProcess(command *exec.Cmd) (func(), error) { return TrackChildProcess(command) }
