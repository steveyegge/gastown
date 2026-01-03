package doctor

import (
	"bytes"
	"os/exec"
	"strings"
)

// BdDaemonCheck verifies that the bd (beads) daemon is running and healthy.
// When the daemon fails to start, it surfaces the actual error (e.g., legacy
// database detected, repo mismatch) and provides actionable fix commands.
type BdDaemonCheck struct {
	FixableCheck
}

// NewBdDaemonCheck creates a new bd daemon check.
func NewBdDaemonCheck() *BdDaemonCheck {
	return &BdDaemonCheck{
		FixableCheck: FixableCheck{
			BaseCheck: BaseCheck{
				CheckName:        "bd-daemon",
				CheckDescription: "Check if bd (beads) daemon is running",
			},
		},
	}
}

// Run checks if the bd daemon is running and healthy.
func (c *BdDaemonCheck) Run(ctx *CheckContext) *CheckResult {
	// Check daemon status
	cmd := exec.Command("bd", "daemon", "--status")
	cmd.Dir = ctx.TownRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := strings.TrimSpace(stdout.String() + stderr.String())

	// Check if daemon is running
	if err == nil && strings.Contains(output, "Daemon is running") {
		// Daemon is running, now check health
		healthCmd := exec.Command("bd", "daemon", "--health")
		healthCmd.Dir = ctx.TownRoot
		var healthOut bytes.Buffer
		healthCmd.Stdout = &healthOut
		_ = healthCmd.Run() // Ignore error, health check is optional

		healthOutput := healthOut.String()
		if strings.Contains(healthOutput, "HEALTHY") {
			return &CheckResult{
				Name:    c.Name(),
				Status:  StatusOK,
				Message: "bd daemon is running and healthy",
			}
		}

		// Daemon running but unhealthy
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusWarning,
			Message: "bd daemon is running but may be unhealthy",
			Details: []string{strings.TrimSpace(healthOutput)},
		}
	}

	// Daemon is not running - try to start it and capture any errors
	startErr := c.tryStartDaemon(ctx)
	if startErr != nil {
		// Parse the error to provide specific guidance
		return c.parseStartError(startErr)
	}

	// Started successfully
	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusOK,
		Message: "bd daemon started successfully",
	}
}

// tryStartDaemon attempts to start the bd daemon and returns any error output.
func (c *BdDaemonCheck) tryStartDaemon(ctx *CheckContext) *startError {
	cmd := exec.Command("bd", "daemon", "--start")
	cmd.Dir = ctx.TownRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return &startError{
			output:   strings.TrimSpace(stdout.String() + stderr.String()),
			exitCode: cmd.ProcessState.ExitCode(),
		}
	}
	return nil
}

// startError holds information about a failed daemon start.
type startError struct {
	output   string
	exitCode int
}

// parseStartError analyzes the error output and returns a helpful CheckResult.
func (c *BdDaemonCheck) parseStartError(err *startError) *CheckResult {
	output := err.output

	// Check for legacy database error
	if strings.Contains(output, "LEGACY DATABASE DETECTED") {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "bd daemon failed: legacy database detected",
			Details: []string{
				"Database was created before bd version 0.17.5",
				"Missing repository fingerprint prevents daemon from starting",
			},
			FixHint: "Run 'bd migrate --update-repo-id' to add fingerprint",
		}
	}

	// Check for database mismatch error
	if strings.Contains(output, "DATABASE MISMATCH DETECTED") {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "bd daemon failed: database belongs to different repository",
			Details: []string{
				"The .beads database was created for a different git repository",
				"This can happen if .beads was copied or if the git remote URL changed",
			},
			FixHint: "Run 'bd migrate --update-repo-id' if URL changed, or 'rm -rf .beads && bd init' for fresh start",
		}
	}

	// Check for already running (not actually an error)
	if strings.Contains(output, "daemon already running") {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusOK,
			Message: "bd daemon is already running",
		}
	}

	// Check for permission/lock errors
	if strings.Contains(output, "lock") || strings.Contains(output, "permission") {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "bd daemon failed: lock or permission issue",
			Details: []string{output},
			FixHint: "Check if another bd daemon is running, or remove .beads/daemon.lock",
		}
	}

	// Check for database corruption
	if strings.Contains(output, "corrupt") || strings.Contains(output, "malformed") {
		return &CheckResult{
			Name:    c.Name(),
			Status:  StatusError,
			Message: "bd daemon failed: database may be corrupted",
			Details: []string{output},
			FixHint: "Run 'bd repair' or 'rm .beads/issues.db && bd sync --from-main'",
		}
	}

	// Generic error with full output
	details := []string{output}
	if output == "" {
		details = []string{"No error output captured (exit code " + string(rune('0'+err.exitCode)) + ")"}
	}

	return &CheckResult{
		Name:    c.Name(),
		Status:  StatusError,
		Message: "bd daemon failed to start",
		Details: details,
		FixHint: "Check 'bd daemon --status' and logs in .beads/daemon.log",
	}
}

// Fix attempts to start the bd daemon.
func (c *BdDaemonCheck) Fix(ctx *CheckContext) error {
	// First check if it's a legacy database issue
	startErr := c.tryStartDaemon(ctx)
	if startErr == nil {
		return nil
	}

	// If legacy database, run migrate first
	if strings.Contains(startErr.output, "LEGACY DATABASE") ||
		strings.Contains(startErr.output, "DATABASE MISMATCH") {

		migrateCmd := exec.Command("bd", "migrate", "--update-repo-id", "--yes")
		migrateCmd.Dir = ctx.TownRoot
		if err := migrateCmd.Run(); err != nil {
			return err
		}

		// Try starting again
		startCmd := exec.Command("bd", "daemon", "--start")
		startCmd.Dir = ctx.TownRoot
		return startCmd.Run()
	}

	// For other errors, just try to start
	startCmd := exec.Command("bd", "daemon", "--start")
	startCmd.Dir = ctx.TownRoot
	return startCmd.Run()
}
