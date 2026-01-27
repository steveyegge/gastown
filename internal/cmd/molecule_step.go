package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// moleculeStepDoneCmd is the "gt mol step done" command.
var moleculeStepDoneCmd = &cobra.Command{
	Use:   "done <step-id>",
	Short: "Complete step and auto-continue to next",
	Long: `Complete a molecule step and automatically continue to the next ready step.

This command handles the step-to-step transition for polecats:

1. Closes the completed step (bd close <step-id>)
2. Extracts the molecule ID from the step
3. Finds the next ready step (dependency-aware)
4. If next step exists:
   - Updates the hook to point to the next step
   - Respawns the pane for a fresh session
5. If molecule complete:
   - Clears the hook
   - Sends POLECAT_DONE to witness
   - Exits the session

IMPORTANT: This is the canonical way to complete molecule steps. Do NOT manually
close steps with 'bd close' - it skips the auto-continuation logic.

Example:
  gt mol step done gt-abc.1    # Complete step 1 of molecule gt-abc`,
	Args: cobra.ExactArgs(1),
	RunE: runMoleculeStepDone,
}

var (
	moleculeStepDryRun bool
)

func init() {
	moleculeStepDoneCmd.Flags().BoolVarP(&moleculeStepDryRun, "dry-run", "n", false, "Show what would be done without executing")
	moleculeStepDoneCmd.Flags().BoolVar(&moleculeJSON, "json", false, "Output as JSON")
}

// StepDoneResult is the result of a step done operation.
type StepDoneResult struct {
	StepID       string `json:"step_id"`
	MoleculeID   string `json:"molecule_id"`
	StepClosed   bool   `json:"step_closed"`
	NextStepID   string `json:"next_step_id,omitempty"`
	NextStepTitle string `json:"next_step_title,omitempty"`
	Complete     bool   `json:"complete"`
	Action       string `json:"action"` // "continue", "done", "no_more_ready"
}

func runMoleculeStepDone(cmd *cobra.Command, args []string) error {
	stepID := args[0]

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}

	// Find town root
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding workspace: %w", err)
	}
	if townRoot == "" {
		return fmt.Errorf("not in a Gas Town workspace")
	}

	// Find beads directory
	workDir, err := findLocalBeadsDir()
	if err != nil {
		return fmt.Errorf("not in a beads workspace: %w", err)
	}

	b := beads.New(workDir)

	// Step 1: Verify the step exists
	step, err := b.Show(stepID)
	if err != nil {
		return fmt.Errorf("step not found: %w", err)
	}

	// Step 2: Extract molecule ID from step ID (gt-xxx.1 -> gt-xxx)
	moleculeID := extractMoleculeIDFromStep(stepID)
	if moleculeID == "" {
		return fmt.Errorf("cannot extract molecule ID from step %s (expected format: gt-xxx.N)", stepID)
	}

	result := StepDoneResult{
		StepID:     stepID,
		MoleculeID: moleculeID,
	}

	// Step 3: Close the step
	if moleculeStepDryRun {
		fmt.Printf("[dry-run] Would close step: %s\n", stepID)
		result.StepClosed = true
	} else {
		if err := b.Close(stepID); err != nil {
			return fmt.Errorf("closing step: %w", err)
		}
		result.StepClosed = true
		fmt.Printf("%s Closed step %s: %s\n", style.Bold.Render("✓"), stepID, step.Title)
	}

	// Step 4: Find the next ready step
	nextStep, allComplete, err := findNextReadyStep(b, moleculeID)
	if err != nil {
		return fmt.Errorf("finding next step: %w", err)
	}

	if allComplete {
		result.Complete = true
		result.Action = "done"
	} else if nextStep != nil {
		result.NextStepID = nextStep.ID
		result.NextStepTitle = nextStep.Title
		result.Action = "continue"
	} else {
		// There are more steps but none are ready (blocked on dependencies)
		result.Action = "no_more_ready"
	}

	// JSON output
	if moleculeJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	// Step 5: Handle next action
	switch result.Action {
	case "continue":
		return handleStepContinue(cwd, townRoot, workDir, nextStep, moleculeStepDryRun)

	case "done":
		return handleMoleculeComplete(cwd, townRoot, moleculeID, moleculeStepDryRun)

	case "no_more_ready":
		fmt.Printf("\n%s All remaining steps are blocked - waiting on dependencies\n",
			style.Dim.Render("ℹ"))
		fmt.Printf("Run 'gt mol progress %s' to see blocked steps\n", moleculeID)
		return nil
	}

	return nil
}

