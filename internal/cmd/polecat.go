package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/util"
	"github.com/steveyegge/gastown/internal/witness"
)

// Polecat command flags
var (
	polecatListJSON  bool
	polecatListAll   bool
	polecatForce     bool
	polecatRemoveAll bool
)

var polecatCmd = &cobra.Command{
	Use:     "polecat",
	Aliases: []string{"polecats"},
	GroupID: GroupAgents,
	Short:   "Manage polecats (ephemeral workers, one task then nuked)",
	RunE:    requireSubcommand,
	Long: `Manage polecat lifecycle in rigs.

Polecats are EPHEMERAL workers: spawned for one task, nuked when done.
There is NO idle state. A polecat is either:
  - Working: Actively doing assigned work
  - Stalled: Session crashed mid-work (needs Witness intervention)
  - Zombie: Finished but gt done failed (needs cleanup)

Self-cleaning model: When work completes, the polecat runs 'gt done',
which pushes the branch, submits to the merge queue, and exits. The
Witness then nukes the sandbox. Polecats don't wait for more work.

Session vs sandbox: The Claude session cycles frequently (handoffs,
compaction). The git worktree (sandbox) persists until nuke. Work
survives session restarts.

Cats build features. Dogs clean up messes.`,
}

var polecatListCmd = &cobra.Command{
	Use:   "list [rig]",
	Short: "List polecats in a rig",
	Long: `List polecats in a rig or all rigs.

In the transient model, polecats exist only while working. The list shows
all currently active polecats with their states:
  - working: Actively working on an issue
  - done: Completed work, waiting for cleanup
  - stuck: Needs assistance

Examples:
  gt polecat list greenplace
  gt polecat list --all
  gt polecat list greenplace --json`,
	RunE: runPolecatList,
}

var polecatAddCmd = &cobra.Command{
	Use:        "add <rig> <name>",
	Short:      "Add a new polecat to a rig (DEPRECATED)",
	Deprecated: "use 'gt polecat identity add' instead. This command will be removed in v1.0.",
	Long: `Add a new polecat to a rig.

DEPRECATED: Use 'gt polecat identity add' instead. This command will be removed in v1.0.

Creates a polecat directory, clones the rig repo, creates a work branch,
and initializes state.

Example:
  gt polecat identity add greenplace Toast  # Preferred
  gt polecat add greenplace Toast           # Deprecated`,
	Args: cobra.ExactArgs(2),
	RunE: runPolecatAdd,
}

var polecatRemoveCmd = &cobra.Command{
	Use:   "remove <rig>/<polecat>... | <rig> --all",
	Short: "Remove polecats from a rig",
	Long: `Remove one or more polecats from a rig.

Fails if session is running (stop first).
Warns if uncommitted changes exist.
Use --force to bypass checks.

Examples:
  gt polecat remove greenplace/Toast
  gt polecat remove greenplace/Toast greenplace/Furiosa
  gt polecat remove greenplace --all
  gt polecat remove greenplace --all --force`,
	Args: cobra.MinimumNArgs(1),
	RunE: runPolecatRemove,
}

var polecatSyncCmd = &cobra.Command{
	Use:   "sync <rig>/<polecat>",
	Short: "Sync beads for a polecat (deprecated with Dolt backend)",
	Long: `Sync beads for a polecat's worktree.

NOTE: With Dolt backend, beads changes are persisted immediately.
This command is a no-op when using Dolt.

Use --all to sync all polecats in a rig.
Use --from-main to only pull (no push).

Examples:
  gt polecat sync greenplace/Toast
  gt polecat sync greenplace --all
  gt polecat sync greenplace/Toast --from-main`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPolecatSync,
}

var polecatStatusCmd = &cobra.Command{
	Use:   "status <rig>/<polecat>",
	Short: "Show detailed status for a polecat",
	Long: `Show detailed status for a polecat.

Displays comprehensive information including:
  - Current lifecycle state (working, done, stuck, idle)
  - Assigned issue (if any)
  - Session status (running/stopped, attached/detached)
  - Session creation time
  - Last activity time

Examples:
  gt polecat status greenplace/Toast
  gt polecat status greenplace/Toast --json`,
	Args: cobra.ExactArgs(1),
	RunE: runPolecatStatus,
}

var (
	polecatSyncAll           bool
	polecatSyncFromMain      bool
	polecatStatusJSON        bool
	polecatGitStateJSON      bool
	polecatGCDryRun          bool
	polecatNukeAll           bool
	polecatNukeDryRun        bool
	polecatNukeForce         bool
	polecatCheckRecoveryJSON bool
)

var polecatGCCmd = &cobra.Command{
	Use:   "gc <rig>",
	Short: "Garbage collect stale polecat branches",
	Long: `Garbage collect stale polecat branches in a rig.

Polecats use unique timestamped branches (polecat/<name>-<timestamp>) to
prevent drift issues. Over time, these branches accumulate when stale
polecats are repaired.

This command removes orphaned branches:
  - Branches for polecats that no longer exist
  - Old timestamped branches (keeps only the current one per polecat)

Examples:
  gt polecat gc greenplace
  gt polecat gc greenplace --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runPolecatGC,
}

var polecatNukeCmd = &cobra.Command{
	Use:   "nuke <rig>/<polecat>... | <rig> --all",
	Short: "Completely destroy a polecat (session, worktree, branch, agent bead)",
	Long: `Completely destroy a polecat and all its artifacts.

This is the nuclear option for post-merge cleanup. It:
  1. Kills the Claude session (if running)
  2. Deletes the git worktree (bypassing all safety checks)
  3. Deletes the polecat branch
  4. Closes the agent bead (if exists)

SAFETY CHECKS: The command refuses to nuke a polecat if:
  - Worktree has unpushed/uncommitted changes
  - Polecat has an open merge request (MR bead)
  - Polecat has work on its hook

Use --force to bypass safety checks (LOSES WORK).
Use --dry-run to see what would happen and safety check status.

Examples:
  gt polecat nuke greenplace/Toast
  gt polecat nuke greenplace/Toast greenplace/Furiosa
  gt polecat nuke greenplace --all
  gt polecat nuke greenplace --all --dry-run
  gt polecat nuke greenplace/Toast --force  # bypass safety checks`,
	Args: cobra.MinimumNArgs(1),
	RunE: runPolecatNuke,
}

