package beads

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	gracefulTimeout = 2 * time.Second
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
	for _, d := range health.Daemons {
		switch d.Status {
		case "healthy":
			// Good
		case "version_mismatch", "stale", "unresponsive":
			needsRestart = true
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

// StopAllBdProcesses stops all bd daemon and activity processes.
// Returns (daemonsKilled, activityKilled, error).
// If dryRun is true, returns counts without stopping anything.
func StopAllBdProcesses(dryRun, force bool) (int, int, error) {
	if _, err := exec.LookPath("bd"); err != nil {
		return 0, 0, nil
	}

	daemonsBefore := CountBdDaemons()
	activityBefore := CountBdActivityProcesses()

	if dryRun {
		return daemonsBefore, activityBefore, nil
	}

	daemonsKilled, daemonsRemaining := stopBdDaemons(force)
	activityKilled, activityRemaining := stopBdActivityProcesses(force)

	if daemonsRemaining > 0 {
		return daemonsKilled, activityKilled, fmt.Errorf("bd daemon shutdown incomplete: %d still running", daemonsRemaining)
	}
	if activityRemaining > 0 {
		return daemonsKilled, activityKilled, fmt.Errorf("bd activity shutdown incomplete: %d still running", activityRemaining)
	}

	return daemonsKilled, activityKilled, nil
}

// CountBdDaemons returns count of running bd daemons.
func CountBdDaemons() int {
	listCmd := exec.Command("bd", "daemon", "list", "--json")
	output, err := listCmd.Output()
	if err != nil {
		return 0
	}
	return parseBdDaemonCount(output)
}

// parseBdDaemonCount parses bd daemon list --json output.
func parseBdDaemonCount(output []byte) int {
	if len(output) == 0 {
		return 0
	}

	var daemons []any
	if err := json.Unmarshal(output, &daemons); err == nil {
		return len(daemons)
	}

	var wrapper struct {
		Daemons []any `json:"daemons"`
		Count   int   `json:"count"`
	}
	if err := json.Unmarshal(output, &wrapper); err == nil {
		if wrapper.Count > 0 {
			return wrapper.Count
		}
		return len(wrapper.Daemons)
	}

	return 0
}

func stopBdDaemons(force bool) (int, int) {
	before := CountBdDaemons()
	if before == 0 {
		return 0, 0
	}

	killCmd := exec.Command("bd", "daemon", "killall")
	_ = killCmd.Run()

	time.Sleep(100 * time.Millisecond)

	after := CountBdDaemons()
	if after == 0 {
		return before, 0
	}

	// Note: pkill -f pattern may match unintended processes in rare cases
	// (e.g., editors with "bd daemon" in file content). This is acceptable
	// as a fallback when bd daemon killall fails.
	if force {
		_ = exec.Command("pkill", "-9", "-f", "bd daemon").Run()
	} else {
		_ = exec.Command("pkill", "-TERM", "-f", "bd daemon").Run()
		time.Sleep(gracefulTimeout)
		if remaining := CountBdDaemons(); remaining > 0 {
			_ = exec.Command("pkill", "-9", "-f", "bd daemon").Run()
		}
	}

	time.Sleep(100 * time.Millisecond)

	final := CountBdDaemons()
	killed := before - final
	if killed < 0 {
		killed = 0 // Race condition: more processes spawned than we killed
	}
	return killed, final
}

// CountBdActivityProcesses returns count of running `bd activity` processes.
func CountBdActivityProcesses() int {
	// Use pgrep -f with wc -l for cross-platform compatibility
	// (macOS pgrep doesn't support -c flag)
	cmd := exec.Command("sh", "-c", "pgrep -f 'bd activity' 2>/dev/null | wc -l")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	count, _ := strconv.Atoi(strings.TrimSpace(string(output)))
	return count
}

func stopBdActivityProcesses(force bool) (int, int) {
	before := CountBdActivityProcesses()
	if before == 0 {
		return 0, 0
	}

	if force {
		_ = exec.Command("pkill", "-9", "-f", "bd activity").Run()
	} else {
		_ = exec.Command("pkill", "-TERM", "-f", "bd activity").Run()
		time.Sleep(gracefulTimeout)
		if remaining := CountBdActivityProcesses(); remaining > 0 {
			_ = exec.Command("pkill", "-9", "-f", "bd activity").Run()
		}
	}

	time.Sleep(100 * time.Millisecond)

	after := CountBdActivityProcesses()
	killed := before - after
	if killed < 0 {
		killed = 0 // Race condition: more processes spawned than we killed
	}
	return killed, after
}
