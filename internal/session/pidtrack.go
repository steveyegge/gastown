package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/steveyegge/gastown/internal/tmux"
)

// pidsDir returns the directory for PID tracking files.
// All PID files live under <townRoot>/.runtime/pids/ since tmux session
// names are globally unique (they include the rig name).
func pidsDir(townRoot string) string {
	return filepath.Join(townRoot, ".runtime", "pids")
}

// pidFile returns the path to a PID file for a given session.
func pidFile(townRoot, sessionID string) string {
	return filepath.Join(pidsDir(townRoot), sessionID+".pid")
}

// TrackSessionPID captures the pane PID of a tmux session and writes it
// to a PID tracking file. This is defense-in-depth: if a session dies
// unexpectedly and KillSessionWithProcesses can't find the tmux pane,
// we still have the PID on disk for cleanup.
//
// This is best-effort — errors are returned but callers should treat them
// as non-fatal since the primary kill mechanism (KillSessionWithProcesses)
// doesn't depend on PID files.
func TrackSessionPID(townRoot, sessionID string, t *tmux.Tmux) error {
	pidStr, err := t.GetPanePID(sessionID)
	if err != nil {
		return fmt.Errorf("getting pane PID: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(pidStr))
	if err != nil {
		return fmt.Errorf("parsing PID %q: %w", pidStr, err)
	}

	return TrackPID(townRoot, sessionID, pid)
}

// TrackPID writes a PID to a tracking file for later cleanup.
func TrackPID(townRoot, sessionID string, pid int) error {
	dir := pidsDir(townRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating pids directory: %w", err)
	}

	path := pidFile(townRoot, sessionID)
	return os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0644)
}

// UntrackPID removes the PID tracking file for a session.
func UntrackPID(townRoot, sessionID string) {
	_ = os.Remove(pidFile(townRoot, sessionID))
}

// KillTrackedPIDs reads all PID files and kills any processes that are
// still running. Returns the number of processes killed and any session
// names that had errors.
//
// This is designed for the shutdown orphan-cleanup phase: after all
// sessions have been killed through normal means, this catches any
// processes that survived (e.g., reparented to init after SIGHUP).
func KillTrackedPIDs(townRoot string) (killed int, errSessions []string) {
	dir := pidsDir(townRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, []string{fmt.Sprintf("read pids dir: %v", err)}
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pid") {
			continue
		}

		sessionID := strings.TrimSuffix(entry.Name(), ".pid")
		path := filepath.Join(dir, entry.Name())

		data, err := os.ReadFile(path)
		if err != nil {
			errSessions = append(errSessions, fmt.Sprintf("%s: read error: %v", sessionID, err))
			continue
		}

		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			// Corrupt PID file — remove it
			_ = os.Remove(path)
			continue
		}

		// Check if process is still alive
		proc, err := os.FindProcess(pid)
		if err != nil {
			_ = os.Remove(path)
			continue
		}

		// Signal 0 checks existence without killing
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			// Process is already dead — clean up PID file
			_ = os.Remove(path)
			continue
		}

		// Process is alive — kill it
		if err := proc.Signal(syscall.SIGTERM); err != nil {
			errSessions = append(errSessions, fmt.Sprintf("%s (PID %d): SIGTERM failed: %v", sessionID, pid, err))
		} else {
			killed++
		}

		// Clean up PID file regardless
		_ = os.Remove(path)
	}

	return killed, errSessions
}