var polecatGitStateCmd = &cobra.Command{
	Use:   "git-state <rig>/<polecat>",
	Short: "Show git state for pre-kill verification",
	Long: `Show git state for a polecat's worktree.

Used by the Witness for pre-kill verification to ensure no work is lost.
Returns whether the worktree is clean (safe to kill) or dirty (needs cleanup).

Checks:
  - Working tree: uncommitted changes
  - Unpushed commits: commits ahead of origin/main
  - Stashes: stashed changes

Examples:
  gt polecat git-state greenplace/Toast
  gt polecat git-state greenplace/Toast --json`,
	Args: cobra.ExactArgs(1),
	RunE: runPolecatGitState,
}

var polecatCheckRecoveryCmd = &cobra.Command{
	Use:   "check-recovery <rig>/<polecat>",
	Short: "Check if polecat needs recovery vs safe to nuke",
	Long: `Check recovery status of a polecat based on cleanup_status in agent bead.

Used by the Witness to determine appropriate cleanup action:
  - SAFE_TO_NUKE: cleanup_status is 'clean' - no work at risk
  - NEEDS_RECOVERY: cleanup_status indicates unpushed/uncommitted work

This prevents accidental data loss when cleaning up dormant polecats.
The Witness should escalate NEEDS_RECOVERY cases to the Mayor.

Examples:
  gt polecat check-recovery greenplace/Toast
  gt polecat check-recovery greenplace/Toast --json`,
	Args: cobra.ExactArgs(1),
	RunE: runPolecatCheckRecovery,
}

var (
	polecatStaleJSON      bool
	polecatStaleThreshold int
	polecatStaleCleanup   bool
)

var (
	polecatOrphansJSON    bool
	polecatOrphansRespawn bool
)

var polecatStaleCmd = &cobra.Command{
	Use:   "stale <rig>",
	Short: "Detect stale polecats that may need cleanup",
	Long: `Detect stale polecats in a rig that are candidates for cleanup.

A polecat is considered stale if:
  - No active tmux session
  - Way behind main (>threshold commits) OR no agent bead
  - Has no uncommitted work that could be lost

The default threshold is 20 commits behind main.

Use --cleanup to automatically nuke stale polecats that are safe to remove.
Use --dry-run with --cleanup to see what would be cleaned.

Examples:
  gt polecat stale greenplace
  gt polecat stale greenplace --threshold 50
  gt polecat stale greenplace --json
  gt polecat stale greenplace --cleanup
  gt polecat stale greenplace --cleanup --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runPolecatStale,
}

var polecatOrphansCmd = &cobra.Command{
	Use:   "orphans <rig>",
	Short: "Detect polecats with hooked work but no active session",
	Long: `Detect orphaned polecats that have hooked work but no running tmux session.

These are polecats whose sessions died (API errors, crashes, etc.) but still have
work assigned to them. Without intervention, this work remains orphaned.

Use --respawn to automatically respawn these polecats to process their hooked work.

Examples:
  gt polecat orphans gastown               # Detect orphans
  gt polecat orphans gastown --json        # JSON output
  gt polecat orphans gastown --respawn     # Detect and respawn`,
	Args: cobra.ExactArgs(1),
	RunE: runPolecatOrphans,
}

func init() {
	// List flags
	polecatListCmd.Flags().BoolVar(&polecatListJSON, "json", false, "Output as JSON")
	polecatListCmd.Flags().BoolVar(&polecatListAll, "all", false, "List polecats in all rigs")

	// Remove flags
	polecatRemoveCmd.Flags().BoolVarP(&polecatForce, "force", "f", false, "Force removal, bypassing checks")
	polecatRemoveCmd.Flags().BoolVar(&polecatRemoveAll, "all", false, "Remove all polecats in the rig")

	// Sync flags
	polecatSyncCmd.Flags().BoolVar(&polecatSyncAll, "all", false, "Sync all polecats in the rig")
	polecatSyncCmd.Flags().BoolVar(&polecatSyncFromMain, "from-main", false, "Pull only, no push")

	// Status flags
	polecatStatusCmd.Flags().BoolVar(&polecatStatusJSON, "json", false, "Output as JSON")

	// Git-state flags
	polecatGitStateCmd.Flags().BoolVar(&polecatGitStateJSON, "json", false, "Output as JSON")

	// GC flags
	polecatGCCmd.Flags().BoolVar(&polecatGCDryRun, "dry-run", false, "Show what would be deleted without deleting")

	// Nuke flags
	polecatNukeCmd.Flags().BoolVar(&polecatNukeAll, "all", false, "Nuke all polecats in the rig")
	polecatNukeCmd.Flags().BoolVar(&polecatNukeDryRun, "dry-run", false, "Show what would be nuked without doing it")
	polecatNukeCmd.Flags().BoolVarP(&polecatNukeForce, "force", "f", false, "Force nuke, bypassing all safety checks (LOSES WORK)")

	// Check-recovery flags
	polecatCheckRecoveryCmd.Flags().BoolVar(&polecatCheckRecoveryJSON, "json", false, "Output as JSON")

	// Stale flags
	polecatStaleCmd.Flags().BoolVar(&polecatStaleJSON, "json", false, "Output as JSON")
	polecatStaleCmd.Flags().IntVar(&polecatStaleThreshold, "threshold", 20, "Commits behind main to consider stale")
	polecatStaleCmd.Flags().BoolVar(&polecatStaleCleanup, "cleanup", false, "Automatically nuke stale polecats")

	// Orphans flags
	polecatOrphansCmd.Flags().BoolVar(&polecatOrphansJSON, "json", false, "Output as JSON")
	polecatOrphansCmd.Flags().BoolVar(&polecatOrphansRespawn, "respawn", false, "Automatically respawn orphaned polecats")

	// Add subcommands
	polecatCmd.AddCommand(polecatListCmd)
	polecatCmd.AddCommand(polecatAddCmd)
	polecatCmd.AddCommand(polecatRemoveCmd)
	polecatCmd.AddCommand(polecatSyncCmd)
	polecatCmd.AddCommand(polecatStatusCmd)
	polecatCmd.AddCommand(polecatGitStateCmd)
	polecatCmd.AddCommand(polecatCheckRecoveryCmd)
	polecatCmd.AddCommand(polecatGCCmd)
	polecatCmd.AddCommand(polecatNukeCmd)
	polecatCmd.AddCommand(polecatStaleCmd)
	polecatCmd.AddCommand(polecatOrphansCmd)

	rootCmd.AddCommand(polecatCmd)
}

