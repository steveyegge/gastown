package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/style"
)

var hookCmd = &cobra.Command{
	Use:     "hook [bead-id]",
	GroupID: GroupWork,
	Short:   "Show or attach work on your hook",
	Long: `Show what's on your hook, or attach new work.

With no arguments, shows your current hook status (alias for 'gt mol status').
With a bead ID, attaches that work to your hook.

The hook is the "durability primitive" - work on your hook survives session
restarts, context compaction, and handoffs. When you restart (via gt handoff),
your SessionStart hook finds the attached work and you continue from where
you left off.

Examples:
  gt hook                           # Show what's on my hook
  gt hook status                    # Same as above
  gt hook gt-abc                    # Attach issue gt-abc to your hook
  gt hook gt-abc -s "Fix the bug"   # With subject for handoff mail
  gt hook gt-abc --if-empty         # Hook only if nothing hooked (idempotent)
  gt hook gt-abc --upsert           # Replace any existing hook (idempotent)

Related commands:
  gt sling <bead>    # Hook + start now (keep context)
  gt handoff <bead>  # Hook + restart (fresh context)
  gt unsling         # Remove work from hook`,
	Args: cobra.MaximumNArgs(1),
	RunE: runHookOrStatus,
}

// hookStatusCmd shows hook status (alias for mol status)
var hookStatusCmd = &cobra.Command{
	Use:   "status [target]",
	Short: "Show what's on your hook",
	Long: `Show what's slung on your hook.

This is an alias for 'gt mol status'. Shows what work is currently
attached to your hook, along with progress information.

Examples:
  gt hook status                    # Show my hook
  gt hook status greenplace/nux     # Show nux's hook`,
	Args: cobra.MaximumNArgs(1),
	RunE: runMoleculeStatus,
}

// hookShowCmd shows hook status in compact one-line format
var hookShowCmd = &cobra.Command{
	Use:   "show [agent]",
	Short: "Show what's on an agent's hook (compact)",
	Long: `Show what's on any agent's hook in compact one-line format.

With no argument, shows your own hook status (auto-detected from context).

Use cases:
- Mayor checking what polecats are working on
- Witness checking polecat status
- Debugging coordination issues
- Quick status overview

Examples:
  gt hook show                         # What's on MY hook? (auto-detect)
  gt hook show gastown/polecats/nux    # What's nux working on?
  gt hook show gastown/witness         # What's the witness hooked to?
  gt hook show mayor                   # What's the mayor working on?

Output format (one line):
  gastown/polecats/nux: gt-abc123 'Fix the widget bug' [in_progress]`,
	Args: cobra.MaximumNArgs(1),
	RunE: runHookShow,
}

var (
	hookSubject string
	hookMessage string
	hookDryRun  bool
	hookForce   bool
	hookIfEmpty bool
	hookUpsert  bool
)

func init() {
	// Flags for attaching work (gt hook <bead-id>)
	hookCmd.Flags().StringVarP(&hookSubject, "subject", "s", "", "Subject for handoff mail (optional)")
	hookCmd.Flags().StringVarP(&hookMessage, "message", "m", "", "Message for handoff mail (optional)")
	hookCmd.Flags().BoolVarP(&hookDryRun, "dry-run", "n", false, "Show what would be done")
	hookCmd.Flags().BoolVarP(&hookForce, "force", "f", false, "Replace existing incomplete hooked bead")
	hookCmd.Flags().BoolVar(&hookIfEmpty, "if-empty", false, "Only hook if empty, exit 0 either way")
	hookCmd.Flags().BoolVar(&hookUpsert, "upsert", false, "Replace existing hook, always succeed")

	// --json flag for status output (used when no args, i.e., gt hook --json)
	hookCmd.Flags().BoolVar(&moleculeJSON, "json", false, "Output as JSON (for status)")
	hookStatusCmd.Flags().BoolVar(&moleculeJSON, "json", false, "Output as JSON")
	hookShowCmd.Flags().BoolVar(&moleculeJSON, "json", false, "Output as JSON")
	hookCmd.AddCommand(hookStatusCmd)
	hookCmd.AddCommand(hookShowCmd)

	rootCmd.AddCommand(hookCmd)
}

