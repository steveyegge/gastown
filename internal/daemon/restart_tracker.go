package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RestartTracker tracks polecat restart attempts with exponential backoff.
// It persists restart history to detect crash loops and enforce backoff delays.
type RestartTracker struct {
	townRoot string
	mu       sync.RWMutex
	state    *RestartState
}

// RestartState holds the restart history for all polecats.
// It is persisted to disk to survive daemon restarts.
type RestartState struct {
	// Polecats maps polecat ID (rig/polecat) to restart tracking info
	Polecats map[string]*PolecatRestarts `json:"polecats"`
}

// PolecatRestarts tracks restart information for a single polecat.
type PolecatRestarts struct {
	// PolecatID is the polecat identifier (rig/polecat format)
	PolecatID string `json:"polecat_id"`

	// RestartCount is the number of consecutive restart attempts
	RestartCount int `json:"restart_count"`

	// FirstRestart is when the current restart sequence began
	FirstRestart time.Time `json:"first_restart"`

	// LastRestart is the most recent restart attempt time
	LastRestart time.Time `json:"last_restart"`

	// LastSuccess is when the polecat last ran successfully
	LastSuccess time.Time `json:"last_success"`

	// CrashLoopDetected indicates if this polecat is in a crash loop
	CrashLoopDetected bool `json:"crash_loop_detected"`

	// CrashLoopDetectedAt is when the crash loop was first detected
	CrashLoopDetectedAt time.Time `json:"crash_loop_detected_at"`
}

// Backoff configuration constants
const (
	// InitialBackoff is the starting backoff duration
	InitialBackoff = 30 * time.Second

	// MaxBackoff is the maximum backoff duration
	MaxBackoff = 10 * time.Minute

	// BackoffMultiplier is the exponential growth factor
	BackoffMultiplier = 2.0

	// CrashLoopThreshold is the number of restarts within the window to trigger crash loop detection
	CrashLoopThreshold = 5

	// CrashLoopWindow is the time window to detect crash loops
	CrashLoopWindow = 15 * time.Minute

	// BackoffResetDuration is how long without crashes to reset the backoff counter
	BackoffResetDuration = 30 * time.Minute
)

// NewRestartTracker creates a new RestartTracker.
func NewRestartTracker(townRoot string) *RestartTracker {
	return &RestartTracker{
		townRoot: townRoot,
		state:    &RestartState{Polecats: make(map[string]*PolecatRestarts)},
	}
}

// Load loads the restart state from disk.
func (rt *RestartTracker) Load() error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	statePath := rt.statePath()
	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			// No existing state - start fresh
			return nil
		}
		return fmt.Errorf("reading restart state: %w", err)
	}

	var state RestartState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("unmarshaling restart state: %w", err)
	}

	rt.state = &state
	return nil
}

// Save saves the restart state to disk.
func (rt *RestartTracker) Save() error {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	statePath := rt.statePath()
	data, err := json.MarshalIndent(rt.state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling restart state: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	if err := os.WriteFile(statePath, data, 0644); err != nil {
		return fmt.Errorf("writing restart state: %w", err)
	}

	return nil
}

// statePath returns the path to the restart state file.
func (rt *RestartTracker) statePath() string {
	return filepath.Join(rt.townRoot, "daemon", "restart_state.json")
}

// RecordRestart records a restart attempt for a polecat.
// It returns the backoff duration to wait before the next restart,
// or an error if the polecat is in a crash loop and should not be restarted.
func (rt *RestartTracker) RecordRestart(polecatID string) (time.Duration, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	now := time.Now()
	info := rt.state.Polecats[polecatID]

	// Check if we should reset the backoff counter
	if info != nil && now.Sub(info.LastRestart) > BackoffResetDuration {
		// Polecat has been stable - reset the counter
		info.RestartCount = 0
		info.FirstRestart = now
		info.CrashLoopDetected = false
	}

	// Initialize or update info
	if info == nil {
		info = &PolecatRestarts{
			PolecatID:   polecatID,
			FirstRestart: now,
		}
		rt.state.Polecats[polecatID] = info
	}

	info.RestartCount++
	info.LastRestart = now

	// Calculate backoff duration
	backoff := rt.calculateBackoff(info.RestartCount)

	// Check for crash loop
	if rt.detectCrashLoop(info) {
		info.CrashLoopDetected = true
		info.CrashLoopDetectedAt = now
		return 0, fmt.Errorf("crash loop detected: polecat %s has crashed %d times in %v",
			polecatID, info.RestartCount, CrashLoopWindow)
	}

	_ = rt.Save()
	return backoff, nil
}

// RecordSuccess records that a polecat started successfully.
// This resets the restart counter.
func (rt *RestartTracker) RecordSuccess(polecatID string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	now := time.Now()
	info := rt.state.Polecats[polecatID]

	if info == nil {
		info = &PolecatRestarts{
			PolecatID: polecatID,
		}
		rt.state.Polecats[polecatID] = info
	}

	info.LastSuccess = now
	info.RestartCount = 0
	info.CrashLoopDetected = false
	info.FirstRestart = time.Time{} // Reset

	_ = rt.Save()
}

// GetBackoff returns the current backoff duration for a polecat.
func (rt *RestartTracker) GetBackoff(polecatID string) time.Duration {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	info := rt.state.Polecats[polecatID]
	if info == nil || info.RestartCount == 0 {
		return 0
	}

	return rt.calculateBackoff(info.RestartCount)
}

// IsInCrashLoop returns true if the polecat is currently in a crash loop.
func (rt *RestartTracker) IsInCrashLoop(polecatID string) bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	info := rt.state.Polecats[polecatID]
	if info == nil {
		return false
	}

	// Check if crash loop status has expired (been too long since last detection)
	if info.CrashLoopDetected && time.Since(info.CrashLoopDetectedAt) > BackoffResetDuration {
		return false
	}

	return info.CrashLoopDetected
}

