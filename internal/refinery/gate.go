package refinery

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	gh "github.com/steveyegge/gastown/internal/github"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/util"
)

// IntegrationGateResult holds the outcome of running gates on an integration branch.
type IntegrationGateResult struct {
	// Success is true when all gates passed.
	Success bool

	// ConvoyID is the convoy that triggered the gate run.
	ConvoyID string

	// IntegrationBranch is the branch that was checked out and tested.
	IntegrationBranch string

	// Error contains failure details (empty on success).
	Error string
}

// RunGatesOnBranch checks out the specified branch, pulls latest, and runs
// all configured pre-merge gates. This is used for integration branch gating
// in batch-pr convoys, where the gates validate accumulated work before
// the PR is marked ready for review.
//
// Unlike the normal merge pipeline (which gates individual MRs), this runs
// gates on a branch that already contains all accumulated polecat work.
func (e *Engineer) RunGatesOnBranch(ctx context.Context, branch string) ProcessResult {
	// Step 1: Fetch latest
	_, _ = fmt.Fprintf(e.output, "[Gate] Fetching latest for branch %s...\n", branch)
	if err := e.git.Fetch("origin"); err != nil {
		return ProcessResult{
			Success: false,
			Error:   fmt.Sprintf("failed to fetch origin: %v", err),
		}
	}

	// Step 2: Check that the branch exists (locally or remote)
	exists, err := e.git.BranchExists(branch)
	if err != nil {
		return ProcessResult{
			Success:        false,
			BranchNotFound: true,
			Error:          fmt.Sprintf("failed to check branch %s: %v", branch, err),
		}
	}
	if !exists {
		remoteExists, _ := e.git.RemoteTrackingBranchExists("origin", branch)
		if !remoteExists {
			return ProcessResult{
				Success:        false,
				BranchNotFound: true,
				Error:          fmt.Sprintf("integration branch %s not found locally or on origin", branch),
			}
		}
	}

	// Step 3: Checkout the branch
	_, _ = fmt.Fprintf(e.output, "[Gate] Checking out %s...\n", branch)
	if err := e.git.Checkout(branch); err != nil {
		return ProcessResult{
			Success: false,
			Error:   fmt.Sprintf("failed to checkout %s: %v", branch, err),
		}
	}

	// Step 4: Pull latest
	if err := e.git.Pull("origin", branch); err != nil {
		_, _ = fmt.Fprintf(e.output, "[Gate] Warning: pull from origin/%s: %v (continuing)\n", branch, err)
	}

	// Step 5: Run pre-merge gates
	if len(e.config.Gates) > 0 {
		_, _ = fmt.Fprintf(e.output, "[Gate] Running gate suite on %s...\n", branch)
		return e.runGatesForPhase(ctx, GatePhasePreMerge)
	}

	// Fallback: legacy test command
	if e.config.RunTests && e.config.TestCommand != "" {
		_, _ = fmt.Fprintf(e.output, "[Gate] Running legacy test command on %s: %s\n", branch, e.config.TestCommand)
		return e.runTests(ctx)
	}

	// No gates configured — pass by default
	_, _ = fmt.Fprintf(e.output, "[Gate] No gates configured, passing by default\n")
	return ProcessResult{Success: true}
}

