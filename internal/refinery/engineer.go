// Package refinery provides the merge queue processing agent.
package refinery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/protocol"
	"github.com/steveyegge/gastown/internal/rig"
)

// MergeQueueConfig holds configuration for the merge queue processor.
type MergeQueueConfig struct {
	// Enabled controls whether the merge queue is active.
	Enabled bool `json:"enabled"`

	// TargetBranch is the default branch to merge to (e.g., "main").
	TargetBranch string `json:"target_branch"`

	// IntegrationBranches enables per-epic integration branches.
	IntegrationBranches bool `json:"integration_branches"`

	// OnConflict is the strategy for handling conflicts: "assign_back" or "auto_rebase".
	OnConflict string `json:"on_conflict"`

	// RunTests controls whether to run tests before merging.
	RunTests bool `json:"run_tests"`

	// TestCommand is the command to run for testing.
	TestCommand string `json:"test_command"`

	// DeleteMergedBranches controls whether to delete branches after merge.
	DeleteMergedBranches bool `json:"delete_merged_branches"`

	// RetryFlakyTests is the number of times to retry flaky tests.
	RetryFlakyTests int `json:"retry_flaky_tests"`

	// PollInterval is how often to check for new MRs.
	PollInterval time.Duration `json:"poll_interval"`

	// MaxConcurrent is the maximum number of MRs to process concurrently.
	MaxConcurrent int `json:"max_concurrent"`
}

// DefaultMergeQueueConfig returns sensible defaults for merge queue configuration.
func DefaultMergeQueueConfig() *MergeQueueConfig {
	return &MergeQueueConfig{
		Enabled:              true,
		TargetBranch:         "main",
		IntegrationBranches:  true,
		OnConflict:           "assign_back",
		RunTests:             true,
		TestCommand:          "",
		DeleteMergedBranches: true,
		RetryFlakyTests:      1,
		PollInterval:         30 * time.Second,
		MaxConcurrent:        1,
	}
}

// MRInfo holds merge request information for display and processing.
// This replaces mrqueue.MR after the mrqueue package removal.
type MRInfo struct {
	ID              string     // Bead ID (e.g., "gt-abc123")
	Branch          string     // Source branch (e.g., "polecat/nux")
	Target          string     // Target branch (e.g., "main")
	SourceIssue     string     // The work item being merged
	Worker          string     // Who did the work
	Rig             string     // Which rig
	Title           string     // MR title
	Priority        int        // Priority (lower = higher priority)
	AgentBead       string     // Agent bead ID that created this MR
	RetryCount      int        // Conflict retry count
	ConvoyID        string     // Parent convoy ID if part of a convoy
	ConvoyCreatedAt *time.Time // Convoy creation time
	CreatedAt       time.Time  // MR creation time
	BlockedBy       string     // Task ID blocking this MR
}

// Engineer is the merge queue processor that polls for ready merge-requests
// and processes them according to the merge queue design.
type Engineer struct {
	rig     *rig.Rig
	beads   *beads.Beads
	git     *git.Git
	config  *MergeQueueConfig
	workDir string
	output  io.Writer    // Output destination for user-facing messages
	router  *mail.Router // Mail router for sending protocol messages

	// stopCh is used for graceful shutdown
	stopCh chan struct{}
}

// NewEngineer creates a new Engineer for the given rig.
func NewEngineer(r *rig.Rig) *Engineer {
	cfg := DefaultMergeQueueConfig()
	// Override target branch with rig's configured default branch
	cfg.TargetBranch = r.DefaultBranch()

	// Determine the git working directory for refinery operations.
	// Prefer refinery/rig worktree, fall back to mayor/rig (legacy architecture).
	// Using rig.Path directly would find town's .git with rig-named remotes instead of "origin".
	gitDir := filepath.Join(r.Path, "refinery", "rig")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		gitDir = filepath.Join(r.Path, "mayor", "rig")
	}

	return &Engineer{
		rig:     r,
		beads:   beads.New(r.Path),
		git:     git.NewGit(gitDir),
		config:  cfg,
		workDir: gitDir,
		output:  os.Stdout,
		router:  mail.NewRouter(r.Path),
		stopCh:  make(chan struct{}),
	}
}

// SetOutput sets the output writer for user-facing messages.
// This is useful for testing or redirecting output.
func (e *Engineer) SetOutput(w io.Writer) {
	e.output = w
}

