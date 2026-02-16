//go:build windows

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// setSysProcAttr sets platform-specific process attributes.
// On Windows, no special attributes needed for process group detachment.
func setSysProcAttr(cmd *exec.Cmd) {
	// No-op on Windows - process will run independently
}

// isProcessAlive checks if a process is still running.
// On Windows, we try to open the process with minimal access.
func isProcessAlive(p *os.Process) bool {
	if p == nil || p.Pid <= 0 {
		return false
	}

	cmd := exec.Command("tasklist",
		"/FI", fmt.Sprintf("PID eq %d", p.Pid),
		"/FO", "CSV",
		"/NH",
	)
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	line := strings.TrimSpace(string(output))
	if line == "" {
		return false
	}

	return !strings.Contains(strings.ToLower(line), "no tasks are running")
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
