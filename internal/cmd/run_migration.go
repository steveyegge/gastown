package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/formula"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/util"
	"github.com/steveyegge/gastown/internal/workspace"
)

// MigrationCheckpoint tracks progress through migration steps.
type MigrationCheckpoint struct {
	FormulaVersion int                `json:"formula_version"`
	TownRoot       string             `json:"town_root"`
	StartedAt      time.Time          `json:"started_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	Steps          map[string]StepRun `json:"steps"`
}

// StepRun tracks execution state for a single migration step.
type StepRun struct {
	ID                string    `json:"id"`
	Title             string    `json:"title"`
	Status            string    `json:"status"` // "pending", "running", "completed", "failed"
	StartedAt         time.Time `json:"started_at,omitempty"`
	CompletedAt       time.Time `json:"completed_at,omitempty"`
	Error             string    `json:"error,omitempty"`
	Output            string    `json:"output,omitempty"`
	CommandsCompleted int       `json:"commands_completed"` // number of commands within the step that finished successfully
}

const migrationCheckpointFile = ".migration-checkpoint.json"

var (
	runMigrationResume  bool
	runMigrationDryRun  bool
	runMigrationStep    string
	runMigrationVerbose bool
	runMigrationTimeout time.Duration
)

var runMigrationCmd = &cobra.Command{
	Use:     "run-migration [town-root]",
	GroupID: GroupWork,
	Short:   "Orchestrate the SQLite-to-Dolt migration workflow",
	Long: `Execute the mol-migration formula with checkpointing and progress tracking.

This command orchestrates the 6-step migration from SQLite-based beads (v0.5.0)
to Dolt-based beads (v0.6.0). Each step is executed sequentially with dependency
tracking and checkpoint-based recovery.

Steps:
  1. detect          Identify rigs needing migration
  2. backup          Backup all beads data
  3. upgrade-binaries Upgrade gt, bd, and install dolt
  4. migrate-town    Migrate town-level beads
  5. migrate-rigs    Migrate each rig's beads
  6. verify          Verify migration success

Features:
  - Checkpointing: Progress saved after each step; resume after interruption
  - Dependency tracking: Steps execute in correct order
  - Error recovery: Failed steps can be retried with --step
  - Dry run: Preview execution plan without running

Examples:
  gt run-migration ~/gt                   # Run full migration
  gt run-migration ~/gt --resume          # Resume from checkpoint
  gt run-migration ~/gt --step=backup     # Run specific step
  gt run-migration ~/gt --dry-run         # Preview plan`,
	Args: cobra.MaximumNArgs(1),
	RunE: runMigration,
}

func init() {
	runMigrationCmd.Flags().BoolVar(&runMigrationResume, "resume", false, "Resume from checkpoint")
	runMigrationCmd.Flags().BoolVar(&runMigrationDryRun, "dry-run", false, "Preview execution plan without running")
	runMigrationCmd.Flags().StringVar(&runMigrationStep, "step", "", "Run a specific step (skip dependency check)")
	runMigrationCmd.Flags().BoolVarP(&runMigrationVerbose, "verbose", "v", false, "Show step output in detail")
	runMigrationCmd.Flags().DurationVar(&runMigrationTimeout, "timeout", 5*time.Minute, "Timeout per migration step (e.g. 10m, 30s)")

	rootCmd.AddCommand(runMigrationCmd)
}