// RunConvoyGates runs the configured gate suite on a convoy's integration branch.
// This is the entry point for batch-pr terminal completion gating.
//
// Parameters:
//   - ctx: context for cancellation
//   - rigPath: path to the rig directory
//   - convoyID: the terminal convoy's bead ID
//   - integrationBranch: the branch to gate (e.g., "convoy/hq-cv-abc")
//   - logger: optional logging function
//
// Returns the gate result with pass/fail and details.
func RunConvoyGates(ctx context.Context, rigPath, convoyID, integrationBranch string, logger func(format string, args ...interface{})) *IntegrationGateResult {
	if logger == nil {
		logger = func(format string, args ...interface{}) {}
	}

	result := &IntegrationGateResult{
		ConvoyID:          convoyID,
		IntegrationBranch: integrationBranch,
	}

	// Create an Engineer for this rig to reuse gate infrastructure.
	r := &rig.Rig{
		Name: filepath.Base(rigPath),
		Path: rigPath,
	}
	eng := NewEngineer(r)
	if err := eng.LoadConfig(); err != nil {
		result.Error = fmt.Sprintf("failed to load rig config: %v", err)
		logger("Gate: convoy %s: %s", convoyID, result.Error)
		return result
	}

	logger("Gate: convoy %s: running gates on integration branch %s", convoyID, integrationBranch)

	processResult := eng.RunGatesOnBranch(ctx, integrationBranch)
	result.Success = processResult.Success
	result.Error = processResult.Error

	if result.Success {
		logger("Gate: convoy %s: all gates passed on %s", convoyID, integrationBranch)
	} else {
		logger("Gate: convoy %s: gates failed on %s: %s", convoyID, integrationBranch, result.Error)
	}

	return result
}

// HandleTerminalConvoy processes a terminal batch-pr convoy by running gates
// on the integration branch and signaling the result. This is called by the
// daemon's convoy manager when all tracked issues in a batch-pr convoy close.
//
// On gate pass: updates convoy bead notes and nudges for draft→ready conversion.
// On gate fail: updates convoy bead notes and creates a failure bead with details.
//
// Parameters:
//   - ctx: context for cancellation
//   - townRoot: path to town root directory
//   - convoyID: the terminal convoy's bead ID
//   - logger: logging function
//   - gtPath: resolved path to the gt binary
func HandleTerminalConvoy(ctx context.Context, townRoot, convoyID string, logger func(format string, args ...interface{}), gtPath string) error {
	if logger == nil {
		logger = func(format string, args ...interface{}) {}
	}

	// Step 1: Get the integration branch from the convoy bead
	hqBeads := beads.New(filepath.Join(townRoot, ".beads"))
	convoyIssue, err := hqBeads.Show(convoyID)
	if err != nil {
		return fmt.Errorf("failed to read convoy %s: %w", convoyID, err)
	}
	integrationBranch := beads.GetIntegrationBranchField(convoyIssue.Description)
	if integrationBranch == "" {
		return fmt.Errorf("convoy %s has no integration_branch configured", convoyID)
	}

	// Step 2: Determine the rig for this convoy.
	// Look for a rig with a refinery worktree in town root.
	rigPath := findRigWithRefinery(townRoot)
	if rigPath == "" {
		return fmt.Errorf("cannot find rig with refinery for convoy %s", convoyID)
	}

	// Step 3: Run gates
	result := RunConvoyGates(ctx, rigPath, convoyID, integrationBranch, logger)

	// Step 4: Handle result
	if result.Success {
		return handleGatePass(hqBeads, townRoot, convoyID, integrationBranch, logger, gtPath)
	}
	return handleGateFailure(hqBeads, townRoot, convoyID, integrationBranch, result.Error, logger)
}

// findRigWithRefinery finds the first rig in townRoot that has a refinery worktree.
func findRigWithRefinery(townRoot string) string {
	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".beads" || entry.Name() == "docs" {
			continue
		}
		candidate := filepath.Join(townRoot, entry.Name())
		refineryRig := filepath.Join(candidate, "refinery", "rig")
		if _, err := os.Stat(refineryRig); err == nil {
			return candidate
		}
	}
	return ""
}

