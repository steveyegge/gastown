// Package deacon provides the Deacon agent infrastructure.
package deacon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// PauseState represents the Deacon pause file contents.
// When paused, the Deacon must not perform any patrol actions.
type PauseState struct {
	// Paused is true if the Deacon is currently paused.
	Paused bool `json:"paused"`

	// Reason explains why the Deacon was paused.
	Reason string `json:"reason,omitempty"`

	// PausedAt is when the Deacon was paused.
	PausedAt time.Time `json:"paused_at"`

	// PausedBy identifies who paused the Deacon (e.g., "human", "mayor").
	PausedBy string `json:"paused_by,omitempty"`
}

// GetPauseFile returns the path to the Deacon pause file.
func GetPauseFile(townRoot string) string {
	return filepath.Join(townRoot, ".runtime", "deacon", "paused.json")
}

// IsPaused checks if the Deacon is currently paused.
// Returns (isPaused, pauseState, error).
// If the pause file doesn't exist, returns (false, nil, nil).
func IsPaused(townRoot string) (bool, *PauseState, error) {
	pauseFile := GetPauseFile(townRoot)

	data, err := os.ReadFile(pauseFile) //nolint:gosec // G304: path is constructed from trusted townRoot
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil, nil
		}
		return false, nil, err
	}

	var state PauseState
	if err := json.Unmarshal(data, &state); err != nil {
		return false, nil, err
	}

	return state.Paused, &state, nil
}

// Pause pauses the Deacon by creating the pause file.
func Pause(townRoot, reason, pausedBy string) error {
	pauseFile := GetPauseFile(townRoot)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(pauseFile), 0755); err != nil {
		return err
	}

	state := PauseState{
		Paused:   true,
		Reason:   reason,
		PausedAt: time.Now().UTC(),
		PausedBy: pausedBy,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(pauseFile, data, 0600)
}

// Resume resumes the Deacon by removing the pause file.
func Resume(townRoot string) error {
	pauseFile := GetPauseFile(townRoot)

	err := os.Remove(pauseFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}
