package refinery

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/util"
)

// Common errors
var (
	ErrNotRunning     = errors.New("refinery not running")
	ErrAlreadyRunning = errors.New("refinery already running")
	ErrNoQueue        = errors.New("no items in queue")
)

// Manager handles refinery lifecycle and queue operations.
type Manager struct {
	rig     *rig.Rig
	workDir string
	output  io.Writer // Output destination for user-facing messages
}

// NewManager creates a new refinery manager for a rig.
func NewManager(r *rig.Rig) *Manager {
	return &Manager{
		rig:     r,
		workDir: r.Path,
		output:  os.Stdout,
	}
}

// SetOutput sets the output writer for user-facing messages.
// This is useful for testing or redirecting output.
func (m *Manager) SetOutput(w io.Writer) {
	m.output = w
}

// SessionName returns the tmux session name for this refinery.
func (m *Manager) SessionName() string {
	return fmt.Sprintf("gt-%s-refinery", m.rig.Name)
}

// IsRunning checks if the refinery session is active.
// ZFC: tmux session existence is the source of truth.
func (m *Manager) IsRunning() (bool, error) {
	t := tmux.NewTmux()
	return t.HasSession(m.SessionName())
}

// Status returns information about the refinery session.
// ZFC-compliant: tmux session is the source of truth.
func (m *Manager) Status() (*tmux.SessionInfo, error) {
	t := tmux.NewTmux()
	sessionID := m.SessionName()

	running, err := t.HasSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return nil, ErrNotRunning
	}

	return t.GetSessionInfo(sessionID)
}

// Start starts the refinery.
// If foreground is true, returns an error (foreground mode deprecated).
// Otherwise, spawns a Claude agent in a tmux session to process the merge queue.
// The agentOverride parameter allows specifying an agent alias to use instead of the town default.
// ZFC-compliant: no state file, tmux session is source of truth.
func (m *Manager) Start(foreground bool, agentOverride string) error {
	t := tmux.NewTmux()
	sessionID := m.SessionName()

	if foreground {
		// Foreground mode is deprecated - the Refinery agent handles merge processing
		return fmt.Errorf("foreground mode is deprecated; use background mode (remove --foreground flag)")
	}

	// Check if session already exists
	running, _ := t.HasSession(sessionID)
	if running {
		// Session exists - check if Claude is actually running (healthy vs zombie)
		// Use IsClaudeRunning for robust detection: Claude can report as "node", "claude",
		// or version number like "2.0.76". IsAgentRunning with just "node" was too strict
		// and caused healthy sessions to be killed. See: gastown#566
		if t.IsClaudeRunning(sessionID) {
			// Healthy - Claude is running
			return ErrAlreadyRunning
		}
		// Zombie - tmux alive but Claude dead. Kill and recreate.
		_, _ = fmt.Fprintln(m.output, "⚠ Detected zombie session (tmux alive, agent dead). Recreating...")
		if err := t.KillSessionWithProcesses(sessionID); err != nil {
			return fmt.Errorf("killing zombie session: %w", err)
		}
	}

	// Note: No PID check per ZFC - tmux session is the source of truth

	// Background mode: spawn a Claude agent in a tmux session
	// The Claude agent handles MR processing using git commands and beads

	// Working directory is the refinery worktree (shares .git with mayor/polecats)
	refineryRigDir := filepath.Join(m.rig.Path, "refinery", "rig")
	if _, err := os.Stat(refineryRigDir); os.IsNotExist(err) {
		// Fall back to mayor/rig (legacy architecture) - ensures we use project git, not town git.
		// Using rig.Path directly would find town's .git with rig-named remotes instead of "origin".
		refineryRigDir = filepath.Join(m.rig.Path, "mayor", "rig")
	}

	// Ensure runtime settings exist in refinery/ (not refinery/rig/) so we don't
	// write into the source repo. Runtime walks up the tree to find settings.
	refineryParentDir := filepath.Join(m.rig.Path, "refinery")
	townRoot := filepath.Dir(m.rig.Path)
	runtimeConfig := config.ResolveRoleAgentConfig("refinery", townRoot, m.rig.Path)
	if err := runtime.EnsureSettingsForRole(refineryParentDir, "refinery", runtimeConfig); err != nil {
		return fmt.Errorf("ensuring runtime settings: %w", err)
	}

	// Build startup command first
	var command string
	if agentOverride != "" {
		var err error
		command, err = config.BuildAgentStartupCommandWithAgentOverride("refinery", m.rig.Name, townRoot, m.rig.Path, "gt prime", agentOverride)
		if err != nil {
			return fmt.Errorf("building startup command with agent override: %w", err)
		}
	} else {
		command = config.BuildAgentStartupCommand("refinery", m.rig.Name, townRoot, m.rig.Path, "gt prime")
	}

	// Create session with command directly to avoid send-keys race condition.
	// See: https://github.com/anthropics/gastown/issues/280
	if err := t.NewSessionWithCommand(sessionID, refineryRigDir, command); err != nil {
		return fmt.Errorf("creating tmux session: %w", err)
	}

	// Set environment variables (non-fatal: session works without these)
	// Use centralized AgentEnv for consistency across all role startup paths
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:          "refinery",
		Rig:           m.rig.Name,
		TownRoot:      townRoot,
		BeadsNoDaemon: true,
	})

	// Add refinery-specific flag
	envVars["GT_REFINERY"] = "1"

	// Set all env vars in tmux session (for debugging) and they'll also be exported to Claude
	for k, v := range envVars {
		_ = t.SetEnvironment(sessionID, k, v)
	}

	// Apply theme (non-fatal: theming failure doesn't affect operation)
	theme := tmux.AssignTheme(m.rig.Name)
	_ = t.ConfigureGasTownSession(sessionID, theme, m.rig.Name, "refinery", "refinery")

	// Accept bypass permissions warning dialog if it appears.
	// Must be before WaitForRuntimeReady to avoid race where dialog blocks prompt detection.
	_ = t.AcceptBypassPermissionsWarning(sessionID)

	// Wait for Claude to start and show its prompt - fatal if Claude fails to launch
	// WaitForRuntimeReady waits for the runtime to be ready
	if err := t.WaitForRuntimeReady(sessionID, runtimeConfig, constants.ClaudeStartTimeout); err != nil {
		// Kill the zombie session before returning error
		_ = t.KillSessionWithProcesses(sessionID)
		return fmt.Errorf("waiting for refinery to start: %w", err)
	}

	// Wait for runtime to be fully ready
	runtime.SleepForReadyDelay(runtimeConfig)
	_ = runtime.RunStartupFallback(t, sessionID, "refinery", runtimeConfig)

	// Propulsion is handled by the CLI prompt ("gt prime") passed at startup.
	// No need for post-startup nudges which are unreliable (text arrives before input is ready).
	// The SessionStart hook also runs "gt prime" as a backup.

	return nil
}

