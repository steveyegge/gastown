// Package registry provides cross-backend session discovery for gastown agents.
//
// SessionRegistry queries beads (the canonical agent registry) and optionally
// health-checks each session via the Coop backend.
//
// Agent beads with the "gt:agent" label are the source of truth for which agents
// exist. Backend metadata (coop_url, etc.) is stored in the bead's notes field
// as key: value pairs.
package registry

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/terminal"
)

// Session represents a discovered agent session with its metadata and health.
type Session struct {
	// ID is the bead identifier (e.g., "hq-mayor", "gt-gastown-crew-k8s").
	ID string

	// Rig is the rig name (empty for town-level agents like mayor/deacon).
	Rig string

	// Role is the agent role type (mayor, deacon, witness, refinery, polecat, crew).
	Role string

	// Name is the agent's name within its role (e.g., "hq", "Toast", "k8s").
	Name string

	// TmuxSession is the computed tmux session name (e.g., "hq-mayor", "gt-gastown-crew-k8s").
	TmuxSession string

	// BackendType is the detected backend: "coop" or "local".
	BackendType string

	// CoopURL is the Coop sidecar HTTP endpoint (only set for coop backend).
	CoopURL string

	// Alive is true if the session responded to health check (only set when
	// DiscoverOpts.CheckLiveness is true).
	Alive bool

	// AgentState is the structured agent state from bead metadata
	// (e.g., "spawning", "working", "done", "stuck").
	AgentState string

	// HookBead is the currently pinned work bead ID.
	HookBead string

	// Target is the execution target: "local" or "k8s".
	Target string
}

// AgentLister lists agent beads. Implementations include beads.Beads (local)
// and a daemon-backed lister for K8s environments.
type AgentLister interface {
	// ListAgentBeads returns all agent beads keyed by bead ID.
	ListAgentBeads() (map[string]*beads.Issue, error)
}

// NotesReader reads agent bead notes (backend metadata).
type NotesReader interface {
	// GetAgentNotes returns the notes field for an agent bead.
	// The notes contain key: value pairs like coop_url, etc.
	GetAgentNotes(agentID string) (string, error)
}

// BeadWriter creates and closes agent beads. This interface is satisfied by
// beads.Beads and allows the registry to manage agent lifecycle.
type BeadWriter interface {
	// CreateOrReopenAgentBead creates an agent bead or reopens a closed one.
	CreateOrReopenAgentBead(id, title string, fields *beads.AgentFields) (*beads.Issue, error)
	// CloseAndClearAgentBead closes an agent bead with a reason.
	CloseAndClearAgentBead(id, reason string) error
	// AddLabel adds a label to a bead.
	AddLabel(id, label string) error
}

// CreateSessionOpts configures session creation.
type CreateSessionOpts struct {
	// ID is the bead identifier (e.g., "gt-gastown-crew-k8s").
	ID string
	// Title is the display name for the agent.
	Title string
	// Rig is the rig name.
	Rig string
	// Role is the agent role (polecat, crew, witness, etc.).
	Role string
	// K8s indicates this is a K8s agent (adds execution_target:k8s label).
	K8s bool
}

// DiscoverOpts controls session discovery behavior.
type DiscoverOpts struct {
	// CheckLiveness enables health-checking each discovered session.
	// When false, sessions are returned without verifying they're alive.
	CheckLiveness bool

	// Timeout is the per-session health check timeout.
	// Defaults to 5 seconds if zero.
	Timeout time.Duration

	// Concurrency is the max number of parallel health checks.
	// Defaults to 10 if zero.
	Concurrency int

	// RigFilter limits discovery to a specific rig. Empty means all rigs.
	RigFilter string
}

// SessionRegistry discovers agent sessions across backends.
type SessionRegistry struct {
	lister AgentLister
	notes  NotesReader
	writer BeadWriter // optional, needed for Create/Destroy operations
}

