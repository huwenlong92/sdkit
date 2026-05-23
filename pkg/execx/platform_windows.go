//go:build windows

package execx

import (
	"os"
	"os/exec"
)

func applyPlatformOptions(cmd *exec.Cmd, cfg config) {}

func defaultShell() (string, []string) {
	return "cmd", []string{"/C"}
}

func terminateCommand(cmd *exec.Cmd, group bool) error {
	if cmd == nil || cmd.Process == nil {
		return ErrProcessNotRunning
	}
	return cmd.Process.Kill()
}

func killCommand(cmd *exec.Cmd, group bool) error {
	if cmd == nil || cmd.Process == nil {
		return ErrProcessNotRunning
	}
	return cmd.Process.Kill()
}

func signalCommand(cmd *exec.Cmd, sig os.Signal, group bool) error {
	if cmd == nil || cmd.Process == nil {
		return ErrProcessNotRunning
	}
	return cmd.Process.Signal(sig)
}
