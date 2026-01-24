package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RestartTracker tracks agent restart attempts with exponential backoff.
// This prevents runaway restart loops when an agent keeps crashing.
type RestartTracker struct {
	mu       sync.RWMutex
	townRoot string
	state    *RestartState
}

// RestartState persists restart tracking data.
type RestartState struct {
	Agents map[string]*AgentRestartInfo `json:"agents"`
}

// AgentRestartInfo tracks restart info for a single agent.
type AgentRestartInfo struct {
	LastRestart    time.Time `json:"last_restart"`
	RestartCount   int       `json:"restart_count"`
	BackoffUntil   time.Time `json:"backoff_until"`
	CrashLoopSince time.Time `json:"crash_loop_since,omitempty"`
}

// Backoff parameters
const (
	initialBackoff    = 30 * time.Second
	maxBackoff        = 10 * time.Minute
	backoffMultiplier = 2.0
	crashLoopWindow   = 15 * time.Minute
	crashLoopCount    = 5
	stabilityPeriod   = 30 * time.Minute
)

// NewRestartTracker creates a new restart tracker.
func NewRestartTracker(townRoot string) *RestartTracker {
	return &RestartTracker{
		townRoot: townRoot,
		state:    &RestartState{Agents: make(map[string]*AgentRestartInfo)},
	}
}

// restartStateFile returns the path to the restart state file.
func (rt *RestartTracker) restartStateFile() string {
	return filepath.Join(rt.townRoot, "daemon", "restart_state.json")
}

// Load loads the restart state from disk.
func (rt *RestartTracker) Load() error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	data, err := os.ReadFile(rt.restartStateFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No state file yet
		}
		return err
	}

	return json.Unmarshal(data, rt.state)
}

// Save persists the restart state to disk.
func (rt *RestartTracker) Save() error {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	data, err := json.MarshalIndent(rt.state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(rt.restartStateFile(), data, 0600)
}

// CanRestart checks if an agent can be restarted (not in backoff).
func (rt *RestartTracker) CanRestart(agentID string) bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	info, exists := rt.state.Agents[agentID]
	if !exists {
		return true
	}

	// Check if in crash loop
	if !info.CrashLoopSince.IsZero() {
		return false
	}

	// Check backoff period
	return time.Now().After(info.BackoffUntil)
}

// RecordRestart records a restart attempt and calculates next backoff.
func (rt *RestartTracker) RecordRestart(agentID string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	now := time.Now()
	info, exists := rt.state.Agents[agentID]
	if !exists {
		info = &AgentRestartInfo{}
		rt.state.Agents[agentID] = info
	}

	// Check if previous restart was stable (long ago)
	if !info.LastRestart.IsZero() && now.Sub(info.LastRestart) > stabilityPeriod {
		// Reset backoff - agent was stable
		info.RestartCount = 0
		info.CrashLoopSince = time.Time{}
	}

	info.LastRestart = now
	info.RestartCount++

	// Calculate backoff with exponential increase
	backoffDuration := initialBackoff
	for i := 1; i < info.RestartCount && backoffDuration < maxBackoff; i++ {
		backoffDuration = time.Duration(float64(backoffDuration) * backoffMultiplier)
	}
	if backoffDuration > maxBackoff {
		backoffDuration = maxBackoff
	}
	info.BackoffUntil = now.Add(backoffDuration)

	// Check for crash loop
	if info.RestartCount >= crashLoopCount {
		windowStart := now.Add(-crashLoopWindow)
		if info.LastRestart.After(windowStart) {
			info.CrashLoopSince = now
		}
	}
}

// RecordSuccess records that an agent is running successfully.
// Call this periodically for healthy agents to reset their backoff.
func (rt *RestartTracker) RecordSuccess(agentID string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	info, exists := rt.state.Agents[agentID]
	if !exists {
		return
	}

	// If agent has been stable for the stability period, reset tracking
	if time.Since(info.LastRestart) > stabilityPeriod {
		info.RestartCount = 0
		info.CrashLoopSince = time.Time{}
		info.BackoffUntil = time.Time{}
	}
}

// IsInCrashLoop returns true if the agent is detected as crash-looping.
func (rt *RestartTracker) IsInCrashLoop(agentID string) bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	info, exists := rt.state.Agents[agentID]
	if !exists {
		return false
	}
	return !info.CrashLoopSince.IsZero()
}

// GetBackoffRemaining returns how long until the agent can be restarted.
func (rt *RestartTracker) GetBackoffRemaining(agentID string) time.Duration {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	info, exists := rt.state.Agents[agentID]
	if !exists {
		return 0
	}

	remaining := time.Until(info.BackoffUntil)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// ClearCrashLoop manually clears the crash loop state for an agent.
func (rt *RestartTracker) ClearCrashLoop(agentID string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	info, exists := rt.state.Agents[agentID]
	if exists {
		info.CrashLoopSince = time.Time{}
		info.RestartCount = 0
		info.BackoffUntil = time.Time{}
	}
}