// runHookOrStatus dispatches to status or hook based on args
func runHookOrStatus(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		// No args - show status
		return runMoleculeStatus(cmd, args)
	}
	// Has arg - attach work
	return runHook(cmd, args)
}

// Hook action constants for JSON output.
const (
	hookActionHooked   = "hooked"   // Successfully hooked the bead
	hookActionSkipped  = "skipped"  // Skipped because hook was occupied (--if-empty)
	hookActionReplaced = "replaced" // Replaced existing hook (--upsert)
)

// Hook skip reason constants for JSON output.
const (
	hookReasonOccupied = "hook_occupied" // Hook already has work attached
)

// hookResult represents the outcome of a hook operation for JSON output.
// Used with --json flag to provide machine-readable results for automation.
type hookResult struct {
	Action   string  `json:"action"`             // One of: hooked, skipped, replaced
	BeadID   string  `json:"bead_id"`            // The bead we attempted to hook
	Previous *string `json:"previous,omitempty"` // Previous bead ID (only for "replaced")
	Reason   string  `json:"reason,omitempty"`   // Skip reason (only for "skipped")
	Current  string  `json:"current,omitempty"`  // Current hooked bead (only for "skipped")
}

func runHook(_ *cobra.Command, args []string) error {
	beadID := args[0]

	// Validate flag combinations
	if hookIfEmpty && hookUpsert {
		return fmt.Errorf("--if-empty and --upsert are mutually exclusive")
	}
	if hookIfEmpty && hookForce {
		return fmt.Errorf("--if-empty and --force are mutually exclusive")
	}

	// Polecats cannot hook - they use gt done for lifecycle
	if polecatName := os.Getenv("GT_POLECAT"); polecatName != "" {
		return fmt.Errorf("polecats cannot hook work (use gt done for handoff)")
	}

	// Verify the bead exists
	if err := verifyBeadExists(beadID); err != nil {
		return err
	}

	// Determine agent identity
	agentID, _, _, err := resolveSelfTarget()
	if err != nil {
		return fmt.Errorf("detecting agent identity: %w", err)
	}

	// Find beads directory
	workDir, err := findLocalBeadsDir()
	if err != nil {
		return fmt.Errorf("not in a beads workspace: %w", err)
	}

	b := beads.New(workDir)

	// Check for existing hooked bead for this agent
	existingPinned, err := b.List(beads.ListOptions{
		Status:   beads.StatusHooked,
		Assignee: agentID,
		Priority: -1,
	})
	if err != nil {
		return fmt.Errorf("checking existing hooked beads: %w", err)
	}

	// If there's an existing hooked bead, handle based on flags
	if len(existingPinned) > 0 {
		existing := existingPinned[0]

		// Skip if it's the same bead we're trying to hook
		if existing.ID == beadID {
			if moleculeJSON {
				return outputHookResult(hookResult{Action: hookActionHooked, BeadID: beadID})
			}
			fmt.Printf("%s Already hooked: %s\n", style.Bold.Render("✓"), beadID)
			return nil
		}

		// --if-empty: Skip if hook is occupied, exit 0 (idempotent)
		if hookIfEmpty {
			if moleculeJSON {
				return outputHookResult(hookResult{
					Action:  hookActionSkipped,
					BeadID:  beadID,
					Reason:  hookReasonOccupied,
					Current: existing.ID,
				})
			}
			fmt.Printf("%s Hook occupied by %s, skipping\n", style.Dim.Render("ℹ"), existing.ID)
			return nil
		}

		// --upsert: Always replace existing hook (idempotent)
		if hookUpsert {
			if hookDryRun {
				fmt.Printf("Would unhook %s and hook %s\n", existing.ID, beadID)
				return nil
			}
			if err := unhookBead(b, existing); err != nil {
				return fmt.Errorf("unhooking existing bead %s: %w", existing.ID, err)
			}
			if err := doHook(beadID, agentID); err != nil {
				return err
			}
			prev := existing.ID
			if moleculeJSON {
				return outputHookResult(hookResult{Action: hookActionReplaced, BeadID: beadID, Previous: &prev})
			}
			fmt.Printf("%s Replaced %s with %s\n", style.Bold.Render("✓"), existing.ID, beadID)
			return nil
		}

		// Check if existing bead is complete (original behavior)
		isComplete, hasAttachment := checkPinnedBeadComplete(b, existing)

		if isComplete {
			// Auto-replace completed bead
			if !moleculeJSON {
				fmt.Printf("%s Replacing completed bead %s...\n", style.Dim.Render("ℹ"), existing.ID)
			}
			if !hookDryRun {
				if hasAttachment {
					// Close completed molecule bead (use bd close --force for pinned)
					closeArgs := []string{"close", existing.ID, "--force",
						"--reason=Auto-replaced by gt hook (molecule complete)"}
					if sessionID := runtime.SessionIDFromEnv(); sessionID != "" {
						closeArgs = append(closeArgs, "--session="+sessionID)
					}
					closeCmd := exec.Command("bd", closeArgs...)
					closeCmd.Stderr = os.Stderr
					if err := closeCmd.Run(); err != nil {
						return fmt.Errorf("closing completed bead %s: %w", existing.ID, err)
					}
				} else {
					// Naked bead - just unpin, don't close (might have value)
					status := "open"
					if err := b.Update(existing.ID, beads.UpdateOptions{Status: &status}); err != nil {
						return fmt.Errorf("unpinning bead %s: %w", existing.ID, err)
					}
				}
			}
		} else if hookForce {
			// Force replace incomplete bead
			if !moleculeJSON {
				fmt.Printf("%s Force-replacing incomplete bead %s...\n", style.Dim.Render("⚠"), existing.ID)
			}
			if !hookDryRun {
				// Unpin by setting status back to open
				status := "open"
				if err := b.Update(existing.ID, beads.UpdateOptions{Status: &status}); err != nil {
					return fmt.Errorf("unpinning bead %s: %w", existing.ID, err)
				}
			}
		} else {
			// Existing incomplete bead blocks new hook
			return fmt.Errorf("existing hooked bead %s is incomplete (%s)\n  Use --force to replace, or complete the existing work first",
				existing.ID, existing.Title)
		}
	}

	// Perform the hook
	if hookDryRun {
		fmt.Printf("Would run: bd update %s --status=hooked --assignee=%s\n", beadID, agentID)
		if hookSubject != "" {
			fmt.Printf("  subject (for handoff mail): %s\n", hookSubject)
		}
		if hookMessage != "" {
			fmt.Printf("  context (for handoff mail): %s\n", hookMessage)
		}
		return nil
	}

	if err := doHook(beadID, agentID); err != nil {
		return err
	}

	if moleculeJSON {
		return outputHookResult(hookResult{Action: hookActionHooked, BeadID: beadID})
	}

	fmt.Printf("%s Work attached to hook (hooked bead)\n", style.Bold.Render("✓"))
	fmt.Printf("  Use 'gt handoff' to restart with this work\n")
	fmt.Printf("  Use 'gt hook' to see hook status\n")

	return nil
}