// handleGatePass processes a successful gate run for a batch-pr convoy.
// It converts the draft PR to ready-for-review, updates the convoy bead
// status to awaiting_review, and notifies the convoy owner.
func handleGatePass(hqBeads *beads.Beads, townRoot, convoyID, integrationBranch string, logger func(format string, args ...interface{}), gtPath string) error {
	logger("Gate: convoy %s: gates PASSED on %s, converting draft PR to ready-for-review", convoyID, integrationBranch)

	convoyIssue, err := hqBeads.Show(convoyID)
	if err != nil {
		return fmt.Errorf("failed to read convoy %s: %w", convoyID, err)
	}

	// Step 1: Convert draft PR to ready-for-review.
	if err := convertDraftPR(convoyIssue.Description, townRoot, convoyID, logger); err != nil {
		// Non-fatal: PR conversion failure shouldn't block status update.
		// The PR may already be non-draft or the token may lack permissions.
		logger("Gate: convoy %s: draft→ready conversion failed: %v (continuing)", convoyID, err)
	}

	// Step 2: Update convoy bead description with gate pass + awaiting_review status.
	notes := fmt.Sprintf("gates_passed: true\ngates_branch: %s\ngates_passed_at: %s\nconvoy_status: awaiting_review",
		integrationBranch, time.Now().UTC().Format(time.RFC3339))
	newDesc := convoyIssue.Description + "\n" + notes
	if updateErr := hqBeads.Update(convoyID, beads.UpdateOptions{Description: &newDesc}); updateErr != nil {
		logger("Gate: convoy %s: failed to update bead: %v", convoyID, updateErr)
	}

	// Step 3: Notify convoy owner that the PR is ready for review.
	convoyFields := beads.ParseConvoyFields(&beads.Issue{Description: convoyIssue.Description})
	prURL := beads.GetPRURLField(convoyIssue.Description)
	notifyConvoyOwner(convoyFields, prURL, townRoot, convoyID, logger, gtPath)

	return nil
}

// convertDraftPR converts a convoy's draft PR to ready-for-review using the
// GitHub GraphQL API. Reads the PR number from the convoy bead description
// and the owner/repo from the rig's git remote URL.
func convertDraftPR(convoyDesc, townRoot, convoyID string, logger func(format string, args ...interface{})) error {
	// Extract PR number from convoy bead description.
	prNumStr := beads.GetPRNumberField(convoyDesc)
	if prNumStr == "" {
		return fmt.Errorf("convoy %s: no pr_number field in description", convoyID)
	}
	prNumber, err := strconv.Atoi(prNumStr)
	if err != nil {
		return fmt.Errorf("convoy %s: invalid pr_number %q: %w", convoyID, prNumStr, err)
	}

	// Determine owner/repo from rig config.
	rigPath := findRigWithRefinery(townRoot)
	if rigPath == "" {
		return fmt.Errorf("convoy %s: cannot find rig with refinery", convoyID)
	}
	rigCfg, err := rig.LoadRigConfig(rigPath)
	if err != nil {
		return fmt.Errorf("convoy %s: load rig config: %w", convoyID, err)
	}
	owner, repo, err := parseGitRemoteURL(rigCfg.GitURL)
	if err != nil {
		return fmt.Errorf("convoy %s: parse git URL: %w", convoyID, err)
	}

	// Call GitHub API to convert draft to ready-for-review.
	client, err := gh.NewClient()
	if err != nil {
		return fmt.Errorf("convoy %s: create github client: %w", convoyID, err)
	}
	if err := client.ConvertDraftToReady(context.Background(), owner, repo, prNumber); err != nil {
		return fmt.Errorf("convoy %s: convert PR #%d to ready: %w", convoyID, prNumber, err)
	}

	logger("Gate: convoy %s: ✓ Draft PR #%d converted to ready-for-review", convoyID, prNumber)
	return nil
}

// sshRemoteRegex matches git@host:owner/repo.git style remote URLs.
var sshRemoteRegex = regexp.MustCompile(`^[\w.+-]+@([^:]+):([^/]+)/([^/]+?)(?:\.git)?$`)

