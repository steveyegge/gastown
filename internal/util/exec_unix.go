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

	// exec.Command leaves Cancel nil. Starting in Go 1.25, Start rejects a
	// non-nil Cancel on commands not created with exec.CommandContext.
	// Preserve group-kill behavior only for context-backed commands.
	if cmd.Cancel != nil {
		cmd.Cancel = func() error {
			if cmd.Process != nil {
				return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
			return nil
		}
	}
}