// PolecatListItem represents a polecat in list output.
type PolecatListItem struct {
	Rig            string        `json:"rig"`
	Name           string        `json:"name"`
	State          polecat.State `json:"state"`
	Issue          string        `json:"issue,omitempty"`
	SessionRunning bool          `json:"session_running"`
}

// getPolecatManager creates a polecat manager for the given rig.
func getPolecatManager(rigName string) (*polecat.Manager, *rig.Rig, error) {
	_, r, err := getRig(rigName)
	if err != nil {
		return nil, nil, err
	}

	polecatGit := git.NewGit(r.Path)
	t := tmux.NewTmux()
	mgr := polecat.NewManager(r, polecatGit, t)

	return mgr, r, nil
}

func runPolecatList(cmd *cobra.Command, args []string) error {
	var rigs []*rig.Rig

	if polecatListAll {
		// List all rigs
		allRigs, _, err := getAllRigs()
		if err != nil {
			return err
		}
		rigs = allRigs
	} else {
		// Need a rig name
		if len(args) < 1 {
			return fmt.Errorf("rig name required (or use --all)")
		}
		_, r, err := getPolecatManager(args[0])
		if err != nil {
			return err
		}
		rigs = []*rig.Rig{r}
	}

	// Collect polecats from all rigs
	t := tmux.NewTmux()
	var allPolecats []PolecatListItem

	for _, r := range rigs {
		polecatGit := git.NewGit(r.Path)
		mgr := polecat.NewManager(r, polecatGit, t)
		polecatMgr := polecat.NewSessionManager(t, r)

		polecats, err := mgr.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to list polecats in %s: %v\n", r.Name, err)
			continue
		}

		for _, p := range polecats {
			running, _ := polecatMgr.IsRunning(p.Name)
			allPolecats = append(allPolecats, PolecatListItem{
				Rig:            r.Name,
				Name:           p.Name,
				State:          p.State,
				Issue:          p.Issue,
				SessionRunning: running,
			})
		}
	}

	// Output
	if polecatListJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(allPolecats)
	}

	if len(allPolecats) == 0 {
		fmt.Println("No active polecats found.")
		return nil
	}

	fmt.Printf("%s\n\n", style.Bold.Render("Active Polecats"))
	for _, p := range allPolecats {
		// Session indicator
		sessionStatus := style.Dim.Render("○")
		if p.SessionRunning {
			sessionStatus = style.Success.Render("●")
		}

		// Display actual state (no normalization - idle means idle)
		displayState := p.State

		// State color
		stateStr := string(displayState)
		switch displayState {
		case polecat.StateWorking:
			stateStr = style.Info.Render(stateStr)
		case polecat.StateStuck:
			stateStr = style.Warning.Render(stateStr)
		case polecat.StateDone:
			stateStr = style.Success.Render(stateStr)
		default:
			stateStr = style.Dim.Render(stateStr)
		}

		fmt.Printf("  %s %s/%s  %s\n", sessionStatus, p.Rig, p.Name, stateStr)
		if p.Issue != "" {
			fmt.Printf("    %s\n", style.Dim.Render(p.Issue))
		}
	}

	return nil
}

func runPolecatAdd(cmd *cobra.Command, args []string) error {
	// Emit deprecation warning
	fmt.Fprintf(os.Stderr, "%s 'gt polecat add' is deprecated. Use 'gt polecat identity add' instead.\n",
		style.Warning.Render("Warning:"))
	fmt.Fprintf(os.Stderr, "         This command will be removed in v1.0.\n\n")

	rigName := args[0]
	polecatName := args[1]

	mgr, _, err := getPolecatManager(rigName)
	if err != nil {
		return err
	}

	fmt.Printf("Adding polecat %s to rig %s...\n", polecatName, rigName)

	p, err := mgr.Add(polecatName)
	if err != nil {
		return fmt.Errorf("adding polecat: %w", err)
	}

	fmt.Printf("%s Polecat %s added.\n", style.SuccessPrefix, p.Name)
	fmt.Printf("  %s\n", style.Dim.Render(p.ClonePath))
	fmt.Printf("  Branch: %s\n", style.Dim.Render(p.Branch))

	return nil
}

func runPolecatRemove(cmd *cobra.Command, args []string) error {
	targets, err := resolvePolecatTargets(args, polecatRemoveAll)
	if err != nil {
		return err
	}

	if len(targets) == 0 {
		fmt.Println("No polecats to remove.")
		return nil
	}

	// Remove each polecat
	t := tmux.NewTmux()
	var removeErrors []string
	removed := 0

	for _, p := range targets {
		// Check if session is running
		if !polecatForce {
			polecatMgr := polecat.NewSessionManager(t, p.r)
			running, _ := polecatMgr.IsRunning(p.polecatName)
			if running {
				removeErrors = append(removeErrors, fmt.Sprintf("%s/%s: session is running (stop first or use --force)", p.rigName, p.polecatName))
				continue
			}
		}

		fmt.Printf("Removing polecat %s/%s...\n", p.rigName, p.polecatName)

		if err := p.mgr.Remove(p.polecatName, polecatForce); err != nil {
			if errors.Is(err, polecat.ErrHasChanges) {
				removeErrors = append(removeErrors, fmt.Sprintf("%s/%s: has uncommitted changes (use --force)", p.rigName, p.polecatName))
			} else {
				removeErrors = append(removeErrors, fmt.Sprintf("%s/%s: %v", p.rigName, p.polecatName, err))
			}
			continue
		}

		fmt.Printf("  %s removed\n", style.Success.Render("✓"))
		removed++
	}

	// Report results
	if len(removeErrors) > 0 {
		fmt.Printf("\n%s Some removals failed:\n", style.Warning.Render("Warning:"))
		for _, e := range removeErrors {
			fmt.Printf("  - %s\n", e)
		}
	}

	if removed > 0 {
		fmt.Printf("\n%s Removed %d polecat(s).\n", style.SuccessPrefix, removed)
	}

	if len(removeErrors) > 0 {
		return fmt.Errorf("%d removal(s) failed", len(removeErrors))
	}

	return nil
}

