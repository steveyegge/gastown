package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var doneCmd = &cobra.Command{
	Use:     "done",
	GroupID: GroupWork,
	Short:   "Signal work ready for merge queue",
	Long: `Signal that your work is complete and ready for the merge queue.

This is a convenience command for polecats that:
1. Submits the current branch to the merge queue
2. Auto-detects issue ID from branch name
3. Notifies the Witness with the exit outcome
4. Optionally exits the Claude session (--exit flag)

Exit statuses:
  COMPLETED      - Work done, MR submitted (default)
  ESCALATED      - Hit blocker, needs human intervention
  DEFERRED       - Work paused, issue still open
  PHASE_COMPLETE - Phase done, awaiting gate (use --phase-complete)

Phase handoff workflow:
  When a molecule has gate steps (async waits), use --phase-complete to signal
  that the current phase is complete but work continues after the gate closes.
  The Witness will recycle this polecat and dispatch a new one when the gate
  resolves.

Examples:
  gt done                              # Submit branch, notify COMPLETED
  gt done --exit                       # Submit and exit Claude session
  gt done --issue gt-abc               # Explicit issue ID
  gt done --status ESCALATED           # Signal blocker, skip MR
  gt done --status DEFERRED            # Pause work, skip MR
  gt done --phase-complete --gate g-x  # Phase done, waiting on gate g-x`,
	RunE: runDone,
}

var (
	doneIssue         string
	donePriority      int
	doneStatus        string
	doneExit          bool
	donePhaseComplete bool
	doneGate          string
)

// Valid exit types for gt done
const (
	ExitCompleted     = "COMPLETED"
	ExitEscalated     = "ESCALATED"
	ExitDeferred      = "DEFERRED"
	ExitPhaseComplete = "PHASE_COMPLETE"
)

func init() {
	doneCmd.Flags().StringVar(&doneIssue, "issue", "", "Source issue ID (default: parse from branch name)")
	doneCmd.Flags().IntVarP(&donePriority, "priority", "p", -1, "Override priority (0-4, default: inherit from issue)")
	doneCmd.Flags().StringVar(&doneStatus, "status", ExitCompleted, "Exit status: COMPLETED, ESCALATED, or DEFERRED")
	doneCmd.Flags().BoolVar(&doneExit, "exit", false, "Exit Claude session after MR submission (self-terminate)")
	doneCmd.Flags().BoolVar(&donePhaseComplete, "phase-complete", false, "Signal phase complete - await gate before continuing")
	doneCmd.Flags().StringVar(&doneGate, "gate", "", "Gate bead ID to wait on (with --phase-complete)")

	rootCmd.AddCommand(doneCmd)
}

