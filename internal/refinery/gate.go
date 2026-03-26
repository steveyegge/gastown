package refinery

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
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

// handleGatePass signals that gates passed for a convoy.
// Updates the convoy bead notes and nudges for draft→ready conversion.
func handleGatePass(hqBeads *beads.Beads, townRoot, convoyID, integrationBranch string, logger func(format string, args ...interface{}), gtPath string) error {
	logger("Gate: convoy %s: gates PASSED on %s, signaling for draft→ready conversion", convoyID, integrationBranch)

	// Update convoy bead description with gate pass status
	notes := fmt.Sprintf("gates_passed: true\ngates_branch: %s\ngates_passed_at: %s",
		integrationBranch, time.Now().UTC().Format(time.RFC3339))

	convoyIssue, err := hqBeads.Show(convoyID)
	if err == nil {
		newDesc := convoyIssue.Description + "\n" + notes
		if updateErr := hqBeads.Update(convoyID, beads.UpdateOptions{Description: &newDesc}); updateErr != nil {
			logger("Gate: convoy %s: failed to update bead: %v", convoyID, updateErr)
		}
	}

	// Nudge the refinery about gate pass so Phase 4.3 can trigger draft→ready conversion.
	nudgeMsg := fmt.Sprintf("GATE_PASSED: convoy=%s branch=%s", convoyID, integrationBranch)
	nudgeCmd := exec.Command(gtPath, "nudge", "refinery", nudgeMsg)
	nudgeCmd.Dir = townRoot
	util.SetProcessGroup(nudgeCmd)
	if err := nudgeCmd.Run(); err != nil {
		logger("Gate: convoy %s: failed to nudge refinery about gate pass: %v", convoyID, err)
	}

	return nil
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
