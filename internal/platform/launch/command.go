package launch

import (
	"context"
	"os"
	"os/exec"
)

func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, name, args...)
	HideCommand(command)
	return command
}

// Keep errors that occur before the agent can open its own logger.
func StartBackgroundLogged(executable, logPath string, args ...string) error {
	output, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer output.Close()
	command := exec.Command(executable, args...)
	command.Stdout, command.Stderr = output, output
	configureBackground(command)
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}

// Command prevents short-lived platform tools from flashing console windows.
func Command(name string, args ...string) *exec.Cmd {
	command := exec.Command(name, args...)
	HideCommand(command)
	return command
}