// Stop stops the refinery.
// ZFC-compliant: tmux session is the source of truth.
func (m *Manager) Stop() error {
	t := tmux.NewTmux()
	sessionID := m.SessionName()

	// Check if tmux session exists
	running, _ := t.HasSession(sessionID)
	if !running {
		return ErrNotRunning
	}

	// Kill the tmux session
	return t.KillSession(sessionID)
}

// Queue returns the current merge queue.
// Uses beads merge-request issues as the source of truth (not git branches).
// ZFC-compliant: beads is the source of truth, no state file.
func (m *Manager) Queue() ([]QueueItem, error) {
	// Query beads for open MRs by label (bd-3q6.10: MRs now use type=task)
	// BeadsPath() returns the git-synced beads location
	b := beads.New(m.rig.BeadsPath())
	issues, err := b.List(beads.ListOptions{
		Label:    "gt:merge-request",
		Status:   "open",
		Priority: -1, // No priority filter
	})
	if err != nil {
		return nil, fmt.Errorf("querying merge queue from beads: %w", err)
	}

	// Score and sort issues by priority score (highest first)
	now := time.Now()
	type scoredIssue struct {
		issue *beads.Issue
		score float64
	}
	scored := make([]scoredIssue, 0, len(issues))
	for _, issue := range issues {
		score := m.calculateIssueScore(issue, now)
		scored = append(scored, scoredIssue{issue: issue, score: score})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Convert scored issues to queue items
	var items []QueueItem
	pos := 1
	for _, s := range scored {
		mr := m.issueToMR(s.issue)
		if mr != nil {
			items = append(items, QueueItem{
				Position: pos,
				MR:       mr,
				Age:      formatAge(mr.CreatedAt),
			})
			pos++
		}
	}

	return items, nil
}

// calculateIssueScore computes the priority score for an MR issue.
// Higher scores mean higher priority (process first).
func (m *Manager) calculateIssueScore(issue *beads.Issue, now time.Time) float64 {
	fields := beads.ParseMRFields(issue)

	// Parse MR creation time
	mrCreatedAt := parseTime(issue.CreatedAt)
	if mrCreatedAt.IsZero() {
		mrCreatedAt = now // Fallback
	}

	// Build score input
	input := ScoreInput{
		Priority:    issue.Priority,
		MRCreatedAt: mrCreatedAt,
		Now:         now,
	}

	// Add fields from MR metadata if available
	if fields != nil {
		input.RetryCount = fields.RetryCount

		// Parse convoy created at if available
		if fields.ConvoyCreatedAt != "" {
			if convoyTime := parseTime(fields.ConvoyCreatedAt); !convoyTime.IsZero() {
				input.ConvoyCreatedAt = &convoyTime
			}
		}
	}

	return ScoreMRWithDefaults(input)
}

// issueToMR converts a beads issue to a MergeRequest.
func (m *Manager) issueToMR(issue *beads.Issue) *MergeRequest {
	if issue == nil {
		return nil
	}

	// Get configured default branch for this rig
	defaultBranch := m.rig.DefaultBranch()

	fields := beads.ParseMRFields(issue)
	if fields == nil {
		// No MR fields in description, construct from title/ID
		return &MergeRequest{
			ID:           issue.ID,
			IssueID:      issue.ID,
			Status:       MROpen,
			CreatedAt:    parseTime(issue.CreatedAt),
			TargetBranch: defaultBranch,
		}
	}

	// Default target to rig's default branch if not specified
	target := fields.Target
	if target == "" {
		target = defaultBranch
	}

	return &MergeRequest{
		ID:           issue.ID,
		Branch:       fields.Branch,
		Worker:       fields.Worker,
		IssueID:      fields.SourceIssue,
		TargetBranch: target,
		Status:       MROpen,
		CreatedAt:    parseTime(issue.CreatedAt),
	}
}

// parseTime parses a time string, returning zero time on error.
func parseTime(s string) time.Time {
	// Try RFC3339 first (most common)
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		// Try date-only format as fallback
		t, _ = time.Parse("2006-01-02", s)
	}
	return t
}