// LoadConfig loads merge queue configuration from the rig's config.json.
func (e *Engineer) LoadConfig() error {
	configPath := filepath.Join(e.rig.Path, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Use defaults if no config file
			return nil
		}
		return fmt.Errorf("reading config: %w", err)
	}

	// Parse config file to extract merge_queue section
	var rawConfig struct {
		MergeQueue json.RawMessage `json:"merge_queue"`
	}
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	if rawConfig.MergeQueue == nil {
		// No merge_queue section, use defaults
		return nil
	}

	// Parse merge_queue section into our config struct
	// We need special handling for poll_interval (string -> Duration)
	var mqRaw struct {
		Enabled              *bool   `json:"enabled"`
		TargetBranch         *string `json:"target_branch"`
		IntegrationBranches  *bool   `json:"integration_branches"`
		OnConflict           *string `json:"on_conflict"`
		RunTests             *bool   `json:"run_tests"`
		TestCommand          *string `json:"test_command"`
		DeleteMergedBranches *bool   `json:"delete_merged_branches"`
		RetryFlakyTests      *int    `json:"retry_flaky_tests"`
		PollInterval         *string `json:"poll_interval"`
		MaxConcurrent        *int    `json:"max_concurrent"`
	}

	if err := json.Unmarshal(rawConfig.MergeQueue, &mqRaw); err != nil {
		return fmt.Errorf("parsing merge_queue config: %w", err)
	}

	// Apply non-nil values to config (preserving defaults for missing fields)
	if mqRaw.Enabled != nil {
		e.config.Enabled = *mqRaw.Enabled
	}
	if mqRaw.TargetBranch != nil {
		e.config.TargetBranch = *mqRaw.TargetBranch
	}
	if mqRaw.IntegrationBranches != nil {
		e.config.IntegrationBranches = *mqRaw.IntegrationBranches
	}
	if mqRaw.OnConflict != nil {
		e.config.OnConflict = *mqRaw.OnConflict
	}
	if mqRaw.RunTests != nil {
		e.config.RunTests = *mqRaw.RunTests
	}
	if mqRaw.TestCommand != nil {
		e.config.TestCommand = *mqRaw.TestCommand
	}
	if mqRaw.DeleteMergedBranches != nil {
		e.config.DeleteMergedBranches = *mqRaw.DeleteMergedBranches
	}
	if mqRaw.RetryFlakyTests != nil {
		e.config.RetryFlakyTests = *mqRaw.RetryFlakyTests
	}
	if mqRaw.MaxConcurrent != nil {
		e.config.MaxConcurrent = *mqRaw.MaxConcurrent
	}
	if mqRaw.PollInterval != nil {
		dur, err := time.ParseDuration(*mqRaw.PollInterval)
		if err != nil {
			return fmt.Errorf("invalid poll_interval %q: %w", *mqRaw.PollInterval, err)
		}
		e.config.PollInterval = dur
	}

	return nil
}

// Config returns the current merge queue configuration.
func (e *Engineer) Config() *MergeQueueConfig {
	return e.config
}

// ProcessResult contains the result of processing a merge request.
type ProcessResult struct {
	Success     bool
	MergeCommit string
	Error       string
	Conflict    bool
	TestsFailed bool
}

// ProcessMR processes a single merge request from a beads issue.
func (e *Engineer) ProcessMR(ctx context.Context, mr *beads.Issue) ProcessResult {
	// Parse MR fields from description
	mrFields := beads.ParseMRFields(mr)
	if mrFields == nil {
		return ProcessResult{
			Success: false,
			Error:   "no MR fields found in description",
		}
	}

	// Log what we're processing
	_, _ = fmt.Fprintln(e.output, "[Engineer] Processing MR:")
	_, _ = fmt.Fprintf(e.output, "  Branch: %s\n", mrFields.Branch)
	_, _ = fmt.Fprintf(e.output, "  Target: %s\n", mrFields.Target)
	_, _ = fmt.Fprintf(e.output, "  Worker: %s\n", mrFields.Worker)

	return e.doMerge(ctx, mrFields.Branch, mrFields.Target, mrFields.SourceIssue)
}

