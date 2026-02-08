// Package ratelimit provides rate limit state management for Claude Pro/Max subscriptions.
//
// When Claude Code sessions hit API rate limits, they stop processing. This package
// provides a mechanism to record when rate limits are hit, when they reset, and allows
// the daemon to automatically wake agents when the rate limit period ends.
//
// Rate limit state is stored in <townRoot>/.runtime/ratelimit/state.json and is
// checked by the daemon on each heartbeat cycle.
package ratelimit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// State represents the current rate limit state.
// When a rate limit is active, ResetAt indicates when it should clear.
type State struct {
	// Active is true if a rate limit is currently in effect.
	Active bool `json:"active"`

	// ResetAt is when the rate limit is expected to reset.
	// The daemon will attempt to wake agents after this time.
	ResetAt time.Time `json:"reset_at"`

	// RecordedAt is when this rate limit was recorded.
	RecordedAt time.Time `json:"recorded_at"`

	// RecordedBy identifies who/what recorded the rate limit.
	// Could be "claude", "deacon", "polecat/name", etc.
	RecordedBy string `json:"recorded_by,omitempty"`

	// Reason provides additional context about the rate limit.
	// e.g., "API rate limit exceeded", "Claude Pro limit reached"
	Reason string `json:"reason,omitempty"`

	// RetryAfterSeconds is the original retry-after value from the API.
	RetryAfterSeconds int `json:"retry_after_seconds,omitempty"`

	// WakeAttempts tracks how many times we've tried to wake after reset.
	// Used to prevent infinite wake loops if rate limit persists.
	WakeAttempts int `json:"wake_attempts,omitempty"`

	// LastWakeAttempt is when we last tried to wake agents.
	LastWakeAttempt time.Time `json:"last_wake_attempt,omitempty"`
}

// GetStateFile returns the path to the rate limit state file.
func GetStateFile(townRoot string) string {
	return filepath.Join(townRoot, ".runtime", "ratelimit", "state.json")
}

// GetState reads the current rate limit state.
// Returns nil if no state file exists (no active rate limit).
func GetState(townRoot string) (*State, error) {
	stateFile := GetStateFile(townRoot)

	data, err := os.ReadFile(stateFile) //nolint:gosec // G304: path is constructed from trusted townRoot
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

// SaveState writes the rate limit state to disk.
func SaveState(townRoot string, state *State) error {
	stateFile := GetStateFile(townRoot)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(stateFile), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(stateFile, data, 0600)
}

// RecordRateLimit records that a rate limit has been hit.
// resetAfter is the duration until the rate limit resets (from Retry-After header).
// recordedBy identifies who recorded this (e.g., "deacon", "gastown/Toast").
// reason provides additional context about the rate limit.
func RecordRateLimit(townRoot string, resetAfter time.Duration, recordedBy, reason string) error {
	now := time.Now().UTC()
	state := &State{
		Active:            true,
		ResetAt:           now.Add(resetAfter),
		RecordedAt:        now,
		RecordedBy:        recordedBy,
		Reason:            reason,
		RetryAfterSeconds: int(resetAfter.Seconds()),
	}

	return SaveState(townRoot, state)
}

// Clear removes the rate limit state file.
// Called when the rate limit has reset and agents have been woken.
func Clear(townRoot string) error {
	stateFile := GetStateFile(townRoot)

	err := os.Remove(stateFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// IsRateLimited checks if a rate limit is currently active.
// Returns (isLimited, state, error).
// If rate limit has expired (current time > ResetAt), returns false.
func IsRateLimited(townRoot string) (bool, *State, error) {
	state, err := GetState(townRoot)
	if err != nil {
		return false, nil, err
	}

	if state == nil || !state.Active {
		return false, nil, nil
	}

	// Check if rate limit has expired
	if time.Now().After(state.ResetAt) {
		return false, state, nil // Expired but state still exists for reference
	}

	return true, state, nil
}

// ShouldWake checks if it's time to wake agents after a rate limit.
// Returns true if:
// 1. A rate limit state exists
// 2. The reset time has passed
// 3. We haven't exceeded max wake attempts
//
// Also updates the wake attempt tracking in the state file.
func ShouldWake(townRoot string) (bool, *State, error) {
	state, err := GetState(townRoot)
	if err != nil {
		return false, nil, err
	}

	if state == nil || !state.Active {
		return false, nil, nil
	}

	now := time.Now()

	// Rate limit hasn't reset yet
	if now.Before(state.ResetAt) {
		return false, state, nil
	}

	// Limit wake attempts to prevent infinite loops
	const maxWakeAttempts = 3
	if state.WakeAttempts >= maxWakeAttempts {
		// Too many attempts - require manual intervention
		return false, state, nil
	}

	// Don't wake too frequently - wait at least 2 minutes between attempts
	const minWakeInterval = 2 * time.Minute
	if !state.LastWakeAttempt.IsZero() && now.Sub(state.LastWakeAttempt) < minWakeInterval {
		return false, state, nil
	}

	return true, state, nil
}

// RecordWakeAttempt updates the state to reflect a wake attempt.
// Called by the daemon when it tries to wake agents after rate limit reset.
func RecordWakeAttempt(townRoot string) error {
	state, err := GetState(townRoot)
	if err != nil {
		return err
	}

	if state == nil {
		return nil // No state to update
	}

	state.WakeAttempts++
	state.LastWakeAttempt = time.Now().UTC()

	return SaveState(townRoot, state)
}

// TimeUntilReset returns the duration until the rate limit resets.
// Returns 0 if no active rate limit or if already past reset time.
func TimeUntilReset(townRoot string) (time.Duration, error) {
	state, err := GetState(townRoot)
	if err != nil {
		return 0, err
	}

	if state == nil || !state.Active {
		return 0, nil
	}

	remaining := time.Until(state.ResetAt)
	if remaining < 0 {
		return 0, nil
	}

	return remaining, nil
}
