// Package feed provides a TUI for the Gas Town activity feed.
// This file implements stuck detection for agents using structured beads data.
// Previous approach used tmux pane scraping with regex patterns, which produced
// false positives (HTML `>`, compiler output matching `error:`). This version
// uses reliable structured signals from beads (hook state, timestamps).
package feed

import (
	"strconv"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
)

// HealthDataSource provides structured data for agent health detection.
// This replaces the old TmuxClient interface that relied on pane scraping.
type HealthDataSource interface {
	// ListAgentBeads returns all agent beads (single efficient query).
	ListAgentBeads() (map[string]*beads.Issue, error)
	// IsSessionAlive checks if a tmux session exists (zombie detection only).
	IsSessionAlive(sessionName string) (bool, error)
}

// AgentState represents the possible states for a GasTown agent.
// Ordered by priority (most urgent first) for sorting.
type AgentState int

const (
	StateGUPPViolation AgentState = iota // >30m no progress with hooked work - CRITICAL
	StateStalled                         // >15m no progress with hooked work
	StateWorking                         // Actively producing output
	StateIdle                            // No hooked work
	StateZombie                          // Dead/crashed session
)

func (s AgentState) String() string {
	switch s {
	case StateGUPPViolation:
		return "gupp"
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
	case StateGUPPViolation, StateStalled, StateZombie:
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

// GUPP threshold constants
const (
	GUPPViolationMinutes    = 30
	StalledThresholdMinutes = 15
)

// ProblemAgent represents an agent that needs attention.
type ProblemAgent struct {
	Name          string
	SessionID     string
	Role          string
	Rig           string
	State         AgentState
	IdleMinutes   int
	LastActivity  time.Time
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

// StuckDetector analyzes agent health using structured beads data.
type StuckDetector struct {
	source HealthDataSource
}

// NewStuckDetector creates a new stuck detector with default data sources.
func NewStuckDetector(bd *beads.Beads) *StuckDetector {
	return NewStuckDetectorWithSource(&defaultHealthSource{
		bd:   bd,
		tmux: tmux.NewTmux(),
	})
}

// NewStuckDetectorWithSource creates a new stuck detector with the given data source.
// This constructor accepts any HealthDataSource implementation, enabling testing with mocks.
func NewStuckDetectorWithSource(source HealthDataSource) *StuckDetector {
	return &StuckDetector{source: source}
}

// CheckAll analyzes all agent beads and returns their health states.
// This replaces the old FindGasTownSessions + AnalyzeSession loop.
func (d *StuckDetector) CheckAll() ([]*ProblemAgent, error) {
	agentBeads, err := d.source.ListAgentBeads()
	if err != nil {
		return nil, err
	}

	var agents []*ProblemAgent
	for id, issue := range agentBeads {
		agent := d.analyzeAgent(id, issue)
		if agent != nil {
			agents = append(agents, agent)
		}
	}

	sortProblemAgents(agents)
	return agents, nil
}

// analyzeAgent determines the health state of a single agent from its bead data.
func (d *StuckDetector) analyzeAgent(id string, issue *beads.Issue) *ProblemAgent {
	rig, role, name, ok := beads.ParseAgentBeadID(id)
	if !ok {
		return nil
	}

	// Derive display name
	displayName := name
	if displayName == "" {
		displayName = role
	}

	// Derive tmux session name from bead ID components
	sessionName := deriveSessionName(rig, role, name)

	agent := &ProblemAgent{
		Name:          displayName,
		SessionID:     sessionName,
		Role:          role,
		Rig:           rig,
		CurrentBeadID: id,
		HasHookedWork: issue.HookBead != "",
	}

	// Parse staleness from UpdatedAt
	updatedAt, err := time.Parse(time.RFC3339, issue.UpdatedAt)
	if err != nil {
		// Try alternate format (some beads use different timestamp formats)
		updatedAt, err = time.Parse("2006-01-02T15:04:05", issue.UpdatedAt)
	}
	if err == nil {
		agent.LastActivity = updatedAt
		agent.IdleMinutes = int(time.Since(updatedAt).Minutes())
	}

	// 1. Zombie check (tmux liveness)
	// On error, treat session as alive (unknown) rather than falsely flagging as zombie
	alive, err := d.source.IsSessionAlive(sessionName)
	if err == nil && !alive {
		agent.State = StateZombie
		agent.ActionHint = "Session dead - may need restart"
		return agent
	}

	hasHook := issue.HookBead != ""

	// 2. GUPP violation (most critical)
	if IsGUPPViolation(hasHook, agent.IdleMinutes) {
		agent.State = StateGUPPViolation
		agent.ActionHint = "GUPP violation: hooked work + " + strconv.Itoa(agent.IdleMinutes) + "m no progress"
		return agent
	}

	// 3. Stalled (hooked work but no recent progress)
	if hasHook && agent.IdleMinutes >= StalledThresholdMinutes {
		agent.State = StateStalled
		agent.ActionHint = "No progress for " + strconv.Itoa(agent.IdleMinutes) + "m"
		return agent
	}

	// 4. Working / Idle
	if hasHook {
		agent.State = StateWorking
	} else {
		agent.State = StateIdle
	}

	return agent
}

// IsGUPPViolation checks if an agent is in GUPP violation.
func IsGUPPViolation(hasHookedWork bool, minutesSinceProgress int) bool {
	return hasHookedWork && minutesSinceProgress >= GUPPViolationMinutes
}

// deriveSessionName maps bead ID components to a tmux session name.
// Uses the naming conventions from internal/session/.
// Note: session.*SessionName functions take a rigPrefix (e.g. "gt"),
// not a rig name (e.g. "gastown"). Use session.PrefixFor(rig) to convert.
func deriveSessionName(rig, role, name string) string {
	switch role {
	case "mayor":
		return session.MayorSessionName()
	case "deacon":
		return session.DeaconSessionName()
	case "witness":
		return session.WitnessSessionName(session.PrefixFor(rig))
	case "refinery":
		return session.RefinerySessionName(session.PrefixFor(rig))
	case "crew":
		return session.CrewSessionName(session.PrefixFor(rig), name)
	case "polecat":
		return session.PolecatSessionName(session.PrefixFor(rig), name)
	default:
		// Fallback: construct from components
		rigPrefix := session.PrefixFor(rig)
		if rig == "" {
			return session.HQPrefix + role
		}
		if name == "" {
			return rigPrefix + "-" + role
		}
		return rigPrefix + "-" + role + "-" + name
	}
}

// sortProblemAgents sorts agents by state priority (problems first)
func sortProblemAgents(agents []*ProblemAgent) {
	for i := 0; i < len(agents); i++ {
		for j := i + 1; j < len(agents); j++ {
			if agents[i].State.Priority() > agents[j].State.Priority() {
				agents[i], agents[j] = agents[j], agents[i]
			} else if agents[i].State.Priority() == agents[j].State.Priority() {
				if agents[i].IdleMinutes < agents[j].IdleMinutes {
					agents[i], agents[j] = agents[j], agents[i]
				}
			}
		}
	}
}

// defaultHealthSource implements HealthDataSource using real beads and tmux.
type defaultHealthSource struct {
	bd   *beads.Beads
	tmux *tmux.Tmux
}

func (s *defaultHealthSource) ListAgentBeads() (map[string]*beads.Issue, error) {
	return s.bd.ListAgentBeads()
}

func (s *defaultHealthSource) IsSessionAlive(sessionName string) (bool, error) {
	return s.tmux.HasSession(sessionName)
}
