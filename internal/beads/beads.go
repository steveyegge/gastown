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

	"github.com/steveyegge/gastown/internal/runtime"
)

// Common errors
// ZFC: Only define errors that don't require stderr parsing for decisions.
// ErrNotARepo and ErrSyncConflict were removed - agents should handle these directly.
var (
	ErrNotInstalled = errors.New("bd not installed: run 'pip install beads-cli' or see https://github.com/anthropics/beads")
	ErrNotFound     = errors.New("issue not found")
)

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
	AgentState string `json:"agent_state,omitempty"` // Agent lifecycle state (spawning, working, done, stuck)
	// Note: role_bead field removed - role definitions are now config-based

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

// ListOptions specifies filters for listing issues.
type ListOptions struct {
	Status     string // "open", "closed", "all"
	Type       string // Deprecated: use Label instead. "task", "bug", "feature", "epic"
	Label      string // Label filter (e.g., "gt:agent", "gt:merge-request")
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
	Ephemeral   bool   // Create as ephemeral (wisp) - not exported to JSONL
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
	isolated bool   // If true, suppress inherited beads env vars (for test isolation)

	// Lazy-cached town root for routing resolution.
	// Populated on first call to getTownRoot() to avoid filesystem walk on every operation.
	townRoot     string
	searchedRoot bool
}

// New creates a new Beads wrapper for the given directory.
func New(workDir string) *Beads {
	return &Beads{workDir: workDir}
}

// NewIsolated creates a Beads wrapper for test isolation.
// This suppresses inherited beads env vars (BD_ACTOR, BEADS_DB) to prevent
// tests from accidentally routing to production databases.
func NewIsolated(workDir string) *Beads {
	return &Beads{workDir: workDir, isolated: true}
}

// NewWithBeadsDir creates a Beads wrapper with an explicit BEADS_DIR.
// This is needed when running from a polecat worktree but accessing town-level beads.
func NewWithBeadsDir(workDir, beadsDir string) *Beads {
	return &Beads{workDir: workDir, beadsDir: beadsDir}
}

// getActor returns the BD_ACTOR value for this context.
// Returns empty string when in isolated mode (tests) to prevent
// inherited actors from routing to production databases.
func (b *Beads) getActor() string {
	if b.isolated {
		return ""
	}
	return os.Getenv("BD_ACTOR")
}

// getTownRoot returns the Gas Town root directory, using lazy caching.
// The town root is found by walking up from workDir looking for mayor/town.json.
// Returns empty string if not in a Gas Town project.
func (b *Beads) getTownRoot() string {
	if !b.searchedRoot {
		b.townRoot = FindTownRoot(b.workDir)
		b.searchedRoot = true
	}
	return b.townRoot
}

// getResolvedBeadsDir returns the beads directory this wrapper is operating on.
// This follows any redirects and returns the actual beads directory path.
func (b *Beads) getResolvedBeadsDir() string {
	if b.beadsDir != "" {
		return b.beadsDir
	}
	return ResolveBeadsDir(b.workDir)
}

// Init initializes a new beads database in the working directory.
// This uses the same environment isolation as other commands.
func (b *Beads) Init(prefix string) error {
	_, err := b.run("init", "--prefix", prefix, "--quiet")
	return err
}

// run executes a bd command and returns stdout.
func (b *Beads) run(args ...string) ([]byte, error) {
	// Use --no-daemon for faster read operations (avoids daemon IPC overhead)
	// The daemon is primarily useful for write coalescing, not reads.
	// Use --allow-stale to prevent failures when db is out of sync with JSONL
	// (e.g., after daemon is killed during shutdown before syncing).
	fullArgs := append([]string{"--no-daemon", "--allow-stale"}, args...)

	// Always explicitly set BEADS_DIR to prevent inherited env vars from
	// causing prefix mismatches. Use explicit beadsDir if set, otherwise
	// resolve from working directory.
	beadsDir := b.beadsDir
	if beadsDir == "" {
		beadsDir = ResolveBeadsDir(b.workDir)
	}

	// In isolated mode, use --db flag to force specific database path
	// This bypasses bd's routing logic that can redirect to .beads-planning
	// Skip --db for init command since it creates the database
	isInit := len(args) > 0 && args[0] == "init"
	if b.isolated && !isInit {
		beadsDB := filepath.Join(beadsDir, "beads.db")
		fullArgs = append([]string{"--db", beadsDB}, fullArgs...)
	}

	cmd := exec.Command("bd", fullArgs...) //nolint:gosec // G204: bd is a trusted internal tool
	cmd.Dir = b.workDir

	// Build environment: filter beads env vars when in isolated mode (tests)
	// to prevent routing to production databases.
	var env []string
	if b.isolated {
		env = filterBeadsEnv(os.Environ())
	} else {
		env = os.Environ()
	}
	cmd.Env = append(env, "BEADS_DIR="+beadsDir)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, b.wrapError(err, stderr.String(), args)
	}

	// Handle bd --no-daemon exit code 0 bug: when issue not found,
	// --no-daemon exits 0 but writes error to stderr with empty stdout.
	// Detect this case and treat as error to avoid JSON parse failures.
	if stdout.Len() == 0 && stderr.Len() > 0 {
		return nil, b.wrapError(fmt.Errorf("command produced no output"), stderr.String(), args)
	}

	return stdout.Bytes(), nil
}

