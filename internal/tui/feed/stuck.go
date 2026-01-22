// Package feed provides a TUI for the Gas Town activity feed.
// This file implements stuck detection for agents.
package feed

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/tmux"
)

// TmuxClient defines the tmux operations needed by StuckDetector.
// This interface enables testing with mock implementations.
type TmuxClient interface {
	HasSession(name string) (bool, error)
	CapturePane(session string, lines int) (string, error)
	GetSessionActivity(session string) (time.Time, error)
	GetSessionSet() (*tmux.SessionSet, error)
}

// AgentState represents the possible states for a GasTown agent.
// Ordered by priority (most urgent first) for sorting.
type AgentState int

const (
	StateGUPPViolation AgentState = iota // >30m no progress with hooked work - CRITICAL
	StateInputRequired                   // Waiting for user input (prompt, YOLO, Enter)
	StateStalled                         // >15m no progress
	StateWorking                         // Actively producing output
	StateIdle                            // No hooked work
	StateZombie                          // Dead/crashed session
)

func (s AgentState) String() string {
	switch s {
	case StateGUPPViolation:
		return "gupp"
	case StateInputRequired:
		return "input"
	case StateStalled:
		return "stalled"
	case StateWorking:
		return "working"
	case StateIdle:
		return "idle"
	case StateZombie:
		return "zombie"
	default:
		return "unknown"
	}
}

// Priority returns the sort priority (lower = more urgent).
func (s AgentState) Priority() int {
	return int(s)
}

// NeedsAttention returns true if this state requires user action.
func (s AgentState) NeedsAttention() bool {
	switch s {
	case StateGUPPViolation, StateInputRequired, StateStalled, StateZombie:
		return true
	default:
		return false
	}
}

// Symbol returns the display symbol for this state.
func (s AgentState) Symbol() string {
	switch s {
	case StateGUPPViolation:
		return "🔥"
	case StateInputRequired:
		return "⌨"
	case StateStalled:
		return "⚠"
	case StateWorking:
		return "●"
	case StateIdle:
		return "○"
	case StateZombie:
		return "💀"
	default:
		return "?"
	}
}

// Label returns the short display label for this state.
func (s AgentState) Label() string {
	switch s {
	case StateGUPPViolation:
		return "GUPP!"
	case StateInputRequired:
		return "INPUT"
	case StateStalled:
		return "STALL"
	case StateWorking:
		return "work"
	case StateIdle:
		return "idle"
	case StateZombie:
		return "dead"
	default:
		return "???"
	}
}

// InputReason indicates why an agent is waiting for input.
type InputReason int

const (
	InputReasonUnknown InputReason = iota
	InputReasonPromptWaiting
	InputReasonYOLOConfirmation
	InputReasonEnterRequired
	InputReasonPermission
)

func (r InputReason) String() string {
	switch r {
	case InputReasonPromptWaiting:
		return "prompt"
	case InputReasonYOLOConfirmation:
		return "[Y/n]"
	case InputReasonEnterRequired:
		return "Enter"
	case InputReasonPermission:
		return "Allow?"
	default:
		return "waiting"
	}
}

// GUPP threshold constants
const (
	GUPPViolationMinutes    = 30
	StalledThresholdMinutes = 15
	InputWaitThresholdSecs  = 120
)

// Pattern represents a regex pattern for detecting stuck states.
type Pattern struct {
	Regex  *regexp.Regexp
	Reason InputReason
}

// DefaultPatterns are the patterns used to detect input-required states.
var DefaultPatterns = []Pattern{
	{regexp.MustCompile(`>\s*$`), InputReasonPromptWaiting},
	{regexp.MustCompile(`claude>\s*$`), InputReasonPromptWaiting},
	{regexp.MustCompile(`\[Y/n\]\s*$`), InputReasonYOLOConfirmation},
	{regexp.MustCompile(`\[y/N\]\s*$`), InputReasonYOLOConfirmation},
	{regexp.MustCompile(`Allow\?\s*`), InputReasonPermission},
	{regexp.MustCompile(`Press Enter`), InputReasonEnterRequired},
	{regexp.MustCompile(`(?i)continue\?`), InputReasonPermission},
	{regexp.MustCompile(`(?i)proceed\?`), InputReasonPermission},
}

