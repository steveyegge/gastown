package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var unslingCmd = &cobra.Command{
	Use:     "unsling [bead-id] [target]",
	Aliases: []string{"unhook"},
	GroupID: GroupWork,
	Short:   "Remove work from an agent's hook",
	Long: `Remove work from an agent's hook (the inverse of sling/hook).

With no arguments, clears your own hook. With a bead ID, only unslings
if that specific bead is currently hooked. With a target, operates on
another agent's hook.

Examples:
  gt unsling                        # Clear my hook (whatever's there)
  gt unsling gt-abc                 # Only unsling if gt-abc is hooked
  gt unsling greenplace/joe            # Clear joe's hook
  gt unsling gt-abc greenplace/joe     # Unsling gt-abc from joe

The bead's status changes from 'hooked' back to 'open'.

Related commands:
  gt sling <bead>    # Hook + start (inverse of unsling)
  gt hook <bead>     # Hook without starting
  gt hook      # See what's on your hook`,
	Args: cobra.MaximumNArgs(2),
	RunE: runUnsling,
}

var (
	unslingDryRun bool
	unslingForce  bool
)

func init() {
	unslingCmd.Flags().BoolVarP(&unslingDryRun, "dry-run", "n", false, "Show what would be done")
	unslingCmd.Flags().BoolVarP(&unslingForce, "force", "f", false, "Unsling even if work is incomplete")
	rootCmd.AddCommand(unslingCmd)
}

func runUnsling(cmd *cobra.Command, args []string) error {
	return runUnslingWith(cmd, args, unslingDryRun, unslingForce)
}

func runUnslingWith(cmd *cobra.Command, args []string, dryRun, force bool) error {
	var targetBeadID string
	var targetAgent string

	// Parse args: [bead-id] [target]
	switch len(args) {
	case 0:
		// No args - unsling self, whatever is hooked
	case 1:
		// Could be bead ID or target agent
		// If it contains "/" or is a known role, treat as target
		if isAgentTarget(args[0]) {
			targetAgent = args[0]
		} else {
			targetBeadID = args[0]
		}
	case 2:
		targetBeadID = args[0]
		targetAgent = args[1]
	}

	// Resolve target agent (default: self)
	var agentID string
	var err error
	if targetAgent != "" {
		agentID, _, _, err = resolveTargetAgent(targetAgent)
		if err != nil {
			return fmt.Errorf("resolving target agent: %w", err)
		}
	} else {
		agentID, _, _, err = resolveSelfTarget()
		if err != nil {
			return fmt.Errorf("detecting agent identity: %w", err)
		}
	}

	// Find town root and rig path for agent beads
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Convert agent ID to agent bead ID first, so we can use prefix-based routing
	agentBeadID := agentIDToBeadID(agentID, townRoot)
	if agentBeadID == "" {
		return fmt.Errorf("could not convert agent ID %s to bead ID", agentID)
	}

	// Resolve the correct beads directory using prefix-based routing.
	// This matches how updateAgentHookBead resolves the directory when setting
	// the hook (via beads.ResolveHookDir). Town-level agents (mayor, deacon)
	// fall back to townRoot since their beads use hq- prefix stored at town level.
	rigName := strings.Split(agentID, "/")[0]
	var fallbackPath string
	if rigName == "mayor" || rigName == "deacon" {
		fallbackPath = townRoot
	} else {
		fallbackPath = filepath.Join(townRoot, rigName)
	}
	beadsPath := beads.ResolveHookDir(townRoot, agentBeadID, fallbackPath)

	b := beads.New(beadsPath)

	// Get the agent bead to find current hook.
	// The agent bead may not exist (e.g., crew members whose agent beads haven't
	// been created yet). This is NOT a fatal error - we fall back to querying
	// for hooked beads by status.
	var agentBead *beads.Issue
	agentBead, err = b.Show(agentBeadID)
	if err != nil {
		// Agent bead not found - this is OK, we'll fall back to status query
		agentBead = nil
	}

	// Check if agent has work hooked (via hook_bead field on agent bead)
	hookedBeadID := ""
	if agentBead != nil {
		hookedBeadID = agentBead.HookBead
	}

	// Fallback: if hook_bead is empty (cleared or agent bead missing), query for
	// beads that still have status=hooked assigned to this agent. This catches
	// stale hooked beads where hook_bead was cleared but bead status wasn't reset.
	// This matches the fallback behavior in runMoleculeStatus.
	if hookedBeadID == "" {
		hookedBeads, listErr := b.List(beads.ListOptions{
			Status:   beads.StatusHooked,
			Assignee: agentID,
			Priority: -1,
		})
		if listErr == nil && len(hookedBeads) > 0 {
			hookedBeadID = hookedBeads[0].ID
		}
	}

	if hookedBeadID == "" {
		// hook_bead is empty, but there may be stale beads with status "hooked"
		// still assigned to this agent (e.g., hook_bead was cleared but bead status
		// wasn't updated). Clean them up so gt hook and gt unsling stay consistent.
		cleaned := cleanStaleHookedBeads(cmd, b, agentID, targetBeadID, townRoot, beadsPath, dryRun)
		if !cleaned {
			if targetAgent != "" {
				fmt.Printf("%s No work hooked for %s\n", style.Dim.Render("ℹ"), agentID)
			} else {
				fmt.Printf("%s Nothing on your hook\n", style.Dim.Render("ℹ"))
			}
		}
		return nil
	}

	// If specific bead requested, verify it matches
	if targetBeadID != "" && hookedBeadID != targetBeadID {
		return fmt.Errorf("bead %s is not hooked (current hook: %s)", targetBeadID, hookedBeadID)
	}

	// Get the hooked bead to check completion and show title.
	// The hooked bead may be in a different database than the agent bead
	// (e.g., agent in rig db, hooked bead in town db), so resolve its path separately.
	hookedBeadPath := beads.ResolveHookDir(townRoot, hookedBeadID, beadsPath)
	hookedB := b
	if hookedBeadPath != beadsPath {
		hookedB = beads.New(hookedBeadPath)
	}
	hookedBead, err := hookedB.Show(hookedBeadID)
	if err != nil {
		// Bead might be deleted - still allow unsling with --force
		if !force {
			return fmt.Errorf("getting hooked bead %s: %w\n  Use --force to unsling anyway", hookedBeadID, err)
		}
		// Force mode - proceed without the bead details
		hookedBead = &beads.Issue{ID: hookedBeadID, Title: "(unknown)"}
	}

	// Check if work is complete (warn if not, unless --force)
	isComplete := hookedBead.Status == "closed"
	if !isComplete && !force {
		return fmt.Errorf("hooked work %s is incomplete (%s)\n  Use --force to unsling anyway",
			hookedBeadID, hookedBead.Title)
	}

	if targetAgent != "" {
		fmt.Printf("%s Unslinging %s from %s...\n", style.Bold.Render("🪝"), hookedBeadID, agentID)
	} else {
		fmt.Printf("%s Unslinging %s...\n", style.Bold.Render("🪝"), hookedBeadID)
	}

	if dryRun {
		fmt.Printf("Would clear hook_bead from agent bead %s\n", agentBeadID)
		return nil
	}

	// Clear the hook from agent bead if it exists (gt-zecmc: removed agent_state update)
	if agentBead != nil {
		if err := b.ClearHookBead(agentBeadID); err != nil {
			// Non-fatal: the hook_bead field may already be cleared.
			// The bead status update below is the more important cleanup.
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: couldn't clear hook from agent bead %s: %v\n", agentBeadID, err)
		}
	}

	// Update hooked bead status from "hooked" back to "open".
	// Previously, only the agent's hook slot was cleared but the bead itself stayed
	// in "hooked" status forever. Now we update the bead to match the documented
	// behavior: "The bead's status changes from 'hooked' back to 'open'."
	if hookedBead.Status == beads.StatusHooked {
		openStatus := "open"
		emptyAssignee := ""
		if err := hookedB.Update(hookedBeadID, beads.UpdateOptions{
			Status:   &openStatus,
			Assignee: &emptyAssignee,
		}); err != nil {
			// Non-fatal: warn but don't fail the unsling. The hook slot is already
			// cleared, so the agent is unblocked. The bead status is a bookkeeping
			// issue that can be fixed manually.
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: couldn't update bead %s status: %v\n", hookedBeadID, err)
		}
	}

	// Log unhook event
	_ = events.LogFeed(events.TypeUnhook, agentID, events.UnhookPayload(hookedBeadID))

	fmt.Printf("%s Work removed from hook\n", style.Bold.Render("✓"))
	fmt.Printf("  Agent %s hook cleared (was: %s)\n", agentID, hookedBeadID)

	return nil
}