// doMerge performs the actual git merge operation.
// This is the core merge logic shared by ProcessMR and ProcessMRFromQueue.
func (e *Engineer) doMerge(ctx context.Context, branch, target, sourceIssue string) ProcessResult {
	// Step 1: Verify source branch exists locally (shared .repo.git with polecats)
	_, _ = fmt.Fprintf(e.output, "[Engineer] Checking local branch %s...\n", branch)
	exists, err := e.git.BranchExists(branch)
	if err != nil {
		return ProcessResult{
			Success: false,
			Error:   fmt.Sprintf("failed to check branch %s: %v", branch, err),
		}
	}
	if !exists {
		return ProcessResult{
			Success: false,
			Error:   fmt.Sprintf("branch %s not found locally", branch),
		}
	}

	// Step 2: Checkout the target branch
	_, _ = fmt.Fprintf(e.output, "[Engineer] Checking out target branch %s...\n", target)
	if err := e.git.Checkout(target); err != nil {
		return ProcessResult{
			Success: false,
			Error:   fmt.Sprintf("failed to checkout target %s: %v", target, err),
		}
	}

	// Make sure target is up to date with origin
	if err := e.git.Pull("origin", target); err != nil {
		// Pull might fail if nothing to pull, that's ok
		_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: pull from origin/%s: %v (continuing)\n", target, err)
	}

	// Step 3: Check for merge conflicts (using local branch)
	_, _ = fmt.Fprintf(e.output, "[Engineer] Checking for conflicts...\n")
	conflicts, err := e.git.CheckConflicts(branch, target)
	if err != nil {
		return ProcessResult{
			Success:  false,
			Conflict: true,
			Error:    fmt.Sprintf("conflict check failed: %v", err),
		}
	}
	if len(conflicts) > 0 {
		return ProcessResult{
			Success:  false,
			Conflict: true,
			Error:    fmt.Sprintf("merge conflicts in: %v", conflicts),
		}
	}

	// Step 4: Run tests if configured
	if e.config.RunTests && e.config.TestCommand != "" {
		_, _ = fmt.Fprintf(e.output, "[Engineer] Running tests: %s\n", e.config.TestCommand)
		result := e.runTests(ctx)
		if !result.Success {
			return ProcessResult{
				Success:     false,
				TestsFailed: true,
				Error:       result.Error,
			}
		}
		_, _ = fmt.Fprintln(e.output, "[Engineer] Tests passed")
	}

	// Step 5: Perform the actual merge
	mergeMsg := fmt.Sprintf("Merge %s into %s", branch, target)
	if sourceIssue != "" {
		mergeMsg = fmt.Sprintf("Merge %s into %s (%s)", branch, target, sourceIssue)
	}
	_, _ = fmt.Fprintf(e.output, "[Engineer] Merging with message: %s\n", mergeMsg)
	if err := e.git.MergeNoFF(branch, mergeMsg); err != nil {
		// ZFC: Use git's porcelain output to detect conflicts instead of parsing stderr.
		// GetConflictingFiles() uses `git diff --diff-filter=U` which is proper.
		conflicts, conflictErr := e.git.GetConflictingFiles()
		if conflictErr == nil && len(conflicts) > 0 {
			_ = e.git.AbortMerge()
			return ProcessResult{
				Success:  false,
				Conflict: true,
				Error:    "merge conflict during actual merge",
			}
		}
		return ProcessResult{
			Success: false,
			Error:   fmt.Sprintf("merge failed: %v", err),
		}
	}

	// Step 6: Get the merge commit SHA
	mergeCommit, err := e.git.Rev("HEAD")
	if err != nil {
		return ProcessResult{
			Success: false,
			Error:   fmt.Sprintf("failed to get merge commit SHA: %v", err),
		}
	}

	// Step 7: Push to origin
	_, _ = fmt.Fprintf(e.output, "[Engineer] Pushing to origin/%s...\n", target)
	if err := e.git.Push("origin", target, false); err != nil {
		return ProcessResult{
			Success: false,
			Error:   fmt.Sprintf("failed to push to origin: %v", err),
		}
	}

	_, _ = fmt.Fprintf(e.output, "[Engineer] Successfully merged: %s\n", mergeCommit[:8])
	return ProcessResult{
		Success:     true,
		MergeCommit: mergeCommit,
	}
}

