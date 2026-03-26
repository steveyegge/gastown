package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/formula"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// moleculeStepFailCmd is the "gt mol step fail" command.
var moleculeStepFailCmd = &cobra.Command{
	Use:   "fail <step-id>",
	Short: "Mark step as failed and run formula rollback steps",
	Long: `Mark a molecule step as failed and execute rollback steps in reverse order.

This command handles step failure for formulas that define rollback_steps:

1. Closes the failed step with a failure reason (bd close --reason)
2. Looks up the formula via the molecule's attached_formula field
3. If the formula defines rollback_steps, executes them in reverse order
   - Runs the optional 'run' shell command for each rollback step
   - Logs a wide event (rollback_step) for each step executed
4. Logs rollback_started and rollback_done events around the sequence
5. Closes the molecule root as failed

Rollback steps are defined in the formula TOML as:

  [[rollback_steps]]
  id = "undo-deploy"
  title = "Undo deployment"
  description = "Roll back the deployed artifacts"
  run = "kubectl rollout undo deployment/myapp"

Rollback steps are executed in REVERSE definition order (last → first).
This ensures later steps undo earlier steps correctly.

Example:
  gt mol step fail hq-abc.3 --reason "build failed"`,
	Args: cobra.ExactArgs(1),
	RunE: runMoleculeStepFail,
}

var (
	moleculeStepFailReason string
	moleculeStepFailDryRun bool
)

func init() {
	moleculeStepFailCmd.Flags().StringVar(&moleculeStepFailReason, "reason", "", "Reason for the step failure")
	moleculeStepFailCmd.Flags().BoolVarP(&moleculeStepFailDryRun, "dry-run", "n", false, "Show what would be done without executing")
}

func runMoleculeStepFail(cmd *cobra.Command, args []string) error {
	stepID := args[0]

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

	// Verify the step exists
	step, err := b.Show(stepID)
	if err != nil {
		return fmt.Errorf("step not found: %w", err)
	}

	// Extract molecule ID
	moleculeID := extractMoleculeIDFromStep(stepID)
	if moleculeID == "" {
		if step.Parent != "" {
			moleculeID = step.Parent
		} else {
			return fmt.Errorf("cannot extract molecule ID from step %s", stepID)
		}
	}

	reason := moleculeStepFailReason
	if reason == "" {
		reason = "step failed"
	}

	actor := detectActor()

	// Log step_failed event
	_ = events.LogFeed(events.TypeStepFailed, actor, map[string]interface{}{
		"step_id":     stepID,
		"step_title":  step.Title,
		"molecule_id": moleculeID,
		"reason":      reason,
	})

	// Step 1: Close the failed step with failure reason
	if moleculeStepFailDryRun {
		fmt.Printf("[dry-run] Would close step %s with reason: %s\n", stepID, reason)
	} else {
		closeArgs := []string{"close", stepID, "--reason", reason}
		if err := BdCmd(closeArgs...).Dir(workDir).Run(); err != nil {
			return fmt.Errorf("closing failed step: %w", err)
		}
		fmt.Printf("%s Closed failed step %s: %s\n", style.Bold.Render("✗"), stepID, step.Title)
	}

	// Step 2: Look up the formula from the molecule root
	rollbackSteps, formulaName, err := loadRollbackSteps(b, moleculeID, workDir, townRoot)
	if err != nil {
		// Non-fatal: rollback requires a formula with rollback_steps defined
		fmt.Printf("%s Could not load rollback steps: %v\n", style.Dim.Render("ℹ"), err)
	}

	// Step 3: Execute rollback steps in reverse order
	if len(rollbackSteps) > 0 {
		fmt.Printf("\n%s Running %d rollback step(s) for formula %s...\n",
			style.Warning.Render("↩"), len(rollbackSteps), formulaName)

		_ = events.LogFeed(events.TypeRollbackStarted, actor, map[string]interface{}{
			"molecule_id":   moleculeID,
			"formula":       formulaName,
			"failed_step":   stepID,
			"rollback_count": len(rollbackSteps),
		})

		errs := executeRollbackSteps(rollbackSteps, moleculeID, formulaName, stepID, actor, moleculeStepFailDryRun)

		_ = events.LogFeed(events.TypeRollbackDone, actor, map[string]interface{}{
			"molecule_id": moleculeID,
			"formula":     formulaName,
			"failed_step": stepID,
			"steps_run":   len(rollbackSteps),
			"errors":      len(errs),
		})

		if len(errs) > 0 {
			fmt.Printf("%s %d rollback step(s) had errors:\n", style.Warning.Render("⚠"), len(errs))
			for _, e := range errs {
				fmt.Printf("  %s\n", e)
			}
		} else {
			fmt.Printf("%s All rollback steps completed\n", style.Bold.Render("✓"))
		}
	} else {
		fmt.Printf("%s No rollback steps defined for this formula\n", style.Dim.Render("ℹ"))
	}

	// Step 4: Close the molecule root as failed
	if moleculeStepFailDryRun {
		fmt.Printf("[dry-run] Would close molecule root %s as failed\n", moleculeID)
	} else {
		closeRootArgs := []string{"close", moleculeID, "--reason", fmt.Sprintf("step %s failed: %s", stepID, reason)}
		if err := BdCmd(closeRootArgs...).Dir(workDir).Run(); err != nil {
			// Non-fatal: molecule root may already be closed or not exist
			style.PrintWarning("could not close molecule root %s: %v", moleculeID, err)
		} else {
			fmt.Printf("%s Molecule %s closed as failed\n", style.Bold.Render("✗"), moleculeID)
		}
	}

	return nil
}

