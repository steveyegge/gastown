package beads

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// BdDaemonInfo represents the status of a single bd daemon instance.
type BdDaemonInfo struct {
	Workspace       string `json:"workspace"`
	SocketPath      string `json:"socket_path"`
	PID             int    `json:"pid"`
	Version         string `json:"version"`
	Status          string `json:"status"`
	Issue           string `json:"issue,omitempty"`
	VersionMismatch bool   `json:"version_mismatch,omitempty"`
}

// BdDaemonHealth represents the overall health of bd daemons.
type BdDaemonHealth struct {
	Total        int            `json:"total"`
	Healthy      int            `json:"healthy"`
	Stale        int            `json:"stale"`
	Mismatched   int            `json:"mismatched"`
	Unresponsive int            `json:"unresponsive"`
	Daemons      []BdDaemonInfo `json:"daemons"`
}

// CheckBdDaemonHealth checks the health of all bd daemons.
// Returns nil if no daemons are running (which is fine, bd will use direct mode).
func CheckBdDaemonHealth() (*BdDaemonHealth, error) {
	cmd := exec.Command("bd", "daemon", "health", "--json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// bd daemon health may fail if bd not installed or other issues
		// Return nil to indicate we can't check (not an error for status display)
		return nil, nil
	}

	var health BdDaemonHealth
	if err := json.Unmarshal(stdout.Bytes(), &health); err != nil {
		return nil, fmt.Errorf("parsing daemon health: %w", err)
	}

	return &health, nil
}

// EnsureBdDaemonHealth checks if bd daemons are healthy and attempts to restart if needed.
// Returns a warning message if there were issues, or empty string if everything is fine.
// This is non-blocking - it will not fail if daemons can't be started.
func EnsureBdDaemonHealth(workDir string) string {
	health, err := CheckBdDaemonHealth()
	if err != nil || health == nil {
		// Can't check daemon health - proceed without warning
		return ""
	}

	// No daemons running is fine - bd will use direct mode
	if health.Total == 0 {
		return ""
	}

	// Check if any daemons need attention
	needsRestart := false
	var issues []string

	for _, d := range health.Daemons {
		switch d.Status {
		case "healthy":
			// Good
		case "version_mismatch":
			needsRestart = true
			issues = append(issues, fmt.Sprintf("%s: version mismatch", d.Workspace))
		case "stale":
			needsRestart = true
			issues = append(issues, fmt.Sprintf("%s: stale", d.Workspace))
		case "unresponsive":
			needsRestart = true
			issues = append(issues, fmt.Sprintf("%s: unresponsive", d.Workspace))
		}
	}

	if !needsRestart {
		return ""
	}

	// Attempt to restart daemons
	if restartErr := restartBdDaemons(); restartErr != nil {
		return fmt.Sprintf("bd daemons unhealthy (restart failed: %v)", restartErr)
	}

	// Verify restart worked
	time.Sleep(500 * time.Millisecond)
	newHealth, err := CheckBdDaemonHealth()
	if err != nil || newHealth == nil {
		return "bd daemons restarted but status unknown"
	}

	if newHealth.Healthy < newHealth.Total {
		return fmt.Sprintf("bd daemons partially healthy (%d/%d)", newHealth.Healthy, newHealth.Total)
	}

	return "" // Successfully restarted
}

// restartBdDaemons restarts all bd daemons.
func restartBdDaemons() error { //nolint:unparam // error return kept for future use
	// Stop all daemons first
	stopCmd := exec.Command("bd", "daemon", "killall")
	_ = stopCmd.Run() // Ignore errors - daemons might not be running

	// Give time for cleanup
	time.Sleep(200 * time.Millisecond)

	// Start daemons for known locations
	// The daemon will auto-start when bd commands are run in those directories
	// Just running any bd command will trigger daemon startup if configured
	return nil
}

// StartBdDaemonIfNeeded starts the bd daemon for a specific workspace if not running.
// This is a best-effort operation - failures are logged but don't block execution.
func StartBdDaemonIfNeeded(workDir string) error {
	cmd := exec.Command("bd", "daemon", "--start")
	cmd.Dir = workDir
	return cmd.Run()
}