func runMigration(cmd *cobra.Command, args []string) error {
	// Resolve town root
	var townRoot string
	if len(args) > 0 {
		var err error
		townRoot, err = filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("resolving path: %w", err)
		}
	} else {
		var err error
		townRoot, err = workspace.FindFromCwd()
		if err != nil {
			return fmt.Errorf("not in a Gas Town workspace and no town-root specified: %w", err)
		}
	}

	// Verify town root exists
	if _, err := os.Stat(townRoot); os.IsNotExist(err) {
		return fmt.Errorf("town root does not exist: %s", townRoot)
	}

	// Parse the mol-migration formula
	f, err := loadMigrationFormula()
	if err != nil {
		return fmt.Errorf("loading migration formula: %w", err)
	}

	// Get topological order of steps
	stepOrder, err := f.TopologicalSort()
	if err != nil {
		return fmt.Errorf("resolving step order: %w", err)
	}

	// Load or create checkpoint
	cp, err := loadMigrationCheckpoint(townRoot)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("loading checkpoint: %w", err)
	}

	if cp != nil && !runMigrationResume && runMigrationStep == "" && !runMigrationDryRun {
		// Existing checkpoint found, prompt to resume
		completedCount := 0
		for _, s := range cp.Steps {
			if s.Status == "completed" {
				completedCount++
			}
		}
		fmt.Printf("%s Existing checkpoint found (%d/%d steps complete, last updated %s)\n",
			style.Bold.Render("!"), completedCount, len(f.Steps), cp.UpdatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Use --resume to continue, or delete %s to start fresh\n",
			filepath.Join(townRoot, migrationCheckpointFile))
		return nil
	}

	if cp == nil || (!runMigrationResume && runMigrationStep == "") {
		cp = &MigrationCheckpoint{
			FormulaVersion: f.Version,
			TownRoot:       townRoot,
			StartedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			Steps:          make(map[string]StepRun),
		}
		// Initialize all steps as pending
		for _, step := range f.Steps {
			cp.Steps[step.ID] = StepRun{
				ID:     step.ID,
				Title:  step.Title,
				Status: "pending",
			}
		}
	}

	// Handle dry-run mode
	if runMigrationDryRun {
		return dryRunMigration(f, cp, stepOrder, townRoot)
	}

	// Handle single-step mode
	if runMigrationStep != "" {
		step := f.GetStep(runMigrationStep)
		if step == nil {
			return fmt.Errorf("unknown step: %s\nAvailable steps: %s", runMigrationStep, strings.Join(stepOrder, ", "))
		}
		return executeMigrationStep(f, cp, step, townRoot)
	}

	// Execute steps in order
	return executeMigrationSteps(f, cp, stepOrder, townRoot)
}

// loadMigrationFormula finds and parses the mol-migration formula.
func loadMigrationFormula() (*formula.Formula, error) {
	// Search in formula search paths
	formulaPath, err := findFormulaFile("mol-migration")
	if err != nil {
		return nil, fmt.Errorf("mol-migration formula not found: %w\n\nRun 'gt formula list' to see available formulas", err)
	}

	return formula.ParseFile(formulaPath)
}

// loadMigrationCheckpoint loads an existing checkpoint from town root.
func loadMigrationCheckpoint(townRoot string) (*MigrationCheckpoint, error) {
	path := filepath.Join(townRoot, migrationCheckpointFile)
	data, err := os.ReadFile(path) //nolint:gosec // G304: path from trusted townRoot
	if err != nil {
		return nil, err
	}

	var cp MigrationCheckpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("parsing checkpoint: %w", err)
	}
	return &cp, nil
}

// saveMigrationCheckpoint persists the checkpoint to disk atomically.
// Uses atomic write (temp file + rename) to prevent corruption if the
// process crashes mid-write, which would break checkpoint-based recovery.
func saveMigrationCheckpoint(townRoot string, cp *MigrationCheckpoint) error {
	cp.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding checkpoint: %w", err)
	}

	path := filepath.Join(townRoot, migrationCheckpointFile)
	return util.AtomicWriteFile(path, data, 0600)
}

// dryRunMigration shows what would be executed.
func dryRunMigration(f *formula.Formula, cp *MigrationCheckpoint, stepOrder []string, townRoot string) error {
	fmt.Printf("%s Migration plan for: %s\n\n", style.Bold.Render("Migration Dry Run"), townRoot)

	completed := make(map[string]bool)
	for id, step := range cp.Steps {
		if step.Status == "completed" {
			completed[id] = true
		}
	}

	for i, stepID := range stepOrder {
		step := f.GetStep(stepID)
		if step == nil {
			continue
		}

		status := "PENDING"
		marker := "○"
		if completed[stepID] {
			status = "COMPLETED"
			marker = style.Bold.Render("✓")
		} else if s, ok := cp.Steps[stepID]; ok && s.Status == "failed" {
			status = "FAILED (will retry)"
			marker = "✗"
		}

		fmt.Printf("  %s %d. %s — %s [%s]\n", marker, i+1, stepID, step.Title, status)

		if len(step.Needs) > 0 {
			fmt.Printf("      needs: %s\n", strings.Join(step.Needs, ", "))
		}
	}

	// Summary
	completedCount := len(completed)
	remaining := len(stepOrder) - completedCount
	fmt.Printf("\n  Total: %d steps, %d completed, %d remaining\n", len(stepOrder), completedCount, remaining)

	return nil
}