// runTests runs the configured test command and returns the result.
func (e *Engineer) runTests(ctx context.Context) ProcessResult {
	if e.config.TestCommand == "" {
		return ProcessResult{Success: true}
	}

	// Run the test command with retries for flaky tests
	maxRetries := e.config.RetryFlakyTests
	if maxRetries < 1 {
		maxRetries = 1
	}

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Retrying tests (attempt %d/%d)...\n", attempt, maxRetries)
		}

		// Note: TestCommand comes from rig's config.json (trusted infrastructure config),
		// not from PR branches. Shell execution is intentional for flexibility (pipes, etc).
		cmd := exec.CommandContext(ctx, "sh", "-c", e.config.TestCommand) //nolint:gosec // G204: TestCommand is from trusted rig config
		cmd.Dir = e.workDir
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		if err == nil {
			return ProcessResult{Success: true}
		}
		lastErr = err

		// Check if context was canceled
		if ctx.Err() != nil {
			return ProcessResult{
				Success: false,
				Error:   "test run canceled",
			}
		}
	}

	return ProcessResult{
		Success:     false,
		TestsFailed: true,
		Error:       fmt.Sprintf("tests failed after %d attempts: %v", maxRetries, lastErr),
	}
}

// handleSuccess handles a successful merge completion.
// Steps:
// 1. Update MR with merge_commit SHA
// 2. Close MR with reason 'merged'
// 3. Close source issue with reference to MR
// 4. Delete source branch if configured
// 5. Log success
func (e *Engineer) handleSuccess(mr *beads.Issue, result ProcessResult) {
	// Parse MR fields from description
	mrFields := beads.ParseMRFields(mr)
	if mrFields == nil {
		mrFields = &beads.MRFields{}
	}

	// 1. Update MR with merge_commit SHA
	mrFields.MergeCommit = result.MergeCommit
	mrFields.CloseReason = "merged"
	newDesc := beads.SetMRFields(mr, mrFields)
	if err := e.beads.Update(mr.ID, beads.UpdateOptions{Description: &newDesc}); err != nil {
		_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to update MR %s with merge commit: %v\n", mr.ID, err)
	}

	// 2. Close MR with reason 'merged'
	if err := e.beads.CloseWithReason("merged", mr.ID); err != nil {
		_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to close MR %s: %v\n", mr.ID, err)
	}

	// 3. Close source issue with reference to MR
	if mrFields.SourceIssue != "" {
		closeReason := fmt.Sprintf("Merged in %s", mr.ID)
		if err := e.beads.CloseWithReason(closeReason, mrFields.SourceIssue); err != nil {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to close source issue %s: %v\n", mrFields.SourceIssue, err)
		} else {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Closed source issue: %s\n", mrFields.SourceIssue)
		}
	}

	// 3.5. Clear agent bead's active_mr reference (traceability cleanup)
	if mrFields.AgentBead != "" {
		if err := e.beads.UpdateAgentActiveMR(mrFields.AgentBead, ""); err != nil {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to clear agent bead %s active_mr: %v\n", mrFields.AgentBead, err)
		}
	}

	// 4. Delete source branch if configured (local and remote)
	// Since the self-cleaning model (Jan 10), polecats push to origin before gt done,
	// so we need to clean up both local and remote branches after merge.
	if e.config.DeleteMergedBranches && mrFields.Branch != "" {
		// Delete local branch
		if err := e.git.DeleteBranch(mrFields.Branch, true); err != nil {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to delete local branch %s: %v\n", mrFields.Branch, err)
		} else {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Deleted local branch: %s\n", mrFields.Branch)
		}
		// Also delete the remote branch (non-fatal if it doesn't exist)
		if err := e.git.DeleteRemoteBranch("origin", mrFields.Branch); err != nil {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to delete remote branch %s: %v\n", mrFields.Branch, err)
		} else {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Deleted remote branch: origin/%s\n", mrFields.Branch)
		}
	}

	// 5. Log success
	_, _ = fmt.Fprintf(e.output, "[Engineer] ✓ Merged: %s (commit: %s)\n", mr.ID, result.MergeCommit)
}

// handleFailure handles a failed merge request.
// Reopens the MR for rework and logs the failure.
func (e *Engineer) handleFailure(mr *beads.Issue, result ProcessResult) {
	// Reopen the MR (back to open status for rework)
	open := "open"
	if err := e.beads.Update(mr.ID, beads.UpdateOptions{Status: &open}); err != nil {
		_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to reopen MR %s: %v\n", mr.ID, err)
	}

	// Log the failure
	_, _ = fmt.Fprintf(e.output, "[Engineer] ✗ Failed: %s - %s\n", mr.ID, result.Error)
}