func runPolecatSync(cmd *cobra.Command, args []string) error {
	// With Dolt backend, beads changes are persisted immediately - no sync needed
	fmt.Println("Note: With Dolt backend, beads changes are persisted immediately.")
	fmt.Println("No sync step is required.")
	return nil
}

// PolecatStatus represents detailed polecat status for JSON output.
type PolecatStatus struct {
	Rig            string        `json:"rig"`
	Name           string        `json:"name"`
	State          polecat.State `json:"state"`
	Issue          string        `json:"issue,omitempty"`
	ClonePath      string        `json:"clone_path"`
	Branch         string        `json:"branch"`
	SessionRunning bool          `json:"session_running"`
	SessionID      string        `json:"session_id,omitempty"`
	Attached       bool          `json:"attached,omitempty"`
	Windows        int           `json:"windows,omitempty"`
	CreatedAt      string        `json:"created_at,omitempty"`
	LastActivity   string        `json:"last_activity,omitempty"`
}

func runPolecatStatus(cmd *cobra.Command, args []string) error {
	rigName, polecatName, err := parseAddress(args[0])
	if err != nil {
		return err
	}

	mgr, r, err := getPolecatManager(rigName)
	if err != nil {
		return err
	}

	// Get polecat info
	p, err := mgr.Get(polecatName)
	if err != nil {
		return fmt.Errorf("polecat '%s' not found in rig '%s'", polecatName, rigName)
	}

	// Get session info
	t := tmux.NewTmux()
	polecatMgr := polecat.NewSessionManager(t, r)
	sessInfo, err := polecatMgr.Status(polecatName)
	if err != nil {
		// Non-fatal - continue without session info
		sessInfo = &polecat.SessionInfo{
			Polecat: polecatName,
			Running: false,
		}
	}

	// JSON output
	if polecatStatusJSON {
		status := PolecatStatus{
			Rig:            rigName,
			Name:           polecatName,
			State:          p.State,
			Issue:          p.Issue,
			ClonePath:      p.ClonePath,
			Branch:         p.Branch,
			SessionRunning: sessInfo.Running,
			SessionID:      sessInfo.SessionID,
			Attached:       sessInfo.Attached,
			Windows:        sessInfo.Windows,
		}
		if !sessInfo.Created.IsZero() {
			status.CreatedAt = sessInfo.Created.Format("2006-01-02 15:04:05")
		}
		if !sessInfo.LastActivity.IsZero() {
			status.LastActivity = sessInfo.LastActivity.Format("2006-01-02 15:04:05")
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	// Human-readable output
	fmt.Printf("%s\n\n", style.Bold.Render(fmt.Sprintf("Polecat: %s/%s", rigName, polecatName)))

	// State with color
	stateStr := string(p.State)
	switch p.State {
	case polecat.StateWorking:
		stateStr = style.Info.Render(stateStr)
	case polecat.StateStuck:
		stateStr = style.Warning.Render(stateStr)
	case polecat.StateDone:
		stateStr = style.Success.Render(stateStr)
	default:
		stateStr = style.Dim.Render(stateStr)
	}
	fmt.Printf("  State:         %s\n", stateStr)

	// Issue
	if p.Issue != "" {
		fmt.Printf("  Issue:         %s\n", p.Issue)
	} else {
		fmt.Printf("  Issue:         %s\n", style.Dim.Render("(none)"))
	}

	// Clone path and branch
	fmt.Printf("  Clone:         %s\n", style.Dim.Render(p.ClonePath))
	fmt.Printf("  Branch:        %s\n", style.Dim.Render(p.Branch))

	// Session info
	fmt.Println()
	fmt.Printf("%s\n", style.Bold.Render("Session"))

	if sessInfo.Running {
		fmt.Printf("  Status:        %s\n", style.Success.Render("running"))
		fmt.Printf("  Session ID:    %s\n", style.Dim.Render(sessInfo.SessionID))

		if sessInfo.Attached {
			fmt.Printf("  Attached:      %s\n", style.Info.Render("yes"))
		} else {
			fmt.Printf("  Attached:      %s\n", style.Dim.Render("no"))
		}

		if sessInfo.Windows > 0 {
			fmt.Printf("  Windows:       %d\n", sessInfo.Windows)
		}

		if !sessInfo.Created.IsZero() {
			fmt.Printf("  Created:       %s\n", sessInfo.Created.Format("2006-01-02 15:04:05"))
		}

		if !sessInfo.LastActivity.IsZero() {
			// Show relative time for activity
			ago := formatActivityTime(sessInfo.LastActivity)
			fmt.Printf("  Last Activity: %s (%s)\n",
				sessInfo.LastActivity.Format("15:04:05"),
				style.Dim.Render(ago))
		}
	} else {
		fmt.Printf("  Status:        %s\n", style.Dim.Render("not running"))
	}

	return nil
}

// formatActivityTime returns a human-readable relative time string.
func formatActivityTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d seconds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}

// GitState represents the git state of a polecat's worktree.
type GitState struct {
	Clean            bool     `json:"clean"`
	UncommittedFiles []string `json:"uncommitted_files"`
	UnpushedCommits  int      `json:"unpushed_commits"`
	StashCount       int      `json:"stash_count"`
}

func runPolecatGitState(cmd *cobra.Command, args []string) error {
	rigName, polecatName, err := parseAddress(args[0])
	if err != nil {
		return err
	}

	mgr, r, err := getPolecatManager(rigName)
	if err != nil {
		return err
	}

	// Verify polecat exists
	p, err := mgr.Get(polecatName)
	if err != nil {
		return fmt.Errorf("polecat '%s' not found in rig '%s'", polecatName, rigName)
	}

	// Get git state from the polecat's worktree
	state, err := getGitState(p.ClonePath)
	if err != nil {
		return fmt.Errorf("getting git state: %w", err)
	}

	// JSON output
	if polecatGitStateJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(state)
	}

	// Human-readable output
	fmt.Printf("%s\n\n", style.Bold.Render(fmt.Sprintf("Git State: %s/%s", r.Name, polecatName)))

	// Working tree status
	if len(state.UncommittedFiles) == 0 {
		fmt.Printf("  Working Tree:  %s\n", style.Success.Render("clean"))
	} else {
		fmt.Printf("  Working Tree:  %s\n", style.Warning.Render("dirty"))
		fmt.Printf("  Uncommitted:   %s\n", style.Warning.Render(fmt.Sprintf("%d files", len(state.UncommittedFiles))))
		for _, f := range state.UncommittedFiles {
			fmt.Printf("                 %s\n", style.Dim.Render(f))
		}
	}

	// Unpushed commits
	if state.UnpushedCommits == 0 {
		fmt.Printf("  Unpushed:      %s\n", style.Success.Render("0 commits"))
	} else {
		fmt.Printf("  Unpushed:      %s\n", style.Warning.Render(fmt.Sprintf("%d commits ahead", state.UnpushedCommits)))
	}

	// Stashes
	if state.StashCount == 0 {
		fmt.Printf("  Stashes:       %s\n", style.Dim.Render("0"))
	} else {
		fmt.Printf("  Stashes:       %s\n", style.Warning.Render(fmt.Sprintf("%d", state.StashCount)))
	}

	// Verdict
	fmt.Println()
	if state.Clean {
		fmt.Printf("  Verdict:       %s\n", style.Success.Render("CLEAN (safe to kill)"))
	} else {
		fmt.Printf("  Verdict:       %s\n", style.Error.Render("DIRTY (needs cleanup)"))
	}

	return nil
}

