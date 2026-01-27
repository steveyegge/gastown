package cmd

import (
	"fmt"

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

	// Find town root for beads resolution
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Use town root for beads client - routing handles prefix-based resolution.
	// FIX (hq-8af330.6): Previously used rig path for non-town agents, but
	// crew beads use gt- prefix which routes via town-level routes.jsonl.
	b := beads.New(townRoot)

	// Convert agent ID to agent bead ID and look up the agent bead
	agentBeadID := agentIDToBeadID(agentID, townRoot)
	if agentBeadID == "" {
		return fmt.Errorf("could not convert agent ID %s to bead ID", agentID)
	}

	// Get the agent bead to find current hook
	agentBead, err := b.Show(agentBeadID)
	if err != nil {
		return fmt.Errorf("getting agent bead %s: %w", agentBeadID, err)
	}

	// Check if agent has work hooked (via hook_bead field)
	hookedBeadID := agentBead.HookBead
	if hookedBeadID == "" {
		if targetAgent != "" {
			fmt.Printf("%s No work hooked for %s\n", style.Dim.Render("ℹ"), agentID)
		} else {
			fmt.Printf("%s Nothing on your hook\n", style.Dim.Render("ℹ"))
		}
		return nil
	}

	// If specific bead requested, verify it matches
	if targetBeadID != "" && hookedBeadID != targetBeadID {
		return fmt.Errorf("bead %s is not hooked (current hook: %s)", targetBeadID, hookedBeadID)
	}

	// Get the hooked bead to check completion and show title
	hookedBead, err := b.Show(hookedBeadID)
	if err != nil {
		// Bead might be deleted - still allow unsling with --force
		if !unslingForce {
			return fmt.Errorf("getting hooked bead %s: %w\n  Use --force to unsling anyway", hookedBeadID, err)
		}
		// Force mode - proceed without the bead details
		hookedBead = &beads.Issue{ID: hookedBeadID, Title: "(unknown)"}
	}

	// Check if work is complete (warn if not, unless --force)
	isComplete := hookedBead.Status == "closed"
	if !isComplete && !unslingForce {
		return fmt.Errorf("hooked work %s is incomplete (%s)\n  Use --force to unsling anyway",
			hookedBeadID, hookedBead.Title)
	}

	if targetAgent != "" {
		fmt.Printf("%s Unslinging %s from %s...\n", style.Bold.Render("🪝"), hookedBeadID, agentID)
	} else {
		fmt.Printf("%s Unslinging %s...\n", style.Bold.Render("🪝"), hookedBeadID)
	}

	if unslingDryRun {
		fmt.Printf("Would clear hook_bead from agent bead %s\n", agentBeadID)
		return nil
	}

	// Clear the hook (gt-zecmc: removed agent_state update - observable from tmux)
	if err := b.ClearHookBead(agentBeadID); err != nil {
		return fmt.Errorf("clearing hook from agent bead %s: %w", agentBeadID, err)
	}

	// Log unhook event
	_ = events.LogFeed(events.TypeUnhook, agentID, events.UnhookPayload(hookedBeadID))

	fmt.Printf("%s Work removed from hook\n", style.Bold.Render("✓"))
	fmt.Printf("  Agent %s hook cleared (was: %s)\n", agentID, hookedBeadID)

	return nil
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
