//go:build unix

package advice

import (
	"os/exec"
	"syscall"
)

// setSysProcAttr sets platform-specific process attributes for hook execution.
// On Unix, we set up a new process group so we can kill child processes on timeout.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup kills the entire process group for proper child process cleanup.
// On Unix, we send SIGKILL to the negative PID (process group).
func killProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}