// getGitState checks the git state of a worktree.
func getGitState(worktreePath string) (*GitState, error) {
	state := &GitState{
		Clean:            true,
		UncommittedFiles: []string{},
	}

	// Check for uncommitted changes (git status --porcelain)
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = worktreePath
	output, err := statusCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}
	if len(output) > 0 {
		lines := splitLines(string(output))
		for _, line := range lines {
			if line != "" {
				// Extract filename (skip the status prefix)
				if len(line) > 3 {
					state.UncommittedFiles = append(state.UncommittedFiles, line[3:])
				} else {
					state.UncommittedFiles = append(state.UncommittedFiles, line)
				}
			}
		}
		state.Clean = false
	}

	// Check for unpushed commits.
	// For branches without upstream (like polecat branches), check against origin/<branch>
	// if it exists, otherwise against origin/<default-branch>.
	// After squash merge, commits may differ but content may be identical.

	// Get current branch
	branchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	branchCmd.Dir = worktreePath
	branchOutput, err := branchCmd.Output()
	currentBranch := strings.TrimSpace(string(branchOutput))

	// Determine the correct reference to compare against
	var compareRef string
	if err == nil && currentBranch != "" && currentBranch != "HEAD" {
		// Check if origin/<branch> exists (for pushed polecat branches)
		remoteBranch := "origin/" + currentBranch
		checkCmd := exec.Command("git", "rev-parse", "--verify", remoteBranch)
		checkCmd.Dir = worktreePath
		if checkErr := checkCmd.Run(); checkErr == nil {
			// Remote branch exists - use it
			compareRef = remoteBranch
		}
	}

	// Fall back to origin/main or origin/master if no remote branch
	if compareRef == "" {
		compareRef = "origin/main"
		checkCmd := exec.Command("git", "rev-parse", "--verify", compareRef)
		checkCmd.Dir = worktreePath
		if checkCmd.Run() != nil {
			compareRef = "origin/master"
		}
	}

	logCmd := exec.Command("git", "log", compareRef+"..HEAD", "--oneline")
	logCmd.Dir = worktreePath
	output, _ = logCmd.Output() // non-fatal: might be a new repo without remote tracking

	if len(output) > 0 {
		lines := splitLines(string(output))
		count := 0
		for _, line := range lines {
			if line != "" {
				count++
			}
		}
		if count > 0 {
			// Commits exist that aren't on the compare ref. But after squash merge,
			// the content may actually be on main with different commit SHAs.
			// Check if there's any actual diff between HEAD and the compare ref.
			diffCmd := exec.Command("git", "diff", compareRef, "HEAD", "--quiet")
			diffCmd.Dir = worktreePath
			diffErr := diffCmd.Run()
			if diffErr == nil {
				// Exit code 0 means no diff - content IS on main (squash merged)
				// Don't count these as unpushed
				state.UnpushedCommits = 0
			} else {
				// Exit code 1 means there's a diff - truly unpushed work
				state.UnpushedCommits = count
				state.Clean = false
			}
		}
	}

	// Check for stashes (git stash list)
	stashCmd := exec.Command("git", "stash", "list")
	stashCmd.Dir = worktreePath
	output, err = stashCmd.Output()
	if err != nil {
		// Ignore stash errors
		output = nil
	}
	if len(output) > 0 {
		lines := splitLines(string(output))
		count := 0
		for _, line := range lines {
			if line != "" {
				count++
			}
		}
		state.StashCount = count
		if count > 0 {
			state.Clean = false
		}
	}

	return state, nil
}

// RecoveryStatus represents whether a polecat needs recovery or is safe to nuke.
type RecoveryStatus struct {
	Rig           string                `json:"rig"`
	Polecat       string                `json:"polecat"`
	CleanupStatus polecat.CleanupStatus `json:"cleanup_status"`
	NeedsRecovery bool                  `json:"needs_recovery"`
	Verdict       string                `json:"verdict"` // SAFE_TO_NUKE or NEEDS_RECOVERY
	Branch        string                `json:"branch,omitempty"`
	Issue         string                `json:"issue,omitempty"`
}

