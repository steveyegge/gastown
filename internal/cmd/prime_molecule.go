package cmd

import (
	"github.com/steveyegge/gastown/internal/cli"
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/deacon"
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
	cmd := exec.Command("bd", "--no-daemon", "mol", "current", moleculeID, "--json")
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
	// Handle bd --no-daemon exit 0 bug: empty stdout means not found
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
		fmt.Println("  2. Check for next step: bd ready")
		fmt.Println("  3. Continue until molecule complete")
	} else {
		// No next step - molecule may be complete
		fmt.Println(style.Bold.Render("✓ MOLECULE COMPLETE"))
		fmt.Println()
		fmt.Println("All steps are done. You may:")
		fmt.Println("  - Report completion to supervisor")
		fmt.Println("  - Check for new work: bd ready")
	}
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

	// Check for in-progress issues
	b := beads.New(ctx.WorkDir)
	issues, err := b.List(beads.ListOptions{
		Status:   "in_progress",
		Assignee: ctx.Polecat,
		Priority: -1,
	})
	if err != nil || len(issues) == 0 {
		return
	}

	// Check if any in-progress issue is a molecule step
	for _, issue := range issues {
		moleculeID := parseMoleculeMetadata(issue.Description)
		if moleculeID == "" {
			continue
		}

		// Get the parent (root) issue ID
		rootID := issue.Parent
		if rootID == "" {
			continue
		}

		// This is a molecule step - show context
		fmt.Println()
		fmt.Printf("%s\n\n", style.Bold.Render("## 🧬 Molecule Workflow"))
		fmt.Printf("You are working on a molecule step.\n")
		fmt.Printf("  Current step: %s\n", issue.ID)
		fmt.Printf("  Molecule: %s\n", moleculeID)
		fmt.Printf("  Root issue: %s\n\n", rootID)

		// Show molecule progress by finding sibling steps
		showMoleculeProgress(b, rootID)

		fmt.Println()
		fmt.Println("**Molecule Work Loop:**")
		fmt.Println("1. Complete current step, then `bd close " + issue.ID + "`")
		fmt.Println("2. Check for next steps: `bd ready --parent " + rootID + "`")
		fmt.Println("3. Work on next ready step(s)")
		fmt.Println("4. When all steps done, run `" + cli.Name() + " done`")
		break // Only show context for first molecule step found
	}
}

// parseMoleculeMetadata extracts molecule info from a step's description.
// Looks for lines like:
//
//	instantiated_from: mol-xyz
func parseMoleculeMetadata(description string) string {
	lines := strings.Split(description, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "instantiated_from:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "instantiated_from:"))
		}
	}
	return ""
}

// showMoleculeProgress displays the progress through a molecule's steps.
func showMoleculeProgress(b *beads.Beads, rootID string) {
	if rootID == "" {
		return
	}

	// Find all children of the root issue
	children, err := b.List(beads.ListOptions{
		Parent:   rootID,
		Status:   "all",
		Priority: -1,
	})
	if err != nil || len(children) == 0 {
		return
	}

	total := len(children)
	done := 0
	inProgress := 0
	var readySteps []string

	for _, child := range children {
		switch child.Status {
		case "closed":
			done++
		case "in_progress":
			inProgress++
		case "open":
			// Check if ready (no open dependencies)
			if len(child.DependsOn) == 0 {
				readySteps = append(readySteps, child.ID)
			}
		}
	}

	fmt.Printf("Progress: %d/%d steps complete", done, total)
	if inProgress > 0 {
		fmt.Printf(" (%d in progress)", inProgress)
	}
	fmt.Println()

	if len(readySteps) > 0 {
		fmt.Printf("Ready steps: %s\n", strings.Join(readySteps, ", "))
	}
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
		CheckInProgress: false,
		WorkLoopSteps: []string{
			"Check next step: `bd ready`",
			"Execute the step (heartbeat, mail, health checks, etc.)",
			"Close step: `bd close <step-id>`",
			"Check next: `bd ready`",
			"At cycle end (loop-or-exit step):\n   - If context LOW:\n     * Squash: `bd mol squash <mol-id> --summary \"<summary>\"`\n     * Create new patrol: `bd mol wisp mol-deacon-patrol`\n     * Continue executing from inbox-check step\n   - If context HIGH:\n     * Send handoff: `" + cli.Name() + " handoff -s \"Deacon patrol\" -m \"<observations>\"`\n     * Exit cleanly (daemon respawns fresh session)",
		},
	}
	outputPatrolContext(cfg)
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
		CheckInProgress: true,
		WorkLoopSteps: []string{
			"Check inbox: `" + cli.Name() + " mail inbox`",
			"Check next step: `bd ready`",
			"Execute the step (survey polecats, inspect, nudge, etc.)",
			"Close step: `bd close <step-id>`",
			"Check next: `bd ready`",
			"At cycle end (loop-or-exit step):\n   - If context LOW:\n     * Squash: `bd mol squash <mol-id> --summary \"<summary>\"`\n     * Create new patrol: `bd mol wisp mol-witness-patrol`\n     * Continue executing from inbox-check step\n   - If context HIGH:\n     * Send handoff: `" + cli.Name() + " handoff -s \"Witness patrol\" -m \"<observations>\"`\n     * Exit cleanly (daemon respawns fresh session)",
		},
	}
	outputPatrolContext(cfg)
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
		CheckInProgress: true,
		WorkLoopSteps: []string{
			"Check inbox: `" + cli.Name() + " mail inbox`",
			"Check next step: `bd ready`",
			"Execute the step (queue scan, process branch, tests, merge)",
			"Close step: `bd close <step-id>`",
			"Check next: `bd ready`",
			"At cycle end (loop-or-exit step):\n   - If context LOW:\n     * Squash: `bd mol squash <mol-id> --summary \"<summary>\"`\n     * Create new patrol: `bd mol wisp mol-refinery-patrol`\n     * Continue executing from inbox-check step\n   - If context HIGH:\n     * Send handoff: `" + cli.Name() + " handoff -s \"Refinery patrol\" -m \"<observations>\"`\n     * Exit cleanly (daemon respawns fresh session)",
		},
	}
	outputPatrolContext(cfg)
}