// ErrorPatterns indicate error states.
var ErrorPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)rate limit`),
	regexp.MustCompile(`(?i)context.*full`),
	regexp.MustCompile(`(?i)error:`),
	regexp.MustCompile(`(?i)failed:`),
}

// ProblemAgent represents an agent that needs attention.
type ProblemAgent struct {
	Name          string
	SessionID     string
	Role          string
	Rig           string
	State         AgentState
	InputReason   InputReason
	IdleMinutes   int
	LastActivity  time.Time
	LastLines     string
	ActionHint    string
	CurrentBeadID string
	HasHookedWork bool
}

// NeedsAttention returns true if agent requires user action.
func (p *ProblemAgent) NeedsAttention() bool {
	return p.State.NeedsAttention()
}

// DurationDisplay returns human-readable duration since last progress.
func (p *ProblemAgent) DurationDisplay() string {
	mins := p.IdleMinutes
	if mins < 1 {
		return "<1m"
	}
	if mins < 60 {
		return strconv.Itoa(mins) + "m"
	}
	hours := mins / 60
	remaining := mins % 60
	if remaining == 0 {
		return strconv.Itoa(hours) + "h"
	}
	return strconv.Itoa(hours) + "h" + strconv.Itoa(remaining) + "m"
}

// StuckDetector analyzes tmux sessions for stuck states.
type StuckDetector struct {
	tmux          TmuxClient
	Patterns      []Pattern
	IdleThreshold time.Duration
}

// NewStuckDetector creates a new stuck detector with default tmux wrapper.
func NewStuckDetector() *StuckDetector {
	return NewStuckDetectorWithClient(tmux.NewTmux())
}

// NewStuckDetectorWithTmux creates a new stuck detector with the given tmux wrapper.
// Deprecated: Use NewStuckDetectorWithClient instead.
func NewStuckDetectorWithTmux(t *tmux.Tmux) *StuckDetector {
	return NewStuckDetectorWithClient(t)
}

// NewStuckDetectorWithClient creates a new stuck detector with the given TmuxClient.
// This constructor accepts any TmuxClient implementation, enabling testing with mocks.
func NewStuckDetectorWithClient(client TmuxClient) *StuckDetector {
	return &StuckDetector{
		tmux:          client,
		Patterns:      DefaultPatterns,
		IdleThreshold: InputWaitThresholdSecs * time.Second,
	}
}

// AnalyzeSession checks a tmux session for stuck indicators.
func (d *StuckDetector) AnalyzeSession(sessionID string) *ProblemAgent {
	agent := &ProblemAgent{
		SessionID: sessionID,
		Name:      sessionID,
		State:     StateWorking,
	}

	// Parse role from session name
	agent.Role = parseRoleFromSession(sessionID)

	// Check if session exists
	exists, _ := d.tmux.HasSession(sessionID)
	if !exists {
		agent.State = StateZombie
		agent.ActionHint = "Session does not exist - may need restart"
		return agent
	}

	// Capture pane content
	content, err := d.tmux.CapturePane(sessionID, 50)
	if err != nil {
		agent.State = StateZombie
		agent.ActionHint = "Cannot capture pane content"
		return agent
	}

	// Get last few lines for display
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) > 5 {
		agent.LastLines = strings.Join(lines[len(lines)-5:], "\n")
	} else {
		agent.LastLines = strings.Join(lines, "\n")
	}

	// Get idle time
	lastActivity, err := d.tmux.GetSessionActivity(sessionID)
	if err == nil {
		agent.LastActivity = lastActivity
		agent.IdleMinutes = int(time.Since(lastActivity).Minutes())
	}

	// Check for input-required patterns
	lastContent := ""
	if len(lines) > 0 {
		// Get last non-empty lines
		for i := len(lines) - 1; i >= 0 && i >= len(lines)-10; i-- {
			if strings.TrimSpace(lines[i]) != "" {
				lastContent = strings.Join(lines[i:], "\n")
				break
			}
		}
	}

	for _, pattern := range d.Patterns {
		if pattern.Regex.MatchString(lastContent) {
			// Pattern matches - check if idle long enough
			if agent.IdleMinutes >= int(d.IdleThreshold.Minutes()) || agent.IdleMinutes >= 2 {
				agent.State = StateInputRequired
				agent.InputReason = pattern.Reason
				agent.ActionHint = getActionHint(pattern.Reason)
				return agent
			}
		}
	}

	// Check for error patterns
	for _, pattern := range ErrorPatterns {
		if pattern.MatchString(content) {
			agent.State = StateStalled
			agent.ActionHint = "Error detected in output"
			return agent
		}
	}

	// Check for stalled (no progress for 15+ minutes)
	if agent.IdleMinutes >= StalledThresholdMinutes {
		agent.State = StateStalled
		agent.ActionHint = "No progress for " + strconv.Itoa(agent.IdleMinutes) + "m"
	}

	return agent
}

// gasTownSessionPrefixes are the prefixes used to identify GasTown agent sessions.
var gasTownSessionPrefixes = []string{"polecat", "mayor", "refinery", "witness", "deacon", "crew", "boot"}

// isGasTownSession returns true if the session name looks like a GasTown agent.
func isGasTownSession(name string) bool {
	for _, prefix := range gasTownSessionPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// FindGasTownSessions returns tmux sessions that look like GasTown agents.
// Uses the efficient GetSessionSet for O(1) lookups.
func (d *StuckDetector) FindGasTownSessions() ([]string, error) {
	sessionSet, err := d.tmux.GetSessionSet()
	if err != nil {
		return nil, err
	}

	var gtSessions []string
	for _, name := range sessionSet.Names() {
		if isGasTownSession(name) {
			gtSessions = append(gtSessions, name)
		}
	}
	return gtSessions, nil
}

// IsGUPPViolation checks if an agent is in GUPP violation.
func IsGUPPViolation(hasHookedWork bool, minutesSinceProgress int) bool {
	return hasHookedWork && minutesSinceProgress >= GUPPViolationMinutes
}

// Helper functions

func parseRoleFromSession(sessionID string) string {
	roles := []string{"polecat", "mayor", "refinery", "witness", "deacon", "crew", "boot"}
	for _, role := range roles {
		if strings.HasPrefix(sessionID, role) {
			return role
		}
	}
	return "unknown"
}

func getActionHint(reason InputReason) string {
	switch reason {
	case InputReasonPromptWaiting:
		return "Agent waiting at prompt - attach and provide input"
	case InputReasonYOLOConfirmation:
		return "YOLO confirmation needed - attach and press Y or n"
	case InputReasonEnterRequired:
		return "Press Enter to continue"
	case InputReasonPermission:
		return "Permission prompt - attach and respond"
	default:
		return "Agent appears stuck - consider attaching"
	}
}
