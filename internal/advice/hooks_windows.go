//go:build windows

package advice

import (
	"os"
	"os/exec"
)

// setSysProcAttr sets platform-specific process attributes for hook execution.
// On Windows, no special attributes needed for process group handling.
func setSysProcAttr(cmd *exec.Cmd) {
	// No-op on Windows
}

// killProcessGroup kills the process.
// On Windows, we can only kill the main process, not the entire process tree.
// Child processes may remain - this is a known limitation.
func killProcessGroup(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