// executeMigrationSteps runs all pending steps in order.
func executeMigrationSteps(f *formula.Formula, cp *MigrationCheckpoint, stepOrder []string, townRoot string) error {
	fmt.Printf("%s Starting migration for: %s\n\n", style.Bold.Render("Migration"), townRoot)

	completed := make(map[string]bool)
	for id, step := range cp.Steps {
		if step.Status == "completed" {
			completed[id] = true
		}
	}

	totalSteps := len(stepOrder)
	for i, stepID := range stepOrder {
		// Skip completed steps
		if completed[stepID] {
			fmt.Printf("  %s [%d/%d] %s — already completed\n",
				style.Bold.Render("✓"), i+1, totalSteps, stepID)
			continue
		}

		step := f.GetStep(stepID)
		if step == nil {
			return fmt.Errorf("internal error: step %s not found in formula", stepID)
		}

		// Verify dependencies are met
		for _, need := range step.Needs {
			if !completed[need] {
				return fmt.Errorf("step %s depends on %s which has not completed", stepID, need)
			}
		}

		// Execute the step
		fmt.Printf("  %s [%d/%d] %s — %s\n",
			style.Bold.Render("→"), i+1, totalSteps, stepID, step.Title)

		if err := executeMigrationStep(f, cp, step, townRoot); err != nil {
			fmt.Printf("  %s [%d/%d] %s — FAILED: %v\n",
				style.Bold.Render("✗"), i+1, totalSteps, stepID, err)
			fmt.Printf("\n  Resume with: gt run-migration %s --resume\n", townRoot)
			fmt.Printf("  Retry step:  gt run-migration %s --step=%s\n", townRoot, stepID)
			return fmt.Errorf("step %s failed: %w", stepID, err)
		}

		completed[stepID] = true
		fmt.Printf("  %s [%d/%d] %s — completed\n",
			style.Bold.Render("✓"), i+1, totalSteps, stepID)
	}

	// All steps completed — remove checkpoint
	cpPath := filepath.Join(townRoot, migrationCheckpointFile)
	_ = os.Remove(cpPath)

	fmt.Printf("\n%s Migration completed successfully!\n", style.Bold.Render("✓"))
	fmt.Printf("  Run 'gt doctor' to verify installation health.\n")

	return nil
}

