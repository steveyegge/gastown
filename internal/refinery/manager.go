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

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/rig"
)

// ErrNoQueue indicates there are no items in the queue.
var ErrNoQueue = errors.New("no items in queue")

// Manager handles refinery status and queue operations.
// Start/Stop operations are handled via factory.Start()/factory.Agents().Stop().
type Manager struct {
	stateManager *agent.StateManager[Refinery]
	agents       agent.AgentObserver // Only needs Exists() for status checks
	rigName      string
	rigPath      string
	address      agent.AgentID
	rig          *rig.Rig
	output       io.Writer // Output destination for user-facing messages
}

// NewManager creates a new refinery manager for a rig.
// The manager handles status queries and queue operations.
// Lifecycle operations (Start/Stop) should use factory.Start()/factory.Agents().Stop().
//
// The agents parameter only needs to implement AgentObserver (Exists, GetInfo, List).
// In production, pass factory.Agents(). In tests, use agent.NewObserverDouble().
func NewManager(agents agent.AgentObserver, r *rig.Rig) *Manager {
	stateFactory := func() *Refinery {
		return &Refinery{RigName: r.Name, State: agent.StateStopped}
	}
	return &Manager{
		stateManager: agent.NewStateManager[Refinery](r.Path, "refinery.json", stateFactory),
		agents:       agents,
		rigName:      r.Name,
		rigPath:      r.Path,
		address:      agent.RefineryAddress(r.Name),
		rig:          r,
		output:       os.Stdout,
	}
}

// SetOutput sets the output writer for user-facing messages.
// This is useful for testing or redirecting output.
func (m *Manager) SetOutput(w io.Writer) {
	m.output = w
}

// refineryDir returns the working directory for the refinery.
// Prefers refinery/rig/, falls back to mayor/rig (legacy).
func (m *Manager) refineryDir() string {
	refineryRigDir := filepath.Join(m.rig.Path, "refinery", "rig")
	if _, err := os.Stat(refineryRigDir); err == nil {
		return refineryRigDir
	}
	// Fall back to mayor/rig (legacy architecture)
	return filepath.Join(m.rig.Path, "mayor", "rig")
}

// Status returns the current refinery status.
// Reconciles persisted state with actual agent existence.
func (m *Manager) Status() (*Refinery, error) {
	ref, err := m.stateManager.Load()
	if err != nil {
		return nil, err
	}

	// Reconcile state with reality (don't persist, just report accurately)
	if ref.IsRunning() && !m.agents.Exists(m.address) {
		ref.SetStopped() // Agent crashed
	}

	return ref, nil
}

// SessionName returns the tmux session name for this refinery.
func (m *Manager) SessionName() string {
	return fmt.Sprintf("gt-%s-refinery", m.rigName)
}

// IsRunning checks if the refinery session is currently active.
func (m *Manager) IsRunning() bool {
	return m.agents.Exists(m.address)
}

// Address returns the agent's AgentID.
func (m *Manager) Address() agent.AgentID {
	return m.address
}

// RigPath returns the path to the rig directory.
func (m *Manager) RigPath() string {
	return m.rigPath
}

// LoadState loads the refinery state from disk.
func (m *Manager) LoadState() (*Refinery, error) {
	return m.stateManager.Load()
}

// SaveState persists the refinery state to disk.
func (m *Manager) SaveState(ref *Refinery) error {
	return m.stateManager.Save(ref)
}

// Queue returns the current merge queue.
// Uses beads merge-request issues as the source of truth (not git branches).
func (m *Manager) Queue() ([]QueueItem, error) {
	// Query beads for open merge-request type issues
	// BeadsPath() returns the git-synced beads location
	b := beads.New(m.rig.BeadsPath())
	issues, err := b.List(beads.ListOptions{
		Type:     "merge-request",
		Status:   "open",
		Priority: -1, // No priority filter
	})
	if err != nil {
		return nil, fmt.Errorf("querying merge queue from beads: %w", err)
	}

	// Load any current processing state
	ref, err := m.LoadState()
	if err != nil {
		return nil, err
	}

	// Build queue items
	var items []QueueItem
	pos := 1

	// Add current processing item
	if ref.CurrentMR != nil {
		items = append(items, QueueItem{
			Position: 0, // 0 = currently processing
			MR:       ref.CurrentMR,
			Age:      formatAge(ref.CurrentMR.CreatedAt),
		})
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
	for _, s := range scored {
		mr := m.issueToMR(s.issue)
		if mr != nil {
			// Skip if this is the currently processing MR
			if ref.CurrentMR != nil && ref.CurrentMR.ID == mr.ID {
				continue
			}
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

// Common errors for MR operations
var (
	ErrMRNotFound  = errors.New("merge request not found")
	ErrMRNotFailed = errors.New("merge request has not failed")
)

// GetMR returns a merge request by ID from the state.
func (m *Manager) GetMR(id string) (*MergeRequest, error) {
	ref, err := m.LoadState()
	if err != nil {
		return nil, err
	}

	// Check if it's the current MR
	if ref.CurrentMR != nil && ref.CurrentMR.ID == id {
		return ref.CurrentMR, nil
	}

	// Check pending MRs
	if ref.PendingMRs != nil {
		if mr, ok := ref.PendingMRs[id]; ok {
			return mr, nil
		}
	}

	return nil, ErrMRNotFound
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

// Retry resets a failed merge request so it can be processed again.
// The processNow parameter is deprecated - the Refinery agent handles processing.
// Clearing the error is sufficient; the agent will pick up the MR in its next patrol cycle.
func (m *Manager) Retry(id string, processNow bool) error {
	ref, err := m.LoadState()
	if err != nil {
		return err
	}

	// Find the MR
	var mr *MergeRequest
	if ref.PendingMRs != nil {
		mr = ref.PendingMRs[id]
	}
	if mr == nil {
		return ErrMRNotFound
	}

	// Verify it's in a failed state (open with an error)
	if mr.Status != MROpen || mr.Error == "" {
		return ErrMRNotFailed
	}

	// Clear the error to mark as ready for retry
	mr.Error = ""

	// Save the state
	if err := m.SaveState(ref); err != nil {
		return err
	}

	// Note: processNow is deprecated (ZFC #5).
	// The Refinery agent handles merge processing.
	// It will pick up this MR in its next patrol cycle.
	if processNow {
		_, _ = fmt.Fprintln(m.output, "Note: --now is deprecated. The Refinery agent will process this MR in its next patrol cycle.")
	}

	return nil
}

// RegisterMR adds a merge request to the pending queue.
func (m *Manager) RegisterMR(mr *MergeRequest) error {
	ref, err := m.LoadState()
	if err != nil {
		return err
	}

	if ref.PendingMRs == nil {
		ref.PendingMRs = make(map[string]*MergeRequest)
	}

	ref.PendingMRs[mr.ID] = mr
	return m.SaveState(ref)
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
	router := mail.NewRouter(m.RigPath())
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