// New creates a SessionRegistry.
//
// lister provides agent bead enumeration (beads.Beads or daemon client).
// notes provides bead notes for backend metadata resolution.
func New(lister AgentLister, notes NotesReader) *SessionRegistry {
	return &SessionRegistry{
		lister: lister,
		notes:  notes,
	}
}

// SetWriter sets the BeadWriter for session lifecycle operations.
// This is optional — only needed for CreateSession/DestroySession.
func (r *SessionRegistry) SetWriter(w BeadWriter) {
	r.writer = w
}

// DiscoverAll discovers all agent sessions, optionally health-checking them.
func (r *SessionRegistry) DiscoverAll(ctx context.Context, opts DiscoverOpts) ([]Session, error) {
	agentBeads, err := r.lister.ListAgentBeads()
	if err != nil {
		return nil, fmt.Errorf("listing agent beads: %w", err)
	}

	var sessions []Session
	for id, issue := range agentBeads {
		s := r.buildSession(id, issue)

		// Apply rig filter
		if opts.RigFilter != "" && s.Rig != opts.RigFilter {
			continue
		}

		sessions = append(sessions, s)
	}

	// Optionally health-check coop sessions
	if opts.CheckLiveness {
		r.healthCheck(ctx, sessions, opts)
	}

	return sessions, nil
}

// DiscoverRig discovers sessions for a specific rig.
func (r *SessionRegistry) DiscoverRig(ctx context.Context, rig string, opts DiscoverOpts) ([]Session, error) {
	opts.RigFilter = rig
	return r.DiscoverAll(ctx, opts)
}

// Lookup finds a single session by bead ID.
func (r *SessionRegistry) Lookup(ctx context.Context, agentID string, checkLiveness bool) (*Session, error) {
	agentBeads, err := r.lister.ListAgentBeads()
	if err != nil {
		return nil, fmt.Errorf("listing agent beads: %w", err)
	}

	issue, ok := agentBeads[agentID]
	if !ok {
		return nil, fmt.Errorf("agent %q not found", agentID)
	}

	s := r.buildSession(agentID, issue)

	if checkLiveness {
		r.healthCheckOne(ctx, &s, 5*time.Second)
	}

	return &s, nil
}

// buildSession creates a Session from a bead ID and Issue.
func (r *SessionRegistry) buildSession(id string, issue *beads.Issue) Session {
	fields := beads.ParseAgentFields(issue.Description)

	s := Session{
		ID:          id,
		Rig:         fields.Rig,
		Role:        fields.RoleType,
		AgentState:  fields.AgentState,
		HookBead:    fields.HookBead,
		BackendType: "coop", // default — all sessions are coop-backed
		Target:      "local",
	}

	// Override with authoritative JSON fields
	if issue.AgentState != "" {
		s.AgentState = issue.AgentState
	}
	if issue.HookBead != "" {
		s.HookBead = issue.HookBead
	}

	// Parse name from ID
	s.Name = parseNameFromID(id, s.Rig, s.Role)

	// Compute tmux session name
	s.TmuxSession = computeTmuxSession(id, s.Rig, s.Role, s.Name)

	// Check for K8s labels
	for _, label := range issue.Labels {
		if label == "execution_target:k8s" {
			s.Target = "k8s"
			break
		}
	}

	// Read backend metadata from notes
	if r.notes != nil {
		if notes, err := r.notes.GetAgentNotes(id); err == nil {
			r.applyNotes(&s, notes)
		}
	}

	return s
}

// applyNotes parses bead notes and sets backend metadata on the session.
func (r *SessionRegistry) applyNotes(s *Session, notes string) {
	for _, line := range strings.Split(notes, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "backend":
			if val == "coop" || val == "k8s" {
				s.BackendType = "coop"
			}
		case "coop_url":
			s.CoopURL = val
			s.BackendType = "coop"
		}
	}
}

// healthCheck runs concurrent health checks on coop-backed sessions.
func (r *SessionRegistry) healthCheck(ctx context.Context, sessions []Session, opts DiscoverOpts) {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 10
	}

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i := range sessions {
		s := &sessions[i]
		if s.BackendType != "coop" || s.CoopURL == "" {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(s *Session) {
			defer wg.Done()
			defer func() { <-sem }()
			r.healthCheckOne(ctx, s, timeout)
		}(s)
	}

	wg.Wait()
}