// doHook performs the actual hook operation and logs the event.
// It uses the bd CLI for discovery-based bead routing.
func doHook(beadID, agentID string) error {
	hookCmd := exec.Command("bd", "update", beadID, "--status=hooked", "--assignee="+agentID)
	hookCmd.Stderr = os.Stderr
	if err := hookCmd.Run(); err != nil {
		return fmt.Errorf("hooking bead: %w", err)
	}

	// Log hook event to activity feed (non-fatal)
	if err := events.LogFeed(events.TypeHook, agentID, events.HookPayload(beadID)); err != nil {
		fmt.Fprintf(os.Stderr, "%s Warning: failed to log hook event: %v\n", style.Dim.Render("⚠"), err)
	}

	return nil
}

// unhookBead removes a bead from the hook by setting status back to open.
func unhookBead(b *beads.Beads, bead *beads.Issue) error {
	status := "open"
	return b.Update(bead.ID, beads.UpdateOptions{Status: &status})
}

// outputHookResult outputs the hook result as JSON.
func outputHookResult(result hookResult) error {
	enc := json.NewEncoder(os.Stdout)
	return enc.Encode(result)
}

// checkPinnedBeadComplete checks if a pinned bead's attached molecule is 100% complete.
// Returns (isComplete, hasAttachment):
// - isComplete=true if no molecule attached OR all molecule steps are closed
// - hasAttachment=true if there's an attached molecule
func checkPinnedBeadComplete(b *beads.Beads, issue *beads.Issue) (isComplete bool, hasAttachment bool) {
	// Check for attached molecule
	attachment := beads.ParseAttachmentFields(issue)
	if attachment == nil || attachment.AttachedMolecule == "" {
		// No molecule attached - consider complete (naked bead)
		return true, false
	}

	// Get progress of attached molecule
	progress, err := getMoleculeProgressInfo(b, attachment.AttachedMolecule)
	if err != nil {
		// Can't determine progress - be conservative, treat as incomplete
		return false, true
	}

	if progress == nil {
		// No steps found - might be a simple issue, treat as complete
		return true, true
	}

	return progress.Complete, true
}