// loadRollbackSteps retrieves the formula's rollback_steps by:
// 1. Loading the molecule root bead to get the formula name
// 2. Finding and parsing the formula file
// Returns the rollback steps (may be empty) and the formula name.
func loadRollbackSteps(b *beads.Beads, moleculeID, workDir, townRoot string) ([]formula.RollbackStep, string, error) {
	// Load the molecule root bead to find the formula name
	mol, err := b.Show(moleculeID)
	if err != nil {
		return nil, "", fmt.Errorf("loading molecule %s: %w", moleculeID, err)
	}

	attachment := beads.ParseAttachmentFields(mol)
	var formulaName string
	if attachment != nil {
		formulaName = attachment.AttachedFormula
	}

	// Also check the title — some wisps embed the formula name as their title
	if formulaName == "" && mol.Title != "" {
		// Wisp title is often set to the formula name when poured via bd mol wisp
		formulaName = mol.Title
	}

	if formulaName == "" {
		return nil, "", fmt.Errorf("molecule %s has no attached_formula", moleculeID)
	}

	// Find and parse the formula file
	formulaPath, err := findFormulaFile(formulaName)
	if err != nil {
		return nil, formulaName, fmt.Errorf("finding formula %q: %w", formulaName, err)
	}

	f, err := formula.ParseFile(formulaPath)
	if err != nil {
		return nil, formulaName, fmt.Errorf("parsing formula %q: %w", formulaName, err)
	}

	return f.RollbackSteps, formulaName, nil
}

// executeRollbackSteps runs the rollback steps in reverse order (last defined → first defined).
// A wide event (TypeRollbackStep) is logged for each step.
// Returns a slice of error strings for any steps that failed (non-fatal: all steps are attempted).
func executeRollbackSteps(steps []formula.RollbackStep, moleculeID, formulaName, failedStepID, actor string, dryRun bool) []string {
	var errs []string

	// Execute in reverse order
	for i := len(steps) - 1; i >= 0; i-- {
		rs := steps[i]
		reverseIdx := len(steps) - 1 - i // 0-indexed position in execution order

		fmt.Printf("\n  %s [%d/%d] %s\n",
			style.Dim.Render("↩"), reverseIdx+1, len(steps), rs.Title)
		if rs.Description != "" {
			// Print first line of description as a hint
			firstLine := strings.SplitN(rs.Description, "\n", 2)[0]
			fmt.Printf("       %s\n", style.Dim.Render(firstLine))
		}

		var runErr error
		var exitCode int

		if rs.Run != "" {
			if dryRun {
				fmt.Printf("       [dry-run] Would run: %s\n", rs.Run)
			} else {
				runErr = rollbackExecFn(rs.ID, rs.Run)
				if runErr != nil {
					errs = append(errs, fmt.Sprintf("rollback step %q: %v", rs.ID, runErr))
					exitCode = 1
					fmt.Printf("       %s run failed: %v\n", style.Warning.Render("⚠"), runErr)
				} else {
					fmt.Printf("       %s run completed\n", style.Dim.Render("✓"))
				}
			}
		}

		// Wide event per rollback step — always log, even on dry-run
		_ = events.LogFeed(events.TypeRollbackStep, actor, map[string]interface{}{
			"molecule_id":    moleculeID,
			"formula":        formulaName,
			"failed_step":    failedStepID,
			"rollback_step":  rs.ID,
			"rollback_title": rs.Title,
			"reverse_index":  reverseIdx,
			"total_steps":    len(steps),
			"run_cmd":        rs.Run,
			"exit_code":      exitCode,
			"dry_run":        dryRun,
		})
	}

	return errs
}

// rollbackExecFn is the seam used by tests to intercept rollback command execution.
// Production code uses executeRollbackCommandReal; tests replace this with a stub.
var rollbackExecFn = func(id, command string) error {
	return executeRollbackCommandReal(command)
}

// executeRollbackCommandReal runs a shell command string for a rollback step.
// Uses /bin/sh -c to allow shell features (pipes, redirection, etc.).
func executeRollbackCommandReal(command string) error {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	cmd := exec.Command("/bin/sh", "-c", command) //nolint:gosec // G204: command from trusted formula file
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
