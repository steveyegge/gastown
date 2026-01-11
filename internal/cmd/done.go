package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
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
4. Exits the Claude session (polecats don't stay alive after completion)

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
  gt done                              # Submit branch, notify COMPLETED, exit session
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
	donePhaseComplete bool
	doneGate          string
	doneCleanupStatus string
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
	doneCmd.Flags().BoolVar(&donePhaseComplete, "phase-complete", false, "Signal phase complete - await gate before continuing")
	doneCmd.Flags().StringVar(&doneGate, "gate", "", "Gate bead ID to wait on (with --phase-complete)")
	doneCmd.Flags().StringVar(&doneCleanupStatus, "cleanup-status", "", "Git cleanup status: clean, uncommitted, unpushed, stash, unknown (ZFC: agent-observed)")

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

	// Get configured default branch for this rig
	defaultBranch := "main" // fallback
	if rigCfg, err := rig.LoadRigConfig(filepath.Join(townRoot, rigName)); err == nil && rigCfg.DefaultBranch != "" {
		defaultBranch = rigCfg.DefaultBranch
	}

	// For COMPLETED, we need an issue ID and branch must not be the default branch
	var mrID string
	if exitType == ExitCompleted {
		if branch == defaultBranch || branch == "master" {
			return fmt.Errorf("cannot submit %s/master branch to merge queue", defaultBranch)
		}
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
		bd := beads.New(beads.ResolveBeadsDir(cwd))

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
		bd := beads.New(beads.ResolveBeadsDir(cwd))
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

	// Notify dispatcher if work was dispatched by another agent
	if issueID != "" {
		if dispatcher := getDispatcherFromBead(cwd, issueID); dispatcher != "" && dispatcher != sender {
			dispatcherNotification := &mail.Message{
				To:      dispatcher,
				From:    sender,
				Subject: fmt.Sprintf("WORK_DONE: %s", issueID),
				Body:    strings.Join(bodyLines, "\n"),
			}
			if err := townRouter.Send(dispatcherNotification); err != nil {
				style.PrintWarning("could not notify dispatcher %s: %v", dispatcher, err)
			} else {
				fmt.Printf("%s Dispatcher %s notified of %s\n", style.Bold.Render("✓"), dispatcher, exitType)
			}
		}
	}

	// Log done event (townlog and activity feed)
	_ = LogDone(townRoot, sender, issueID)
	_ = events.LogFeed(events.TypeDone, sender, events.DonePayload(issueID, branch))

	// Update agent bead state (ZFC: self-report completion)
	updateAgentStateOnDone(cwd, townRoot, exitType, issueID)

	// Always exit session - polecats don't stay alive after completion
	fmt.Println()
	fmt.Printf("%s Session exiting (done means gone)\n", style.Bold.Render("→"))
	fmt.Printf("  Witness will handle worktree cleanup.\n")
	fmt.Printf("  Goodbye!\n")
	os.Exit(0)

	return nil // unreachable, but keeps compiler happy
}

// updateAgentStateOnDone clears the agent's hook and reports cleanup status.
// Per gt-zecmc: observable states ("done", "idle") removed - use tmux to discover.
// Non-observable states ("stuck", "awaiting-gate") are still set since they represent
// intentional agent decisions that can't be observed from tmux.
//
// Also self-reports cleanup_status for ZFC compliance (#10).
func updateAgentStateOnDone(cwd, townRoot, exitType, _ string) { // issueID unused but kept for future audit logging
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

	// Use rig path for slot commands - bd slot doesn't route from town root
	var beadsPath string
	switch ctx.Role {
	case RoleMayor, RoleDeacon:
		beadsPath = townRoot
	default:
		beadsPath = filepath.Join(townRoot, ctx.Rig)
	}
	bd := beads.New(beadsPath)

	// BUG FIX (gt-vwjz6): Close hooked beads before clearing the hook.
	// Previously, the agent's hook_bead slot was cleared but the hooked bead itself
	// stayed status=hooked forever. Now we close the hooked bead before clearing.
	if agentBead, err := bd.Show(agentBeadID); err == nil && agentBead.HookBead != "" {
		hookedBeadID := agentBead.HookBead
		// Only close if the hooked bead exists and is still in "hooked" status
		if hookedBead, err := bd.Show(hookedBeadID); err == nil && hookedBead.Status == beads.StatusHooked {
			if err := bd.Close(hookedBeadID); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: couldn't close hooked bead %s: %v\n", hookedBeadID, err)
			}
		}
	}

	// Clear the hook (work is done) - gt-zecmc
	if err := bd.ClearHookBead(agentBeadID); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: couldn't clear agent %s hook: %v\n", agentBeadID, err)
	}

	// Only set non-observable states - "stuck" and "awaiting-gate" are intentional
	// agent decisions that can't be discovered from tmux. Skip "done" and "idle"
	// since those are observable (no session = done, session + no hook = idle).
	switch exitType {
	case ExitEscalated:
		// "stuck" = agent is requesting help - not observable from tmux
		if _, err := bd.Run("agent", "state", agentBeadID, "stuck"); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: couldn't set agent %s to stuck: %v\n", agentBeadID, err)
		}
	case ExitPhaseComplete:
		// "awaiting-gate" = agent is waiting for external trigger - not observable
		if _, err := bd.Run("agent", "state", agentBeadID, "awaiting-gate"); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: couldn't set agent %s to awaiting-gate: %v\n", agentBeadID, err)
		}
	// ExitCompleted and ExitDeferred don't set state - observable from tmux
	}

	// ZFC #10: Self-report cleanup status
	// Agent observes git state and passes cleanup status via --cleanup-status flag
	if doneCleanupStatus != "" {
		cleanupStatus := parseCleanupStatus(doneCleanupStatus)
		if cleanupStatus != polecat.CleanupUnknown {
			if err := bd.UpdateAgentCleanupStatus(agentBeadID, string(cleanupStatus)); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: couldn't update agent %s cleanup status: %v\n", agentBeadID, err)
				return
			}
		}
	}
}

// getDispatcherFromBead retrieves the dispatcher agent ID from the bead's attachment fields.
// Returns empty string if no dispatcher is recorded.
func getDispatcherFromBead(cwd, issueID string) string {
	if issueID == "" {
		return ""
	}

	bd := beads.New(beads.ResolveBeadsDir(cwd))
	issue, err := bd.Show(issueID)
	if err != nil {
		return ""
	}

	fields := beads.ParseAttachmentFields(issue)
	if fields == nil {
		return ""
	}

	return fields.DispatchedBy
}

// parseCleanupStatus converts a string flag value to a CleanupStatus.
// ZFC: Agent observes git state and passes the appropriate status.
func parseCleanupStatus(s string) polecat.CleanupStatus {
	switch strings.ToLower(s) {
	case "clean":
		return polecat.CleanupClean
	case "uncommitted", "has_uncommitted":
		return polecat.CleanupUncommitted
	case "stash", "has_stash":
		return polecat.CleanupStash
	case "unpushed", "has_unpushed":
		return polecat.CleanupUnpushed
	default:
		return polecat.CleanupUnknown
	}
}
