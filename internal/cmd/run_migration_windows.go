//go:build windows

package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
)

// setMigrationProcAttr sets a Cancel function on Windows that kills the
// process tree using taskkill /T (tree kill).
func setMigrationProcAttr(c *exec.Cmd) {
	c.Cancel = func() error {
		// taskkill /F /T /PID kills the process and all its children.
		kill := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(c.Process.Pid))
		if err := kill.Run(); err != nil {
			return fmt.Errorf("taskkill failed: %w", err)
		}
		return nil
	}
}

// migrationShellCmd returns an exec.Cmd that runs cmdStr in cmd.exe.
func migrationShellCmd(ctx context.Context, cmdStr string) *exec.Cmd {
	return exec.CommandContext(ctx, "cmd", "/c", cmdStr)
}
