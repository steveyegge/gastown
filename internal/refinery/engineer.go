// Package refinery provides the merge queue processing agent.
package refinery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/protocol"
	"github.com/steveyegge/gastown/internal/rig"
)

// DefaultStaleClaimTimeout is the default duration after which a claimed MR
// is considered abandoned and eligible for re-claim. This is conservative
// to avoid re-claiming MRs that are legitimately processing long test suites.
// Can be overridden per-rig via MergeQueueConfig.StaleClaimTimeout.
const DefaultStaleClaimTimeout = 30 * time.Minute

// isClaimStale checks if a claimed MR should be considered abandoned based on
// its UpdatedAt timestamp and configured timeout. Returns true if the claim
// is stale (eligible for re-claim), false if the claim is recent or the
// timestamp is invalid/missing.
func isClaimStale(updatedAt string, timeout time.Duration) (stale bool, parseErr error) {
	if updatedAt == "" {
		return false, nil // No timestamp - assume claim is valid
	}
	t, err := time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return false, err // Caller should log the parse error
	}
	return time.Since(t) >= timeout, nil
}

// GateConfig defines a single quality gate command.
type GateConfig struct {
	// Cmd is the shell command to execute.
	Cmd string `json:"cmd"`

	// Timeout is the maximum time the gate command may run.
	// Zero means no timeout (inherits context deadline).
	Timeout time.Duration `json:"timeout"`
}

// GateResult holds the outcome of a single gate execution.
type GateResult struct {
	Name    string
	Success bool
	Error   string
	Elapsed time.Duration
}

// MergeQueueConfig holds configuration for the merge queue processor.
//
// Note: Integration branch gating (polecat/refinery enabled flags) is handled at
// MR creation time via config.MergeQueueConfig and formula injection, not here.
// The Engineer's job is to merge whatever target the MR specifies — it doesn't
// need to know whether integration branches are enabled.
type MergeQueueConfig struct {
	// Enabled controls whether the merge queue is active.
	Enabled bool `json:"enabled"`

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

	// StaleClaimTimeout is how long a claimed MR can go without updates before
	// being considered abandoned and eligible for re-claim. This handles the
	// case where a refinery crashes mid-merge, leaving an MR permanently claimed.
	// Set conservatively to avoid re-claiming MRs with long-running test suites.
	// NOTE: Only one refinery instance runs per rig (enforced by ErrAlreadyRunning
	// in manager.go), so concurrent re-claim is not a concern in practice.
	StaleClaimTimeout time.Duration `json:"stale_claim_timeout"`

	// Gates defines named quality gate commands to run before merging.
	// When non-empty, gates replace the legacy RunTests/TestCommand path.
	// Each gate runs as a shell command with an optional per-gate timeout.
	Gates map[string]*GateConfig `json:"gates"`

	// GatesParallel controls whether gates run concurrently.
	// When true, all gates start simultaneously; any failure = overall failure.
	GatesParallel bool `json:"gates_parallel"`
}