// runHookShow displays another agent's hook in compact one-line format.
func runHookShow(cmd *cobra.Command, args []string) error {
	var target string
	if len(args) > 0 {
		target = args[0]
	} else {
		// Auto-detect current agent from context
		agentID, _, _, err := resolveSelfTarget()
		if err != nil {
			return fmt.Errorf("auto-detecting agent (use explicit argument): %w", err)
		}
		target = agentID
	}

	// Find beads directory
	workDir, err := findLocalBeadsDir()
	if err != nil {
		return fmt.Errorf("not in a beads workspace: %w", err)
	}

	b := beads.New(workDir)

	// Query for hooked beads assigned to the target
	hookedBeads, err := b.List(beads.ListOptions{
		Status:   beads.StatusHooked,
		Assignee: target,
		Priority: -1,
	})
	if err != nil {
		return fmt.Errorf("listing hooked beads: %w", err)
	}

	// If nothing found, try scanning all rigs for town-level roles
	if len(hookedBeads) == 0 && isTownLevelRole(target) {
		townRoot, err := findTownRoot()
		if err == nil && townRoot != "" {
			hookedBeads = scanAllRigsForHookedBeads(townRoot, target)
		}
	}

	// JSON output
	if moleculeJSON {
		type compactInfo struct {
			Agent  string `json:"agent"`
			BeadID string `json:"bead_id,omitempty"`
			Title  string `json:"title,omitempty"`
			Status string `json:"status"`
		}
		info := compactInfo{Agent: target}
		if len(hookedBeads) > 0 {
			info.BeadID = hookedBeads[0].ID
			info.Title = hookedBeads[0].Title
			info.Status = hookedBeads[0].Status
		} else {
			info.Status = "empty"
		}
		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(info)
	}

	// Compact one-line output
	if len(hookedBeads) == 0 {
		fmt.Printf("%s: (empty)\n", target)
		return nil
	}

	bead := hookedBeads[0]
	fmt.Printf("%s: %s '%s' [%s]\n", target, bead.ID, bead.Title, bead.Status)
	return nil
}

// findTownRoot finds the Gas Town root directory.
func findTownRoot() (string, error) {
	cmd := exec.Command("gt", "root")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