// MergeResult contains the result of a merge attempt.
type MergeResult struct {
	Success     bool
	MergeCommit string // SHA of merge commit on success
	Error       string
	Conflict    bool
	TestsFailed bool
}

// ProcessMR is deprecated - the Refinery agent now handles all merge processing.
//
// ZFC #5: Move merge/conflict decisions from Go to Refinery agent
//
// The agent runs git commands directly and makes decisions based on output:
//   - Agent attempts merge: git checkout -b temp origin/polecat/<worker>
//   - Agent detects conflict and decides: retry, notify polecat, escalate
//   - Agent runs tests and decides: proceed, rollback, retry
//   - Agent pushes: git push origin main
//
// This function is kept for backwards compatibility but always returns an error
// indicating that the agent should handle merge processing.
//
// Deprecated: Use the Refinery agent (Claude) for merge processing.
func (m *Manager) ProcessMR(mr *MergeRequest) MergeResult {
	return MergeResult{
		Error: "ProcessMR is deprecated - the Refinery agent handles merge processing (ZFC #5)",
	}
}

// completeMR marks an MR as complete.
// For success, pass closeReason (e.g., CloseReasonMerged).
// For failures that should return to open, pass empty closeReason.
// ZFC-compliant: no state file, just updates MR and emits events.
// Deprecated: The Refinery agent handles merge processing (ZFC #5).
func (m *Manager) completeMR(mr *MergeRequest, closeReason CloseReason, errMsg string) {
	mr.Error = errMsg
	actor := fmt.Sprintf("%s/refinery", m.rig.Name)

	if closeReason != "" {
		// Close the MR (in_progress → closed)
		if err := mr.Close(closeReason); err != nil {
			// Log error but continue - this shouldn't happen
			_, _ = fmt.Fprintf(m.output, "Warning: failed to close MR: %v\n", err)
		}
		if closeReason == CloseReasonSuperseded {
			// Emit merge_skipped event
			_ = events.LogFeed(events.TypeMergeSkipped, actor, events.MergePayload(mr.ID, mr.Worker, mr.Branch, "superseded"))
		}
	} else {
		// Reopen the MR for rework (in_progress → open)
		if err := mr.Reopen(); err != nil {
			// Log error but continue
			_, _ = fmt.Fprintf(m.output, "Warning: failed to reopen MR: %v\n", err)
		}
	}
}