func runDone(cmd *cobra.Command, args []string) error {
	// Handle --phase-complete flag (overrides --status)
	var exitType string
	if donePhaseComplete {
		exitType = ExitPhaseComplete
		if doneGate == "" {
			return fmt.Errorf("--phase-complete requires --gate <gate-id>")
		}
	} else {
		// Validate exit status
		exitType = strings.ToUpper(doneStatus)
		if exitType != ExitCompleted && exitType != ExitEscalated && exitType != ExitDeferred {
			return fmt.Errorf("invalid exit status '%s': must be COMPLETED, ESCALATED, or DEFERRED", doneStatus)
		}
	}

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Find current rig
	rigName, _, err := findCurrentRig(townRoot)
	if err != nil {
		return err
	}

	// Initialize git for the current directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting current directory: %w", err)
	}
	g := git.NewGit(cwd)

	// Get current branch
	branch, err := g.CurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}

	// Parse branch info
	info := parseBranchName(branch)

	// Override with explicit flags
	issueID := doneIssue
	if issueID == "" {
		issueID = info.Issue
	}
	worker := info.Worker

	// Determine polecat name from sender detection
	sender := detectSender()
	polecatName := ""
	if parts := strings.Split(sender, "/"); len(parts) >= 2 {
		polecatName = parts[len(parts)-1]
	}

	// Get agent bead ID for cross-referencing
	var agentBeadID string
	if roleInfo, err := GetRoleWithContext(cwd, townRoot); err == nil {
		ctx := RoleContext{
			Role:     roleInfo.Role,
			Rig:      roleInfo.Rig,
			Polecat:  roleInfo.Polecat,
			TownRoot: townRoot,
			WorkDir:  cwd,
		}
		agentBeadID = getAgentBeadID(ctx)
	}

	// For COMPLETED, we need an issue ID and branch must not be main
	var mrID string
	if exitType == ExitCompleted {
		if branch == "main" || branch == "master" {
			return fmt.Errorf("cannot submit main/master branch to merge queue")
		}

		// Check for unpushed commits - branch must be pushed before MR creation
		// Use BranchPushedToRemote which handles polecat branches without upstream tracking
		pushed, unpushedCount, err := g.BranchPushedToRemote(branch, "origin")
		if err != nil {
			return fmt.Errorf("checking if branch is pushed: %w", err)
		}
		if !pushed {
			return fmt.Errorf("branch has %d unpushed commit(s); run 'git push -u origin %s' first", unpushedCount, branch)
		}

		// Detect the repo's default branch (main vs master)
		defaultBranch := g.RemoteDefaultBranch()

		// Check that branch has commits ahead of default branch (prevents submitting stale branches)
		aheadCount, err := g.CommitsAhead(defaultBranch, branch)
		if err != nil {
			return fmt.Errorf("checking commits ahead of %s: %w", defaultBranch, err)
		}
		if aheadCount == 0 {
			return fmt.Errorf("branch '%s' has 0 commits ahead of %s; nothing to merge", branch, defaultBranch)
		}

		if issueID == "" {
			return fmt.Errorf("cannot determine source issue from branch '%s'; use --issue to specify", branch)
		}

		// Initialize beads
		bd := beads.New(cwd)

		// Determine target branch (auto-detect integration branch if applicable)
		target := defaultBranch
		autoTarget, err := detectIntegrationBranch(bd, g, issueID)
		if err == nil && autoTarget != "" {
			target = autoTarget
		}

		// Get source issue for priority inheritance
		var priority int
		if donePriority >= 0 {
			priority = donePriority
		} else {
			// Try to inherit from source issue
			sourceIssue, err := bd.Show(issueID)
			if err != nil {
				priority = 2 // Default
			} else {
				priority = sourceIssue.Priority
			}
		}

		// Check if MR bead already exists for this branch (idempotency)
		existingMR, err := bd.FindMRForBranch(branch)
		if err != nil {
			style.PrintWarning("could not check for existing MR: %v", err)
			// Continue with creation attempt - Create will fail if duplicate
		}

		if existingMR != nil {
			// MR already exists - use it instead of creating a new one
			mrID = existingMR.ID
			fmt.Printf("%s MR already exists (idempotent)\n", style.Bold.Render("✓"))
			fmt.Printf("  MR ID: %s\n", style.Bold.Render(mrID))
		} else {
			// Build MR bead title and description
			title := fmt.Sprintf("Merge: %s", issueID)
			description := fmt.Sprintf("branch: %s\ntarget: %s\nsource_issue: %s\nrig: %s",
				branch, target, issueID, rigName)
			if worker != "" {
				description += fmt.Sprintf("\nworker: %s", worker)
			}
			if agentBeadID != "" {
				description += fmt.Sprintf("\nagent_bead: %s", agentBeadID)
			}

			// Add conflict resolution tracking fields (initialized, updated by Refinery)
			description += "\nretry_count: 0"
			description += "\nlast_conflict_sha: null"
			description += "\nconflict_task_id: null"

			// Create MR bead (ephemeral wisp - will be cleaned up after merge)
			mrIssue, err := bd.Create(beads.CreateOptions{
				Title:       title,
				Type:        "merge-request",
				Priority:    priority,
				Description: description,
			})
			if err != nil {
				return fmt.Errorf("creating merge request bead: %w", err)
			}
			mrID = mrIssue.ID

			// Update agent bead with active_mr reference (for traceability)
			if agentBeadID != "" {
				if err := bd.UpdateAgentActiveMR(agentBeadID, mrID); err != nil {
					style.PrintWarning("could not update agent bead with active_mr: %v", err)
				}
			}

			// Success output
			fmt.Printf("%s Work submitted to merge queue\n", style.Bold.Render("✓"))
			fmt.Printf("  MR ID: %s\n", style.Bold.Render(mrID))
		}
		fmt.Printf("  Source: %s\n", branch)
		fmt.Printf("  Target: %s\n", target)
		fmt.Printf("  Issue: %s\n", issueID)
		if worker != "" {
			fmt.Printf("  Worker: %s\n", worker)
		}
		fmt.Printf("  Priority: P%d\n", priority)
		fmt.Println()
		fmt.Printf("%s\n", style.Dim.Render("The Refinery will process your merge request."))
	} else if exitType == ExitPhaseComplete {
		// Phase complete - register as waiter on gate, then recycle
		fmt.Printf("%s Phase complete, awaiting gate\n", style.Bold.Render("→"))
		fmt.Printf("  Gate: %s\n", doneGate)
		if issueID != "" {
			fmt.Printf("  Issue: %s\n", issueID)
		}
		fmt.Printf("  Branch: %s\n", branch)
		fmt.Println()
		fmt.Printf("%s\n", style.Dim.Render("Witness will dispatch new polecat when gate closes."))

		// Register this polecat as a waiter on the gate
		bd := beads.New(cwd)
		if err := bd.AddGateWaiter(doneGate, sender); err != nil {
			style.PrintWarning("could not register as gate waiter: %v", err)
		} else {
			fmt.Printf("%s Registered as waiter on gate %s\n", style.Bold.Render("✓"), doneGate)
		}
	} else {
		// For ESCALATED or DEFERRED, just print status
		fmt.Printf("%s Signaling %s\n", style.Bold.Render("→"), exitType)
		if issueID != "" {
			fmt.Printf("  Issue: %s\n", issueID)
		}
		fmt.Printf("  Branch: %s\n", branch)
	}

	// Notify Witness about completion
	// Use town-level beads for cross-agent mail
	townRouter := mail.NewRouter(townRoot)
	witnessAddr := fmt.Sprintf("%s/witness", rigName)

	// Build notification body
	var bodyLines []string
	bodyLines = append(bodyLines, fmt.Sprintf("Exit: %s", exitType))
	if issueID != "" {
		bodyLines = append(bodyLines, fmt.Sprintf("Issue: %s", issueID))
	}
	if mrID != "" {
		bodyLines = append(bodyLines, fmt.Sprintf("MR: %s", mrID))
	}
	if doneGate != "" {
		bodyLines = append(bodyLines, fmt.Sprintf("Gate: %s", doneGate))
	}
	bodyLines = append(bodyLines, fmt.Sprintf("Branch: %s", branch))

	doneNotification := &mail.Message{
		To:      witnessAddr,
		From:    sender,
		Subject: fmt.Sprintf("POLECAT_DONE %s", polecatName),
		Body:    strings.Join(bodyLines, "\n"),
	}

	fmt.Printf("\nNotifying Witness...\n")
	if err := townRouter.Send(doneNotification); err != nil {
		style.PrintWarning("could not notify witness: %v", err)
	} else {
		fmt.Printf("%s Witness notified of %s\n", style.Bold.Render("✓"), exitType)
	}

	// Log done event (townlog and activity feed)
	LogDone(townRoot, sender, issueID)
	_ = events.LogFeed(events.TypeDone, sender, events.DonePayload(issueID, branch))

	// Update agent bead state (ZFC: self-report completion)
	updateAgentStateOnDone(cwd, townRoot, exitType, issueID)

	// Handle session self-termination if requested
	if doneExit {
		fmt.Println()
		fmt.Printf("%s Session self-terminating (--exit flag)\n", style.Bold.Render("→"))
		fmt.Printf("  Witness will handle worktree cleanup.\n")
		fmt.Printf("  Goodbye!\n")
		os.Exit(0)
	}

	return nil
}

