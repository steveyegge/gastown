// Package circuit implements a per-rig pipeline circuit breaker that tracks
// consecutive failures in witness and refinery stages. When failures exceed
// a threshold, the circuit opens (halting dispatch) and escalates to mayor.
//
// Three states:
//   - CLOSED: Normal operation, dispatch allowed.
//   - OPEN: Failures exceeded threshold, dispatch blocked.
//   - HALF_OPEN: After timeout, one test dispatch is allowed.
//
// State is persisted to <townRoot>/.runtime/circuit-<rig>.json and survives
// process restarts. Follows the atomic write pattern from capacity/state.go.
package circuit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// State represents the circuit breaker state.
type State string

const (
	StateClosed   State = "closed"
	StateOpen     State = "open"
	StateHalfOpen State = "half_open"
)

// Stage identifies a pipeline stage tracked by the circuit breaker.
type Stage string

const (
	StageWitness  Stage = "witness"
	StageRefinery Stage = "refinery"
)

// Default thresholds. Overridable per-rig via config if needed.
const (
	DefaultFailureThreshold = 3
	DefaultOpenTimeout      = 5 * time.Minute
)

// StageState tracks failure counts and timing for a single pipeline stage.
type StageState struct {
	ConsecutiveFailures int    `json:"consecutive_failures"`
	LastFailureAt       string `json:"last_failure_at,omitempty"`
	LastFailureReason   string `json:"last_failure_reason,omitempty"`
	LastSuccessAt       string `json:"last_success_at,omitempty"`
}

// Breaker is the per-rig circuit breaker state. Persisted as JSON.
type Breaker struct {
	Rig       string                `json:"rig"`
	State     State                 `json:"state"`
	OpenedAt  string                `json:"opened_at,omitempty"`
	OpenedBy  Stage                 `json:"opened_by,omitempty"`
	ResetAt   string                `json:"reset_at,omitempty"`
	ResetBy   string                `json:"reset_by,omitempty"`
	Stages    map[Stage]*StageState `json:"stages"`
	Threshold int                   `json:"threshold"`
}

// NewBreaker creates a zero-value breaker for the given rig (CLOSED state).
func NewBreaker(rig string) *Breaker {
	return &Breaker{
		Rig:       rig,
		State:     StateClosed,
		Threshold: DefaultFailureThreshold,
		Stages: map[Stage]*StageState{
			StageWitness:  {},
			StageRefinery: {},
		},
	}
}

// stateFile returns the path to the circuit breaker state file for a rig.
func stateFile(townRoot, rig string) string {
	return filepath.Join(townRoot, ".runtime", fmt.Sprintf("circuit-%s.json", rig))
}

// Load loads the circuit breaker state for a rig. Returns a zero-value breaker
// (CLOSED) if the file doesn't exist.
func Load(townRoot, rig string) (*Breaker, error) {
	path := stateFile(townRoot, rig)
	data, err := os.ReadFile(path) //nolint:gosec // G304: path constructed internally
	if err != nil {
		if os.IsNotExist(err) {
			return NewBreaker(rig), nil
		}
		return nil, fmt.Errorf("reading circuit state: %w", err)
	}

	var b Breaker
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parsing circuit state: %w", err)
	}

	// Ensure stages map is populated (handles legacy files missing a stage)
	if b.Stages == nil {
		b.Stages = make(map[Stage]*StageState)
	}
	if b.Stages[StageWitness] == nil {
		b.Stages[StageWitness] = &StageState{}
	}
	if b.Stages[StageRefinery] == nil {
		b.Stages[StageRefinery] = &StageState{}
	}
	if b.Threshold == 0 {
		b.Threshold = DefaultFailureThreshold
	}
	return &b, nil
}

// Save writes the breaker state to disk atomically (temp + rename).
func Save(townRoot string, b *Breaker) error {
	path := stateFile(townRoot, b.Rig)
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating runtime dir: %w", err)
	}

	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling circuit state: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".circuit-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

// RecordFailure records a failure for the given stage. If consecutive failures
// reach the threshold and the circuit is CLOSED, it transitions to OPEN.
// Returns true if the circuit just opened (caller should escalate).
func (b *Breaker) RecordFailure(stage Stage, reason string) bool {
	s := b.Stages[stage]
	if s == nil {
		s = &StageState{}
		b.Stages[stage] = s
	}

	s.ConsecutiveFailures++
	s.LastFailureAt = time.Now().UTC().Format(time.RFC3339)
	s.LastFailureReason = reason

	if b.State == StateClosed && s.ConsecutiveFailures >= b.Threshold {
		b.State = StateOpen
		b.OpenedAt = s.LastFailureAt
		b.OpenedBy = stage
		return true
	}

	// HALF_OPEN test dispatch failed → back to OPEN
	if b.State == StateHalfOpen {
		b.State = StateOpen
		b.OpenedAt = s.LastFailureAt
		b.OpenedBy = stage
		return true
	}

	return false
}

