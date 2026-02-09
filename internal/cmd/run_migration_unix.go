//go:build !windows

package cmd

import (
	"context"
	"os/exec"
	"syscall"
)

// setMigrationProcAttr puts the process in its own process group so we can
// kill all children on timeout, not just the bash process.
func setMigrationProcAttr(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		return syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
}

// migrationShellCmd returns an exec.Cmd that runs cmdStr in a shell.
func migrationShellCmd(ctx context.Context, cmdStr string) *exec.Cmd {
	return exec.CommandContext(ctx, "bash", "-c", cmdStr)
}