// runTests executes the test command.
// Deprecated: The Refinery agent runs tests directly via shell commands (ZFC #5).
func (m *Manager) runTests(testCmd string) error {
	parts := strings.Fields(testCmd)
	if len(parts) == 0 {
		return nil
	}

	return util.ExecRun(m.workDir, parts[0], parts[1:]...)
}

// getMergeConfig loads the merge configuration from disk.
// Returns default config if not configured.
// Deprecated: Configuration is read by the agent from settings (ZFC #5).
func (m *Manager) getMergeConfig() MergeConfig {
	mergeConfig := DefaultMergeConfig()

	// Check settings/config.json for merge_queue settings
	settingsPath := filepath.Join(m.rig.Path, "settings", "config.json")
	settings, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		return mergeConfig
	}

	// Apply merge_queue config if present
	if settings.MergeQueue != nil {
		mq := settings.MergeQueue
		mergeConfig.TestCommand = mq.TestCommand
		mergeConfig.RunTests = mq.RunTests
		mergeConfig.DeleteMergedBranches = mq.DeleteMergedBranches
		// Note: PushRetryCount and PushRetryDelayMs use defaults if not explicitly set
	}

	return mergeConfig
}

// pushWithRetry pushes to the target branch with exponential backoff retry.
// Deprecated: The Refinery agent decides retry strategy (ZFC #5).
func (m *Manager) pushWithRetry(targetBranch string, config MergeConfig) error {
	var lastErr error
	delay := time.Duration(config.PushRetryDelayMs) * time.Millisecond

	for attempt := 0; attempt <= config.PushRetryCount; attempt++ {
		if attempt > 0 {
			_, _ = fmt.Fprintf(m.output, "Push retry %d/%d after %v\n", attempt, config.PushRetryCount, delay)
			time.Sleep(delay)
			delay *= 2 // Exponential backoff
		}

		err := util.ExecRun(m.workDir, "git", "push", "origin", targetBranch)
		if err == nil {
			return nil // Success
		}
		lastErr = err
	}

	return fmt.Errorf("push failed after %d retries: %v", config.PushRetryCount, lastErr)
}