// parseGitRemoteURL extracts the owner and repo name from a git remote URL.
// Supports both SSH (git@github.com:owner/repo.git) and HTTPS formats.
func parseGitRemoteURL(remoteURL string) (owner, repo string, err error) {
	remoteURL = strings.TrimSpace(remoteURL)

	// SSH: git@github.com:owner/repo.git
	if m := sshRemoteRegex.FindStringSubmatch(remoteURL); m != nil {
		return m[2], m[3], nil
	}

	// HTTPS: https://github.com/owner/repo.git
	u := strings.TrimSuffix(remoteURL, ".git")
	for _, prefix := range []string{"https://", "http://"} {
		if strings.HasPrefix(u, prefix) {
			u = u[len(prefix):]
			break
		}
	}

	// Expect: host/owner/repo
	parts := strings.SplitN(u, "/", 4)
	if len(parts) < 3 {
		return "", "", fmt.Errorf("cannot parse owner/repo from remote URL: %s", remoteURL)
	}
	return parts[1], parts[2], nil
}

// notifyConvoyOwner nudges the convoy owner and watchers that the PR is ready
// for review. Uses nudges (zero Dolt overhead) rather than mail.
func notifyConvoyOwner(fields *beads.ConvoyFields, prURL, townRoot, convoyID string, logger func(format string, args ...interface{}), gtPath string) {
	msg := fmt.Sprintf("PR_READY_FOR_REVIEW: convoy=%s", convoyID)
	if prURL != "" {
		msg += " pr=" + prURL
	}

	// Collect unique addresses to nudge.
	seen := make(map[string]bool)
	var addrs []string
	if fields != nil {
		for _, addr := range fields.NotificationAddresses() {
			if !seen[addr] {
				addrs = append(addrs, addr)
				seen[addr] = true
			}
		}
		for _, addr := range fields.NudgeNotificationAddresses() {
			if !seen[addr] {
				addrs = append(addrs, addr)
				seen[addr] = true
			}
		}
	}
	// Always include the overseer as fallback owner.
	if !seen["crew/overseer"] {
		addrs = append(addrs, "crew/overseer")
	}

	for _, addr := range addrs {
		nudgeCmd := exec.Command(gtPath, "nudge", addr, msg)
		nudgeCmd.Dir = townRoot
		util.SetProcessGroup(nudgeCmd)
		if err := nudgeCmd.Run(); err != nil {
			logger("Gate: convoy %s: failed to nudge %s: %v", convoyID, addr, err)
		}
	}
}

// handleGateFailure records gate failure details on the convoy bead and creates
// a failure bead for visibility.
func handleGateFailure(hqBeads *beads.Beads, townRoot, convoyID, integrationBranch, errMsg string, logger func(format string, args ...interface{})) error {
	logger("Gate: convoy %s: gates FAILED on %s: %s", convoyID, integrationBranch, errMsg)

	// Update convoy bead description with gate failure details
	notes := fmt.Sprintf("gates_passed: false\ngates_branch: %s\ngates_error: %s",
		integrationBranch, errMsg)

	convoyIssue, err := hqBeads.Show(convoyID)
	if err == nil {
		newDesc := convoyIssue.Description + "\n" + notes
		if updateErr := hqBeads.Update(convoyID, beads.UpdateOptions{Description: &newDesc}); updateErr != nil {
			logger("Gate: convoy %s: failed to update bead: %v", convoyID, updateErr)
		}
	}

	// Create a failure bead for visibility and tracking
	title := fmt.Sprintf("Gate failure on %s", integrationBranch)
	desc := fmt.Sprintf("convoy: %s\nbranch: %s\nerror: %s", convoyID, integrationBranch, errMsg)
	_, createErr := hqBeads.Create(beads.CreateOptions{
		Title:       title,
		Type:        "bug",
		Priority:    2,
		Description: desc,
	})
	if createErr != nil {
		logger("Gate: convoy %s: failed to create failure bead: %v", convoyID, createErr)
	}

	return fmt.Errorf("gates failed on %s: %s", integrationBranch, errMsg)
}