// cleanStaleHookedBeads finds and cleans up beads with status "hooked" assigned to
// agentID when the agent bead's hook_bead field is already null. This handles the
// inconsistency where hook_bead was cleared (e.g., by another process) but the
// bead's status wasn't updated back to "open". Without this, gt hook shows the
// stale hook (via fallback query) but gt unsling says "Nothing on your hook".
// Returns true if any stale beads were cleaned up.
func cleanStaleHookedBeads(cmd *cobra.Command, b *beads.Beads, agentID, targetBeadID, townRoot, beadsPath string, dryRun bool) bool {
	// Query for beads with status=hooked assigned to this agent
	staleBeads, err := b.List(beads.ListOptions{
		Status:   beads.StatusHooked,
		Assignee: agentID,
		Priority: -1,
	})
	if err != nil || len(staleBeads) == 0 {
		return false
	}

	// If a specific bead was requested, filter to only that one
	if targetBeadID != "" {
		var filtered []*beads.Issue
		for _, sb := range staleBeads {
			if sb.ID == targetBeadID {
				filtered = append(filtered, sb)
			}
		}
		if len(filtered) == 0 {
			return false
		}
		staleBeads = filtered
	}

	if dryRun {
		for _, sb := range staleBeads {
			fmt.Printf("Would clean up stale hooked bead %s (%s)\n", sb.ID, sb.Title)
		}
		return true
	}

	// Clean up each stale hooked bead
	for _, sb := range staleBeads {
		fmt.Printf("%s Cleaning up stale hooked bead %s...\n", style.Bold.Render("🪝"), sb.ID)

		// Resolve the correct beads directory for this bead
		stalePath := beads.ResolveHookDir(townRoot, sb.ID, beadsPath)
		staleB := b
		if stalePath != beadsPath {
			staleB = beads.New(stalePath)
		}

		openStatus := "open"
		emptyAssignee := ""
		if err := staleB.Update(sb.ID, beads.UpdateOptions{
			Status:   &openStatus,
			Assignee: &emptyAssignee,
		}); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: couldn't clean up stale bead %s: %v\n", sb.ID, err)
			continue
		}
		fmt.Printf("%s Cleaned up stale bead %s (was hooked, now open)\n", style.Bold.Render("✓"), sb.ID)
	}
	return true
}

// isAgentTarget checks if a string looks like an agent target rather than a bead ID.
// Agent targets contain "/" or are known role names.
func isAgentTarget(s string) bool {
	// Contains "/" means it's a path like "greenplace/joe"
	for _, c := range s {
		if c == '/' {
			return true
		}
	}

	// Known role names
	switch s {
	case "mayor", "deacon", "witness", "refinery", "crew":
		return true
	}

	return false
}
