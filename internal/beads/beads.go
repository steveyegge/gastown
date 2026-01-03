// Package beads provides a wrapper for the bd (beads) CLI.
package beads

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Common errors
var (
	ErrNotInstalled = errors.New("bd not installed: run 'pip install beads-cli' or see https://github.com/anthropics/beads")
	ErrNotARepo     = errors.New("not a beads repository (no .beads directory found)")
	ErrSyncConflict = errors.New("beads sync conflict")
	ErrNotFound     = errors.New("issue not found")
)

// ResolveBeadsDir returns the actual beads directory, following any redirect.
// If workDir/.beads/redirect exists, it reads the redirect path and resolves it
// relative to workDir (not the .beads directory). Otherwise, returns workDir/.beads.
//
// This is essential for crew workers and polecats that use shared beads via redirect.
// The redirect file contains a relative path like "../../mayor/rig/.beads".
//
// Example: if we're at crew/max/ and .beads/redirect contains "../../mayor/rig/.beads",
// the redirect is resolved from crew/max/ (not crew/max/.beads/), giving us
// mayor/rig/.beads at the rig root level.
//
// Circular redirect detection: If the resolved path equals the original beads directory,
// this indicates an errant redirect file that should be removed. The function logs a
// warning and returns the original beads directory.
func ResolveBeadsDir(workDir string) string {
	beadsDir := filepath.Join(workDir, ".beads")
	redirectPath := filepath.Join(beadsDir, "redirect")

	// Check for redirect file
	data, err := os.ReadFile(redirectPath) //nolint:gosec // G304: path is constructed internally
	if err != nil {
		// No redirect, use local .beads
		return beadsDir
	}

	// Read and clean the redirect path
	redirectTarget := strings.TrimSpace(string(data))
	if redirectTarget == "" {
		return beadsDir
	}

	// Resolve relative to workDir (the redirect is written from the perspective
	// of being inside workDir, not inside workDir/.beads)
	// e.g., redirect contains "../../mayor/rig/.beads"
	// from crew/max/, this resolves to mayor/rig/.beads
	resolved := filepath.Join(workDir, redirectTarget)

	// Clean the path to resolve .. components
	resolved = filepath.Clean(resolved)

	// Detect circular redirects: if resolved path equals original beads dir,
	// this is an errant redirect file (e.g., redirect in mayor/rig/.beads pointing to itself)
	if resolved == beadsDir {
		fmt.Fprintf(os.Stderr, "Warning: circular redirect detected in %s (points to itself), ignoring redirect\n", redirectPath)
		// Remove the errant redirect file to prevent future warnings
		if err := os.Remove(redirectPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not remove errant redirect file: %v\n", err)
		}
		return beadsDir
	}

	// Detect redirect chains: check if resolved path also has a redirect
	resolvedRedirect := filepath.Join(resolved, "redirect")
	if _, err := os.Stat(resolvedRedirect); err == nil {
		fmt.Fprintf(os.Stderr, "Warning: redirect chain detected: %s -> %s (which also has a redirect)\n", beadsDir, resolved)
		// Don't follow chains - just return the first resolved path
		// The target's redirect is likely errant and should be removed
	}

	return resolved
}

