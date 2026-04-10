package witness

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Step transition protocol patterns.
var (
	PatternStepAdvanced  = regexp.MustCompile(`^STEP_ADVANCED\s+(\S+)\s+(\S+)`)
	PatternStepFailed    = regexp.MustCompile(`^STEP_FAILED\s+(\S+)\s+(\S+)`)
	PatternStepRetry     = regexp.MustCompile(`^STEP_RETRY\s+(\S+)\s+(\S+)`)
	PatternStepTriaged   = regexp.MustCompile(`^STEP_TRIAGED\s+(\S+)\s+(\S+)`)
	PatternStepEscalated = regexp.MustCompile(`^STEP_ESCALATED\s+(\S+)\s+(\S+)`)
)

// Step transition protocol types.
const (
	ProtoStepAdvanced  ProtocolType = "step_advanced"
	ProtoStepFailed    ProtocolType = "step_failed"
	ProtoStepRetry     ProtocolType = "step_retry"
	ProtoStepTriaged   ProtocolType = "step_triaged"
	ProtoStepEscalated ProtocolType = "step_escalated"
)

// StepAdvancedPayload contains parsed data from a STEP_ADVANCED message.
type StepAdvancedPayload struct {
	PolecatName string
	FromStep    string
	ToStep      string
	Outcome     string
	Timestamp   time.Time
}

// StepFailedPayload contains parsed data from a STEP_FAILED message.
type StepFailedPayload struct {
	PolecatName string
	Step        string
	Attempt     int
	Error       string
	Timestamp   time.Time
}

// StepTriagedPayload contains parsed data from a STEP_TRIAGED message.
type StepTriagedPayload struct {
	PolecatName string
	Step        string
	Verdict     string
	Reason      string
	Timestamp   time.Time
}

// ParseStepAdvanced parses a STEP_ADVANCED protocol message.
func ParseStepAdvanced(subject, body string) (*StepAdvancedPayload, error) {
	matches := PatternStepAdvanced.FindStringSubmatch(subject)
	if len(matches) < 3 {
		return nil, fmt.Errorf("invalid STEP_ADVANCED subject: %s", subject)
	}

	payload := &StepAdvancedPayload{
		PolecatName: matches[1],
		ToStep:      matches[2],
		Timestamp:   time.Now(),
	}

	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "FromStep:"):
			payload.FromStep = strings.TrimSpace(strings.TrimPrefix(line, "FromStep:"))
		case strings.HasPrefix(line, "ToStep:"):
			payload.ToStep = strings.TrimSpace(strings.TrimPrefix(line, "ToStep:"))
		case strings.HasPrefix(line, "Outcome:"):
			payload.Outcome = strings.TrimSpace(strings.TrimPrefix(line, "Outcome:"))
		}
	}

	return payload, nil
}

// ParseStepFailed parses a STEP_FAILED protocol message.
func ParseStepFailed(subject, body string) (*StepFailedPayload, error) {
	matches := PatternStepFailed.FindStringSubmatch(subject)
	if len(matches) < 3 {
		return nil, fmt.Errorf("invalid STEP_FAILED subject: %s", subject)
	}

	payload := &StepFailedPayload{
		PolecatName: matches[1],
		Step:        matches[2],
		Timestamp:   time.Now(),
	}

	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Step:"):
			payload.Step = strings.TrimSpace(strings.TrimPrefix(line, "Step:"))
		case strings.HasPrefix(line, "Attempt:"):
			v := strings.TrimSpace(strings.TrimPrefix(line, "Attempt:"))
			if n, err := strconv.Atoi(v); err == nil {
				payload.Attempt = n
			}
		case strings.HasPrefix(line, "Error:"):
			payload.Error = strings.TrimSpace(strings.TrimPrefix(line, "Error:"))
		}
	}

	return payload, nil
}