// ProcessMRInfo processes a merge request from MRInfo.
func (e *Engineer) ProcessMRInfo(ctx context.Context, mr *MRInfo) ProcessResult {
	// MR fields are directly on the struct
	_, _ = fmt.Fprintln(e.output, "[Engineer] Processing MR:")
	_, _ = fmt.Fprintf(e.output, "  Branch: %s\n", mr.Branch)
	_, _ = fmt.Fprintf(e.output, "  Target: %s\n", mr.Target)
	_, _ = fmt.Fprintf(e.output, "  Worker: %s\n", mr.Worker)
	_, _ = fmt.Fprintf(e.output, "  Source: %s\n", mr.SourceIssue)

	// Use the shared merge logic
	return e.doMerge(ctx, mr.Branch, mr.Target, mr.SourceIssue)
}

// HandleMRInfoSuccess handles a successful merge from MRInfo.
func (e *Engineer) HandleMRInfoSuccess(mr *MRInfo, result ProcessResult) {
	// Release merge slot if this was a conflict resolution
	// The slot is held while conflict resolution is in progress
	holder := e.rig.Name + "/refinery"
	if err := e.beads.MergeSlotRelease(holder); err != nil {
		// Not an error if slot wasn't held - it's optional
		// Only log if it seems like an actual issue
		errStr := err.Error()
		if !strings.Contains(errStr, "not held") && !strings.Contains(errStr, "not found") {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to release merge slot: %v\n", err)
		}
	} else {
		_, _ = fmt.Fprintf(e.output, "[Engineer] Released merge slot\n")
	}

	// Update and close the MR bead
	if mr.ID != "" {
		// Fetch the MR bead to update its fields
		mrBead, err := e.beads.Show(mr.ID)
		if err != nil {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to fetch MR bead %s: %v\n", mr.ID, err)
		} else {
			// Update MR with merge_commit SHA and close_reason
			mrFields := beads.ParseMRFields(mrBead)
			if mrFields == nil {
				mrFields = &beads.MRFields{}
			}
			mrFields.MergeCommit = result.MergeCommit
			mrFields.CloseReason = "merged"
			newDesc := beads.SetMRFields(mrBead, mrFields)
			if err := e.beads.Update(mr.ID, beads.UpdateOptions{Description: &newDesc}); err != nil {
				_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to update MR %s with merge commit: %v\n", mr.ID, err)
			}
		}

		// Close MR bead with reason 'merged'
		if err := e.beads.CloseWithReason("merged", mr.ID); err != nil {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to close MR %s: %v\n", mr.ID, err)
		} else {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Closed MR bead: %s\n", mr.ID)
		}
	}

	// 1. Close source issue with reference to MR
	if mr.SourceIssue != "" {
		closeReason := fmt.Sprintf("Merged in %s", mr.ID)
		if err := e.beads.CloseWithReason(closeReason, mr.SourceIssue); err != nil {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to close source issue %s: %v\n", mr.SourceIssue, err)
		} else {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Closed source issue: %s\n", mr.SourceIssue)
		}
	}

	// 1.5. Clear agent bead's active_mr reference (traceability cleanup)
	if mr.AgentBead != "" {
		if err := e.beads.UpdateAgentActiveMR(mr.AgentBead, ""); err != nil {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to clear agent bead %s active_mr: %v\n", mr.AgentBead, err)
		}
	}

	// 2. Delete source branch if configured (both local and remote)
	// After the polecat self-nuke fix, branches are pushed to origin before the
	// worktree is deleted, so we need to clean up both local and remote copies.
	if e.config.DeleteMergedBranches && mr.Branch != "" {
		// Delete local branch
		if err := e.git.DeleteBranch(mr.Branch, true); err != nil {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to delete local branch %s: %v\n", mr.Branch, err)
		} else {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Deleted local branch: %s\n", mr.Branch)
		}
		// Delete remote branch from origin
		if err := e.git.DeleteRemoteBranch("origin", mr.Branch); err != nil {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to delete remote branch %s from origin: %v\n", mr.Branch, err)
		} else {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Deleted remote branch: %s\n", mr.Branch)
		}
	}

	// 3. Log success
	_, _ = fmt.Fprintf(e.output, "[Engineer] ✓ Merged: %s (commit: %s)\n", mr.ID, result.MergeCommit)
}