// ClearCrashLoop manually clears the crash loop status for a polecat.
// This is useful for manual intervention to allow restart attempts.
func (rt *RestartTracker) ClearCrashLoop(polecatID string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	info := rt.state.Polecats[polecatID]
	if info != nil {
		info.CrashLoopDetected = false
		info.RestartCount = 0
		info.FirstRestart = time.Time{}
		_ = rt.Save()
	}
}

// calculateBackoff calculates exponential backoff based on restart count.
func (rt *RestartTracker) calculateBackoff(restartCount int) time.Duration {
	// Formula: initial * (multiplier ^ (count - 1))
	// But capped at MaxBackoff
	backoff := time.Duration(float64(InitialBackoff) * pow(BackoffMultiplier, restartCount-1))

	if backoff > MaxBackoff {
		backoff = MaxBackoff
	}

	return backoff
}

// detectCrashLoop checks if the polecat is in a crash loop.
// A crash loop is detected when:
// 1. The number of restarts exceeds the threshold
// 2. The restarts occurred within the crash loop window
func (rt *RestartTracker) detectCrashLoop(info *PolecatRestarts) bool {
	if info.RestartCount < CrashLoopThreshold {
		return false
	}

	// Check if all restarts happened within the crash loop window
	windowStart := time.Now().Add(-CrashLoopWindow)
	if info.FirstRestart.Before(windowStart) {
		// First restart was too long ago - not a crash loop
		return false
	}

	return true
}

// pow returns base^exp as a float64.
func pow(base float64, exp int) float64 {
	result := 1.0
	for i := 0; i < exp; i++ {
		result *= base
	}
	return result
}

// ShouldRestart determines if a polecat should be restarted now.
// It returns false if the polecat is in a crash loop or if backoff has not elapsed.
func (rt *RestartTracker) ShouldRestart(polecatID string) (bool, string) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	info := rt.state.Polecats[polecatID]
	if info == nil {
		return true, "" // No history - allow restart
	}

	// Check for crash loop
	if info.CrashLoopDetected && time.Since(info.CrashLoopDetectedAt) < BackoffResetDuration {
		return false, fmt.Sprintf("crash loop detected: %d restarts in %v", info.RestartCount, CrashLoopWindow)
	}

	// Check backoff
	if info.RestartCount > 0 {
		backoff := rt.calculateBackoff(info.RestartCount)
		elapsed := time.Since(info.LastRestart)
		if elapsed < backoff {
			remaining := backoff - elapsed
			return false, fmt.Sprintf("backoff in effect: %v remaining", remaining.Round(time.Second))
		}
	}

	return true, ""
}

// GetStatus returns the restart status for a polecat.
func (rt *RestartTracker) GetStatus(polecatID string) *PolecatRestarts {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	if info := rt.state.Polecats[polecatID]; info != nil {
		// Return a copy to avoid race conditions
		copy := *info
		return &copy
	}

	return nil
}

// GetAllStatus returns restart status for all tracked polecats.
func (rt *RestartTracker) GetAllStatus() map[string]*PolecatRestarts {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	result := make(map[string]*PolecatRestarts, len(rt.state.Polecats))
	for k, v := range rt.state.Polecats {
		copy := *v
		result[k] = &copy
	}

	return result
}
