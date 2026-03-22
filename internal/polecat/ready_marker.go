package polecat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// ReadyMarkerFreshnessThreshold is the maximum age of a ready marker before
// it's considered stale. A stale marker indicates a leftover from a previous
// session rather than a fresh signal from the current SessionStart hook.
const ReadyMarkerFreshnessThreshold = 5 * time.Minute

type readyMarker struct {
	Timestamp time.Time `json:"timestamp"`
}

func readyMarkerDir(townRoot string) string {
	return filepath.Join(townRoot, ".runtime", "ready")
}

func readyMarkerFile(townRoot, sessionName string) string {
	return filepath.Join(readyMarkerDir(townRoot), sessionName+".ready")
}

// WriteReadyMarker writes a ready marker file for a polecat session.
// Called by the SessionStart hook (gt prime --hook) to signal that the hook
// ran successfully and the agent is ready to receive work.
func WriteReadyMarker(townRoot, sessionName string) error {
	dir := readyMarkerDir(townRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	m := readyMarker{Timestamp: time.Now().UTC()}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}

	return os.WriteFile(readyMarkerFile(townRoot, sessionName), data, 0644)
}

// ReadyMarkerExists returns true if a fresh ready marker exists for the session.
// Returns false if the marker is missing, unreadable, or older than
// ReadyMarkerFreshnessThreshold.
func ReadyMarkerExists(townRoot, sessionName string) bool {
	data, err := os.ReadFile(readyMarkerFile(townRoot, sessionName))
	if err != nil {
		return false
	}

	var m readyMarker
	if err := json.Unmarshal(data, &m); err != nil {
		return false
	}

	return time.Since(m.Timestamp) < ReadyMarkerFreshnessThreshold
}

// RemoveReadyMarker removes the ready marker file for a session.
// Called during session cleanup. Best-effort: errors are silently ignored.
func RemoveReadyMarker(townRoot, sessionName string) {
	_ = os.Remove(readyMarkerFile(townRoot, sessionName))
}