// HandleMRInfoFailure handles a failed merge from MRInfo.
// For conflicts, creates a resolution task and blocks the MR until resolved.
// This enables non-blocking delegation: the queue continues to the next MR.
func (e *Engineer) HandleMRInfoFailure(mr *MRInfo, result ProcessResult) {
	// Notify Witness of the failure so polecat can be alerted
	// Determine failure type from result
	failureType := "build"
	if result.Conflict {
		failureType = "conflict"
	} else if result.TestsFailed {
		failureType = "tests"
	}
	msg := protocol.NewMergeFailedMessage(e.rig.Name, mr.Worker, mr.Branch, mr.SourceIssue, mr.Target, failureType, result.Error)
	if err := e.router.Send(msg); err != nil {
		fmt.Fprintf(e.output, "[Engineer] Warning: failed to send MERGE_FAILED to witness: %v\n", err)
	} else {
		fmt.Fprintf(e.output, "[Engineer] Notified witness of merge failure for %s\n", mr.Worker)
	}

	// If this was a conflict, create a conflict-resolution task for dispatch
	// and block the MR until the task is resolved (non-blocking delegation)
	if result.Conflict {
		taskID, err := e.createConflictResolutionTaskForMR(mr, result)
		if err != nil {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to create conflict resolution task: %v\n", err)
		} else if taskID != "" {
			// Block the MR on the conflict resolution task using beads dependency
			// When the task closes, the MR unblocks and re-enters the ready queue
			if err := e.beads.AddDependency(mr.ID, taskID); err != nil {
				_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to block MR on task: %v\n", err)
			} else {
				_, _ = fmt.Fprintf(e.output, "[Engineer] MR %s blocked on conflict task %s (non-blocking delegation)\n", mr.ID, taskID)
			}
		}
	}

	// Log the failure - MR stays in queue but may be blocked
	_, _ = fmt.Fprintf(e.output, "[Engineer] ✗ Failed: %s - %s\n", mr.ID, result.Error)
	if mr.BlockedBy != "" {
		_, _ = fmt.Fprintln(e.output, "[Engineer] MR blocked pending conflict resolution - queue continues to next MR")
	} else {
		_, _ = fmt.Fprintln(e.output, "[Engineer] MR remains in queue for retry")
	}
}

