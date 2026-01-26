//go:build windows

package daemon

import (
	"os"
	"os/exec"
)

// setSysProcAttr sets platform-specific process attributes.
// On Windows, no special attributes needed for process group detachment.
func setSysProcAttr(cmd *exec.Cmd) {
	// No-op on Windows - process will run independently
}

// isProcessAlive checks if a process is still running.
// On Windows, we try to open the process with minimal access.
func isProcessAlive(p *os.Process) bool {
	// On Windows, FindProcess always succeeds, and Signal(0) may not work.
	// The best we can do is try to signal and see if it fails.
	// A killed process will return an error.
	err := p.Signal(os.Signal(nil))
	// If err is nil or "not supported", process may still be alive
	// If err mentions "finished" or similar, process is dead
	return err == nil
}

// sendTermSignal sends a termination signal.
// On Windows, there's no SIGTERM - we use Kill() directly.
func sendTermSignal(p *os.Process) error {
	return p.Kill()
}

// sendKillSignal sends a kill signal.
// On Windows, Kill() is the only option.
func sendKillSignal(p *os.Process) error {
	return p.Kill()
}
