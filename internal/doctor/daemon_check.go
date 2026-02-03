package doctor

import (
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/steveyegge/gastown/internal/daemon"
)

// DaemonCheck verifies the daemon is running.
type DaemonCheck struct {
	FixableCheck
}

// NewDaemonCheck creates a new daemon check.
func NewDaemonCheck() *DaemonCheck {
	return &DaemonCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "daemon",
				CheckDescription: "Check if Gas Town daemon is running",
				CheckCategory:    CategoryInfrastructure,
			},
		},
	}
}

// Run checks if the daemon is running.
func (c *DaemonCheck) Run(ctx *CheckContext) *CheckResult {
	running, pid, err := daemon.IsRunning(ctx.TownRoot)
	if err != nil {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "Failed to check daemon status",
			Details: []string{err.Error()},
		}
	}

	if running {
		// Get more info about daemon state
		state, err := daemon.LoadState(ctx.TownRoot)
		details := []string{}
		if err == nil && !state.StartedAt.IsZero() {
			uptime := time.Since(state.StartedAt).Round(time.Second)
			details = append(details, "Uptime: "+uptime.String())
			if state.HeartbeatCount > 0 {
				details = append(details, "Heartbeats: "+strconv.Itoa(int(state.HeartbeatCount)))
			}
		}

		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "Daemon is running (PID " + strconv.Itoa(pid) + ")",
			Details: details,
		}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusWarning,
		Message: "Daemon is not running",
		FixHint: "Run 'gt daemon start' or 'gt doctor --fix'",
	}
}

// Fix starts the daemon.
func (c *DaemonCheck) Fix(ctx *CheckContext) error {
	// Find gt executable
	gtPath, err := os.Executable()
	if err != nil {
		return err
	}

	// Start daemon in background (detach from parent I/O - daemon uses its own logging)
	cmd := exec.Command(gtPath, "daemon", "run")
	cmd.Dir = ctx.TownRoot
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return err
	}

	// Wait a moment for daemon to initialize
	time.Sleep(300 * time.Millisecond)

	return nil
}