func runPolecatCheckRecovery(cmd *cobra.Command, args []string) error {
	rigName, polecatName, err := parseAddress(args[0])
	if err != nil {
		return err
	}

	mgr, r, err := getPolecatManager(rigName)
	if err != nil {
		return err
	}

	// Verify polecat exists and get info
	p, err := mgr.Get(polecatName)
	if err != nil {
		return fmt.Errorf("polecat '%s' not found in rig '%s'", polecatName, rigName)
	}

	// Get cleanup_status from agent bead
	// We need to read it directly from beads since manager doesn't expose it
	rigPath := r.Path
	bd := beads.New(rigPath)
	agentBeadID := polecatBeadIDForRig(r, rigName, polecatName)
	_, fields, err := bd.GetAgentBead(agentBeadID)

	status := RecoveryStatus{
		Rig:     rigName,
		Polecat: polecatName,
		Branch:  p.Branch,
		Issue:   p.Issue,
	}

	if err != nil || fields == nil {
		// No agent bead or no cleanup_status - fall back to git check
		// This handles polecats that haven't self-reported yet
		gitState, gitErr := getGitState(p.ClonePath)
		if gitErr != nil {
			status.CleanupStatus = polecat.CleanupUnknown
			status.NeedsRecovery = true
			status.Verdict = "NEEDS_RECOVERY"
		} else if gitState.Clean {
			status.CleanupStatus = polecat.CleanupClean
			status.NeedsRecovery = false
			status.Verdict = "SAFE_TO_NUKE"
		} else if gitState.UnpushedCommits > 0 {
			status.CleanupStatus = polecat.CleanupUnpushed
			status.NeedsRecovery = true
			status.Verdict = "NEEDS_RECOVERY"
		} else if gitState.StashCount > 0 {
			status.CleanupStatus = polecat.CleanupStash
			status.NeedsRecovery = true
			status.Verdict = "NEEDS_RECOVERY"
		} else {
			status.CleanupStatus = polecat.CleanupUncommitted
			status.NeedsRecovery = true
			status.Verdict = "NEEDS_RECOVERY"
		}
	} else {
		// Use cleanup_status from agent bead
		status.CleanupStatus = polecat.CleanupStatus(fields.CleanupStatus)
		if status.CleanupStatus.IsSafe() {
			status.NeedsRecovery = false
			status.Verdict = "SAFE_TO_NUKE"
		} else {
			// RequiresRecovery covers uncommitted, stash, unpushed
			// Unknown/empty also treated conservatively
			status.NeedsRecovery = true
			status.Verdict = "NEEDS_RECOVERY"
		}
	}

	// JSON output
	if polecatCheckRecoveryJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	// Human-readable output
	fmt.Printf("%s\n\n", style.Bold.Render(fmt.Sprintf("Recovery Status: %s/%s", rigName, polecatName)))
	fmt.Printf("  Cleanup Status:  %s\n", status.CleanupStatus)
	if status.Branch != "" {
		fmt.Printf("  Branch:          %s\n", status.Branch)
	}
	if status.Issue != "" {
		fmt.Printf("  Issue:           %s\n", status.Issue)
	}
	fmt.Println()

	if status.NeedsRecovery {
		fmt.Printf("  Verdict:         %s\n", style.Error.Render("NEEDS_RECOVERY"))
		fmt.Println()
		fmt.Printf("  %s This polecat has unpushed/uncommitted work.\n", style.Warning.Render("⚠"))
		fmt.Println("  Escalate to Mayor for recovery before cleanup.")
	} else {
		fmt.Printf("  Verdict:         %s\n", style.Success.Render("SAFE_TO_NUKE"))
		fmt.Println()
		fmt.Printf("  %s Safe to nuke - no work at risk.\n", style.Success.Render("✓"))
	}

	return nil
}

func runPolecatGC(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	mgr, r, err := getPolecatManager(rigName)
	if err != nil {
		return err
	}

	fmt.Printf("Garbage collecting stale polecat branches in %s...\n\n", r.Name)

	if polecatGCDryRun {
		// Dry run - list branches that would be deleted
		repoGit := git.NewGit(r.Path)

		// List all polecat branches
		branches, err := repoGit.ListBranches("polecat/*")
		if err != nil {
			return fmt.Errorf("listing branches: %w", err)
		}

		if len(branches) == 0 {
			fmt.Println("No polecat branches found.")
			return nil
		}

		// Get current branches
		polecats, err := mgr.List()
		if err != nil {
			return fmt.Errorf("listing polecats: %w", err)
		}

		currentBranches := make(map[string]bool)
		for _, p := range polecats {
			currentBranches[p.Branch] = true
		}

		// Show what would be deleted
		toDelete := 0
		for _, branch := range branches {
			if !currentBranches[branch] {
				fmt.Printf("  Would delete: %s\n", style.Dim.Render(branch))
				toDelete++
			} else {
				fmt.Printf("  Keep (in use): %s\n", style.Success.Render(branch))
			}
		}

		fmt.Printf("\nWould delete %d branch(es), keep %d\n", toDelete, len(branches)-toDelete)
		return nil
	}

	// Actually clean up
	deleted, err := mgr.CleanupStaleBranches()
	if err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	if deleted == 0 {
		fmt.Println("No stale branches to clean up.")
	} else {
		fmt.Printf("%s Deleted %d stale branch(es).\n", style.SuccessPrefix, deleted)
	}

	return nil
}

// splitLines splits a string into non-empty lines.
func splitLines(s string) []string {
	var lines []string
	for _, line := range filepath.SplitList(s) {
		if line != "" {
			lines = append(lines, line)
		}
	}
	// filepath.SplitList doesn't work for newlines, use strings.Split instead
	lines = nil
	for _, line := range strings.Split(s, "\n") {
		lines = append(lines, line)
	}
	return lines
}

