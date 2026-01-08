// Package status provides multi-signal status detection for worker activity.
// It implements a 7-state model that aggregates signals from git commits,
// beads updates, and tmux session activity to determine worker status.
package status

import (
	"time"
)

// State represents one of the 7 possible worker states.
type State string

const (
	// StateActive indicates recent activity (commits, beads updates, or session activity within threshold).
	StateActive State = "active"

	// StateThinking indicates the worker is likely processing (< 15 min quiet).
	StateThinking State = "thinking"

	// StateSlow indicates extended quiet period (15-30 min).
	StateSlow State = "slow"

	// StateUnresponsive indicates very long quiet period (> 30 min).
	StateUnresponsive State = "unresponsive"

	// StateDead indicates no tmux session exists for the worker.
	StateDead State = "dead"

	// StateBlocked indicates the worker is waiting on dependencies.
	StateBlocked State = "blocked"

	// StateDone indicates the assigned issue is closed.
	StateDone State = "done"
)

// Thresholds for time-based state transitions.
const (
	ThresholdActive      = 5 * time.Minute  // Below this: Active
	ThresholdThinking    = 15 * time.Minute // Below this: Thinking
	ThresholdSlow        = 30 * time.Minute // Below this: Slow, above: Unresponsive
)

// ColorClass returns the CSS class for a given state.
func (s State) ColorClass() string {
	switch s {
	case StateActive:
		return "status-active"
	case StateThinking:
		return "status-thinking"
	case StateSlow:
		return "status-slow"
	case StateUnresponsive:
		return "status-unresponsive"
	case StateDead:
		return "status-dead"
	case StateBlocked:
		return "status-blocked"
	case StateDone:
		return "status-done"
	default:
		return "status-unknown"
	}
}

// Label returns a human-readable label for the state.
func (s State) Label() string {
	switch s {
	case StateActive:
		return "Active"
	case StateThinking:
		return "Thinking"
	case StateSlow:
		return "Slow"
	case StateUnresponsive:
		return "Unresponsive"
	case StateDead:
		return "Dead"
	case StateBlocked:
		return "Blocked"
	case StateDone:
		return "Done"
	default:
		return "Unknown"
	}
}

// Icon returns an emoji icon for the state.
func (s State) Icon() string {
	switch s {
	case StateActive:
		return "🟢"
	case StateThinking:
		return "🔵"
	case StateSlow:
		return "🟡"
	case StateUnresponsive:
		return "🔴"
	case StateDead:
		return "💀"
	case StateBlocked:
		return "🚧"
	case StateDone:
		return "✅"
	default:
		return "❓"
	}
}

// IsHealthy returns true if the state indicates healthy worker operation.
func (s State) IsHealthy() bool {
	return s == StateActive || s == StateThinking || s == StateDone
}

// NeedsAttention returns true if the state indicates potential issues.
func (s State) NeedsAttention() bool {
	return s == StateSlow || s == StateUnresponsive || s == StateDead || s == StateBlocked
}

// Signals holds all collected activity signals for a worker.
type Signals struct {
	// GitCommit is the timestamp of the most recent git commit in the worktree.
	GitCommit *time.Time

	// BeadsUpdate is the issue's updated_at timestamp.
	BeadsUpdate *time.Time

	// SessionActivity is the tmux session's last activity timestamp.
	SessionActivity *time.Time

	// SessionExists indicates whether the tmux session exists.
	SessionExists bool

	// IsBlocked indicates whether the issue has open blocking dependencies.
	IsBlocked bool

	// IsClosed indicates whether the issue is closed.
	IsClosed bool
}

// MostRecentActivity returns the most recent activity timestamp from all signals.
// Returns nil if no activity signals are available.
func (s *Signals) MostRecentActivity() *time.Time {
	var most *time.Time

	if s.GitCommit != nil {
		most = s.GitCommit
	}

	if s.BeadsUpdate != nil {
		if most == nil || s.BeadsUpdate.After(*most) {
			most = s.BeadsUpdate
		}
	}

	if s.SessionActivity != nil {
		if most == nil || s.SessionActivity.After(*most) {
			most = s.SessionActivity
		}
	}

	return most
}

// Status holds the computed status for a worker.
type Status struct {
	// State is the computed 7-state status.
	State State

	// TimeSinceActivity is the duration since the most recent activity signal.
	TimeSinceActivity time.Duration

	// MostRecentSignal indicates which signal was most recent.
	MostRecentSignal string

	// Signals contains the raw signal data used for computation.
	Signals Signals
}

// Compute determines the worker status from collected signals.
func Compute(signals Signals) Status {
	status := Status{
		Signals: signals,
	}

	// Priority 1: Terminal states
	if signals.IsClosed {
		status.State = StateDone
		return status
	}

	if !signals.SessionExists {
		status.State = StateDead
		return status
	}

	if signals.IsBlocked {
		status.State = StateBlocked
		return status
	}

	// Priority 2: Time-based states from most recent activity
	mostRecent := signals.MostRecentActivity()
	if mostRecent == nil {
		// No activity data but session exists - treat as unresponsive
		status.State = StateUnresponsive
		return status
	}

	// Determine which signal was most recent
	status.MostRecentSignal = determineMostRecentSignal(signals, *mostRecent)

	// Calculate time since activity
	status.TimeSinceActivity = time.Since(*mostRecent)
	if status.TimeSinceActivity < 0 {
		// Handle clock skew
		status.TimeSinceActivity = 0
	}

	// Determine state based on time thresholds
	status.State = stateFromDuration(status.TimeSinceActivity)

	return status
}

// determineMostRecentSignal identifies which signal matched the most recent timestamp.
func determineMostRecentSignal(s Signals, recent time.Time) string {
	if s.GitCommit != nil && s.GitCommit.Equal(recent) {
		return "git"
	}
	if s.BeadsUpdate != nil && s.BeadsUpdate.Equal(recent) {
		return "beads"
	}
	if s.SessionActivity != nil && s.SessionActivity.Equal(recent) {
		return "session"
	}
	return "unknown"
}

// stateFromDuration determines the time-based state from a duration.
func stateFromDuration(d time.Duration) State {
	switch {
	case d < ThresholdActive:
		return StateActive
	case d < ThresholdThinking:
		return StateThinking
	case d < ThresholdSlow:
		return StateSlow
	default:
		return StateUnresponsive
	}
}

// FormatDuration returns a human-readable duration string.
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		return formatInt(mins) + "m"
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		return formatInt(hours) + "h"
	}
	days := int(d.Hours() / 24)
	return formatInt(days) + "d"
}

func formatInt(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}
