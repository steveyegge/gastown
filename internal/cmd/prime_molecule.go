package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/cli"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/deacon"
	"github.com/steveyegge/gastown/internal/formula"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
)

// MoleculeCurrentOutput represents the JSON output of bd mol current.
type MoleculeCurrentOutput struct {
	MoleculeID    string `json:"molecule_id"`
	MoleculeTitle string `json:"molecule_title"`
	NextStep      *struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
	} `json:"next_step"`
	Completed int `json:"completed"`
	Total     int `json:"total"`
}

// showMoleculeExecutionPrompt calls bd mol current and shows the current step
// with execution instructions. This is the core of the Propulsion Principle.
func showMoleculeExecutionPrompt(workDir, moleculeID string) {
	// Call bd mol current with JSON output
	cmd := exec.Command("bd", "mol", "current", moleculeID, "--json")
	cmd.Dir = workDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Fall back to simple message if bd mol current fails
		fmt.Println(style.Bold.Render("→ PROPULSION PRINCIPLE: Work is on your hook. RUN IT."))
		fmt.Println("  Begin working on this molecule immediately.")
		fmt.Printf("  Check status with: bd mol current %s\n", moleculeID)
		return
	}
	// Handle bd exit 0 bug: empty stdout means not found
	if stdout.Len() == 0 {
		fmt.Println(style.Bold.Render("→ PROPULSION PRINCIPLE: Work is on your hook. RUN IT."))
		fmt.Println("  Begin working on this molecule immediately.")
		return
	}

	// Parse JSON output - it's an array with one element
	var outputs []MoleculeCurrentOutput
	if err := json.Unmarshal(stdout.Bytes(), &outputs); err != nil || len(outputs) == 0 {
		// Fall back to simple message
		fmt.Println(style.Bold.Render("→ PROPULSION PRINCIPLE: Work is on your hook. RUN IT."))
		fmt.Println("  Begin working on this molecule immediately.")
		return
	}
	output := outputs[0]

	// Show molecule progress
	fmt.Printf("**Progress:** %d/%d steps complete\n\n",
		output.Completed, output.Total)

	// Show current step if available
	if output.NextStep != nil {
		step := output.NextStep
		fmt.Printf("%s\n\n", style.Bold.Render("## 🎬 CURRENT STEP: "+step.Title))
		fmt.Printf("**Step ID:** %s\n", step.ID)
		fmt.Printf("**Status:** %s (ready to execute)\n\n", step.Status)

		// Show step description if available
		if step.Description != "" {
			fmt.Println("### Instructions")
			fmt.Println()
			// Indent the description for readability
			lines := strings.Split(step.Description, "\n")
			for _, line := range lines {
				fmt.Printf("%s\n", line)
			}
			fmt.Println()
		}

		// The propulsion directive
		fmt.Println(style.Bold.Render("→ EXECUTE THIS STEP NOW."))
		fmt.Println()
		fmt.Println("When complete:")
		fmt.Printf("  1. Close the step: bd close %s\n", step.ID)
		fmt.Printf("  2. Check for next step: bd mol current %s\n", moleculeID)
		fmt.Println("  3. Continue until molecule complete")
	} else {
		// No next step - molecule may be complete
		fmt.Println(style.Bold.Render("✓ MOLECULE COMPLETE"))
		fmt.Println()
		fmt.Println("All steps are done. You may:")
		fmt.Println("  - Report completion to supervisor")
		fmt.Println("  - Check for new work: bd mol current")
	}
}

// showFormulaSteps renders the formula steps inline in the prime output.
// Agents read these steps instead of materializing them as wisp rows.
// The label parameter customizes the section header (e.g., "Patrol Steps", "Work Steps").
func showFormulaSteps(formulaName, label string) {
	content, err := formula.GetEmbeddedFormulaContent(formulaName)
	if err != nil {
		style.PrintWarning("could not load formula %s: %v", formulaName, err)
		return
	}

	f, err := formula.Parse(content)
	if err != nil {
		style.PrintWarning("could not parse formula %s: %v", formulaName, err)
		return
	}

	if len(f.Steps) == 0 {
		return
	}

	fmt.Println()
	fmt.Printf("**%s** (%d steps from %s):\n", label, len(f.Steps), formulaName)
	for i, step := range f.Steps {
		fmt.Printf("  %d. **%s** — %s\n", i+1, step.Title, truncateDescription(step.Description, 120))
	}
	fmt.Println()
}