// updateAgentStateOnDone updates the agent bead state when work is complete.
// Maps exit type to agent state:
//   - COMPLETED → "done"
//   - ESCALATED → "stuck"
//   - DEFERRED → "idle"
//   - PHASE_COMPLETE → "awaiting-gate"
//
// Also self-reports cleanup_status for ZFC compliance (#10).
func updateAgentStateOnDone(cwd, townRoot, exitType, issueID string) {
	// Get role context
	roleInfo, err := GetRoleWithContext(cwd, townRoot)
	if err != nil {
		return
	}

	ctx := RoleContext{
		Role:     roleInfo.Role,
		Rig:      roleInfo.Rig,
		Polecat:  roleInfo.Polecat,
		TownRoot: townRoot,
		WorkDir:  cwd,
	}

	agentBeadID := getAgentBeadID(ctx)
	if agentBeadID == "" {
		return
	}

	// Map exit type to agent state
	var newState string
	switch exitType {
	case ExitCompleted:
		newState = "done"
	case ExitEscalated:
		newState = "stuck"
	case ExitDeferred:
		newState = "idle"
	case ExitPhaseComplete:
		newState = "awaiting-gate"
	default:
		return
	}

	// Update agent bead with new state and clear hook_bead (work is done)
	// Use town root for routing - ensures cross-beads references work
	bd := beads.New(townRoot)
	emptyHook := ""
	if err := bd.UpdateAgentState(agentBeadID, newState, &emptyHook); err != nil {
		// Log warning instead of silent ignore - helps debug cross-beads issues
		fmt.Fprintf(os.Stderr, "Warning: couldn't update agent %s state on done: %v\n", agentBeadID, err)
		return
	}

	// ZFC #10: Self-report cleanup status
	// Compute git state and report so Witness can decide removal safety
	cleanupStatus := computeCleanupStatus(cwd)
	if cleanupStatus != "" {
		if err := bd.UpdateAgentCleanupStatus(agentBeadID, cleanupStatus); err != nil {
			// Log warning instead of silent ignore
			fmt.Fprintf(os.Stderr, "Warning: couldn't update agent %s cleanup status: %v\n", agentBeadID, err)
			return
		}
	}
}

// computeCleanupStatus checks git state and returns the cleanup status.
// Returns the most critical issue: has_unpushed > has_stash > has_uncommitted > clean
func computeCleanupStatus(cwd string) string {
	g := git.NewGit(cwd)
	status, err := g.CheckUncommittedWork()
	if err != nil {
		// If we can't check, report unknown - Witness should be cautious
		return "unknown"
	}

	// Check in priority order (most critical first)
	if status.UnpushedCommits > 0 {
		return "has_unpushed"
	}
	if status.StashCount > 0 {
		return "has_stash"
	}
	if status.HasUncommittedChanges {
		return "has_uncommitted"
	}
	return "clean"
}