func runPolecatNuke(cmd *cobra.Command, args []string) error {
	targets, err := resolvePolecatTargets(args, polecatNukeAll)
	if err != nil {
		return err
	}

	if len(targets) == 0 {
		fmt.Println("No polecats to nuke.")
		return nil
	}

	// Safety checks: refuse to nuke polecats with active work unless --force is set
	if !polecatNukeForce && !polecatNukeDryRun {
		var blocked []*SafetyCheckResult
		for _, p := range targets {
			result := checkPolecatSafety(p)
			if result.Blocked {
				blocked = append(blocked, result)
			}
		}

		if len(blocked) > 0 {
			displaySafetyCheckBlocked(blocked)
			return fmt.Errorf("blocked: %d polecat(s) have active work", len(blocked))
		}
	}

	// Nuke each polecat
	t := tmux.NewTmux()
	var nukeErrors []string
	nuked := 0

	for _, p := range targets {
		if polecatNukeDryRun {
			fmt.Printf("Would nuke %s/%s:\n", p.rigName, p.polecatName)
			fmt.Printf("  - Kill session: gt-%s-%s\n", p.rigName, p.polecatName)
			fmt.Printf("  - Delete worktree: %s/polecats/%s\n", p.r.Path, p.polecatName)
			fmt.Printf("  - Reject any open MRs for the branch\n")
			fmt.Printf("  - Delete branch (if exists)\n")
			fmt.Printf("  - Close agent bead: %s\n", polecatBeadIDForRig(p.r, p.rigName, p.polecatName))

			displayDryRunSafetyCheck(p)
			fmt.Println()
			continue
		}

		if polecatNukeForce {
			fmt.Printf("%s Nuking %s/%s (--force)...\n", style.Warning.Render("⚠"), p.rigName, p.polecatName)
		} else {
			fmt.Printf("Nuking %s/%s...\n", p.rigName, p.polecatName)
		}

		// Step 1: Kill session (force mode - no graceful shutdown)
		polecatMgr := polecat.NewSessionManager(t, p.r)
		running, _ := polecatMgr.IsRunning(p.polecatName)
		if running {
			if err := polecatMgr.Stop(p.polecatName, true); err != nil {
				fmt.Printf("  %s session kill failed: %v\n", style.Warning.Render("⚠"), err)
				// Continue anyway - worktree removal will still work
			} else {
				fmt.Printf("  %s killed session\n", style.Success.Render("✓"))
			}
		}

		// Step 2: Get polecat info before deletion (for branch name)
		polecatInfo, err := p.mgr.Get(p.polecatName)
		var branchToDelete string
		if err == nil && polecatInfo != nil {
			branchToDelete = polecatInfo.Branch
		}

		// Step 3: Delete worktree (nuclear mode - bypass all safety checks)
		if err := p.mgr.RemoveWithOptions(p.polecatName, true, true); err != nil {
			if errors.Is(err, polecat.ErrPolecatNotFound) {
				fmt.Printf("  %s worktree already gone\n", style.Dim.Render("○"))
			} else {
				nukeErrors = append(nukeErrors, fmt.Sprintf("%s/%s: worktree removal failed: %v", p.rigName, p.polecatName, err))
				continue
			}
		} else {
			fmt.Printf("  %s deleted worktree\n", style.Success.Render("✓"))
		}

		// Step 4: Reject any open MRs for this branch before deleting it
		// This prevents MQ/git sync inconsistency where MR exists but branch is gone
		if branchToDelete != "" {
			mayorRigPath := filepath.Join(p.r.Path, "mayor", "rig")
			bd := beads.New(mayorRigPath)
			mr, err := bd.FindMRForBranch(branchToDelete)
			if err == nil && mr != nil {
				// Found an open MR for this branch - reject it
				rejected := "closed"
				reason := "polecat nuked"
				if err := bd.Update(mr.ID, beads.UpdateOptions{Status: &rejected}); err == nil {
					// Also update close_reason field
					desc := mr.Description
					if desc == "" {
						desc = fmt.Sprintf("close_reason: %s", reason)
					} else if !strings.Contains(desc, "close_reason:") {
						desc = fmt.Sprintf("%s\nclose_reason: %s", desc, reason)
					}
					_ = bd.Update(mr.ID, beads.UpdateOptions{Description: &desc})
					fmt.Printf("  %s rejected MR %s (polecat nuked)\n", style.Warning.Render("⚠"), mr.ID)
				}
				// Non-fatal if MR rejection fails - continue with nuke
			}
		}

		// Step 5: Delete branch (if we know it)
		// Use bare repo if it exists (matches where worktree was created), otherwise mayor/rig
		if branchToDelete != "" {
			var repoGit *git.Git
			bareRepoPath := filepath.Join(p.r.Path, ".repo.git")
			if info, err := os.Stat(bareRepoPath); err == nil && info.IsDir() {
				repoGit = git.NewGitWithDir(bareRepoPath, "")
			} else {
				repoGit = git.NewGit(filepath.Join(p.r.Path, "mayor", "rig"))
			}
			if err := repoGit.DeleteBranch(branchToDelete, true); err != nil {
				// Non-fatal - branch might already be gone
				fmt.Printf("  %s branch delete: %v\n", style.Dim.Render("○"), err)
			} else {
				fmt.Printf("  %s deleted branch %s\n", style.Success.Render("✓"), branchToDelete)
			}
		}

		// Step 6: Close agent bead (if exists)
		agentBeadID := polecatBeadIDForRig(p.r, p.rigName, p.polecatName)
		closeArgs := []string{"close", agentBeadID, "--reason=nuked"}
		if sessionID := runtime.SessionIDFromEnv(); sessionID != "" {
			closeArgs = append(closeArgs, "--session="+sessionID)
		}
		closeCmd := exec.Command("bd", closeArgs...)
		closeCmd.Dir = filepath.Join(p.r.Path, "mayor", "rig")
		if err := closeCmd.Run(); err != nil {
			// Non-fatal - agent bead might not exist
			fmt.Printf("  %s agent bead not found or already closed\n", style.Dim.Render("○"))
		} else {
			fmt.Printf("  %s closed agent bead %s\n", style.Success.Render("✓"), agentBeadID)
		}

		nuked++
	}

	// Report results
	if polecatNukeDryRun {
		fmt.Printf("\n%s Would nuke %d polecat(s).\n", style.Info.Render("ℹ"), len(targets))
		return nil
	}

	if len(nukeErrors) > 0 {
		fmt.Printf("\n%s Some nukes failed:\n", style.Warning.Render("Warning:"))
		for _, e := range nukeErrors {
			fmt.Printf("  - %s\n", e)
		}
	}

	if nuked > 0 {
		fmt.Printf("\n%s Nuked %d polecat(s).\n", style.SuccessPrefix, nuked)
	}

	// Final cleanup: Kill any orphaned Claude processes that escaped the session termination.
	// This catches processes that called setsid() or were reparented during session shutdown.
	if !polecatNukeDryRun {
		cleanupOrphanedProcesses()
	}

	if len(nukeErrors) > 0 {
		return fmt.Errorf("%d nuke(s) failed", len(nukeErrors))
	}

	return nil
}