// Issue represents a beads issue.
type Issue struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Priority    int      `json:"priority"`
	Type        string   `json:"issue_type"`
	CreatedAt   string   `json:"created_at"`
	CreatedBy   string   `json:"created_by,omitempty"`
	UpdatedAt   string   `json:"updated_at"`
	ClosedAt    string   `json:"closed_at,omitempty"`
	Parent      string   `json:"parent,omitempty"`
	Assignee    string   `json:"assignee,omitempty"`
	Children    []string `json:"children,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
	Blocks      []string `json:"blocks,omitempty"`
	BlockedBy   []string `json:"blocked_by,omitempty"`
	Labels      []string `json:"labels,omitempty"`

	// Agent bead slots (type=agent only)
	HookBead   string `json:"hook_bead,omitempty"`   // Current work attached to agent's hook
	RoleBead   string `json:"role_bead,omitempty"`   // Role definition bead (shared)
	AgentState string `json:"agent_state,omitempty"` // Agent lifecycle state (spawning, working, done, stuck)

	// Counts from list output
	DependencyCount int `json:"dependency_count,omitempty"`
	DependentCount  int `json:"dependent_count,omitempty"`
	BlockedByCount  int `json:"blocked_by_count,omitempty"`

	// Detailed dependency info from show output
	Dependencies []IssueDep `json:"dependencies,omitempty"`
	Dependents   []IssueDep `json:"dependents,omitempty"`
}

// IssueDep represents a dependency or dependent issue with its relation.
type IssueDep struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Status         string `json:"status"`
	Priority       int    `json:"priority"`
	Type           string `json:"issue_type"`
	DependencyType string `json:"dependency_type,omitempty"`
}

// Delegation represents a work delegation relationship between work units.
// Delegation links a parent work unit to a child work unit, tracking who
// delegated the work and to whom, along with any terms of the delegation.
// This enables work distribution with credit cascade - work flows down,
// validation and credit flow up.
type Delegation struct {
	// Parent is the work unit ID that delegated the work
	Parent string `json:"parent"`

	// Child is the work unit ID that received the delegated work
	Child string `json:"child"`

	// DelegatedBy is the entity (hop:// URI or actor string) that delegated
	DelegatedBy string `json:"delegated_by"`

	// DelegatedTo is the entity (hop:// URI or actor string) receiving delegation
	DelegatedTo string `json:"delegated_to"`

	// Terms contains optional conditions of the delegation
	Terms *DelegationTerms `json:"terms,omitempty"`

	// CreatedAt is when the delegation was created
	CreatedAt string `json:"created_at,omitempty"`
}

// DelegationTerms holds optional terms/conditions for a delegation.
type DelegationTerms struct {
	// Portion describes what part of the parent work is delegated
	Portion string `json:"portion,omitempty"`

	// Deadline is the expected completion date
	Deadline string `json:"deadline,omitempty"`

	// AcceptanceCriteria describes what constitutes completion
	AcceptanceCriteria string `json:"acceptance_criteria,omitempty"`

	// CreditShare is the percentage of credit that flows to the delegate (0-100)
	CreditShare int `json:"credit_share,omitempty"`
}

// ListOptions specifies filters for listing issues.
type ListOptions struct {
	Status     string // "open", "closed", "all"
	Type       string // "task", "bug", "feature", "epic"
	Priority   int    // 0-4, -1 for no filter
	Parent     string // filter by parent ID
	Assignee   string // filter by assignee (e.g., "gastown/Toast")
	NoAssignee bool   // filter for issues with no assignee
}

// CreateOptions specifies options for creating an issue.
type CreateOptions struct {
	Title       string
	Type        string // "task", "bug", "feature", "epic"
	Priority    int    // 0-4
	Description string
	Parent      string
	Actor       string // Who is creating this issue (populates created_by)
}

// UpdateOptions specifies options for updating an issue.
type UpdateOptions struct {
	Title        *string
	Status       *string
	Priority     *int
	Description  *string
	Assignee     *string
	AddLabels    []string // Labels to add
	RemoveLabels []string // Labels to remove
	SetLabels    []string // Labels to set (replaces all existing)
}

// SyncStatus represents the sync status of the beads repository.
type SyncStatus struct {
	Branch    string
	Ahead     int
	Behind    int
	Conflicts []string
}

// Beads wraps bd CLI operations for a working directory.
type Beads struct {
	workDir  string
	beadsDir string // Optional BEADS_DIR override for cross-database access
}

// New creates a new Beads wrapper for the given directory.
func New(workDir string) *Beads {
	return &Beads{workDir: workDir}
}

// NewWithBeadsDir creates a Beads wrapper with an explicit BEADS_DIR.
// This is needed when running from a polecat worktree but accessing town-level beads.
func NewWithBeadsDir(workDir, beadsDir string) *Beads {
	return &Beads{workDir: workDir, beadsDir: beadsDir}
}

// run executes a bd command and returns stdout.
func (b *Beads) run(args ...string) ([]byte, error) {
	// Use --no-daemon for faster read operations (avoids daemon IPC overhead)
	// The daemon is primarily useful for write coalescing, not reads
	fullArgs := append([]string{"--no-daemon"}, args...)
	cmd := exec.Command("bd", fullArgs...) //nolint:gosec // G204: bd is a trusted internal tool
	cmd.Dir = b.workDir

	// Set BEADS_DIR if specified (enables cross-database access)
	if b.beadsDir != "" {
		cmd.Env = append(os.Environ(), "BEADS_DIR="+b.beadsDir)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, b.wrapError(err, stderr.String(), args)
	}

	return stdout.Bytes(), nil
}

// wrapError wraps bd errors with context.
func (b *Beads) wrapError(err error, stderr string, args []string) error {
	stderr = strings.TrimSpace(stderr)

	// Check for bd not installed
	if execErr, ok := err.(*exec.Error); ok && errors.Is(execErr.Err, exec.ErrNotFound) {
		return ErrNotInstalled
	}

	// Detect specific error types from stderr
	if strings.Contains(stderr, "not a beads repository") ||
		strings.Contains(stderr, "No .beads directory") ||
		strings.Contains(stderr, ".beads") && strings.Contains(stderr, "not found") {
		return ErrNotARepo
	}
	if strings.Contains(stderr, "sync conflict") || strings.Contains(stderr, "CONFLICT") {
		return ErrSyncConflict
	}
	if strings.Contains(stderr, "not found") || strings.Contains(stderr, "Issue not found") {
		return ErrNotFound
	}

	if stderr != "" {
		return fmt.Errorf("bd %s: %s", strings.Join(args, " "), stderr)
	}
	return fmt.Errorf("bd %s: %w", strings.Join(args, " "), err)
}

// List returns issues matching the given options.
func (b *Beads) List(opts ListOptions) ([]*Issue, error) {
	args := []string{"list", "--json"}

	if opts.Status != "" {
		args = append(args, "--status="+opts.Status)
	}
	if opts.Type != "" {
		args = append(args, "--type="+opts.Type)
	}
	if opts.Priority >= 0 {
		args = append(args, fmt.Sprintf("--priority=%d", opts.Priority))
	}
	if opts.Parent != "" {
		args = append(args, "--parent="+opts.Parent)
	}
	if opts.Assignee != "" {
		args = append(args, "--assignee="+opts.Assignee)
	}
	if opts.NoAssignee {
		args = append(args, "--no-assignee")
	}

	out, err := b.run(args...)
	if err != nil {
		return nil, err
	}

	var issues []*Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing bd list output: %w", err)
	}

	return issues, nil
}

// ListByAssignee returns all issues assigned to a specific assignee.
// The assignee is typically in the format "rig/polecatName" (e.g., "gastown/Toast").
func (b *Beads) ListByAssignee(assignee string) ([]*Issue, error) {
	return b.List(ListOptions{
		Status:   "all", // Include both open and closed for state derivation
		Assignee: assignee,
		Priority: -1, // No priority filter
	})
}

// GetAssignedIssue returns the first open issue assigned to the given assignee.
// Returns nil if no open issue is assigned.
func (b *Beads) GetAssignedIssue(assignee string) (*Issue, error) {
	issues, err := b.List(ListOptions{
		Status:   "open",
		Assignee: assignee,
		Priority: -1,
	})
	if err != nil {
		return nil, err
	}

	// Also check in_progress status explicitly
	if len(issues) == 0 {
		issues, err = b.List(ListOptions{
			Status:   "in_progress",
			Assignee: assignee,
			Priority: -1,
		})
		if err != nil {
			return nil, err
		}
	}

	if len(issues) == 0 {
		return nil, nil
	}

	return issues[0], nil
}

// Ready returns issues that are ready to work (not blocked).
func (b *Beads) Ready() ([]*Issue, error) {
	out, err := b.run("ready", "--json")
	if err != nil {
		return nil, err
	}

	var issues []*Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing bd ready output: %w", err)
	}

	return issues, nil
}

// ReadyWithType returns ready issues filtered by type.
// Uses bd ready --type flag for server-side filtering.
func (b *Beads) ReadyWithType(issueType string) ([]*Issue, error) {
	out, err := b.run("ready", "--json", "--type", issueType, "-n", "100")
	if err != nil {
		return nil, err
	}

	var issues []*Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing bd ready output: %w", err)
	}

	return issues, nil
}

// Show returns detailed information about an issue.
func (b *Beads) Show(id string) (*Issue, error) {
	out, err := b.run("show", id, "--json")
	if err != nil {
		return nil, err
	}

	// bd show --json returns an array with one element
	var issues []*Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing bd show output: %w", err)
	}

	if len(issues) == 0 {
		return nil, ErrNotFound
	}

	return issues[0], nil
}

// ShowMultiple fetches multiple issues by ID in a single bd call.
// Returns a map of ID to Issue. Missing IDs are not included in the map.
func (b *Beads) ShowMultiple(ids []string) (map[string]*Issue, error) {
	if len(ids) == 0 {
		return make(map[string]*Issue), nil
	}

	// bd show supports multiple IDs
	args := append([]string{"show", "--json"}, ids...)
	out, err := b.run(args...)
	if err != nil {
		// If bd fails, return empty map (some IDs might not exist)
		return make(map[string]*Issue), nil
	}

	var issues []*Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing bd show output: %w", err)
	}

	result := make(map[string]*Issue, len(issues))
	for _, issue := range issues {
		result[issue.ID] = issue
	}

	return result, nil
}

// ListAgentBeads returns all agent beads in a single query.
// Returns a map of agent bead ID to Issue.
func (b *Beads) ListAgentBeads() (map[string]*Issue, error) {
	out, err := b.run("list", "--type=agent", "--json")
	if err != nil {
		return nil, err
	}

	var issues []*Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing bd list output: %w", err)
	}

	result := make(map[string]*Issue, len(issues))
	for _, issue := range issues {
		result[issue.ID] = issue
	}

	return result, nil
}

// Blocked returns issues that are blocked by dependencies.
func (b *Beads) Blocked() ([]*Issue, error) {
	out, err := b.run("blocked", "--json")
	if err != nil {
		return nil, err
	}

	var issues []*Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing bd blocked output: %w", err)
	}

	return issues, nil
}

// Create creates a new issue and returns it.
// If opts.Actor is empty, it defaults to the BD_ACTOR environment variable.
// This ensures created_by is populated for issue provenance tracking.
func (b *Beads) Create(opts CreateOptions) (*Issue, error) {
	args := []string{"create", "--json"}

	if opts.Title != "" {
		args = append(args, "--title="+opts.Title)
	}
	if opts.Type != "" {
		args = append(args, "--type="+opts.Type)
	}
	if opts.Priority >= 0 {
		args = append(args, fmt.Sprintf("--priority=%d", opts.Priority))
	}
	if opts.Description != "" {
		args = append(args, "--description="+opts.Description)
	}
	if opts.Parent != "" {
		args = append(args, "--parent="+opts.Parent)
	}
	// Default Actor from BD_ACTOR env var if not specified
	actor := opts.Actor
	if actor == "" {
		actor = os.Getenv("BD_ACTOR")
	}
	if actor != "" {
		args = append(args, "--actor="+actor)
	}

	out, err := b.run(args...)
	if err != nil {
		return nil, err
	}

	var issue Issue
	if err := json.Unmarshal(out, &issue); err != nil {
		return nil, fmt.Errorf("parsing bd create output: %w", err)
	}

	return &issue, nil
}

// Update updates an existing issue.
func (b *Beads) Update(id string, opts UpdateOptions) error {
	args := []string{"update", id}

	if opts.Title != nil {
		args = append(args, "--title="+*opts.Title)
	}
	if opts.Status != nil {
		args = append(args, "--status="+*opts.Status)
	}
	if opts.Priority != nil {
		args = append(args, fmt.Sprintf("--priority=%d", *opts.Priority))
	}
	if opts.Description != nil {
		args = append(args, "--description="+*opts.Description)
	}
	if opts.Assignee != nil {
		args = append(args, "--assignee="+*opts.Assignee)
	}
	// Label operations: set-labels replaces all, otherwise use add/remove
	if len(opts.SetLabels) > 0 {
		for _, label := range opts.SetLabels {
			args = append(args, "--set-labels="+label)
		}
	} else {
		for _, label := range opts.AddLabels {
			args = append(args, "--add-label="+label)
		}
		for _, label := range opts.RemoveLabels {
			args = append(args, "--remove-label="+label)
		}
	}

	_, err := b.run(args...)
	return err
}

// Close closes one or more issues.
// If CLAUDE_SESSION_ID is set in the environment, it is passed to bd close
// for work attribution tracking (see decision 009-session-events-architecture.md).
func (b *Beads) Close(ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	args := append([]string{"close"}, ids...)

	// Pass session ID for work attribution if available
	if sessionID := os.Getenv("CLAUDE_SESSION_ID"); sessionID != "" {
		args = append(args, "--session="+sessionID)
	}

	_, err := b.run(args...)
	return err
}

// CloseWithReason closes one or more issues with a reason.
// If CLAUDE_SESSION_ID is set in the environment, it is passed to bd close
// for work attribution tracking (see decision 009-session-events-architecture.md).
func (b *Beads) CloseWithReason(reason string, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	args := append([]string{"close"}, ids...)
	args = append(args, "--reason="+reason)

	// Pass session ID for work attribution if available
	if sessionID := os.Getenv("CLAUDE_SESSION_ID"); sessionID != "" {
		args = append(args, "--session="+sessionID)
	}

	_, err := b.run(args...)
	return err
}

// Release moves an in_progress issue back to open status.
// This is used to recover stuck steps when a worker dies mid-task.
// It clears the assignee so the step can be claimed by another worker.
func (b *Beads) Release(id string) error {
	return b.ReleaseWithReason(id, "")
}

// ReleaseWithReason moves an in_progress issue back to open status with a reason.
// The reason is added as a note to the issue for tracking purposes.
func (b *Beads) ReleaseWithReason(id, reason string) error {
	args := []string{"update", id, "--status=open", "--assignee="}

	// Add reason as a note if provided
	if reason != "" {
		args = append(args, "--notes=Released: "+reason)
	}

	_, err := b.run(args...)
	return err
}

// AddDependency adds a dependency: issue depends on dependsOn.
func (b *Beads) AddDependency(issue, dependsOn string) error {
	_, err := b.run("dep", "add", issue, dependsOn)
	return err
}

// RemoveDependency removes a dependency.
func (b *Beads) RemoveDependency(issue, dependsOn string) error {
	_, err := b.run("dep", "remove", issue, dependsOn)
	return err
}

// AddDelegation creates a delegation relationship from parent to child work unit.
// The delegation tracks who delegated (delegatedBy) and who received (delegatedTo),
// along with optional terms. Delegations enable credit cascade - when child work
// is completed, credit flows up to the parent work unit and its delegator.
//
// Note: This is stored as metadata on the child issue until bd CLI has native
// delegation support. Once bd supports `bd delegate add`, this will be updated.
func (b *Beads) AddDelegation(d *Delegation) error {
	if d.Parent == "" || d.Child == "" {
		return fmt.Errorf("delegation requires both parent and child work unit IDs")
	}
	if d.DelegatedBy == "" || d.DelegatedTo == "" {
		return fmt.Errorf("delegation requires both delegated_by and delegated_to entities")
	}

	// Store delegation as JSON in the child issue's delegated_from slot
	delegationJSON, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("marshaling delegation: %w", err)
	}

	// Set the delegated_from slot on the child issue
	_, err = b.run("slot", "set", d.Child, "delegated_from", string(delegationJSON))
	if err != nil {
		return fmt.Errorf("setting delegation slot: %w", err)
	}

	// Also add a dependency so child blocks parent (work must complete before parent can close)
	if err := b.AddDependency(d.Parent, d.Child); err != nil {
		// Log but don't fail - the delegation is still recorded
		fmt.Printf("Warning: could not add blocking dependency for delegation: %v\n", err)
	}

	return nil
}

// RemoveDelegation removes a delegation relationship.
func (b *Beads) RemoveDelegation(parent, child string) error {
	// Clear the delegated_from slot on the child
	_, err := b.run("slot", "clear", child, "delegated_from")
	if err != nil {
		return fmt.Errorf("clearing delegation slot: %w", err)
	}

	// Also remove the blocking dependency
	if err := b.RemoveDependency(parent, child); err != nil {
		// Log but don't fail
		fmt.Printf("Warning: could not remove blocking dependency: %v\n", err)
	}

	return nil
}

// GetDelegation retrieves the delegation information for a child work unit.
// Returns nil if the issue has no delegation.
func (b *Beads) GetDelegation(child string) (*Delegation, error) {
	// Get the issue to read its slot
	issue, err := b.Show(child)
	if err != nil {
		return nil, fmt.Errorf("getting issue: %w", err)
	}

	// The slot would be in the description or a separate field
	// For now, we'll need to parse from the bd slot get command
	out, err := b.run("slot", "get", child, "delegated_from")
	if err != nil {
		// No delegation slot means no delegation
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no slot") {
			return nil, nil
		}
		return nil, fmt.Errorf("getting delegation slot: %w", err)
	}

	slotValue := strings.TrimSpace(string(out))
	if slotValue == "" || slotValue == "null" {
		return nil, nil
	}

	var delegation Delegation
	if err := json.Unmarshal([]byte(slotValue), &delegation); err != nil {
		return nil, fmt.Errorf("parsing delegation: %w", err)
	}

	// Keep issue reference for context (not used currently but available)
	_ = issue

	return &delegation, nil
}

// ListDelegationsFrom returns all delegations from a parent work unit.
// This searches for issues that have delegated_from pointing to the parent.
func (b *Beads) ListDelegationsFrom(parent string) ([]*Delegation, error) {
	// List all issues that depend on this parent (delegated work blocks parent)
	issues, err := b.List(ListOptions{Status: "all"})
	if err != nil {
		return nil, fmt.Errorf("listing issues: %w", err)
	}

	var delegations []*Delegation
	for _, issue := range issues {
		d, err := b.GetDelegation(issue.ID)
		if err != nil {
			continue // Skip issues with errors
		}
		if d != nil && d.Parent == parent {
			delegations = append(delegations, d)
		}
	}

	return delegations, nil
}

// Sync syncs beads with remote.
func (b *Beads) Sync() error {
	_, err := b.run("sync")
	return err
}

// SyncFromMain syncs beads updates from main branch.
func (b *Beads) SyncFromMain() error {
	_, err := b.run("sync", "--from-main")
	return err
}

// SyncStatus returns the sync status without performing a sync.
func (b *Beads) SyncStatus() (*SyncStatus, error) {
	out, err := b.run("sync", "--status", "--json")
	if err != nil {
		// If sync branch doesn't exist, return empty status
		if strings.Contains(err.Error(), "does not exist") {
			return &SyncStatus{}, nil
		}
		return nil, err
	}

	var status SyncStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, fmt.Errorf("parsing bd sync status output: %w", err)
	}

	return &status, nil
}

// Stats returns repository statistics.
func (b *Beads) Stats() (string, error) {
	out, err := b.run("stats")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// IsBeadsRepo checks if the working directory is a beads repository.
func (b *Beads) IsBeadsRepo() bool {
	_, err := b.run("list", "--limit=1")
	return err == nil || !errors.Is(err, ErrNotARepo)
}

// AgentFields holds structured fields for agent beads.
// These are stored as "key: value" lines in the description.
type AgentFields struct {
	RoleType          string // polecat, witness, refinery, deacon, mayor
	Rig               string // Rig name (empty for global agents like mayor/deacon)
	AgentState        string // spawning, working, done, stuck
	HookBead          string // Currently pinned work bead ID
	RoleBead          string // Role definition bead ID (canonical location; may not exist yet)
	CleanupStatus     string // ZFC: polecat self-reports git state (clean, has_uncommitted, has_stash, has_unpushed)
	ActiveMR          string // Currently active merge request bead ID (for traceability)
	NotificationLevel string // DND mode: verbose, normal, muted (default: normal)
}

// Notification level constants
const (
	NotifyVerbose = "verbose" // All notifications (mail, convoy events, etc.)
	NotifyNormal  = "normal"  // Important events only (default)
	NotifyMuted   = "muted"   // Silent/DND mode - batch for later
)

// FormatAgentDescription creates a description string from agent fields.
func FormatAgentDescription(title string, fields *AgentFields) string {
	if fields == nil {
		return title
	}

	var lines []string
	lines = append(lines, title)
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("role_type: %s", fields.RoleType))

	if fields.Rig != "" {
		lines = append(lines, fmt.Sprintf("rig: %s", fields.Rig))
	} else {
		lines = append(lines, "rig: null")
	}

	lines = append(lines, fmt.Sprintf("agent_state: %s", fields.AgentState))

	if fields.HookBead != "" {
		lines = append(lines, fmt.Sprintf("hook_bead: %s", fields.HookBead))
	} else {
		lines = append(lines, "hook_bead: null")
	}

	if fields.RoleBead != "" {
		lines = append(lines, fmt.Sprintf("role_bead: %s", fields.RoleBead))
	} else {
		lines = append(lines, "role_bead: null")
	}

	if fields.CleanupStatus != "" {
		lines = append(lines, fmt.Sprintf("cleanup_status: %s", fields.CleanupStatus))
	} else {
		lines = append(lines, "cleanup_status: null")
	}

	if fields.ActiveMR != "" {
		lines = append(lines, fmt.Sprintf("active_mr: %s", fields.ActiveMR))
	} else {
		lines = append(lines, "active_mr: null")
	}

	if fields.NotificationLevel != "" {
		lines = append(lines, fmt.Sprintf("notification_level: %s", fields.NotificationLevel))
	} else {
		lines = append(lines, "notification_level: null")
	}

	return strings.Join(lines, "\n")
}

// ParseAgentFields extracts agent fields from an issue's description.
func ParseAgentFields(description string) *AgentFields {
	fields := &AgentFields{}

	for _, line := range strings.Split(description, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}

		key := strings.TrimSpace(line[:colonIdx])
		value := strings.TrimSpace(line[colonIdx+1:])
		if value == "null" || value == "" {
			value = ""
		}

		switch strings.ToLower(key) {
		case "role_type":
			fields.RoleType = value
		case "rig":
			fields.Rig = value
		case "agent_state":
			fields.AgentState = value
		case "hook_bead":
			fields.HookBead = value
		case "role_bead":
			fields.RoleBead = value
		case "cleanup_status":
			fields.CleanupStatus = value
		case "active_mr":
			fields.ActiveMR = value
		case "notification_level":
			fields.NotificationLevel = value
		}
	}

	return fields
}

// CreateAgentBead creates an agent bead for tracking agent lifecycle.
// The ID format is: <prefix>-<rig>-<role>-<name> (e.g., gt-gastown-polecat-Toast)
// Use AgentBeadID() helper to generate correct IDs.
// The created_by field is populated from BD_ACTOR env var for provenance tracking.
func (b *Beads) CreateAgentBead(id, title string, fields *AgentFields) (*Issue, error) {
	description := FormatAgentDescription(title, fields)

	args := []string{"create", "--json",
		"--id=" + id,
		"--type=agent",
		"--title=" + title,
		"--description=" + description,
	}

	// Default actor from BD_ACTOR env var for provenance tracking
	if actor := os.Getenv("BD_ACTOR"); actor != "" {
		args = append(args, "--actor="+actor)
	}

	out, err := b.run(args...)
	if err != nil {
		return nil, err
	}

	var issue Issue
	if err := json.Unmarshal(out, &issue); err != nil {
		return nil, fmt.Errorf("parsing bd create output: %w", err)
	}

	// Set the role slot if specified (this is the authoritative storage)
	if fields != nil && fields.RoleBead != "" {
		if _, err := b.run("slot", "set", id, "role", fields.RoleBead); err != nil {
			// Non-fatal: warn but continue
			fmt.Printf("Warning: could not set role slot: %v\n", err)
		}
	}

	return &issue, nil
}

// UpdateAgentState updates the agent_state field in an agent bead.
// Optionally updates hook_bead if provided.
//
// IMPORTANT: This function uses the proper bd commands to update agent fields:
// - `bd agent state` for agent_state (uses SQLite column directly)
// - `bd slot set/clear` for hook_bead (uses SQLite column directly)
//
// This ensures consistency with `bd slot show` and other beads commands.
// Previously, this function embedded these fields in the description text,
// which caused inconsistencies with bd slot commands (see GH #gt-9v52).
func (b *Beads) UpdateAgentState(id string, state string, hookBead *string) error {
	// Update agent state using bd agent state command
	// This updates the agent_state column directly in SQLite
	_, err := b.run("agent", "state", id, state)
	if err != nil {
		return fmt.Errorf("updating agent state: %w", err)
	}

	// Update hook_bead if provided
	if hookBead != nil {
		if *hookBead != "" {
			// Set the hook using bd slot set
			// This updates the hook_bead column directly in SQLite
			_, err = b.run("slot", "set", id, "hook", *hookBead)
			if err != nil {
				// If slot is already occupied, clear it first then retry
				// This handles re-slinging scenarios where we're updating the hook
				errStr := err.Error()
				if strings.Contains(errStr, "already occupied") {
					_, _ = b.run("slot", "clear", id, "hook")
					_, err = b.run("slot", "set", id, "hook", *hookBead)
				}
				if err != nil {
					return fmt.Errorf("setting hook: %w", err)
				}
			}
		} else {
			// Clear the hook
			_, err = b.run("slot", "clear", id, "hook")
			if err != nil {
				return fmt.Errorf("clearing hook: %w", err)
			}
		}
	}

	return nil
}

// UpdateAgentCleanupStatus updates the cleanup_status field in an agent bead.
// This is called by the polecat to self-report its git state (ZFC compliance).
// Valid statuses: clean, has_uncommitted, has_stash, has_unpushed
func (b *Beads) UpdateAgentCleanupStatus(id string, cleanupStatus string) error {
	// First get current issue to preserve other fields
	issue, err := b.Show(id)
	if err != nil {
		return err
	}

	// Parse existing fields
	fields := ParseAgentFields(issue.Description)
	fields.CleanupStatus = cleanupStatus

	// Format new description
	description := FormatAgentDescription(issue.Title, fields)

	return b.Update(id, UpdateOptions{Description: &description})
}

// UpdateAgentActiveMR updates the active_mr field in an agent bead.
// This links the agent to their current merge request for traceability.
// Pass empty string to clear the field (e.g., after merge completes).
func (b *Beads) UpdateAgentActiveMR(id string, activeMR string) error {
	// First get current issue to preserve other fields
	issue, err := b.Show(id)
	if err != nil {
		return err
	}

	// Parse existing fields
	fields := ParseAgentFields(issue.Description)
	fields.ActiveMR = activeMR

	// Format new description
	description := FormatAgentDescription(issue.Title, fields)

	return b.Update(id, UpdateOptions{Description: &description})
}

// UpdateAgentNotificationLevel updates the notification_level field in an agent bead.
// Valid levels: verbose, normal, muted (DND mode).
// Pass empty string to reset to default (normal).
func (b *Beads) UpdateAgentNotificationLevel(id string, level string) error {
	// Validate level
	if level != "" && level != NotifyVerbose && level != NotifyNormal && level != NotifyMuted {
		return fmt.Errorf("invalid notification level %q: must be verbose, normal, or muted", level)
	}

	// First get current issue to preserve other fields
	issue, err := b.Show(id)
	if err != nil {
		return err
	}

	// Parse existing fields
	fields := ParseAgentFields(issue.Description)
	fields.NotificationLevel = level

	// Format new description
	description := FormatAgentDescription(issue.Title, fields)

	return b.Update(id, UpdateOptions{Description: &description})
}

// GetAgentNotificationLevel returns the notification level for an agent.
// Returns "normal" if not set (the default).
func (b *Beads) GetAgentNotificationLevel(id string) (string, error) {
	_, fields, err := b.GetAgentBead(id)
	if err != nil {
		return "", err
	}
	if fields == nil {
		return NotifyNormal, nil
	}
	if fields.NotificationLevel == "" {
		return NotifyNormal, nil
	}
	return fields.NotificationLevel, nil
}

// DeleteAgentBead permanently deletes an agent bead.
// Uses --hard --force for immediate permanent deletion (no tombstone).
func (b *Beads) DeleteAgentBead(id string) error {
	_, err := b.run("delete", id, "--hard", "--force")
	return err
}

// GetAgentBead retrieves an agent bead by ID.
// Returns nil if not found.
func (b *Beads) GetAgentBead(id string) (*Issue, *AgentFields, error) {
	issue, err := b.Show(id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	if issue.Type != "agent" {
		return nil, nil, fmt.Errorf("issue %s is not an agent bead (type: %s)", id, issue.Type)
	}

	fields := ParseAgentFields(issue.Description)
	return issue, fields, nil
}

// Agent bead ID naming convention:
//   prefix-rig-role-name
//
// Examples:
//   - gt-mayor (town-level, no rig)
//   - gt-deacon (town-level, no rig)
//   - gt-gastown-witness (rig-level singleton)
//   - gt-gastown-refinery (rig-level singleton)
//   - gt-gastown-crew-max (rig-level named agent)
//   - gt-gastown-polecat-Toast (rig-level named agent)

// AgentBeadIDWithPrefix generates an agent bead ID using the specified prefix.
// The prefix should NOT include the hyphen (e.g., "gt", "bd", not "gt-", "bd-").
// For town-level agents (mayor, deacon), pass empty rig and name.
// For rig-level singletons (witness, refinery), pass empty name.
// For named agents (crew, polecat), pass all three.
func AgentBeadIDWithPrefix(prefix, rig, role, name string) string {
	if rig == "" {
		// Town-level agent: prefix-mayor, prefix-deacon
		return prefix + "-" + role
	}
	if name == "" {
		// Rig-level singleton: prefix-rig-witness, prefix-rig-refinery
		return prefix + "-" + rig + "-" + role
	}
	// Rig-level named agent: prefix-rig-role-name
	return prefix + "-" + rig + "-" + role + "-" + name
}

// AgentBeadID generates the canonical agent bead ID using "gt" prefix.
// For non-gastown rigs, use AgentBeadIDWithPrefix with the rig's configured prefix.
func AgentBeadID(rig, role, name string) string {
	return AgentBeadIDWithPrefix("gt", rig, role, name)
}

// MayorBeadID returns the Mayor agent bead ID.
func MayorBeadID() string {
	return "gt-mayor"
}

// DeaconBeadID returns the Deacon agent bead ID.
func DeaconBeadID() string {
	return "gt-deacon"
}

// DogBeadID returns a Dog agent bead ID.
// Dogs are town-level agents, so they follow the pattern: gt-dog-<name>
func DogBeadID(name string) string {
	return "gt-dog-" + name
}

// DogRoleBeadID returns the Dog role bead ID.
func DogRoleBeadID() string {
	return RoleBeadID("dog")
}

// CreateDogAgentBead creates an agent bead for a dog.
// Dogs use a different schema than other agents - they use labels for metadata.
// Returns the created issue or an error.
func (b *Beads) CreateDogAgentBead(name, location string) (*Issue, error) {
	title := fmt.Sprintf("Dog: %s", name)
	labels := []string{
		"role_type:dog",
		"rig:town",
		"location:" + location,
	}

	args := []string{
		"create", "--json",
		"--type=agent",
		"--role-type=dog",
		"--title=" + title,
		"--labels=" + strings.Join(labels, ","),
	}

	// Default actor from BD_ACTOR env var for provenance tracking
	if actor := os.Getenv("BD_ACTOR"); actor != "" {
		args = append(args, "--actor="+actor)
	}

	out, err := b.run(args...)
	if err != nil {
		return nil, err
	}

	var issue Issue
	if err := json.Unmarshal(out, &issue); err != nil {
		return nil, fmt.Errorf("parsing bd create output: %w", err)
	}

	return &issue, nil
}

// FindDogAgentBead finds the agent bead for a dog by name.
// Searches for agent beads with role_type:dog and matching title.
// Returns nil if not found.
func (b *Beads) FindDogAgentBead(name string) (*Issue, error) {
	// List all agent beads and filter by role_type:dog label
	issues, err := b.List(ListOptions{
		Type:     "agent",
		Status:   "all",
		Priority: -1, // No priority filter
	})
	if err != nil {
		return nil, fmt.Errorf("listing agents: %w", err)
	}

	expectedTitle := fmt.Sprintf("Dog: %s", name)
	for _, issue := range issues {
		// Check title match and role_type:dog label
		if issue.Title == expectedTitle {
			for _, label := range issue.Labels {
				if label == "role_type:dog" {
					return issue, nil
				}
			}
		}
	}

	return nil, nil
}

// DeleteDogAgentBead finds and deletes the agent bead for a dog.
// Returns nil if the bead doesn't exist (idempotent).
func (b *Beads) DeleteDogAgentBead(name string) error {
	issue, err := b.FindDogAgentBead(name)
	if err != nil {
		return fmt.Errorf("finding dog bead: %w", err)
	}
	if issue == nil {
		return nil // Already doesn't exist - idempotent
	}

	err = b.DeleteAgentBead(issue.ID)
	if err != nil {
		return fmt.Errorf("deleting bead %s: %w", issue.ID, err)
	}
	return nil
}

// WitnessBeadIDWithPrefix returns the Witness agent bead ID for a rig using the specified prefix.
func WitnessBeadIDWithPrefix(prefix, rig string) string {
	return AgentBeadIDWithPrefix(prefix, rig, "witness", "")
}

// WitnessBeadID returns the Witness agent bead ID for a rig using "gt" prefix.
func WitnessBeadID(rig string) string {
	return WitnessBeadIDWithPrefix("gt", rig)
}

// RefineryBeadIDWithPrefix returns the Refinery agent bead ID for a rig using the specified prefix.
func RefineryBeadIDWithPrefix(prefix, rig string) string {
	return AgentBeadIDWithPrefix(prefix, rig, "refinery", "")
}

// RefineryBeadID returns the Refinery agent bead ID for a rig using "gt" prefix.
func RefineryBeadID(rig string) string {
	return RefineryBeadIDWithPrefix("gt", rig)
}

// CrewBeadIDWithPrefix returns a Crew worker agent bead ID using the specified prefix.
func CrewBeadIDWithPrefix(prefix, rig, name string) string {
	return AgentBeadIDWithPrefix(prefix, rig, "crew", name)
}

// CrewBeadID returns a Crew worker agent bead ID using "gt" prefix.
func CrewBeadID(rig, name string) string {
	return CrewBeadIDWithPrefix("gt", rig, name)
}

// PolecatBeadIDWithPrefix returns a Polecat agent bead ID using the specified prefix.
func PolecatBeadIDWithPrefix(prefix, rig, name string) string {
	return AgentBeadIDWithPrefix(prefix, rig, "polecat", name)
}

// PolecatBeadID returns a Polecat agent bead ID using "gt" prefix.
func PolecatBeadID(rig, name string) string {
	return PolecatBeadIDWithPrefix("gt", rig, name)
}

// ParseAgentBeadID parses an agent bead ID into its components.
// Returns rig, role, name, and whether parsing succeeded.
// For town-level agents, rig will be empty.
// For singletons, name will be empty.
// Accepts any valid prefix (e.g., "gt-", "bd-"), not just "gt-".
func ParseAgentBeadID(id string) (rig, role, name string, ok bool) {
	// Find the prefix (everything before the first hyphen)
	// Valid prefixes are 2-3 characters (e.g., "gt", "bd", "hq")
	hyphenIdx := strings.Index(id, "-")
	if hyphenIdx < 2 || hyphenIdx > 3 {
		return "", "", "", false
	}

	rest := id[hyphenIdx+1:]
	parts := strings.Split(rest, "-")

	switch len(parts) {
	case 1:
		// Town-level: gt-mayor, bd-deacon
		return "", parts[0], "", true
	case 2:
		// Could be rig-level singleton (gt-gastown-witness) or
		// town-level named (gt-dog-alpha for dogs)
		if parts[0] == "dog" {
			// Dogs are town-level named agents: gt-dog-<name>
			return "", "dog", parts[1], true
		}
		// Rig-level singleton: gt-gastown-witness
		return parts[0], parts[1], "", true
	case 3:
		// Rig-level named: gt-gastown-crew-max, bd-beads-polecat-pearl
		return parts[0], parts[1], parts[2], true
	default:
		// Handle names with hyphens: gt-gastown-polecat-my-agent-name
		// or gt-dog-my-agent-name
		if len(parts) >= 3 {
			if parts[0] == "dog" {
				// Dog with hyphenated name: gt-dog-my-dog-name
				return "", "dog", strings.Join(parts[1:], "-"), true
			}
			return parts[0], parts[1], strings.Join(parts[2:], "-"), true
		}
		return "", "", "", false
	}
}

// IsAgentSessionBead returns true if the bead ID represents an agent session molecule.
// Agent session beads follow patterns like gt-mayor, bd-beads-witness, gt-gastown-crew-joe.
// Supports any valid prefix (e.g., "gt-", "bd-"), not just "gt-".
// These are used to track agent state and update frequently, which can create noise.
func IsAgentSessionBead(beadID string) bool {
	_, role, _, ok := ParseAgentBeadID(beadID)
	if !ok {
		return false
	}
	// Known agent roles
	switch role {
	case "mayor", "deacon", "witness", "refinery", "crew", "polecat", "dog":
		return true
	default:
		return false
	}
}

// Role bead ID naming convention:
//   gt-<role>-role
//
// Examples:
//   - gt-mayor-role
//   - gt-deacon-role
//   - gt-witness-role
//   - gt-refinery-role
//   - gt-crew-role
//   - gt-polecat-role

// RoleBeadID returns the role bead ID for a given role type.
// Role beads define lifecycle configuration for each agent type.
func RoleBeadID(roleType string) string {
	return "gt-" + roleType + "-role"
}

// MayorRoleBeadID returns the Mayor role bead ID.
func MayorRoleBeadID() string {
	return RoleBeadID("mayor")
}

// DeaconRoleBeadID returns the Deacon role bead ID.
func DeaconRoleBeadID() string {
	return RoleBeadID("deacon")
}

// WitnessRoleBeadID returns the Witness role bead ID.
func WitnessRoleBeadID() string {
	return RoleBeadID("witness")
}

// RefineryRoleBeadID returns the Refinery role bead ID.
func RefineryRoleBeadID() string {
	return RoleBeadID("refinery")
}

// CrewRoleBeadID returns the Crew role bead ID.
func CrewRoleBeadID() string {
	return RoleBeadID("crew")
}

// PolecatRoleBeadID returns the Polecat role bead ID.
func PolecatRoleBeadID() string {
	return RoleBeadID("polecat")
}

// GetRoleConfig looks up a role bead and returns its parsed RoleConfig.
// Returns nil, nil if the role bead doesn't exist or has no config.
func (b *Beads) GetRoleConfig(roleBeadID string) (*RoleConfig, error) {
	issue, err := b.Show(roleBeadID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if issue.Type != "role" {
		return nil, fmt.Errorf("bead %s is not a role bead (type: %s)", roleBeadID, issue.Type)
	}

	return ParseRoleConfig(issue.Description), nil
}

// FindMRForBranch searches for an existing merge-request bead for the given branch.
// Returns the MR bead if found, nil if not found.
// This enables idempotent `gt done` - if an MR already exists, we skip creation.
func (b *Beads) FindMRForBranch(branch string) (*Issue, error) {
	// List all merge-request beads (open status only - closed MRs are already processed)
	issues, err := b.List(ListOptions{
		Status: "open",
		Type:   "merge-request",
	})
	if err != nil {
		return nil, err
	}

	// Search for one matching this branch
	// MR description format: "branch: <branch>\ntarget: ..."
	branchPrefix := "branch: " + branch + "\n"
	for _, issue := range issues {
		if strings.HasPrefix(issue.Description, branchPrefix) {
			return issue, nil
		}
	}

	return nil, nil
}

// AddGateWaiter registers an agent as a waiter on a gate bead.
// When the gate closes, the waiter will receive a wake notification via gt gate wake.
// The waiter is typically the polecat's address (e.g., "gastown/polecats/Toast").
func (b *Beads) AddGateWaiter(gateID, waiter string) error {
	// Use bd gate add-waiter to register the waiter on the gate
	// This adds the waiter to the gate's native waiters field
	_, err := b.run("gate", "add-waiter", gateID, waiter)
	if err != nil {
		return fmt.Errorf("adding gate waiter: %w", err)
	}
	return nil
}

// ===== Merge Slot Functions (serialized conflict resolution) =====

// MergeSlotStatus represents the result of checking a merge slot.
type MergeSlotStatus struct {
	ID        string   `json:"id"`
	Available bool     `json:"available"`
	Holder    string   `json:"holder,omitempty"`
	Waiters   []string `json:"waiters,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// MergeSlotCreate creates the merge slot bead for the current rig.