// Run executes a bd command and returns stdout.
// This is a public wrapper around the internal run method for cases where
// callers need to run arbitrary bd commands.
func (b *Beads) Run(args ...string) ([]byte, error) {
	return b.run(args...)
}

// wrapError wraps bd errors with context.
// ZFC: Avoid parsing stderr to make decisions. Transport errors to agents instead.
// Exception: ErrNotInstalled (exec.ErrNotFound) and ErrNotFound (issue lookup) are
// acceptable as they enable basic error handling without decision-making.
func (b *Beads) wrapError(err error, stderr string, args []string) error {
	stderr = strings.TrimSpace(stderr)

	// Check for bd not installed
	if execErr, ok := err.(*exec.Error); ok && errors.Is(execErr.Err, exec.ErrNotFound) {
		return ErrNotInstalled
	}

	// ErrNotFound is widely used for issue lookups - acceptable exception
	// Match various "not found" error patterns from bd
	if strings.Contains(stderr, "not found") || strings.Contains(stderr, "Issue not found") ||
		strings.Contains(stderr, "no issue found") {
		return ErrNotFound
	}

	if stderr != "" {
		return fmt.Errorf("bd %s: %s", strings.Join(args, " "), stderr)
	}
	return fmt.Errorf("bd %s: %w", strings.Join(args, " "), err)
}

// filterBeadsEnv removes beads-related environment variables from the given
// environment slice. This ensures test isolation by preventing inherited
// BD_ACTOR, BEADS_DB, GT_ROOT, HOME etc. from routing commands to production databases.
func filterBeadsEnv(environ []string) []string {
	filtered := make([]string, 0, len(environ))
	for _, env := range environ {
		// Skip beads-related env vars that could interfere with test isolation
		// BD_ACTOR, BEADS_* - direct beads config
		// GT_ROOT - causes bd to find global routes file
		// HOME - causes bd to find ~/.beads-planning routing
		if strings.HasPrefix(env, "BD_ACTOR=") ||
			strings.HasPrefix(env, "BEADS_") ||
			strings.HasPrefix(env, "GT_ROOT=") ||
			strings.HasPrefix(env, "HOME=") {
			continue
		}
		filtered = append(filtered, env)
	}
	return filtered
}