// createConflictResolutionTaskForMR creates a dispatchable task for resolving merge conflicts.
// This task will be picked up by bd ready and can be slung to a fresh polecat (spawned on demand).
// Returns the created task's ID for blocking the MR until resolution.
//
// Task format:
//
//	Title: Resolve merge conflicts: <original-issue-title>
//	Type: task
//	Priority: inherit from original + boost (P2 -> P1)
//	Parent: original MR bead
//	Description: metadata including branch, conflict SHA, etc.
//
// Merge Slot Integration:
// Before creating a conflict resolution task, we acquire the merge-slot for this rig.
// This serializes conflict resolution - only one polecat can resolve conflicts at a time.
// If the slot is already held, we skip creating the task and let the MR stay in queue.
// When the current resolution completes and merges, the slot is released.
func (e *Engineer) createConflictResolutionTaskForMR(mr *MRInfo, _ ProcessResult) (string, error) { // result unused but kept for future merge diagnostics
	// === MERGE SLOT GATE: Serialize conflict resolution ===
	// Ensure merge slot exists (idempotent)
	slotID, err := e.beads.MergeSlotEnsureExists()
	if err != nil {
		_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: could not ensure merge slot: %v\n", err)
		// Continue anyway - slot is optional for now
	} else {
		// Try to acquire the merge slot
		holder := e.rig.Name + "/refinery"
		status, err := e.beads.MergeSlotAcquire(holder, false)
		if err != nil {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: could not acquire merge slot: %v\n", err)
			// Continue anyway - slot is optional
		} else if !status.Available && status.Holder != "" && status.Holder != holder {
			// Slot is held by someone else - skip creating the task
			// The MR stays in queue and will retry when slot is released
			_, _ = fmt.Fprintf(e.output, "[Engineer] Merge slot held by %s - deferring conflict resolution\n", status.Holder)
			_, _ = fmt.Fprintf(e.output, "[Engineer] MR %s will retry after current resolution completes\n", mr.ID)
			return "", nil // Not an error - just deferred
		}
		// Either we acquired the slot, or status indicates we already hold it
		_, _ = fmt.Fprintf(e.output, "[Engineer] Acquired merge slot: %s\n", slotID)
	}

	// Get the current main SHA for conflict tracking
	mainSHA, err := e.git.Rev("origin/" + mr.Target)
	if err != nil {
		mainSHA = "unknown-sha"
	}

	// Get the original issue title if we have a source issue
	originalTitle := mr.SourceIssue
	if mr.SourceIssue != "" {
		if sourceIssue, err := e.beads.Show(mr.SourceIssue); err == nil && sourceIssue != nil {
			originalTitle = sourceIssue.Title
		}
	}

	// Priority boost: decrease priority number (lower = higher priority)
	// P2 -> P1, P1 -> P0, P0 stays P0
	boostedPriority := mr.Priority - 1
	if boostedPriority < 0 {
		boostedPriority = 0
	}

	// Increment retry count for tracking
	retryCount := mr.RetryCount + 1

	// Build the task description with metadata
	description := fmt.Sprintf(`Resolve merge conflicts for branch %s

## Metadata
- Original MR: %s
- Branch: %s
- Conflict with: %s@%s
- Original issue: %s
- Retry count: %d

## Instructions
1. Check out the branch: git checkout %s
2. Rebase onto target: git rebase origin/%s
3. Resolve conflicts in your editor
4. Complete the rebase: git add . && git rebase --continue
5. Force-push the resolved branch: git push -f
6. Close this task: bd close <this-task-id>

The Refinery will automatically retry the merge after you force-push.`,
		mr.Branch,
		mr.ID,
		mr.Branch,
		mr.Target, mainSHA[:8],
		mr.SourceIssue,
		retryCount,
		mr.Branch,
		mr.Target,
	)

	// Create the conflict resolution task
	taskTitle := fmt.Sprintf("Resolve merge conflicts: %s", originalTitle)
	task, err := e.beads.Create(beads.CreateOptions{
		Title:       taskTitle,
		Type:        "task",
		Priority:    boostedPriority,
		Description: description,
		Actor:       e.rig.Name + "/refinery",
	})
	if err != nil {
		return "", fmt.Errorf("creating conflict resolution task: %w", err)
	}

	// The conflict task's ID is returned so the MR can be blocked on it.
	// When the task closes, the MR unblocks and re-enters the ready queue.

	_, _ = fmt.Fprintf(e.output, "[Engineer] Created conflict resolution task: %s (P%d)\n", task.ID, task.Priority)

	return task.ID, nil
}

// IsBeadOpen checks if a bead is still open (not closed).
// This is used as a status checker to filter blocked MRs.
func (e *Engineer) IsBeadOpen(beadID string) (bool, error) {
	issue, err := e.beads.Show(beadID)
	if err != nil {
		// If we can't find the bead, treat as not open (fail open - allow MR to proceed)
		return false, nil
	}
	// "closed" status means the bead is done
	return issue.Status != "closed", nil
}

// ListReadyMRs returns MRs that are ready for processing:
// - Not claimed by another worker (checked via assignee field)
// - Not blocked by an open task (handled by bd ready)
// Sorted by priority (highest first).
//
// This queries beads for merge-request wisps.
func (e *Engineer) ListReadyMRs() ([]*MRInfo, error) {
	// Query beads for ready merge-request issues
	issues, err := e.beads.ReadyWithType("merge-request")
	if err != nil {
		return nil, fmt.Errorf("querying beads for merge-requests: %w", err)
	}

	// Convert beads issues to MRInfo
	var mrs []*MRInfo
	for _, issue := range issues {
		// Skip closed MRs (workaround for bd list not respecting --status filter)
		if issue.Status != "open" {
			continue
		}

		fields := beads.ParseMRFields(issue)
		if fields == nil {
			continue // Skip issues without MR fields
		}

		// Skip if already assigned (claimed by another worker)
		if issue.Assignee != "" {
			// TODO: Add stale claim detection based on updated_at
			continue
		}

		// Parse convoy created_at if present
		var convoyCreatedAt *time.Time
		if fields.ConvoyCreatedAt != "" {
			if t, err := time.Parse(time.RFC3339, fields.ConvoyCreatedAt); err == nil {
				convoyCreatedAt = &t
			}
		}

		// Parse issue created_at
		var createdAt time.Time
		if issue.CreatedAt != "" {
			if t, err := time.Parse(time.RFC3339, issue.CreatedAt); err == nil {
				createdAt = t
			}
		}

		mr := &MRInfo{
			ID:              issue.ID,
			Branch:          fields.Branch,
			Target:          fields.Target,
			SourceIssue:     fields.SourceIssue,
			Worker:          fields.Worker,
			Rig:             fields.Rig,
			Title:           issue.Title,
			Priority:        issue.Priority,
			AgentBead:       fields.AgentBead,
			RetryCount:      fields.RetryCount,
			ConvoyID:        fields.ConvoyID,
			ConvoyCreatedAt: convoyCreatedAt,
			CreatedAt:       createdAt,
		}
		mrs = append(mrs, mr)
	}

	return mrs, nil
}

