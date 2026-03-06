//go:build !windows

// exec_unix.go configures process group IDs on child commands so that
// killing the parent also terminates all descendants, preventing orphans.
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