// formatAge formats a duration since the given time.
func formatAge(t time.Time) string {
	d := time.Since(t)

	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

// notifyWorkerConflict sends a conflict notification to a polecat.
func (m *Manager) notifyWorkerConflict(mr *MergeRequest) {
	router := mail.NewRouter(m.workDir)
	msg := &mail.Message{
		From:    fmt.Sprintf("%s/refinery", m.rig.Name),
		To:      fmt.Sprintf("%s/%s", m.rig.Name, mr.Worker),
		Subject: "Merge conflict - rebase required",
		Body: fmt.Sprintf(`Your branch %s has conflicts with %s.

Please rebase your changes:
  git fetch origin
  git rebase origin/%s
  git push -f

Then the Refinery will retry the merge.`,
			mr.Branch, mr.TargetBranch, mr.TargetBranch),
		Priority: mail.PriorityHigh,
	}
	_ = router.Send(msg) // best-effort notification
}

// notifyWorkerMerged sends a success notification to a polecat.
func (m *Manager) notifyWorkerMerged(mr *MergeRequest) {
	router := mail.NewRouter(m.workDir)
	msg := &mail.Message{
		From:    fmt.Sprintf("%s/refinery", m.rig.Name),
		To:      fmt.Sprintf("%s/%s", m.rig.Name, mr.Worker),
		Subject: "Work merged successfully",
		Body: fmt.Sprintf(`Your branch %s has been merged to %s.

Issue: %s
Thank you for your contribution!`,
			mr.Branch, mr.TargetBranch, mr.IssueID),
	}
	_ = router.Send(msg) // best-effort notification
}

// Common errors for MR operations
var (
	ErrMRNotFound  = errors.New("merge request not found")
	ErrMRNotFailed = errors.New("merge request has not failed")
)

// GetMR returns a merge request by ID.
// ZFC-compliant: delegates to FindMR which uses beads as source of truth.
// Deprecated: Use FindMR directly for more flexible matching.
func (m *Manager) GetMR(id string) (*MergeRequest, error) {
	return m.FindMR(id)
}

// FindMR finds a merge request by ID or branch name in the queue.
func (m *Manager) FindMR(idOrBranch string) (*MergeRequest, error) {
	queue, err := m.Queue()
	if err != nil {
		return nil, err
	}

	for _, item := range queue {
		// Match by ID
		if item.MR.ID == idOrBranch {
			return item.MR, nil
		}
		// Match by branch name (with or without polecat/ prefix)
		if item.MR.Branch == idOrBranch {
			return item.MR, nil
		}
		if constants.BranchPolecatPrefix+idOrBranch == item.MR.Branch {
			return item.MR, nil
		}
		// Match by worker name (partial match for convenience)
		if strings.Contains(item.MR.ID, idOrBranch) {
			return item.MR, nil
		}
	}

	return nil, ErrMRNotFound
}

// Retry is deprecated - the Refinery agent handles retry logic autonomously.
// ZFC-compliant: no state file, agent uses beads issue status.
// The agent will automatically retry failed MRs in its patrol cycle.
func (m *Manager) Retry(_ string, _ bool) error {
	_, _ = fmt.Fprintln(m.output, "Note: Retry is deprecated. The Refinery agent handles retries autonomously via beads.")
	return nil
}

// RegisterMR is deprecated - MRs are registered via beads merge-request issues.
// ZFC-compliant: beads is the source of truth, not state file.
// Use 'gt mr create' or create a merge-request type bead directly.
func (m *Manager) RegisterMR(_ *MergeRequest) error {
	return fmt.Errorf("RegisterMR is deprecated: use beads to create merge-request issues")
}

// RejectMR manually rejects a merge request.
// It closes the MR with rejected status and optionally notifies the worker.
// Returns the rejected MR for display purposes.
func (m *Manager) RejectMR(idOrBranch string, reason string, notify bool) (*MergeRequest, error) {
	mr, err := m.FindMR(idOrBranch)
	if err != nil {
		return nil, err
	}

	// Verify MR is open or in_progress (can't reject already closed)
	if mr.IsClosed() {
		return nil, fmt.Errorf("%w: MR is already closed with reason: %s", ErrClosedImmutable, mr.CloseReason)
	}

	// Close the bead in storage with the rejection reason
	b := beads.New(m.rig.BeadsPath())
	if err := b.CloseWithReason("rejected: "+reason, mr.ID); err != nil {
		return nil, fmt.Errorf("failed to close MR bead: %w", err)
	}

	// Update in-memory state for return value
	if err := mr.Close(CloseReasonRejected); err != nil {
		// Non-fatal: bead is already closed, just log
		_, _ = fmt.Fprintf(m.output, "Warning: failed to update MR state: %v\n", err)
	}
	mr.Error = reason

	// Optionally notify worker
	if notify {
		m.notifyWorkerRejected(mr, reason)
	}

	return mr, nil
}

// notifyWorkerRejected sends a rejection notification to a polecat.
func (m *Manager) notifyWorkerRejected(mr *MergeRequest, reason string) {
	router := mail.NewRouter(m.workDir)
	msg := &mail.Message{
		From:    fmt.Sprintf("%s/refinery", m.rig.Name),
		To:      fmt.Sprintf("%s/%s", m.rig.Name, mr.Worker),
		Subject: "Merge request rejected",
		Body: fmt.Sprintf(`Your merge request has been rejected.

Branch: %s
Issue: %s
Reason: %s

Please review the feedback and address the issues before resubmitting.`,
			mr.Branch, mr.IssueID, reason),
		Priority: mail.PriorityNormal,
	}
	_ = router.Send(msg) // best-effort notification
}

// findTownRoot walks up directories to find the town root.
func findTownRoot(startPath string) string {
	path := startPath
	for {
		// Check for mayor/ subdirectory (indicates town root)
		if _, err := os.Stat(filepath.Join(path, "mayor")); err == nil {
			return path
		}
		// Check for config.json with type: workspace
		configPath := filepath.Join(path, "config.json")
		if data, err := os.ReadFile(configPath); err == nil {
			if strings.Contains(string(data), `"type": "workspace"`) {
				return path
			}
		}

		parent := filepath.Dir(path)
		if parent == path {
			break // Reached root
		}
		path = parent
	}
	return ""
}