// ListBlockedMRs returns MRs that are blocked by open tasks.
// Useful for monitoring/reporting.
//
// This queries beads for blocked merge-request issues.
func (e *Engineer) ListBlockedMRs() ([]*MRInfo, error) {
	// Query all merge-request issues (both ready and blocked)
	issues, err := e.beads.List(beads.ListOptions{
		Status:   "open",
		Label:    "gt:merge-request",
		Priority: -1, // No priority filter
	})
	if err != nil {
		return nil, fmt.Errorf("querying beads for merge-requests: %w", err)
	}

	// Filter for blocked issues (those with open blockers)
	var mrs []*MRInfo
	for _, issue := range issues {
		// Skip if not blocked
		if len(issue.BlockedBy) == 0 {
			continue
		}

		// Check if any blocker is still open
		hasOpenBlocker := false
		for _, blockerID := range issue.BlockedBy {
			isOpen, err := e.IsBeadOpen(blockerID)
			if err == nil && isOpen {
				hasOpenBlocker = true
				break
			}
		}
		if !hasOpenBlocker {
			continue // All blockers are closed, not blocked
		}

		fields := beads.ParseMRFields(issue)
		if fields == nil {
			continue
		}

		// Parse convoy created_at if present
		var convoyCreatedAt *time.Time
		if fields.ConvoyCreatedAt != "" {
			if t, err := time.Parse(time.RFC3339, fields.ConvoyCreatedAt); err == nil {
				convoyCreatedAt = &t
			}
		}

		// Parse issue created_at
		var createdAt time.Time
		if issue.CreatedAt != "" {
			if t, err := time.Parse(time.RFC3339, issue.CreatedAt); err == nil {
				createdAt = t
			}
		}

		// Use the first open blocker as BlockedBy
		blockedBy := ""
		for _, blockerID := range issue.BlockedBy {
			isOpen, err := e.IsBeadOpen(blockerID)
			if err == nil && isOpen {
				blockedBy = blockerID
				break
			}
		}

		mr := &MRInfo{
			ID:              issue.ID,
			Branch:          fields.Branch,
			Target:          fields.Target,
			SourceIssue:     fields.SourceIssue,
			Worker:          fields.Worker,
			Rig:             fields.Rig,
			Title:           issue.Title,
			Priority:        issue.Priority,
			AgentBead:       fields.AgentBead,
			RetryCount:      fields.RetryCount,
			ConvoyID:        fields.ConvoyID,
			ConvoyCreatedAt: convoyCreatedAt,
			CreatedAt:       createdAt,
			BlockedBy:       blockedBy,
		}
		mrs = append(mrs, mr)
	}

	return mrs, nil
}

// ClaimMR claims an MR for processing by setting the assignee field.
// This replaces mrqueue.Claim() for beads-based MRs.
// The workerID is typically the refinery's identifier (e.g., "gastown/refinery").
func (e *Engineer) ClaimMR(mrID, workerID string) error {
	return e.beads.Update(mrID, beads.UpdateOptions{
		Assignee: &workerID,
	})
}

// ReleaseMR releases a claimed MR back to the queue by clearing the assignee.
// This replaces mrqueue.Release() for beads-based MRs.
func (e *Engineer) ReleaseMR(mrID string) error {
	empty := ""
	return e.beads.Update(mrID, beads.UpdateOptions{
		Assignee: &empty,
	})
}