// cleanupOrphanedProcesses kills Claude processes that survived session termination.
// Uses aggressive zombie detection via tmux session verification.
func cleanupOrphanedProcesses() {
	results, err := util.CleanupZombieClaudeProcesses()
	if err != nil {
		// Non-fatal: log and continue
		fmt.Printf("  %s orphan cleanup check failed: %v\n", style.Dim.Render("○"), err)
		return
	}

	if len(results) == 0 {
		return
	}

	// Report what was cleaned up
	var killed, escalated int
	for _, r := range results {
		switch r.Signal {
		case "SIGTERM", "SIGKILL":
			killed++
		case "UNKILLABLE":
			escalated++
		}
	}

	if killed > 0 {
		fmt.Printf("  %s cleaned up %d orphaned process(es)\n", style.Success.Render("✓"), killed)
	}
	if escalated > 0 {
		fmt.Printf("  %s %d process(es) survived SIGKILL (unkillable)\n", style.Warning.Render("⚠"), escalated)
	}
}

func runPolecatStale(cmd *cobra.Command, args []string) error {
	rigName := args[0]
	mgr, r, err := getPolecatManager(rigName)
	if err != nil {
		return err
	}

	fmt.Printf("Detecting stale polecats in %s (threshold: %d commits behind main)...\n\n", r.Name, polecatStaleThreshold)

	staleInfos, err := mgr.DetectStalePolecats(polecatStaleThreshold)
	if err != nil {
		return fmt.Errorf("detecting stale polecats: %w", err)
	}

	if len(staleInfos) == 0 {
		fmt.Println("No polecats found.")
		return nil
	}

	// JSON output
	if polecatStaleJSON {
		return json.NewEncoder(os.Stdout).Encode(staleInfos)
	}

	// Summary counts
	var staleCount, safeCount int
	for _, info := range staleInfos {
		if info.IsStale {
			staleCount++
		} else {
			safeCount++
		}
	}

	// Display results
	for _, info := range staleInfos {
		statusIcon := style.Success.Render("●")
		statusText := "active"
		if info.IsStale {
			statusIcon = style.Warning.Render("○")
			statusText = "stale"
		}

		fmt.Printf("%s %s (%s)\n", statusIcon, style.Bold.Render(info.Name), statusText)

		// Session status
		if info.HasActiveSession {
			fmt.Printf("    Session: %s\n", style.Success.Render("running"))
		} else {
			fmt.Printf("    Session: %s\n", style.Dim.Render("stopped"))
		}

		// Commits behind
		if info.CommitsBehind > 0 {
			behindStyle := style.Dim
			if info.CommitsBehind >= polecatStaleThreshold {
				behindStyle = style.Warning
			}
			fmt.Printf("    Behind main: %s\n", behindStyle.Render(fmt.Sprintf("%d commits", info.CommitsBehind)))
		}

		// Agent state
		if info.AgentState != "" {
			fmt.Printf("    Agent state: %s\n", info.AgentState)
		} else {
			fmt.Printf("    Agent state: %s\n", style.Dim.Render("no bead"))
		}

		// Uncommitted work
		if info.HasUncommittedWork {
			fmt.Printf("    Uncommitted: %s\n", style.Error.Render("yes"))
		}

		// Reason
		fmt.Printf("    Reason: %s\n", info.Reason)
		fmt.Println()
	}

	// Summary
	fmt.Printf("Summary: %d stale, %d active\n", staleCount, safeCount)

	// Cleanup if requested
	if polecatStaleCleanup && staleCount > 0 {
		fmt.Println()
		if polecatNukeDryRun {
			fmt.Printf("Would clean up %d stale polecat(s):\n", staleCount)
			for _, info := range staleInfos {
				if info.IsStale {
					fmt.Printf("  - %s: %s\n", info.Name, info.Reason)
				}
			}
		} else {
			fmt.Printf("Cleaning up %d stale polecat(s)...\n", staleCount)
			nuked := 0
			for _, info := range staleInfos {
				if !info.IsStale {
					continue
				}
				fmt.Printf("  Nuking %s...", info.Name)
				if err := mgr.RemoveWithOptions(info.Name, true, false); err != nil {
					fmt.Printf(" %s (%v)\n", style.Error.Render("failed"), err)
				} else {
					fmt.Printf(" %s\n", style.Success.Render("done"))
					nuked++
				}
			}
			fmt.Printf("\n%s Nuked %d stale polecat(s).\n", style.SuccessPrefix, nuked)
		}
	}

	return nil
}

// OrphanInfo represents an orphaned polecat with hooked work.
type OrphanInfo struct {
	Rig      string `json:"rig"`
	Polecat  string `json:"polecat"`
	HookBead string `json:"hook_bead"`
}

func runPolecatOrphans(cmd *cobra.Command, args []string) error {
	rigName := args[0]
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Find polecats with hooked work but no active session
	orphans, err := witness.FindPolecatsWithHookedWork(workDir, rigName)
	if err != nil {
		return fmt.Errorf("finding orphaned polecats: %w", err)
	}

	// Convert to output format
	var orphanInfos []OrphanInfo
	for _, o := range orphans {
		orphanInfos = append(orphanInfos, OrphanInfo{
			Rig:      o.RigName,
			Polecat:  o.PolecatName,
			HookBead: o.HookBead,
		})
	}

	// JSON output
	if polecatOrphansJSON {
		return json.NewEncoder(os.Stdout).Encode(orphanInfos)
	}

	// Human-readable output
	if len(orphanInfos) == 0 {
		fmt.Printf("No orphaned polecats found in %s.\n", rigName)
		return nil
	}

	fmt.Printf("Found %d orphaned polecat(s) in %s:\n\n", len(orphanInfos), rigName)
	for _, o := range orphanInfos {
		fmt.Printf("%s %s\n", style.Warning.Render("○"), style.Bold.Render(o.Polecat))
		fmt.Printf("    Hooked work: %s\n", o.HookBead)
		fmt.Println()
	}

	// Respawn if requested
	if polecatOrphansRespawn {
		fmt.Printf("Respawning %d orphaned polecat(s)...\n", len(orphanInfos))
		respawned := 0
		for _, o := range orphanInfos {
			fmt.Printf("  Respawning %s...", o.Polecat)
			if err := witness.RespawnPolecatWithHookedWork(workDir, o.Rig, o.Polecat); err != nil {
				fmt.Printf(" %s (%v)\n", style.Error.Render("failed"), err)
			} else {
				fmt.Printf(" %s\n", style.Success.Render("done"))
				respawned++
			}
		}
		fmt.Printf("\n%s Respawned %d orphaned polecat(s).\n", style.SuccessPrefix, respawned)
	} else {
		fmt.Println("Use --respawn to respawn these polecats.")
	}

	return nil
}