// extractMoleculeIDFromStep extracts the molecule ID from a step ID.
// Step IDs have format: mol-id.N where N is the step number.
// Examples:
//   gt-abc.1 -> gt-abc
//   gt-xyz.3 -> gt-xyz
//   bd-mol-abc.2 -> bd-mol-abc
func extractMoleculeIDFromStep(stepID string) string {
	// Find the last dot
	lastDot := strings.LastIndex(stepID, ".")
	if lastDot == -1 {
		return "" // No dot - not a step ID format
	}

	// Check if what's after the dot is a number (step suffix)
	suffix := stepID[lastDot+1:]
	if len(suffix) == 0 {
		return "" // Trailing dot - no suffix
	}
	for _, c := range suffix {
		if c < '0' || c > '9' {
			return "" // Not a numeric suffix
		}
	}

	return stepID[:lastDot]
}

// findNextReadyStep finds the next ready step in a molecule.
// Returns (nextStep, allComplete, error).
// If all steps are complete, returns (nil, true, nil).
// If no steps are ready but some are blocked/in_progress, returns (nil, false, nil).
func findNextReadyStep(b *beads.Beads, moleculeID string) (*beads.Issue, bool, error) {
	// Get all children of the molecule
	children, err := b.List(beads.ListOptions{
		Parent:   moleculeID,
		Status:   "all",
		Priority: -1,
	})
	if err != nil {
		return nil, false, fmt.Errorf("listing molecule steps: %w", err)
	}

	if len(children) == 0 {
		return nil, true, nil // No steps = complete
	}

	// Build set of closed step IDs and collect open step IDs
	// Note: "open" means not started. "in_progress" means someone's working on it.
	// We only consider "open" steps as candidates for the next step.
	closedIDs := make(map[string]bool)
	var openStepIDs []string
	hasNonClosedSteps := false

	for _, child := range children {
		switch child.Status {
		case "closed":
			closedIDs[child.ID] = true
		case "open":
			openStepIDs = append(openStepIDs, child.ID)
			hasNonClosedSteps = true
		default:
			// in_progress or other status - not closed, not available
			hasNonClosedSteps = true
		}
	}

	// Check if all complete
	if !hasNonClosedSteps {
		return nil, true, nil
	}

	// No open steps to check
	if len(openStepIDs) == 0 {
		return nil, false, nil
	}

	// Fetch full details for open steps to get dependency info.
	// bd list doesn't return dependencies, but bd show does.
	openStepsMap, err := b.ShowMultiple(openStepIDs)
	if err != nil {
		return nil, false, fmt.Errorf("fetching step details: %w", err)
	}

	// Find ready steps (open steps with all dependencies closed)
	for _, stepID := range openStepIDs {
		step, ok := openStepsMap[stepID]
		if !ok {
			continue
		}

		// Check dependencies using the Dependencies field (from bd show),
		// not DependsOn (which is empty from bd list).
		// Only "blocks" type dependencies block progress - ignore "parent-child".
		allDepsClosed := true
		hasBlockingDeps := false
		for _, dep := range step.Dependencies {
			if dep.DependencyType != "blocks" {
				continue // Skip parent-child and other non-blocking relationships
			}
			hasBlockingDeps = true
			if !closedIDs[dep.ID] {
				allDepsClosed = false
				break
			}
		}

		if !hasBlockingDeps || allDepsClosed {
			return step, false, nil
		}
	}

	// No ready steps (all blocked or in_progress)
	return nil, false, nil
}