// showFormulaStepsFull renders formula steps with full descriptions.
// Used for polecat work formulas where step details are the primary instructions.
func showFormulaStepsFull(formulaName string) {
	content, err := formula.GetEmbeddedFormulaContent(formulaName)
	if err != nil {
		style.PrintWarning("could not load formula %s: %v", formulaName, err)
		return
	}

	f, err := formula.Parse(content)
	if err != nil {
		style.PrintWarning("could not parse formula %s: %v", formulaName, err)
		return
	}

	if len(f.Steps) == 0 {
		return
	}

	fmt.Println()
	fmt.Printf("**Formula Checklist** (%d steps from %s):\n\n", len(f.Steps), formulaName)
	for i, step := range f.Steps {
		fmt.Printf("### Step %d: %s\n\n", i+1, step.Title)
		if step.Description != "" {
			fmt.Println(step.Description)
			fmt.Println()
		}
	}
}

// truncateDescription truncates a multi-line description to a single line summary.
func truncateDescription(desc string, maxLen int) string {
	// Take just the first line
	if idx := strings.IndexByte(desc, '\n'); idx >= 0 {
		desc = desc[:idx]
	}
	desc = strings.TrimSpace(desc)
	if len(desc) > maxLen {
		desc = desc[:maxLen-3] + "..."
	}
	if desc == "" {
		desc = "(no description)"
	}
	return desc
}

// outputMoleculeContext checks if the agent is working on a molecule step and shows progress.
func outputMoleculeContext(ctx RoleContext) {
	// Applies to polecats, crew workers, deacon, witness, and refinery
	if ctx.Role != RolePolecat && ctx.Role != RoleCrew && ctx.Role != RoleDeacon && ctx.Role != RoleWitness && ctx.Role != RoleRefinery {
		return
	}

	// For Deacon, use special patrol molecule handling
	if ctx.Role == RoleDeacon {
		outputDeaconPatrolContext(ctx)
		return
	}

	// For Witness, use special patrol molecule handling (auto-bonds on startup)
	if ctx.Role == RoleWitness {
		outputWitnessPatrolContext(ctx)
		return
	}

	// For Refinery, use special patrol molecule handling (auto-bonds on startup)
	if ctx.Role == RoleRefinery {
		outputRefineryPatrolContext(ctx)
		return
	}

	// For polecats with root-only wisps, formula steps are shown inline
	// in outputMoleculeWorkflow() via the attached_formula field.
	// No child-based tracking needed.
}


// outputDeaconPatrolContext shows patrol molecule status for the Deacon.
// Deacon uses wisps (Wisp:true issues in main .beads/) for patrol cycles.
// Deacon is a town-level role, so it uses town root beads (not rig beads).
func outputDeaconPatrolContext(ctx RoleContext) {
	// Check if Deacon is paused - if so, output PAUSED message and skip patrol context
	paused, state, err := deacon.IsPaused(ctx.TownRoot)
	if err == nil && paused {
		outputDeaconPausedMessage(state)
		return
	}

	cfg := PatrolConfig{
		RoleName:        "deacon",
		PatrolMolName:   "mol-deacon-patrol",
		BeadsDir:        ctx.TownRoot, // Town-level role uses town root beads
		Assignee:        "deacon",
		HeaderEmoji:     "🔄",
		HeaderTitle:     "Patrol Status (Wisp-based)",
		WorkLoopSteps: []string{
			"Work through each patrol step in sequence (see checklist below)",
			"At cycle end:\n   - If context LOW:\n     * Report and loop: `" + cli.Name() + " patrol report --summary \"<brief summary of observations>\"`\n     * This closes the current patrol and starts a new cycle\n   - If context HIGH:\n     * Send handoff: `" + cli.Name() + " handoff -s \"Deacon patrol\" -m \"<observations>\"`\n     * Exit cleanly (daemon respawns fresh session)",
		},
	}
	outputPatrolContext(cfg)
	showFormulaSteps("mol-deacon-patrol", "Patrol Steps")
}

