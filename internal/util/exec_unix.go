//go:build !windows

package util

import (
	"os/exec"
	"syscall"
)

// SetProcessGroup configures a command to run in its own process group so that
// context cancellation kills the entire process tree, preventing orphaned children.
func SetProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
}

// SetDetachedProcessGroup configures a command to run in its own session
// (Setsid) and process group (Setpgid), fully detaching it from the parent.
// Without Setsid, the child shares a session with the parent and receives
// SIGHUP when the parent's terminal exits — causing daemon shutdown.
func SetDetachedProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Setsid: true}
}
