//go:build !windows

package execx

import (
	"os"
	"os/exec"
	"syscall"
)

func applyPlatformOptions(cmd *exec.Cmd, cfg config) {
	if !cfg.killProcessGroup {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func defaultShell() (string, []string) {
	return "sh", []string{"-c"}
}

func terminateCommand(cmd *exec.Cmd, group bool) error {
	return signalCommand(cmd, syscall.SIGTERM, group)
}

func killCommand(cmd *exec.Cmd, group bool) error {
	return signalCommand(cmd, syscall.SIGKILL, group)
}

func signalCommand(cmd *exec.Cmd, sig os.Signal, group bool) error {
	if cmd == nil || cmd.Process == nil {
		return ErrProcessNotRunning
	}
	sysSig, ok := sig.(syscall.Signal)
	if !ok {
		return cmd.Process.Signal(sig)
	}
	if group {
		return syscall.Kill(-cmd.Process.Pid, sysSig)
	}
	return cmd.Process.Signal(sig)
}