// List returns issues matching the given options.
func (b *Beads) List(opts ListOptions) ([]*Issue, error) {
	args := []string{"list", "--json"}

	if opts.Status != "" {
		args = append(args, "--status="+opts.Status)
	}
	// Prefer Label over Type (Type is deprecated)
	if opts.Label != "" {
		args = append(args, "--label="+opts.Label)
	} else if opts.Type != "" {
		// Deprecated: convert type to label for backward compatibility
		args = append(args, "--label=gt:"+opts.Type)
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
// The assignee is typically in the format "rig/polecats/polecatName" (e.g., "gastown/polecats/Toast").
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

// ReadyWithType returns ready issues filtered by label.
// Uses bd ready --label flag for server-side filtering.
// The issueType is converted to a gt:<type> label (e.g., "molecule" -> "gt:molecule").
func (b *Beads) ReadyWithType(issueType string) ([]*Issue, error) {
	out, err := b.run("ready", "--json", "--label", "gt:"+issueType, "-n", "100")
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
	// Set issue type AND add gt:<type> label for backwards compatibility
	if opts.Type != "" {
		args = append(args, "--type="+opts.Type)
		args = append(args, "--labels=gt:"+opts.Type)
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
	if opts.Ephemeral {
		args = append(args, "--ephemeral")
	}
	// Default Actor from BD_ACTOR env var if not specified
	// Uses getActor() to respect isolated mode (tests)
	actor := opts.Actor
	if actor == "" {
		actor = b.getActor()
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

// CreateWithID creates an issue with a specific ID.
// This is useful for agent beads, role beads, and other beads that need
// deterministic IDs rather than auto-generated ones.
func (b *Beads) CreateWithID(id string, opts CreateOptions) (*Issue, error) {
	args := []string{"create", "--json", "--id=" + id}
	if NeedsForceForID(id) {
		args = append(args, "--force")
	}

	if opts.Title != "" {
		args = append(args, "--title="+opts.Title)
	}
	// Set issue type AND add gt:<type> label for backwards compatibility
	if opts.Type != "" {
		args = append(args, "--type="+opts.Type)
		args = append(args, "--labels=gt:"+opts.Type)
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
	// Uses getActor() to respect isolated mode (tests)
	actor := opts.Actor
	if actor == "" {
		actor = b.getActor()
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

// AddLabel adds a label to an issue.
func (b *Beads) AddLabel(id, label string) error {
	_, err := b.run("label", "add", id, label)
	return err
}

// Close closes one or more issues.
// If a runtime session ID is set in the environment, it is passed to bd close
// for work attribution tracking (see decision 009-session-events-architecture.md).
func (b *Beads) Close(ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	args := append([]string{"close"}, ids...)

	// Pass session ID for work attribution if available
	if sessionID := runtime.SessionIDFromEnv(); sessionID != "" {
		args = append(args, "--session="+sessionID)
	}

	_, err := b.run(args...)
	return err
}

// CloseWithReason closes one or more issues with a reason.
// If a runtime session ID is set in the environment, it is passed to bd close
// for work attribution tracking (see decision 009-session-events-architecture.md).
func (b *Beads) CloseWithReason(reason string, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}

	args := append([]string{"close"}, ids...)
	args = append(args, "--reason="+reason)

	// Pass session ID for work attribution if available
	if sessionID := runtime.SessionIDFromEnv(); sessionID != "" {
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

// GetSyncStatus returns the sync status without performing a sync.
func (b *Beads) GetSyncStatus() (*SyncStatus, error) {
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
// ZFC: Check file existence directly instead of parsing bd errors.
func (b *Beads) IsBeadsRepo() bool {
	beadsDir := ResolveBeadsDir(b.workDir)
	info, err := os.Stat(beadsDir)
	return err == nil && info.IsDir()
}

// primeContent is the Gas Town PRIME.md content that provides essential context
// for crew workers. This is the fallback if the SessionStart hook fails.
const primeContent = `# Gas Town Worker Context

> **Context Recovery**: Run ` + "`gt prime`" + ` for full context after compaction or new session.

## The Propulsion Principle (GUPP)

**If you find work on your hook, YOU RUN IT.**

No confirmation. No waiting. No announcements. The hook having work IS the assignment.
This is physics, not politeness. Gas Town is a steam engine - you are a piston.

**Failure mode we're preventing:**
- Agent starts with work on hook
- Agent announces itself and waits for human to say "ok go"
- Human is AFK / trusting the engine to run
- Work sits idle. The whole system stalls.

## Startup Protocol

1. Check your hook: ` + "`gt mol status`" + `
2. If work is hooked → EXECUTE (no announcement, no waiting)
3. If hook empty → Check mail: ` + "`gt mail inbox`" + `
4. Still nothing? Wait for user instructions

## Key Commands

- ` + "`gt prime`" + ` - Get full role context (run after compaction)
- ` + "`gt mol status`" + ` - Check your hooked work
- ` + "`gt mail inbox`" + ` - Check for messages
- ` + "`bd ready`" + ` - Find available work (no blockers)

## Session Close Protocol

Before signaling completion:
1. git status (check what changed)
2. git add <files> (stage code changes)
3. git commit -m "..." (commit code)
4. git push (push to remote)
5. ` + "`gt done`" + ` (submit to merge queue and exit)

**Polecats MUST call ` + "`gt done`" + ` - this submits work and exits the session.**
`

// ProvisionPrimeMD writes the Gas Town PRIME.md file to the specified beads directory.
// This provides essential Gas Town context (GUPP, startup protocol) as a fallback
// if the SessionStart hook fails. The PRIME.md is read by bd prime.
//
// The beadsDir should be the actual beads directory (after following any redirect).
// Returns nil if PRIME.md already exists (idempotent).
func ProvisionPrimeMD(beadsDir string) error {
	primePath := filepath.Join(beadsDir, "PRIME.md")

	// Check if already exists - don't overwrite customizations
	if _, err := os.Stat(primePath); err == nil {
		return nil // Already exists, don't overwrite
	}

	// Create .beads directory if it doesn't exist
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		return fmt.Errorf("creating beads dir: %w", err)
	}

	// Write PRIME.md
	if err := os.WriteFile(primePath, []byte(primeContent), 0644); err != nil {
		return fmt.Errorf("writing PRIME.md: %w", err)
	}

	return nil
}

// ProvisionPrimeMDForWorktree provisions PRIME.md for a worktree by following its redirect.
// This is the main entry point for crew/polecat provisioning.
func ProvisionPrimeMDForWorktree(worktreePath string) error {
	// Resolve the beads directory (follows redirect chain)
	beadsDir := ResolveBeadsDir(worktreePath)

	// Provision PRIME.md in the target directory
	return ProvisionPrimeMD(beadsDir)
}
