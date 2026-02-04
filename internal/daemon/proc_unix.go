//go:build unix

package daemon

import (
	"os"
	"os/exec"
	"syscall"
)

// setSysProcAttr sets platform-specific process attributes.
// On Unix, we detach from the process group so the server survives daemon restart.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// isProcessAlive checks if a process is still running.
func isProcessAlive(p *os.Process) bool {
	return p.Signal(syscall.Signal(0)) == nil
}

// sendTermSignal sends SIGTERM for graceful shutdown.
func sendTermSignal(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}

// sendKillSignal sends SIGKILL for forced termination.
func sendKillSignal(p *os.Process) error {
	return p.Signal(syscall.SIGKILL)
}