// outputWitnessPatrolContext shows patrol molecule status for the Witness.
// Witness AUTO-BONDS its patrol molecule on startup if one isn't already running.
func outputWitnessPatrolContext(ctx RoleContext) {
	cfg := PatrolConfig{
		RoleName:        "witness",
		PatrolMolName:   "mol-witness-patrol",
		BeadsDir:        ctx.WorkDir,
		Assignee:        ctx.Rig + "/witness",
		HeaderEmoji:     constants.EmojiWitness,
		HeaderTitle:     "Witness Patrol Status",
		WorkLoopSteps: []string{
			"Work through each patrol step in sequence (see checklist below)",
			"At cycle end:\n   - If context LOW:\n     * Report and loop: `" + cli.Name() + " patrol report --summary \"<brief summary of observations>\"`\n     * This closes the current patrol and starts a new cycle\n   - If context HIGH:\n     * Send handoff: `" + cli.Name() + " handoff -s \"Witness patrol\" -m \"<observations>\"`\n     * Exit cleanly (daemon respawns fresh session)",
		},
	}
	outputPatrolContext(cfg)
	showFormulaSteps("mol-witness-patrol", "Patrol Steps")
}

// outputRefineryPatrolContext shows patrol molecule status for the Refinery.
// Refinery AUTO-BONDS its patrol molecule on startup if one isn't already running.
func outputRefineryPatrolContext(ctx RoleContext) {
	cfg := PatrolConfig{
		RoleName:        "refinery",
		PatrolMolName:   "mol-refinery-patrol",
		BeadsDir:        ctx.WorkDir,
		Assignee:        ctx.Rig + "/refinery",
		HeaderEmoji:     "🔧",
		HeaderTitle:     "Refinery Patrol Status",
		ExtraVars:       buildRefineryPatrolVars(ctx),
		WorkLoopSteps: []string{
			"Work through each patrol step in sequence (see checklist below)",
			"At cycle end:\n   - If context LOW:\n     * Report and loop: `" + cli.Name() + " patrol report --summary \"<brief summary of observations>\"`\n     * This closes the current patrol and starts a new cycle\n   - If context HIGH:\n     * Send handoff: `" + cli.Name() + " handoff -s \"Refinery patrol\" -m \"<observations>\"`\n     * Exit cleanly (daemon respawns fresh session)",
		},
	}
	outputPatrolContext(cfg)
	showFormulaSteps("mol-refinery-patrol", "Patrol Steps")
}

// buildRefineryPatrolVars loads rig MQ settings and returns --var key=value
// strings for the refinery patrol formula.
func buildRefineryPatrolVars(ctx RoleContext) []string {
	var vars []string
	if ctx.TownRoot == "" || ctx.Rig == "" {
		return vars
	}
	rigPath := filepath.Join(ctx.TownRoot, ctx.Rig)

	// Always inject target_branch from rig config — this is independent of
	// merge queue settings and must not be gated behind MQ existence.
	// Without this, rigs with no settings/config.json or no merge_queue
	// section get the formula default ("main") instead of their configured
	// default_branch.
	defaultBranch := "main"
	rigCfg, err := rig.LoadRigConfig(rigPath)
	if err == nil && rigCfg.DefaultBranch != "" {
		defaultBranch = rigCfg.DefaultBranch
	}
	vars = append(vars, fmt.Sprintf("target_branch=%s", defaultBranch))

	// MQ-specific vars require settings/config.json with a merge_queue section
	settingsPath := filepath.Join(rigPath, "settings", "config.json")
	settings, sErr := config.LoadRigSettings(settingsPath)
	if sErr != nil || settings == nil || settings.MergeQueue == nil {
		return vars
	}
	mq := settings.MergeQueue

	vars = append(vars, fmt.Sprintf("integration_branch_refinery_enabled=%t", mq.IsRefineryIntegrationEnabled()))
	vars = append(vars, fmt.Sprintf("integration_branch_auto_land=%t", mq.IsIntegrationBranchAutoLandEnabled()))
	vars = append(vars, fmt.Sprintf("run_tests=%t", mq.IsRunTestsEnabled()))
	if mq.SetupCommand != "" {
		vars = append(vars, fmt.Sprintf("setup_command=%s", mq.SetupCommand))
	}
	if mq.TypecheckCommand != "" {
		vars = append(vars, fmt.Sprintf("typecheck_command=%s", mq.TypecheckCommand))
	}
	if mq.LintCommand != "" {
		vars = append(vars, fmt.Sprintf("lint_command=%s", mq.LintCommand))
	}
	if mq.TestCommand != "" {
		vars = append(vars, fmt.Sprintf("test_command=%s", mq.TestCommand))
	}
	if mq.BuildCommand != "" {
		vars = append(vars, fmt.Sprintf("build_command=%s", mq.BuildCommand))
	}
	vars = append(vars, fmt.Sprintf("delete_merged_branches=%t", mq.IsDeleteMergedBranchesEnabled()))
	return vars
}