// ParseStepTriaged parses a STEP_TRIAGED protocol message.
func ParseStepTriaged(subject, body string) (*StepTriagedPayload, error) {
	matches := PatternStepTriaged.FindStringSubmatch(subject)
	if len(matches) < 3 {
		return nil, fmt.Errorf("invalid STEP_TRIAGED subject: %s", subject)
	}

	payload := &StepTriagedPayload{
		PolecatName: matches[1],
		Step:        matches[2],
		Timestamp:   time.Now(),
	}

	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Step:"):
			payload.Step = strings.TrimSpace(strings.TrimPrefix(line, "Step:"))
		case strings.HasPrefix(line, "Verdict:"):
			payload.Verdict = strings.TrimSpace(strings.TrimPrefix(line, "Verdict:"))
		case strings.HasPrefix(line, "Reason:"):
			payload.Reason = strings.TrimSpace(strings.TrimPrefix(line, "Reason:"))
		}
	}

	return payload, nil
}

// StepBudget defines resource limits for a polecat's formula execution.
type StepBudget struct {
	MaxSteps  int
	MaxTokens int64
}

// StepState is the current orchestration state for a polecat as seen by the witness.
type StepState struct {
	CurrentStep    string
	StepCount      int
	RetryCount     int
	TokensUsed     int64
	Budget         *StepBudget
	LastTransition time.Time
}

// StepTracker is the witness's view of step orchestration state across polecats.
// Thread-safe: accessed from witness patrol and bus subscriber goroutines.
type StepTracker struct {
	mu     sync.RWMutex
	states map[string]*StepState // keyed by polecat name
}

// NewStepTracker creates a new StepTracker.
func NewStepTracker() *StepTracker {
	return &StepTracker{
		states: make(map[string]*StepState),
	}
}

// RecordAdvance records a step transition for a polecat.
func (t *StepTracker) RecordAdvance(polecat, fromStep, toStep string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state, ok := t.states[polecat]
	if !ok {
		state = &StepState{}
		t.states[polecat] = state
	}
	state.CurrentStep = toStep
	state.StepCount++
	state.RetryCount = 0
	state.LastTransition = time.Now()
}

// RecordFailure records a step failure for a polecat.
func (t *StepTracker) RecordFailure(polecat, step string, attempt int, errMsg string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state, ok := t.states[polecat]
	if !ok {
		state = &StepState{CurrentStep: step}
		t.states[polecat] = state
	}
	state.RetryCount = attempt
	state.LastTransition = time.Now()
}

// SetBudget sets resource limits for a polecat.
func (t *StepTracker) SetBudget(polecat string, budget StepBudget) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state, ok := t.states[polecat]
	if !ok {
		state = &StepState{}
		t.states[polecat] = state
	}
	state.Budget = &budget
}

// AddTokens adds to the token count for a polecat.
func (t *StepTracker) AddTokens(polecat string, tokens int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if state, ok := t.states[polecat]; ok {
		state.TokensUsed += tokens
	}
}

// GetState returns the current step state for a polecat, or nil if unknown.
func (t *StepTracker) GetState(polecat string) *StepState {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.states[polecat]
}

// IsOverBudget checks if a polecat has exceeded its resource limits.
func (t *StepTracker) IsOverBudget(polecat string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	state, ok := t.states[polecat]
	if !ok || state.Budget == nil {
		return false
	}
	if state.Budget.MaxSteps > 0 && state.StepCount > state.Budget.MaxSteps {
		return true
	}
	if state.Budget.MaxTokens > 0 && state.TokensUsed > state.Budget.MaxTokens {
		return true
	}
	return false
}

// IsStale checks if a polecat hasn't had a step transition in the given duration.
func (t *StepTracker) IsStale(polecat string, maxAge time.Duration) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	state, ok := t.states[polecat]
	if !ok {
		return false
	}
	return time.Since(state.LastTransition) > maxAge
}

// Remove clears tracking state for a polecat.
func (t *StepTracker) Remove(polecat string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.states, polecat)
}