// handleStepContinue handles continuing to the next step.
func handleStepContinue(cwd, townRoot, _ string, nextStep *beads.Issue, dryRun bool) error { // workDir unused but kept for signature consistency
	fmt.Printf("\n%s Next step: %s\n", style.Bold.Render("→"), nextStep.ID)
	fmt.Printf("  %s\n", nextStep.Title)

	// Detect agent identity
	roleInfo, err := GetRoleWithContext(cwd, townRoot)
	if err != nil {
		return fmt.Errorf("detecting role: %w", err)
	}

	roleCtx := RoleContext{
		Role:     roleInfo.Role,
		Rig:      roleInfo.Rig,
		Polecat:  roleInfo.Polecat,
		TownRoot: townRoot,
		WorkDir:  cwd,
	}
	agentID := buildAgentIdentity(roleCtx)
	if agentID == "" {
		return fmt.Errorf("cannot determine agent identity (role: %s)", roleCtx.Role)
	}

	// Get git root for hook files
	gitRoot, err := getGitRoot()
	if err != nil {
		return fmt.Errorf("finding git root: %w", err)
	}

	if dryRun {
		fmt.Printf("\n[dry-run] Would pin next step: %s\n", nextStep.ID)
		fmt.Printf("[dry-run] Would respawn pane\n")
		return nil
	}

	// Pin the next step bead
	pinCmd := exec.Command("bd", "--no-daemon", "update", nextStep.ID, "--status=pinned", "--assignee="+agentID)
	pinCmd.Dir = gitRoot
	pinCmd.Stderr = os.Stderr
	if err := pinCmd.Run(); err != nil {
		return fmt.Errorf("pinning next step: %w", err)
	}

	fmt.Printf("%s Next step pinned: %s\n", style.Bold.Render("📌"), nextStep.ID)

	// Respawn the pane
	if !tmux.IsInsideTmux() {
		// Not in tmux - just print next action
		fmt.Printf("\n%s Not in tmux - start new session with 'gt prime'\n",
			style.Dim.Render("ℹ"))
		return nil
	}

	pane := os.Getenv("TMUX_PANE")
	if pane == "" {
		return fmt.Errorf("TMUX_PANE not set")
	}

	// Get current session for restart command
	currentSession, err := getCurrentTmuxSession()
	if err != nil {
		return fmt.Errorf("getting session name: %w", err)
	}

	restartCmd, err := buildRestartCommand(currentSession)
	if err != nil {
		return fmt.Errorf("building restart command: %w", err)
	}

	fmt.Printf("\n%s Respawning for next step...\n", style.Bold.Render("🔄"))

	t := tmux.NewTmux()

	// Kill all processes in the pane before respawning to prevent process leaks
	if err := t.KillPaneProcesses(pane); err != nil {
		// Non-fatal but log the warning
		style.PrintWarning("could not kill pane processes: %v", err)
	}

	// Clear history before respawn
	if err := t.ClearHistory(pane); err != nil {
		// Non-fatal
		style.PrintWarning("could not clear history: %v", err)
	}

	return t.RespawnPane(pane, restartCmd)
}

// handleMoleculeComplete handles when a molecule is complete.
func handleMoleculeComplete(cwd, townRoot, moleculeID string, dryRun bool) error {
	fmt.Printf("\n%s Molecule complete!\n", style.Bold.Render("🎉"))

	// Detect agent identity
	roleInfo, err := GetRoleWithContext(cwd, townRoot)
	if err != nil {
		return fmt.Errorf("detecting role: %w", err)
	}

	roleCtx := RoleContext{
		Role:     roleInfo.Role,
		Rig:      roleInfo.Rig,
		Polecat:  roleInfo.Polecat,
		TownRoot: townRoot,
		WorkDir:  cwd,
	}
	agentID := buildAgentIdentity(roleCtx)

	// Get git root for hook files
	gitRoot, err := getGitRoot()
	if err != nil {
		return fmt.Errorf("finding git root: %w", err)
	}

	if dryRun {
		fmt.Printf("[dry-run] Would unpin work for %s\n", agentID)
		fmt.Printf("[dry-run] Would send POLECAT_DONE to witness\n")
		return nil
	}

	// Unpin the molecule bead (set status to open, will be closed by gt done or manually)
	workDir, err := findLocalBeadsDir()
	if err == nil {
		b := beads.New(workDir)
		pinnedBeads, err := b.List(beads.ListOptions{
			Status:   beads.StatusPinned,
			Assignee: agentID,
			Priority: -1,
		})
		if err == nil && len(pinnedBeads) > 0 {
			// Unpin by setting status to open
			unpinCmd := exec.Command("bd", "--no-daemon", "update", pinnedBeads[0].ID, "--status=open")
			unpinCmd.Dir = gitRoot
			unpinCmd.Stderr = os.Stderr
			if err := unpinCmd.Run(); err != nil {
				style.PrintWarning("could not unpin bead: %v", err)
			} else {
				fmt.Printf("%s Work unpinned\n", style.Bold.Render("✓"))
			}
		}
	}

	// For polecats, use gt done to signal completion
	if roleCtx.Role == RolePolecat {
		fmt.Printf("%s Signaling completion to witness...\n", style.Bold.Render("📤"))

		doneCmd := exec.Command("gt", "done", "--exit", "DEFERRED")
		doneCmd.Stdout = os.Stdout
		doneCmd.Stderr = os.Stderr
		return doneCmd.Run()
	}

	// For other roles, just print completion message
	fmt.Printf("\nMolecule %s is complete. Ready for next assignment.\n", moleculeID)
	return nil
}

// getGitRoot is defined in prime.go