// RecordSuccess records a success for the given stage. Resets consecutive
// failures. If the circuit is HALF_OPEN, transitions to CLOSED.
func (b *Breaker) RecordSuccess(stage Stage) {
	s := b.Stages[stage]
	if s == nil {
		s = &StageState{}
		b.Stages[stage] = s
	}

	s.ConsecutiveFailures = 0
	s.LastSuccessAt = time.Now().UTC().Format(time.RFC3339)

	if b.State == StateHalfOpen {
		b.State = StateClosed
		b.OpenedAt = ""
		b.OpenedBy = ""
	}
}

// Reset manually forces the circuit to CLOSED state (mayor override).
func (b *Breaker) Reset(actor string) {
	b.State = StateClosed
	b.OpenedAt = ""
	b.OpenedBy = ""
	b.ResetAt = time.Now().UTC().Format(time.RFC3339)
	b.ResetBy = actor
	for _, s := range b.Stages {
		s.ConsecutiveFailures = 0
	}
}

// CheckDispatch evaluates whether dispatch is allowed. Returns nil if allowed,
// or an error with a descriptive message if blocked.
//
// State transitions:
//   - CLOSED → allow
//   - OPEN + timeout expired → transition to HALF_OPEN, allow one dispatch
//   - OPEN + timeout not expired → block
//   - HALF_OPEN → allow (test dispatch in progress)
func (b *Breaker) CheckDispatch() error {
	switch b.State {
	case StateClosed:
		return nil

	case StateHalfOpen:
		return nil

	case StateOpen:
		if b.OpenedAt != "" {
			openedAt, err := time.Parse(time.RFC3339, b.OpenedAt)
			if err == nil && time.Since(openedAt) >= DefaultOpenTimeout {
				// Timeout expired → HALF_OPEN: allow one test dispatch
				b.State = StateHalfOpen
				return nil
			}
		}
		stage := b.Stages[b.OpenedBy]
		reason := ""
		if stage != nil {
			reason = stage.LastFailureReason
		}
		return &OpenError{
			Rig:       b.Rig,
			Stage:     b.OpenedBy,
			OpenedAt:  b.OpenedAt,
			Reason:    reason,
			Threshold: b.Threshold,
		}
	}
	return nil
}

// OpenError is returned when dispatch is blocked by an open circuit.
type OpenError struct {
	Rig       string
	Stage     Stage
	OpenedAt  string
	Reason    string
	Threshold int
}

func (e *OpenError) Error() string {
	return fmt.Sprintf(
		"circuit breaker OPEN for rig %q (stage: %s, %d consecutive failures)\n"+
			"  Opened at: %s\n"+
			"  Last failure: %s\n"+
			"  Reset with: gt circuit reset %s",
		e.Rig, e.Stage, e.Threshold, e.OpenedAt, e.Reason, e.Rig,
	)
}

// RecordFailureAndSave is a convenience function that loads state, records a failure,
// saves state, and returns whether the circuit just opened (for escalation).
func RecordFailureAndSave(townRoot, rig string, stage Stage, reason string) (justOpened bool, err error) {
	b, err := Load(townRoot, rig)
	if err != nil {
		return false, err
	}
	justOpened = b.RecordFailure(stage, reason)
	if err := Save(townRoot, b); err != nil {
		return justOpened, err
	}
	return justOpened, nil
}

// RecordSuccessAndSave is a convenience function that loads state, records a success,
// and saves state.
func RecordSuccessAndSave(townRoot, rig string, stage Stage) error {
	b, err := Load(townRoot, rig)
	if err != nil {
		return err
	}
	b.RecordSuccess(stage)
	return Save(townRoot, b)
}

// CheckDispatchForRig loads state and checks if dispatch is allowed.
// If the state transitions from OPEN to HALF_OPEN, it saves the updated state.
func CheckDispatchForRig(townRoot, rig string) error {
	b, err := Load(townRoot, rig)
	if err != nil {
		return err
	}
	prevState := b.State
	if err := b.CheckDispatch(); err != nil {
		return err
	}
	// Save if state transitioned (OPEN → HALF_OPEN)
	if b.State != prevState {
		return Save(townRoot, b)
	}
	return nil
}