// healthCheckOne checks a single session's liveness via its coop backend.
func (r *SessionRegistry) healthCheckOne(ctx context.Context, s *Session, timeout time.Duration) {
	if s.BackendType != "coop" || s.CoopURL == "" {
		return
	}

	b := terminal.NewCoopBackend(terminal.CoopConfig{Timeout: timeout})
	b.AddSession("claude", s.CoopURL)

	alive, err := b.HasSession("claude")
	if err == nil {
		s.Alive = alive
	}
}

// parseNameFromID extracts the agent name from a bead ID.
func parseNameFromID(id, rig, role string) string {
	switch {
	case id == "hq-mayor":
		return "hq"
	case id == "hq-deacon":
		return "hq"
	case id == "hq-boot":
		return "hq"
	}

	// For rig-prefixed IDs like "gt-gastown-crew-k8s" or "gt-gastown-witness"
	parts := strings.SplitN(id, "-", 4)
	if len(parts) >= 3 {
		// prefix-rig-role or prefix-rig-role-name
		if len(parts) == 4 {
			return parts[3]
		}
		return parts[2]
	}

	return id
}

// computeTmuxSession computes the tmux session name for an agent.
// Town-level: "hq-mayor", "hq-deacon"
// Rig-level: "gt-<rig>-<type>" or "gt-<rig>-crew-<name>"
func computeTmuxSession(id, rig, role, name string) string {
	// Town-level agents use their bead ID directly
	if strings.HasPrefix(id, "hq-") {
		return id
	}
	// For everything else, the bead ID IS the tmux session name
	return id
}

// --- Session lifecycle operations ---

// CreateSession creates an agent bead, triggering the K8s controller to create
// a pod (when K8s=true). The controller watches for agent beads with
// agent_state=spawning and execution_target:k8s labels.
func (r *SessionRegistry) CreateSession(opts CreateSessionOpts) (*Session, error) {
	if r.writer == nil {
		return nil, fmt.Errorf("registry: no BeadWriter configured (call SetWriter)")
	}
	if opts.ID == "" {
		return nil, fmt.Errorf("registry: session ID is required")
	}

	fields := &beads.AgentFields{
		RoleType:   opts.Role,
		Rig:        opts.Rig,
		AgentState: "spawning",
	}

	issue, err := r.writer.CreateOrReopenAgentBead(opts.ID, opts.Title, fields)
	if err != nil {
		return nil, fmt.Errorf("creating agent bead: %w", err)
	}

	// Add K8s label so the controller picks up this agent
	if opts.K8s {
		if err := r.writer.AddLabel(opts.ID, "execution_target:k8s"); err != nil {
			return nil, fmt.Errorf("adding k8s label: %w", err)
		}
	}

	s := r.buildSession(issue.ID, issue)
	if opts.K8s {
		s.Target = "k8s"
		s.BackendType = "coop"
	}

	return &s, nil
}

// DestroySession closes the agent bead, triggering the K8s controller to
// delete the pod.
func (r *SessionRegistry) DestroySession(agentID, reason string) error {
	if r.writer == nil {
		return fmt.Errorf("registry: no BeadWriter configured (call SetWriter)")
	}
	return r.writer.CloseAndClearAgentBead(agentID, reason)
}

// RestartSession restarts an agent by switching the coop session in-place.
func (r *SessionRegistry) RestartSession(ctx context.Context, agentID string) error {
	s, err := r.Lookup(ctx, agentID, false)
	if err != nil {
		return err
	}
	if s.BackendType != "coop" || s.CoopURL == "" {
		return fmt.Errorf("restart only supported for coop sessions (got backend=%s)", s.BackendType)
	}

	b := terminal.NewCoopBackend(terminal.CoopConfig{Timeout: 30 * time.Second})
	b.AddSession("claude", s.CoopURL)
	return b.RespawnPane("claude")
}
