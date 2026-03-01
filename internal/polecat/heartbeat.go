package polecat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// SessionHeartbeatStaleThreshold is the age at which a polecat session heartbeat
// is considered stale, indicating the agent process is likely dead.
// Configurable via operational.polecat.heartbeat_stale_threshold in settings/config.json.
const SessionHeartbeatStaleThreshold = 3 * time.Minute

// SessionHeartbeat represents a polecat session's heartbeat file.
type SessionHeartbeat struct {
	Timestamp time.Time `json:"timestamp"`
}

// heartbeatsDir returns the directory for polecat session heartbeat files.
// Heartbeats live under <townRoot>/.runtime/heartbeats/, parallel to .runtime/pids/.
func heartbeatsDir(townRoot string) string {
	return filepath.Join(townRoot, ".runtime", "heartbeats")
}

// heartbeatFile returns the path to a heartbeat file for a given session.
func heartbeatFile(townRoot, sessionName string) string {
	return filepath.Join(heartbeatsDir(townRoot), sessionName+".json")
}

// TouchSessionHeartbeat writes or updates the heartbeat file for a polecat session.
// This is best-effort: errors are silently ignored because heartbeat signals
// are non-critical and should not interrupt gt commands.
func TouchSessionHeartbeat(townRoot, sessionName string) {
	dir := heartbeatsDir(townRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}

	hb := SessionHeartbeat{
		Timestamp: time.Now().UTC(),
	}

	data, err := json.Marshal(hb)
	if err != nil {
		return
	}

	_ = os.WriteFile(heartbeatFile(townRoot, sessionName), data, 0644)
}

// ReadSessionHeartbeat reads the heartbeat for a polecat session.
// Returns nil if the file doesn't exist or can't be read.
func ReadSessionHeartbeat(townRoot, sessionName string) *SessionHeartbeat {
	data, err := os.ReadFile(heartbeatFile(townRoot, sessionName))
	if err != nil {
		return nil
	}

	var hb SessionHeartbeat
	if err := json.Unmarshal(data, &hb); err != nil {
		return nil
	}

	return &hb
}

// IsSessionHeartbeatStale returns true if the session's heartbeat is older than
// the stale threshold, or if no heartbeat file exists.
//
// When no heartbeat file exists, this returns false to avoid false positives
// during the rollout period where sessions may not yet be touching heartbeats.
// The caller should fall back to other liveness checks in that case.
func IsSessionHeartbeatStale(townRoot, sessionName string) (stale bool, exists bool) {
	hb := ReadSessionHeartbeat(townRoot, sessionName)
	if hb == nil {
		return false, false
	}
	return time.Since(hb.Timestamp) >= SessionHeartbeatStaleThreshold, true
}

// RemoveSessionHeartbeat removes the heartbeat file for a session.
// Called during session cleanup.
func RemoveSessionHeartbeat(townRoot, sessionName string) {
	_ = os.Remove(heartbeatFile(townRoot, sessionName))
}