// DefaultMergeQueueConfig returns sensible defaults for merge queue configuration.
func DefaultMergeQueueConfig() *MergeQueueConfig {
	return &MergeQueueConfig{
		Enabled:              true,
		OnConflict:           "assign_back",
		RunTests:             true,
		TestCommand:          "",
		DeleteMergedBranches: true,
		RetryFlakyTests:      1,
		PollInterval:         30 * time.Second,
		MaxConcurrent:        1,
		StaleClaimTimeout:    DefaultStaleClaimTimeout,
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

	// Raw data for agent-side queue health analysis (ZFC: agent decides, Go transports)
	UpdatedAt          time.Time // When the MR was last updated
	Assignee           string    // Who claimed this MR (empty = unclaimed)
	BranchExistsLocal  bool      // Whether the MR branch exists locally
	BranchExistsRemote bool      // Whether the MR branch exists in remote tracking refs
}

// MRAnomaly represents an MR queue health problem that can stall processing.
type MRAnomaly struct {
	ID       string        `json:"id"`
	Branch   string        `json:"branch"`
	Type     string        `json:"type"`     // stale-claim | orphaned-branch
	Severity string        `json:"severity"` // warning | critical
	Assignee string        `json:"assignee,omitempty"`
	Age      time.Duration `json:"age,omitempty"`
	Detail   string        `json:"detail"`
}

const (
	staleClaimWarningAfter  = 2 * time.Hour
	staleClaimCriticalAfter = 6 * time.Hour
)

// errMergeSlotTimeout is returned by acquireMainPushSlot when retries are
// exhausted due to slot contention. Infrastructure errors (beads down,
// permission errors) return a different error so callers can distinguish
// transient contention from real failures that need operator attention.
var errMergeSlotTimeout = errors.New("merge slot contention timeout")

// mergeSlotSeq is a package-level counter for unique merge slot holder IDs.
// Using time.Now().UnixNano() alone is insufficient on Windows where timer
// resolution can cause identical timestamps across concurrent goroutines.
var mergeSlotSeq uint64

// Engineer is the merge queue processor that polls for ready merge-requests
// and processes them according to the merge queue design.
type Engineer struct {
	rig                   *rig.Rig
	beads                 *beads.Beads
	git                   *git.Git
	config                *MergeQueueConfig
	workDir               string
	output                io.Writer    // Output destination for user-facing messages
	router                *mail.Router // Mail router for sending protocol messages
	mergeSlotEnsureExists func() (string, error)
	mergeSlotAcquire      func(holder string, addWaiter bool) (*beads.MergeSlotStatus, error)
	mergeSlotRelease      func(holder string) error
	mergeSlotMaxRetries   int           // Max retries for slot acquisition (0 = no retry)
	mergeSlotRetryBackoff time.Duration // Initial backoff between retries
}

// NewEngineer creates a new Engineer for the given rig.
func NewEngineer(r *rig.Rig) *Engineer {
	cfg := DefaultMergeQueueConfig()

	// Determine the git working directory for refinery operations.
	// Prefer refinery/rig worktree, fall back to mayor/rig (legacy architecture).
	// Using rig.Path directly would find town's .git with rig-named remotes instead of "origin".
	gitDir := filepath.Join(r.Path, "refinery", "rig")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		gitDir = filepath.Join(r.Path, "mayor", "rig")
	}
	beadsClient := beads.New(r.Path)

	return &Engineer{
		rig:     r,
		beads:   beadsClient,
		git:     git.NewGit(gitDir),
		config:  cfg,
		workDir: gitDir,
		output:  os.Stdout,
		router:  mail.NewRouter(r.Path),
		mergeSlotEnsureExists: func() (string, error) {
			return beadsClient.MergeSlotEnsureExists()
		},
		mergeSlotAcquire: func(holder string, addWaiter bool) (*beads.MergeSlotStatus, error) {
			return beadsClient.MergeSlotAcquire(holder, addWaiter)
		},
		mergeSlotRelease: func(holder string) error {
			return beadsClient.MergeSlotRelease(holder)
		},
		mergeSlotMaxRetries:   10,
		mergeSlotRetryBackoff: 500 * time.Millisecond,
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
		Enabled              *bool                      `json:"enabled"`
		OnConflict           *string                    `json:"on_conflict"`
		RunTests             *bool                      `json:"run_tests"`
		TestCommand          *string                    `json:"test_command"`
		DeleteMergedBranches *bool                      `json:"delete_merged_branches"`
		RetryFlakyTests      *int                       `json:"retry_flaky_tests"`
		PollInterval         *string                    `json:"poll_interval"`
		MaxConcurrent        *int                       `json:"max_concurrent"`
		StaleClaimTimeout    *string                    `json:"stale_claim_timeout"`
		Gates                map[string]*gateConfigRaw  `json:"gates"`
		GatesParallel        *bool                      `json:"gates_parallel"`
	}

	if err := json.Unmarshal(rawConfig.MergeQueue, &mqRaw); err != nil {
		return fmt.Errorf("parsing merge_queue config: %w", err)
	}

	// Apply non-nil values to config (preserving defaults for missing fields)
	if mqRaw.Enabled != nil {
		e.config.Enabled = *mqRaw.Enabled
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
	if mqRaw.StaleClaimTimeout != nil {
		dur, err := time.ParseDuration(*mqRaw.StaleClaimTimeout)
		if err != nil {
			return fmt.Errorf("invalid stale_claim_timeout %q: %w", *mqRaw.StaleClaimTimeout, err)
		}
		if dur <= 0 {
			return fmt.Errorf("stale_claim_timeout must be positive, got %v", dur)
		}
		e.config.StaleClaimTimeout = dur
	}

	// Parse gates configuration
	if mqRaw.Gates != nil {
		e.config.Gates = make(map[string]*GateConfig, len(mqRaw.Gates))
		for name, raw := range mqRaw.Gates {
			gc := &GateConfig{Cmd: raw.Cmd}
			if raw.Timeout != "" {
				dur, err := time.ParseDuration(raw.Timeout)
				if err != nil {
					return fmt.Errorf("invalid timeout for gate %q: %w", name, err)
				}
				if dur <= 0 {
					return fmt.Errorf("gate %q timeout must be positive, got %v", name, dur)
				}
				gc.Timeout = dur
			}
			e.config.Gates[name] = gc
		}
	}
	if mqRaw.GatesParallel != nil {
		e.config.GatesParallel = *mqRaw.GatesParallel
	}

	return nil
}

// gateConfigRaw is the JSON-friendly representation of a gate config
// with timeout as a string duration.
type gateConfigRaw struct {
	Cmd     string `json:"cmd"`
	Timeout string `json:"timeout"`
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
	SlotTimeout bool // Merge slot contention timeout (distinct from build/test failure)
}

// doMerge performs the actual git merge operation.
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

	// Step 3.5: Push submodule commits if the branch changes submodule pointers.
	// The refinery owns all remote pushes — submodule commits must land before the
	// parent pointer is merged, otherwise main gets dangling submodule references.
	subChanges, err := e.git.SubmoduleChanges(target, branch)
	if err != nil {
		_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: could not check submodule changes: %v\n", err)
	}
	if len(subChanges) > 0 {
		// Ensure submodules are initialized in the refinery worktree
		if initErr := git.InitSubmodules(e.git.WorkDir()); initErr != nil {
			return ProcessResult{
				Success: false,
				Error:   fmt.Sprintf("failed to init submodules in refinery worktree: %v", initErr),
			}
		}
		for _, sc := range subChanges {
			if sc.NewSHA == "" {
				continue // Submodule removed, nothing to push
			}
			_, _ = fmt.Fprintf(e.output, "[Engineer] Pushing submodule %s (commit %s)...\n", sc.Path, sc.NewSHA[:8])
			if pushErr := e.git.PushSubmoduleCommit(sc.Path, sc.NewSHA, "origin"); pushErr != nil {
				return ProcessResult{
					Success: false,
					Error:   fmt.Sprintf("failed to push submodule %s: %v", sc.Path, pushErr),
				}
			}
		}
		_, _ = fmt.Fprintf(e.output, "[Engineer] Pushed %d submodule(s)\n", len(subChanges))
	}

	// Step 4: Run quality gates (or legacy tests) if configured
	if len(e.config.Gates) > 0 {
		// New gates system: run configured quality gates
		gateResult := e.runGates(ctx)
		if !gateResult.Success {
			return gateResult
		}
	} else if e.config.RunTests && e.config.TestCommand != "" {
		// Legacy test command path (backward compatible)
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

	// Step 5: Perform the actual merge using squash merge
	// Get the original commit message from the polecat branch to preserve the
	// conventional commit format (feat:/fix:) instead of creating redundant merge commits
	originalMsg, err := e.git.GetBranchCommitMessage(branch)
	if err != nil {
		// Fallback to a descriptive message if we can't get the original
		originalMsg = fmt.Sprintf("Squash merge %s into %s", branch, target)
		if sourceIssue != "" {
			originalMsg = fmt.Sprintf("Squash merge %s into %s (%s)", branch, target, sourceIssue)
		}
		_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: could not get original commit message: %v\n", err)
	}
	_, _ = fmt.Fprintf(e.output, "[Engineer] Squash merging with message: %s\n", strings.TrimSpace(originalMsg))
	if err := e.git.MergeSquash(branch, originalMsg); err != nil {
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

	// Step 7: Acquire merge slot before push to serialize writes to the default branch.
	// Only serialize pushes to the rig's default branch (typically main).
	// Integration-branch and feature-branch pushes don't need serialization.
	var pushHolder string
	if target == e.rig.DefaultBranch() {
		var slotErr error
		pushHolder, slotErr = e.acquireMainPushSlot(ctx)
		if slotErr != nil {
			// Reset the checked-out target branch to origin to undo the local squash commit.
			// ResetHard is required because target is the current branch (checked out in Step 2).
			if resetErr := e.git.ResetHard("origin/" + target); resetErr != nil {
				_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to reset %s after slot failure: %v\n", target, resetErr)
			}
			// Only classify as SlotTimeout for actual contention (retries exhausted).
			// Infrastructure errors (beads down, permission errors) should surface
			// through the normal failure/notification path for operator visibility.
			return ProcessResult{
				Success:     false,
				SlotTimeout: errors.Is(slotErr, errMergeSlotTimeout),
				Error:       fmt.Sprintf("failed to acquire merge slot before push: %v", slotErr),
			}
		}
		defer func() {
			// pushHolder is empty when the self-conflict bypass fires — conflict-resolution
			// owns the slot, so we must not release it here.
			if pushHolder != "" {
				if releaseErr := e.mergeSlotRelease(pushHolder); releaseErr != nil {
					_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to release merge slot for push (%s): %v\n", pushHolder, releaseErr)
				}
			}
		}()
	}

	// Step 8: Push to origin
	_, _ = fmt.Fprintf(e.output, "[Engineer] Pushing to origin/%s...\n", target)
	if err := e.git.Push("origin", target, false); err != nil {
		// Reset the checked-out target branch to undo the local squash commit.
		// Without this, the next retry could see stale local state from the failed push.
		if resetErr := e.git.ResetHard("origin/" + target); resetErr != nil {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to reset %s after push failure: %v\n", target, resetErr)
		}
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

func (e *Engineer) acquireMainPushSlot(ctx context.Context) (string, error) {
	slotID, err := e.mergeSlotEnsureExists()
	if err != nil {
		return "", fmt.Errorf("ensure merge slot exists: %w", err)
	}

	seq := atomic.AddUint64(&mergeSlotSeq, 1)
	holder := fmt.Sprintf("%s/refinery/push/%d-%d", e.rig.Name, time.Now().UnixNano(), seq)

	// The conflict-resolution path holds the slot with holder "rigName/refinery".
	// Both push and conflict-resolution run in the same single-threaded refinery
	// agent, so if our own rig holds the slot for conflict resolution, we can
	// safely proceed without re-acquiring — no concurrent push is possible.
	selfConflictHolder := e.rig.Name + "/refinery"

	backoff := e.mergeSlotRetryBackoff
	if backoff == 0 {
		backoff = 500 * time.Millisecond
	}

	for attempt := 0; attempt <= e.mergeSlotMaxRetries; attempt++ {
		if attempt > 0 {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Merge slot held, retrying in %v (attempt %d/%d)...\n", backoff, attempt, e.mergeSlotMaxRetries)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return "", ctx.Err()
			}
			backoff = min(backoff*2, 10*time.Second)
		}

		status, err := e.mergeSlotAcquire(holder, false)
		if err != nil {
			return "", fmt.Errorf("acquire merge slot %s (%s): %w", slotID, holder, err)
		}
		if status == nil {
			return "", fmt.Errorf("acquire merge slot %s (%s): empty status", slotID, holder)
		}
		if status.Available || status.Holder == holder {
			return holder, nil
		}
		// Slot held by our own conflict-resolution path — safe to proceed.
		if status.Holder == selfConflictHolder {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Merge slot held by conflict-resolution path, proceeding\n")
			return "", nil // No holder to release — conflict-resolution owns the slot
		}
	}

	return "", fmt.Errorf("merge slot %s: %w after %d retries", slotID, errMergeSlotTimeout, e.mergeSlotMaxRetries)
}

// ValidateTestCommand validates that a test command is safe to execute.
// TestCommand comes from the rig's operator-controlled config.json, not from
// user input or PR branches. This validation provides defense-in-depth for the
// trusted infrastructure config path.
func ValidateTestCommand(cmd string) error {
	if strings.TrimSpace(cmd) == "" {
		return fmt.Errorf("test command must not be empty")
	}
	return nil
}

// runTests runs the configured test command and returns the result.
func (e *Engineer) runTests(ctx context.Context) ProcessResult {
	if err := ValidateTestCommand(e.config.TestCommand); err != nil {
		return ProcessResult{
			Success: false,
			Error:   fmt.Sprintf("invalid test command: %v", err),
		}
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

		// Trust boundary: TestCommand comes from rig's config.json (operator-controlled
		// infrastructure config), not from PR branches or user input. Shell execution
		// is intentional for flexibility (pipes, env vars, etc).
		_, _ = fmt.Fprintf(e.output, "[Engineer] Executing test command: %s\n", e.config.TestCommand)
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

// runGate executes a single quality gate command and returns the result.
func (e *Engineer) runGate(ctx context.Context, name string, gate *GateConfig) GateResult {
	start := time.Now()

	if strings.TrimSpace(gate.Cmd) == "" {
		return GateResult{
			Name:    name,
			Success: false,
			Error:   "gate command is empty",
			Elapsed: time.Since(start),
		}
	}

	// Apply per-gate timeout if configured
	gateCtx := ctx
	if gate.Timeout > 0 {
		var cancel context.CancelFunc
		gateCtx, cancel = context.WithTimeout(ctx, gate.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(gateCtx, "sh", "-c", gate.Cmd) //nolint:gosec // G204: Gate commands are from trusted rig config
	cmd.Dir = e.workDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	elapsed := time.Since(start)

	if err == nil {
		return GateResult{
			Name:    name,
			Success: true,
			Elapsed: elapsed,
		}
	}

	errMsg := fmt.Sprintf("%v", err)
	if gateCtx.Err() == context.DeadlineExceeded {
		errMsg = fmt.Sprintf("timed out after %v", gate.Timeout)
	}
	if stderrStr := strings.TrimSpace(stderr.String()); stderrStr != "" {
		// Cap stderr to avoid huge error messages
		if len(stderrStr) > 500 {
			stderrStr = stderrStr[:500] + "..."
		}
		errMsg = fmt.Sprintf("%s: %s", errMsg, stderrStr)
	}

	return GateResult{
		Name:    name,
		Success: false,
		Error:   errMsg,
		Elapsed: elapsed,
	}
}

// runGates executes all configured quality gates and returns a ProcessResult.
// Gates run in parallel if GatesParallel is true; otherwise sequentially.
// Any single gate failure means overall failure.
func (e *Engineer) runGates(ctx context.Context) ProcessResult {
	gates := e.config.Gates
	if len(gates) == 0 {
		return ProcessResult{Success: true}
	}

	// Sort gate names for deterministic ordering
	names := make([]string, 0, len(gates))
	for name := range gates {
		names = append(names, name)
	}
	sort.Strings(names)

	_, _ = fmt.Fprintf(e.output, "[Engineer] Running %d quality gate(s) (parallel=%v)\n", len(names), e.config.GatesParallel)

	var results []GateResult

	if e.config.GatesParallel {
		results = make([]GateResult, len(names))
		var wg sync.WaitGroup
		for i, name := range names {
			wg.Add(1)
			go func(idx int, gateName string) {
				defer wg.Done()
				_, _ = fmt.Fprintf(e.output, "[Engineer] Gate %q: starting (%s)\n", gateName, gates[gateName].Cmd)
				results[idx] = e.runGate(ctx, gateName, gates[gateName])
			}(i, name)
		}
		wg.Wait()
	} else {
		for _, name := range names {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Gate %q: starting (%s)\n", name, gates[name].Cmd)
			result := e.runGate(ctx, name, gates[name])
			results = append(results, result)
			if !result.Success {
				// Sequential mode: stop on first failure
				break
			}
		}
	}

	// Report results
	var failures []string
	for _, r := range results {
		if r.Success {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Gate %q: passed (%v)\n", r.Name, r.Elapsed.Truncate(time.Millisecond))
		} else {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Gate %q: FAILED (%v) - %s\n", r.Name, r.Elapsed.Truncate(time.Millisecond), r.Error)
			failures = append(failures, fmt.Sprintf("%s: %s", r.Name, r.Error))
		}
	}

	if len(failures) > 0 {
		return ProcessResult{
			Success:     false,
			TestsFailed: true,
			Error:       fmt.Sprintf("quality gates failed: %s", strings.Join(failures, "; ")),
		}
	}

	_, _ = fmt.Fprintln(e.output, "[Engineer] All quality gates passed")
	return ProcessResult{Success: true}
}

// syncCrewWorkspaces pulls latest changes to all crew workspaces.
// This ensures crew members have access to newly merged code without manual sync.
func (e *Engineer) syncCrewWorkspaces() {
	crewGit := git.NewGit(e.rig.Path)
	crewMgr := crew.NewManager(e.rig, crewGit)

	workers, err := crewMgr.List()
	if err != nil {
		_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to list crew workspaces: %v\n", err)
		return
	}

	if len(workers) == 0 {
		return
	}

	_, _ = fmt.Fprintf(e.output, "[Engineer] Syncing %d crew workspace(s)...\n", len(workers))

	for _, worker := range workers {
		result, err := crewMgr.Pristine(worker.Name)
		if err != nil {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to sync crew/%s: %v\n", worker.Name, err)
			continue
		}
		if result.Pulled {
			_, _ = fmt.Fprintf(e.output, "[Engineer] ✓ Synced crew/%s\n", worker.Name)
		} else if result.PullError != "" {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: crew/%s pull failed: %s\n", worker.Name, result.PullError)
		}
	}
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
	if err := e.mergeSlotRelease(holder); err != nil {
		// Best-effort: slot release failures are always non-fatal.
		// Slot may not have been held (optional acquisition) or may have expired.
		_, _ = fmt.Fprintf(e.output, "[Engineer] Note: merge slot release: %v\n", err)
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

	// 2. Delete source branch if configured (local and remote)
	if e.config.DeleteMergedBranches && mr.Branch != "" {
		if err := e.git.DeleteBranch(mr.Branch, true); err != nil {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to delete local branch %s: %v\n", mr.Branch, err)
		} else {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Deleted local branch: %s\n", mr.Branch)
		}
		if err := e.git.DeleteRemoteBranch("origin", mr.Branch); err != nil {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to delete remote branch %s: %v\n", mr.Branch, err)
		} else {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Deleted remote branch: %s\n", mr.Branch)
		}
	}

	// 3. Check and auto-close completed convoys
	// After closing a source issue, its parent convoy may now be complete.
	// Run convoy check to auto-close and notify subscribers.
	e.postMergeConvoyCheck(mr)

	// 4. Log success
	_, _ = fmt.Fprintf(e.output, "[Engineer] ✓ Merged: %s (commit: %s)\n", mr.ID, result.MergeCommit)
}

// HandleMRInfoFailure handles a failed merge from MRInfo.
// For conflicts, creates a resolution task and blocks the MR until resolved.
// For slot timeouts, the MR stays in queue for automatic retry without notifying polecats.
// This enables non-blocking delegation: the queue continues to the next MR.
func (e *Engineer) HandleMRInfoFailure(mr *MRInfo, result ProcessResult) {
	// Slot timeout is transient infrastructure contention — not a build/test/conflict failure.
	// The MR stays in queue and will be retried on the next poll cycle.
	// No polecat notification needed since there's nothing for a worker to fix.
	if result.SlotTimeout {
		_, _ = fmt.Fprintf(e.output, "[Engineer] ✗ Slot timeout: %s - %s\n", mr.ID, result.Error)
		_, _ = fmt.Fprintln(e.output, "[Engineer] MR remains in queue for automatic retry (slot contention)")
		return
	}

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
	slotID, err := e.mergeSlotEnsureExists()
	slotHolder := "" // tracks acquired slot for cleanup on error
	if err != nil {
		_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: could not ensure merge slot: %v\n", err)
		// Continue anyway - slot is optional for now
	} else {
		// Try to acquire the merge slot
		holder := e.rig.Name + "/refinery"
		status, err := e.mergeSlotAcquire(holder, false)
		if err != nil {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: could not acquire merge slot: %v\n", err)
			// Continue anyway - slot is optional
		} else if status == nil {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: merge slot returned nil status\n")
			// Continue anyway - slot is optional
		} else if !status.Available && status.Holder != "" && status.Holder != holder {
			// Slot is held by someone else - skip creating the task
			// The MR stays in queue and will retry when slot is released
			_, _ = fmt.Fprintf(e.output, "[Engineer] Merge slot held by %s - deferring conflict resolution\n", status.Holder)
			_, _ = fmt.Fprintf(e.output, "[Engineer] MR %s will retry after current resolution completes\n", mr.ID)
			return "", nil // Not an error - just deferred
		} else {
			slotHolder = holder
			_, _ = fmt.Fprintf(e.output, "[Engineer] Acquired merge slot: %s\n", slotID)
		}
	}
	// Release slot on error to prevent permanent blockage
	releaseSlotOnError := func() {
		if slotHolder != "" {
			_ = e.mergeSlotRelease(slotHolder)
		}
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
		releaseSlotOnError()
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

// issueToMRInfo converts a beads issue (with parsed MR fields) into an MRInfo.
// Shared by ListReadyMRs, ListBlockedMRs, and ListAllOpenMRs.
func issueToMRInfo(issue *beads.Issue, fields *beads.MRFields) *MRInfo {
	// Parse convoy created_at if present
	var convoyCreatedAt *time.Time
	if fields.ConvoyCreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, fields.ConvoyCreatedAt); err == nil {
			convoyCreatedAt = &t
		}
	}

	// Parse issue timestamps
	var createdAt, updatedAt time.Time
	if issue.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, issue.CreatedAt); err == nil {
			createdAt = t
		}
	}
	if issue.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339, issue.UpdatedAt); err == nil {
			updatedAt = t
		}
	}

	return &MRInfo{
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
		UpdatedAt:       updatedAt,
		Assignee:        issue.Assignee,
	}
}

// firstOpenBlocker returns the ID of the first open blocker for an issue,
// or empty string if none are open.
func (e *Engineer) firstOpenBlocker(issue *beads.Issue) string {
	for _, blockerID := range issue.BlockedBy {
		isOpen, err := e.IsBeadOpen(blockerID)
		if err == nil && isOpen {
			return blockerID
		}
	}
	return ""
}

// ListReadyMRs returns MRs that are ready for processing:
// - Not claimed by another worker (checked via assignee field)
// - Not blocked by an open task (checked via firstOpenBlocker)
// Sorted by priority (highest first).
//
// Uses bd list instead of bd ready because MRs are ephemeral beads and
// bd ready filters out ephemeral issues (see gt-t5t6y). This matches the
// pattern used by ListBlockedMRs and ListAllOpenMRs.
func (e *Engineer) ListReadyMRs() ([]*MRInfo, error) {
	// Query beads for all open merge-request issues.
	// Cannot use ReadyWithType here because bd ready excludes ephemeral beads,
	// and MRs are ephemeral by design. Use List + manual blocker check instead.
	issues, err := e.beads.List(beads.ListOptions{
		Status:   "open",
		Label:    "gt:merge-request",
		Priority: -1, // No priority filter
	})
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

		// Skip blocked MRs (replaces bd ready's blocker filtering)
		if blockedBy := e.firstOpenBlocker(issue); blockedBy != "" {
			continue
		}

		// Belt-and-suspenders: skip MRs labeled gt:owned-direct.
		// These MRs shouldn't exist (gt done skips MR creation for owned+direct
		// convoys), but if one slips through, the refinery should not process it.
		if beads.HasLabel(issue, "gt:owned-direct") {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Skipping MR %s: owned+direct convoy (belt-and-suspenders)\n", issue.ID)
			continue
		}

		fields := beads.ParseMRFields(issue)
		if fields == nil {
			continue // Skip issues without MR fields
		}

		// Skip if already assigned, unless claim is stale (allows re-claim after crash).
		// NOTE: Only one refinery runs per rig (enforced by ErrAlreadyRunning in
		// manager.go), so concurrent re-claim race conditions are not a concern.
		if issue.Assignee != "" {
			stale, parseErr := isClaimStale(issue.UpdatedAt, e.config.StaleClaimTimeout)
			if parseErr != nil {
				_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: could not parse UpdatedAt for %s: %v (treating claim as valid)\n",
					issue.ID, parseErr)
			}
			if !stale {
				continue
			}
			_, _ = fmt.Fprintf(e.output, "[Engineer] Stale claim detected: %s (assignee: %s, updated: %s) — eligible for re-claim\n",
				issue.ID, issue.Assignee, issue.UpdatedAt)
		}

		mrs = append(mrs, issueToMRInfo(issue, fields))
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
		blockedBy := e.firstOpenBlocker(issue)
		if blockedBy == "" {
			continue // All blockers are closed, not blocked
		}

		fields := beads.ParseMRFields(issue)
		if fields == nil {
			continue
		}

		mr := issueToMRInfo(issue, fields)
		mr.BlockedBy = blockedBy
		mrs = append(mrs, mr)
	}

	return mrs, nil
}

// ListAllOpenMRs returns all open merge requests with full raw data.
// Unlike ListReadyMRs/ListBlockedMRs, this performs no filtering — it returns
// claimed, unclaimed, blocked, and unblocked MRs. It also checks branch existence
// so agents can detect orphaned MRs. Designed for agent-side queue health analysis
// (ZFC: Go transports data, agent decides what's interesting).
func (e *Engineer) ListAllOpenMRs() ([]*MRInfo, error) {
	issues, err := e.beads.List(beads.ListOptions{
		Status:   "open",
		Label:    "gt:merge-request",
		Priority: -1,
	})
	if err != nil {
		return nil, fmt.Errorf("querying beads for merge-requests: %w", err)
	}

	var mrs []*MRInfo
	for _, issue := range issues {
		if issue.Status != "open" {
			continue
		}

		fields := beads.ParseMRFields(issue)
		if fields == nil {
			continue
		}

		mr := issueToMRInfo(issue, fields)

		// Check branch existence (local + remote tracking refs)
		mr.BranchExistsLocal, _ = e.git.BranchExists(fields.Branch)
		mr.BranchExistsRemote, _ = e.git.RemoteTrackingBranchExists("origin", fields.Branch)
		mr.BlockedBy = e.firstOpenBlocker(issue)

		mrs = append(mrs, mr)
	}

	return mrs, nil
}

// ListQueueAnomalies finds stale claims and orphaned branches in open MRs.
// This gives Witness/Refinery patrols deterministic signals for deadlock risk.
func (e *Engineer) ListQueueAnomalies(now time.Time) ([]*MRAnomaly, error) {
	issues, err := e.beads.List(beads.ListOptions{
		Status:   "open",
		Label:    "gt:merge-request",
		Priority: -1,
	})
	if err != nil {
		return nil, fmt.Errorf("querying beads for merge-requests: %w", err)
	}

	return detectQueueAnomalies(issues, now, func(branch string) (bool, bool, error) {
		localExists, err := e.git.BranchExists(branch)
		if err != nil {
			return false, false, err
		}
		remoteTrackingExists, err := e.git.RemoteTrackingBranchExists("origin", branch)
		if err != nil {
			return false, false, err
		}
		return localExists, remoteTrackingExists, nil
	}), nil
}

func detectQueueAnomalies(
	issues []*beads.Issue,
	now time.Time,
	branchExistsFn func(branch string) (localExists bool, remoteTrackingExists bool, err error),
) []*MRAnomaly {
	var anomalies []*MRAnomaly

	for _, issue := range issues {
		if issue == nil || issue.Status != "open" {
			continue
		}
		fields := beads.ParseMRFields(issue)
		if fields == nil || fields.Branch == "" {
			continue
		}

		// 1) Stale claim detection.
		if issue.Assignee != "" {
			updatedAt, err := time.Parse(time.RFC3339, issue.UpdatedAt)
			if err == nil {
				age := now.Sub(updatedAt)
				if age >= staleClaimWarningAfter {
					severity := "warning"
					if age >= staleClaimCriticalAfter {
						severity = "critical"
					}
					anomalies = append(anomalies, &MRAnomaly{
						ID:       issue.ID,
						Branch:   fields.Branch,
						Type:     "stale-claim",
						Severity: severity,
						Assignee: issue.Assignee,
						Age:      age,
						Detail:   "MR is claimed but not progressing",
					})
				}
			}
		}

		// 2) Orphaned branch detection.
		localExists, remoteTrackingExists, err := branchExistsFn(fields.Branch)
		if err == nil && !localExists && !remoteTrackingExists {
			anomalies = append(anomalies, &MRAnomaly{
				ID:       issue.ID,
				Branch:   fields.Branch,
				Type:     "orphaned-branch",
				Severity: "critical",
				Detail:   "MR branch is missing locally and in origin/* tracking refs",
			})
		}
	}

	return anomalies
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

// postMergeConvoyCheck runs convoy completion checks after a successful merge.
//
// When a source issue is closed by a merge, any convoy tracking that issue may
// now be complete (all tracked issues closed). This method:
//  1. Runs `gt convoy check` to auto-close completed convoys and notify subscribers
//  2. For completed convoys with integration branches (swarms), triggers landing
//  3. Cleans up stale polecat branches from completed work
//
// All operations are best-effort: failures are logged but don't affect merge success.
func (e *Engineer) postMergeConvoyCheck(mr *MRInfo) {
	// Find town root from rig path (rig is at ~/gt/<rigname>, town is ~/gt)
	townRoot := filepath.Dir(e.rig.Path)
	townBeads := filepath.Join(townRoot, ".beads")

	// Quick check: does town-level beads exist?
	if _, err := os.Stat(townBeads); os.IsNotExist(err) {
		return
	}

	// Step 1: Run `gt convoy check` to auto-close completed convoys.
	// This handles cross-rig convoy completion: convoys in town beads (hq-*)
	// tracking issues in rig beads (gt-*) won't auto-close via bd close alone.
	closedConvoys := e.checkAndCloseCompletedConvoys(townRoot, townBeads)

	// Step 2: For each closed convoy, check if it has a swarm with an
	// integration branch that needs landing.
	for _, convoy := range closedConvoys {
		e.landConvoySwarm(townRoot, convoy)
	}

	// Step 3: Notify deacon of convoy-eligible merges for immediate feeding.
	// When the merged MR is part of a convoy, send a structured CONVOY_NEEDS_FEEDING
	// protocol message so the deacon can immediately feed the next ready issue
	// instead of waiting for the next patrol cycle (up to 10 minutes).
	e.notifyDeaconConvoyFeeding(mr)

	// Step 4: Clean up stale branches from completed work.
	// Prune remote tracking refs that no longer exist on origin.
	if e.config.DeleteMergedBranches {
		e.pruneStaleRemoteRefs()
	}
}

// notifyDeaconConvoyFeeding sends a CONVOY_NEEDS_FEEDING protocol message to
// the deacon when the merged MR is part of a convoy. This triggers immediate
// convoy feeding instead of waiting for the next deacon patrol cycle (up to
// 10 minutes). An event is also emitted to wake the deacon from await-signal.
func (e *Engineer) notifyDeaconConvoyFeeding(mr *MRInfo) {
	if mr.ConvoyID == "" {
		return
	}

	msg := protocol.NewConvoyNeedsFeedingMessage(e.rig.Name, mr.ConvoyID, mr.SourceIssue)
	if err := e.router.Send(msg); err != nil {
		_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to notify deacon of convoy feeding for %s: %v\n", mr.ConvoyID, err)
	} else {
		_, _ = fmt.Fprintf(e.output, "[Engineer] Notified deacon: CONVOY_NEEDS_FEEDING %s\n", mr.ConvoyID)
	}

	// Emit event to wake deacon from await-signal (router.Send doesn't write
	// to .events.jsonl, but await-signal watches the events file).
	_ = events.LogFeed(events.TypeMail, e.rig.Name+"/refinery", events.MailPayload("deacon/", "CONVOY_NEEDS_FEEDING "+mr.ConvoyID))
}

// convoyInfo holds minimal info about a closed convoy for post-merge processing.
type convoyInfo struct {
	ID          string
	Title       string
	Description string
}

// checkAndCloseCompletedConvoys finds and closes convoys where all tracked issues
// are complete. Returns the list of convoys that were closed.
func (e *Engineer) checkAndCloseCompletedConvoys(townRoot, townBeads string) []convoyInfo {
	// List all open convoys
	listCmd := exec.Command("bd", "list", "--type=convoy", "--status=open", "--json")
	listCmd.Dir = townBeads
	var stdout bytes.Buffer
	listCmd.Stdout = &stdout

	if err := listCmd.Run(); err != nil {
		_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to list convoys: %v\n", err)
		return nil
	}

	var convoys []struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Status      string `json:"status"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &convoys); err != nil {
		_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to parse convoy list: %v\n", err)
		return nil
	}

	var closed []convoyInfo

	for _, convoy := range convoys {
		// Get tracked issues for this convoy via bd dep list
		depCmd := exec.Command("bd", "dep", "list", convoy.ID, "--direction=down", "--type=tracks", "--json")
		depCmd.Dir = townRoot
		var depOut bytes.Buffer
		depCmd.Stdout = &depOut

		if err := depCmd.Run(); err != nil {
			continue
		}

		var deps []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(depOut.Bytes(), &deps); err != nil {
			continue
		}

		// Refresh statuses from home rigs (cross-rig lookup)
		allClosed := true
		for _, dep := range deps {
			// Unwrap external:prefix:id format
			depID := dep.ID
			if strings.HasPrefix(depID, "external:") {
				parts := strings.SplitN(depID, ":", 3)
				if len(parts) == 3 {
					depID = parts[2]
				}
			}

			// Get fresh status from home rig via bd show with routing
			showCmd := exec.Command("bd", "show", depID, "--json")
			showCmd.Dir = townRoot
			var showOut bytes.Buffer
			showCmd.Stdout = &showOut

			if err := showCmd.Run(); err != nil || showOut.Len() == 0 {
				// Can't verify - treat as open to be safe
				allClosed = false
				break
			}

			var issues []struct {
				Status string `json:"status"`
			}
			if err := json.Unmarshal(showOut.Bytes(), &issues); err != nil || len(issues) == 0 {
				allClosed = false
				break
			}

			if issues[0].Status != "closed" && issues[0].Status != "tombstone" {
				allClosed = false
				break
			}
		}

		if !allClosed {
			continue
		}

		// All tracked issues are complete - close the convoy
		reason := "All tracked issues completed"
		if len(deps) == 0 {
			reason = "Empty convoy — auto-closed as definitionally complete"
		}

		closeCmd := exec.Command("bd", "close", convoy.ID, "-r", reason)
		closeCmd.Dir = townBeads

		if err := closeCmd.Run(); err != nil {
			_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to close convoy %s: %v\n", convoy.ID, err)
			continue
		}

		_, _ = fmt.Fprintf(e.output, "[Engineer] Auto-closed convoy %s: %s\n", convoy.ID, convoy.Title)
		closed = append(closed, convoyInfo{
			ID:          convoy.ID,
			Title:       convoy.Title,
			Description: convoy.Description,
		})

		// Send convoy completion notifications (owner + notify addresses)
		e.notifyConvoyCompletion(townRoot, convoy.ID, convoy.Title, convoy.Description)
	}

	return closed
}

// notifyConvoyCompletion sends notifications to convoy owner and notify addresses.
func (e *Engineer) notifyConvoyCompletion(townRoot, convoyID, title, description string) {
	notified := make(map[string]bool)

	for _, line := range strings.Split(description, "\n") {
		var addr string
		if strings.HasPrefix(line, "Owner: ") {
			addr = strings.TrimPrefix(line, "Owner: ")
		} else if strings.HasPrefix(line, "Notify: ") {
			addr = strings.TrimPrefix(line, "Notify: ")
		}

		if addr != "" && !notified[addr] {
			mailCmd := exec.Command("gt", "mail", "send", addr,
				"-s", fmt.Sprintf("🚚 Convoy landed: %s", title),
				"-m", fmt.Sprintf("Convoy %s has completed.\n\nAll tracked issues are now closed.\n\nClosed by: %s/refinery", convoyID, e.rig.Name))
			mailCmd.Dir = townRoot
			if err := mailCmd.Run(); err != nil {
				_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: could not notify %s: %v\n", addr, err)
			}
			notified[addr] = true
		}
	}
}

// landConvoySwarm checks if a completed convoy has an associated swarm with an
// integration branch, and triggers landing if so.
func (e *Engineer) landConvoySwarm(townRoot string, convoy convoyInfo) {
	// Check if convoy description mentions a molecule/swarm
	var moleculeID string
	for _, line := range strings.Split(convoy.Description, "\n") {
		if strings.HasPrefix(line, "Molecule: ") {
			moleculeID = strings.TrimPrefix(line, "Molecule: ")
			break
		}
	}

	if moleculeID == "" {
		return // No swarm/molecule associated with this convoy
	}

	// Check if the molecule has an integration branch (swarm/* pattern)
	integrationBranch := fmt.Sprintf("swarm/%s", moleculeID)
	branchExists, err := e.git.BranchExists(integrationBranch)
	if err != nil || !branchExists {
		// Also check remote
		remoteExists, _ := e.git.RemoteTrackingBranchExists("origin", integrationBranch)
		if !remoteExists {
			return // No integration branch to land
		}
	}

	_, _ = fmt.Fprintf(e.output, "[Engineer] Landing integration branch %s for convoy %s...\n", integrationBranch, convoy.ID)

	// Use gt swarm land to perform the landing
	landCmd := exec.Command("gt", "swarm", "land", moleculeID)
	landCmd.Dir = townRoot
	var landOut, landErr bytes.Buffer
	landCmd.Stdout = &landOut
	landCmd.Stderr = &landErr

	if err := landCmd.Run(); err != nil {
		_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to land swarm %s: %v (%s)\n",
			moleculeID, err, strings.TrimSpace(landErr.String()))
		return
	}

	_, _ = fmt.Fprintf(e.output, "[Engineer] ✓ Landed integration branch for convoy %s\n", convoy.ID)
}

// pruneStaleRemoteRefs prunes remote tracking refs that no longer exist on origin.
// This cleans up refs from branches that were deleted on the remote after merge.
func (e *Engineer) pruneStaleRemoteRefs() {
	if err := e.git.FetchPrune("origin"); err != nil {
		_, _ = fmt.Fprintf(e.output, "[Engineer] Warning: failed to prune stale remote refs: %v\n", err)
	}
}