// executeMigrationStep runs a single migration step with checkpointing.
func executeMigrationStep(_ *formula.Formula, cp *MigrationCheckpoint, step *formula.Step, townRoot string) error {
	// Update checkpoint: step running (preserve per-command progress from prior attempt)
	sr := StepRun{
		ID:        step.ID,
		Title:     step.Title,
		Status:    "running",
		StartedAt: time.Now(),
	}
	if prev, ok := cp.Steps[step.ID]; ok && prev.CommandsCompleted > 0 {
		sr.CommandsCompleted = prev.CommandsCompleted
	}
	cp.Steps[step.ID] = sr
	if err := saveMigrationCheckpoint(townRoot, cp); err != nil {
		return fmt.Errorf("saving checkpoint: %w", err)
	}

	// Extract executable commands from step description
	commands := extractCommands(step.Description, townRoot)

	if len(commands) == 0 {
		// No executable commands found — step is prose-only.
		// Mark as completed since the agent orchestrates these manually.
		fmt.Printf("    %s No auto-executable commands in step (prose instructions)\n",
			style.Dim.Render("note:"))
		fmt.Printf("    %s Step description:\n", style.Dim.Render(""))
		// Show a short summary of the step
		lines := strings.Split(step.Description, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "```") {
				fmt.Printf("      %s\n", trimmed)
			}
		}
	} else {
		// Execute commands sequentially, skipping already-completed ones on retry
		for cmdIdx, cmdStr := range commands {
			if cmdIdx < sr.CommandsCompleted {
				if runMigrationVerbose {
					fmt.Printf("    %s %s (already completed)\n", style.Dim.Render("skip:"), cmdStr)
				}
				continue
			}

			if runMigrationVerbose {
				fmt.Printf("    %s %s\n", style.Dim.Render("$"), cmdStr)
			}

			ctx, cancel := context.WithTimeout(context.Background(), runMigrationTimeout)
			c := migrationShellCmd(ctx, cmdStr)
			c.Dir = townRoot
			c.Env = append(os.Environ(), "GT_MIGRATION=1")
			// Set platform-specific process group attributes so we can
			// kill all children on timeout, not just the bash process.
			setMigrationProcAttr(c)

			output, err := c.CombinedOutput()
			// Capture ctx.Err() before cancel() could modify context state
			ctxErr := ctx.Err()
			cancel()
			outputStr := strings.TrimSpace(string(output))

			if runMigrationVerbose && outputStr != "" {
				// Indent output
				for _, line := range strings.Split(outputStr, "\n") {
					fmt.Printf("      %s\n", line)
				}
			}

			if err != nil {
				// Distinguish timeout from other failures
				errMsg := fmt.Sprintf("command failed: %s\nerror: %v\noutput: %s", cmdStr, err, outputStr)
				if ctxErr == context.DeadlineExceeded {
					errMsg = fmt.Sprintf("command timed out after %s: %s\noutput: %s", runMigrationTimeout, cmdStr, outputStr)
				}

				// Update checkpoint: step failed (preserving commands_completed for retry)
				sr.Status = "failed"
				sr.Error = errMsg
				sr.CompletedAt = time.Now()
				cp.Steps[step.ID] = sr
				if saveErr := saveMigrationCheckpoint(townRoot, cp); saveErr != nil {
					fmt.Fprintf(os.Stderr, "    %s failed to save checkpoint: %v\n", style.Bold.Render("warning:"), saveErr)
				}

				if ctxErr == context.DeadlineExceeded {
					return fmt.Errorf("command timed out after %s: %s\n  output: %s", runMigrationTimeout, cmdStr, truncateOutput(outputStr, 500))
				}
				return fmt.Errorf("command failed: %s\n  %v\n  output: %s", cmdStr, err, truncateOutput(outputStr, 500))
			}

			// Track per-command progress
			sr.CommandsCompleted = cmdIdx + 1
			cp.Steps[step.ID] = sr
			if saveErr := saveMigrationCheckpoint(townRoot, cp); saveErr != nil {
				fmt.Fprintf(os.Stderr, "    %s failed to save checkpoint: %v\n", style.Bold.Render("warning:"), saveErr)
			}

			// Capture last output for checkpoint
			if outputStr != "" {
				sr.Output = truncateOutput(outputStr, 2000)
			}
		}
	}

	// Update checkpoint: step completed
	sr.Status = "completed"
	sr.CompletedAt = time.Now()
	cp.Steps[step.ID] = sr
	if err := saveMigrationCheckpoint(townRoot, cp); err != nil {
		return fmt.Errorf("saving checkpoint after completion: %w", err)
	}

	return nil
}

// extractCommands parses bash code blocks from a step description and
// returns them as executable command strings. Template variables like
// {{town_root}} are replaced with actual values.
func extractCommands(description, townRoot string) []string {
	var commands []string

	// Replace template variables
	description = strings.ReplaceAll(description, "{{town_root}}", townRoot)

	// Parse bash code blocks
	lines := strings.Split(description, "\n")
	inCodeBlock := false
	var currentBlock []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```bash") || strings.HasPrefix(trimmed, "```sh") {
			inCodeBlock = true
			currentBlock = nil
			continue
		}

		if trimmed == "```" && inCodeBlock {
			inCodeBlock = false
			if len(currentBlock) > 0 {
				// Join multi-line commands and add as a single command
				block := strings.Join(currentBlock, "\n")
				block = strings.TrimSpace(block)
				if block != "" && !isCommentOnly(block) {
					commands = append(commands, block)
				}
			}
			currentBlock = nil
			continue
		}

		if inCodeBlock {
			// Skip pure comment lines in code blocks
			if !strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "#!") {
				currentBlock = append(currentBlock, line)
			}
		}
	}

	return commands
}

// isCommentOnly returns true if the block contains only comments and whitespace.
func isCommentOnly(block string) bool {
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return false
		}
	}
	return true
}

// truncateOutput shortens a string to maxLen, adding "..." if truncated.
func truncateOutput(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 4 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