// The slot is used for serialized conflict resolution in the merge queue.
// Returns the slot ID if successful.
func (b *Beads) MergeSlotCreate() (string, error) {
	out, err := b.run("merge-slot", "create", "--json")
	if err != nil {
		return "", fmt.Errorf("creating merge slot: %w", err)
	}

	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("parsing merge-slot create output: %w", err)
	}

	return result.ID, nil
}

// MergeSlotCheck checks the availability of the merge slot.
// Returns the current status including holder and waiters if held.
func (b *Beads) MergeSlotCheck() (*MergeSlotStatus, error) {
	out, err := b.run("merge-slot", "check", "--json")
	if err != nil {
		// Check if slot doesn't exist
		if strings.Contains(err.Error(), "not found") {
			return &MergeSlotStatus{Error: "not found"}, nil
		}
		return nil, fmt.Errorf("checking merge slot: %w", err)
	}

	var status MergeSlotStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, fmt.Errorf("parsing merge-slot check output: %w", err)
	}

	return &status, nil
}

// MergeSlotAcquire attempts to acquire the merge slot for exclusive access.
// If holder is empty, defaults to BD_ACTOR environment variable.
// If addWaiter is true and the slot is held, the requester is added to the waiters queue.
// Returns the acquisition result.
func (b *Beads) MergeSlotAcquire(holder string, addWaiter bool) (*MergeSlotStatus, error) {
	args := []string{"merge-slot", "acquire", "--json"}
	if holder != "" {
		args = append(args, "--holder="+holder)
	}
	if addWaiter {
		args = append(args, "--wait")
	}

	out, err := b.run(args...)
	if err != nil {
		// Parse the output even on error - it may contain useful info
		var status MergeSlotStatus
		if jsonErr := json.Unmarshal(out, &status); jsonErr == nil {
			return &status, nil
		}
		return nil, fmt.Errorf("acquiring merge slot: %w", err)
	}

	var status MergeSlotStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, fmt.Errorf("parsing merge-slot acquire output: %w", err)
	}

	return &status, nil
}

// MergeSlotRelease releases the merge slot after conflict resolution completes.
// If holder is provided, it verifies the slot is held by that holder before releasing.
func (b *Beads) MergeSlotRelease(holder string) error {
	args := []string{"merge-slot", "release", "--json"}
	if holder != "" {
		args = append(args, "--holder="+holder)
	}

	out, err := b.run(args...)
	if err != nil {
		return fmt.Errorf("releasing merge slot: %w", err)
	}

	var result struct {
		Released bool   `json:"released"`
		Error    string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return fmt.Errorf("parsing merge-slot release output: %w", err)
	}

	if !result.Released && result.Error != "" {
		return fmt.Errorf("slot release failed: %s", result.Error)
	}

	return nil
}

// MergeSlotEnsureExists creates the merge slot if it doesn't exist.
// This is idempotent - safe to call multiple times.
func (b *Beads) MergeSlotEnsureExists() (string, error) {
	// Check if slot exists first
	status, err := b.MergeSlotCheck()
	if err != nil {
		return "", err
	}

	if status.Error == "not found" {
		// Create it
		return b.MergeSlotCreate()
	}

	return status.ID, nil
}
