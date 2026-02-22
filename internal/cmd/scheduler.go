package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/scheduler/capacity"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	schedulerStatusJSON bool
	schedulerListJSON   bool
	schedulerClearBead  string
	schedulerRunBatch   int
	schedulerRunDryRun  bool
)

var schedulerCmd = &cobra.Command{
	Use:     "scheduler",
	GroupID: GroupWork,
	Short:   "Manage dispatch scheduler",
	Long: `Manage the capacity-controlled dispatch scheduler.

Subcommands:
  gt scheduler status    # Show scheduler state
  gt scheduler list      # List all scheduled beads
  gt scheduler run       # Manual dispatch trigger
  gt scheduler pause     # Pause dispatch
  gt scheduler resume    # Resume dispatch
  gt scheduler clear     # Remove beads from scheduler

Config:
  gt config set scheduler.max_polecats 5    # Enable deferred dispatch
  gt config set scheduler.max_polecats -1   # Direct dispatch (default)`,
	RunE: requireSubcommand,
}

var schedulerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show scheduler state: pending, capacity, active polecats",
	RunE:  runSchedulerStatus,
}

var schedulerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all scheduled beads with titles, rig, blocked status",
	RunE:  runSchedulerList,
}

var schedulerPauseCmd = &cobra.Command{
	Use:   "pause",
	Short: "Pause all scheduler dispatch (town-wide)",
	RunE:  runSchedulerPause,
}

var schedulerResumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume scheduler dispatch",
	RunE:  runSchedulerResume,
}

var schedulerClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Remove beads from the scheduler",
	Long: `Remove beads from the scheduler by clearing gt:queued labels.

Without --bead, removes ALL beads from the scheduler.
With --bead, removes only the specified bead.`,
	RunE: runSchedulerClear,
}

var schedulerRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Manually trigger scheduler dispatch",
	Long: `Manually trigger dispatch of scheduled work.

This dispatches scheduled beads using the same logic as the daemon heartbeat,
but can be run ad-hoc. Useful for testing or when the daemon is not running.

  gt scheduler run                  # Dispatch using config defaults
  gt scheduler run --batch 5        # Dispatch up to 5
  gt scheduler run --dry-run        # Preview what would dispatch`,
	RunE: runSchedulerRun,
}

func init() {
	// Status flags
	schedulerStatusCmd.Flags().BoolVar(&schedulerStatusJSON, "json", false, "Output as JSON")

	// List flags
	schedulerListCmd.Flags().BoolVar(&schedulerListJSON, "json", false, "Output as JSON")

	// Clear flags
	schedulerClearCmd.Flags().StringVar(&schedulerClearBead, "bead", "", "Remove specific bead from scheduler")

	// Run flags
	schedulerRunCmd.Flags().IntVar(&schedulerRunBatch, "batch", 0, "Override batch size (0 = use config)")
	schedulerRunCmd.Flags().BoolVar(&schedulerRunDryRun, "dry-run", false, "Preview what would dispatch")

	// Build command tree (flat — no intermediary "capacity" level)
	schedulerCmd.AddCommand(schedulerStatusCmd)
	schedulerCmd.AddCommand(schedulerListCmd)
	schedulerCmd.AddCommand(schedulerPauseCmd)
	schedulerCmd.AddCommand(schedulerResumeCmd)
	schedulerCmd.AddCommand(schedulerClearCmd)
	schedulerCmd.AddCommand(schedulerRunCmd)

	rootCmd.AddCommand(schedulerCmd)
}

// scheduledBeadInfo holds info about a scheduled bead for display.
type scheduledBeadInfo struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	TargetRig string `json:"target_rig"`
	Blocked   bool   `json:"blocked,omitempty"`
}

func runSchedulerStatus(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	state, err := capacity.LoadState(townRoot)
	if err != nil {
		return fmt.Errorf("loading scheduler state: %w", err)
	}

	scheduled, err := listScheduledBeads(townRoot)
	if err != nil {
		return fmt.Errorf("listing scheduled beads: %w", err)
	}

	activePolecats := countActivePolecats()

	if schedulerStatusJSON {
		out := struct {
			Paused         bool               `json:"paused"`
			PausedBy       string             `json:"paused_by,omitempty"`
			ScheduledTotal int                `json:"queued_total"`
			ScheduledReady int                `json:"queued_ready"`
			ActivePolecats int                `json:"active_polecats"`
			LastDispatchAt string             `json:"last_dispatch_at,omitempty"`
			Beads          []scheduledBeadInfo `json:"beads"`
		}{
			Paused:         state.Paused,
			PausedBy:       state.PausedBy,
			ScheduledTotal: len(scheduled),
			ActivePolecats: activePolecats,
			LastDispatchAt: state.LastDispatchAt,
			Beads:          scheduled,
		}
		for _, b := range scheduled {
			if !b.Blocked {
				out.ScheduledReady++
			}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	readyCount := 0
	for _, b := range scheduled {
		if !b.Blocked {
			readyCount++
		}
	}

	fmt.Printf("%s\n\n", style.Bold.Render("Scheduler Status"))
	if state.Paused {
		fmt.Printf("  State:    %s (by %s)\n", style.Warning.Render("PAUSED"), state.PausedBy)
	} else {
		fmt.Printf("  State:    active\n")
	}
	fmt.Printf("  Scheduled: %d total, %d ready\n", len(scheduled), readyCount)
	fmt.Printf("  Active:    %d polecats\n", activePolecats)
	if state.LastDispatchAt != "" {
		fmt.Printf("  Last dispatch: %s (%d beads)\n", state.LastDispatchAt, state.LastDispatchCount)
	}

	return nil
}

func runSchedulerList(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	scheduled, err := listScheduledBeads(townRoot)
	if err != nil {
		return fmt.Errorf("listing scheduled beads: %w", err)
	}

	if schedulerListJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(scheduled)
	}

	if len(scheduled) == 0 {
		fmt.Println("No beads scheduled.")
		fmt.Println("Enable deferred dispatch with: gt config set scheduler.max_polecats <N>")
		return nil
	}

	byRig := make(map[string][]scheduledBeadInfo)
	for _, b := range scheduled {
		byRig[b.TargetRig] = append(byRig[b.TargetRig], b)
	}

	fmt.Printf("%s (%d beads)\n\n", style.Bold.Render("Scheduled Work"), len(scheduled))
	for rig, beads := range byRig {
		fmt.Printf("  %s (%d):\n", style.Bold.Render(rig), len(beads))
		for _, b := range beads {
			indicator := "○"
			if b.Blocked {
				indicator = "⏸"
			}
			fmt.Printf("    %s %s: %s\n", indicator, b.ID, b.Title)
		}
		fmt.Println()
	}

	return nil
}

func runSchedulerPause(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	state, err := capacity.LoadState(townRoot)
	if err != nil {
		return fmt.Errorf("loading scheduler state: %w", err)
	}

	if state.Paused {
		fmt.Printf("%s Scheduler is already paused (by %s)\n", style.Dim.Render("○"), state.PausedBy)
		return nil
	}

	actor := detectActor()
	state.SetPaused(actor)
	if err := capacity.SaveState(townRoot, state); err != nil {
		return fmt.Errorf("saving scheduler state: %w", err)
	}

	fmt.Printf("%s Scheduler paused\n", style.Bold.Render("⏸"))
	return nil
}

func runSchedulerResume(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	state, err := capacity.LoadState(townRoot)
	if err != nil {
		return fmt.Errorf("loading scheduler state: %w", err)
	}

	if !state.Paused {
		fmt.Printf("%s Scheduler is not paused\n", style.Dim.Render("○"))
		return nil
	}

	state.SetResumed()
	if err := capacity.SaveState(townRoot, state); err != nil {
		return fmt.Errorf("saving scheduler state: %w", err)
	}

	fmt.Printf("%s Scheduler resumed\n", style.Bold.Render("▶"))
	return nil
}

func runSchedulerClear(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	if schedulerClearBead != "" {
		if err := unscheduleBeadLabels(schedulerClearBead); err != nil {
			return fmt.Errorf("clearing bead %s from scheduler: %w", schedulerClearBead, err)
		}
		fmt.Printf("%s Removed %s from scheduler\n", style.Bold.Render("✓"), schedulerClearBead)
		return nil
	}

	ids, err := listAllScheduledBeadIDs(townRoot)
	if err != nil {
		return fmt.Errorf("listing scheduled beads: %w", err)
	}

	if len(ids) == 0 {
		fmt.Println("Scheduler is already empty.")
		return nil
	}

	cleared := 0
	for _, id := range ids {
		if err := unscheduleBeadLabels(id); err != nil {
			fmt.Printf("  %s Could not clear %s: %v\n", style.Dim.Render("Warning:"), id, err)
			continue
		}
		cleared++
	}

	fmt.Printf("%s Cleared %d bead(s) from scheduler\n", style.Bold.Render("✓"), cleared)
	return nil
}

func runSchedulerRun(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	_, err = dispatchScheduledWork(townRoot, detectActor(), schedulerRunBatch, schedulerRunDryRun)
	return err
}

// listScheduledBeads returns all beads with the gt:queued label across all rig DBs.
// Populates Blocked by reconciling bd list (all scheduled) vs bd ready (unblocked).
func listScheduledBeads(townRoot string) ([]scheduledBeadInfo, error) {
	var result []scheduledBeadInfo
	seen := make(map[string]bool)

	dirs := beadsSearchDirs(townRoot)

	readyIDs := make(map[string]bool)
	for _, dir := range dirs {
		readyCmd := exec.Command("bd", "ready", "--label", capacity.LabelScheduled, "--json", "--limit=0")
		readyCmd.Dir = dir
		readyOut, err := readyCmd.Output()
		if err != nil {
			continue
		}
		var readyBeads []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
		}
		if err := json.Unmarshal(readyOut, &readyBeads); err == nil {
			for _, b := range readyBeads {
				if meta := capacity.ParseMetadata(b.Description); meta != nil && meta.DispatchFailures >= maxDispatchFailures {
					continue
				}
				readyIDs[b.ID] = true
			}
		}
	}

	var lastErr error
	failCount := 0
	for _, dir := range dirs {
		beads, err := listScheduledBeadsFrom(dir)
		if err != nil {
			failCount++
			lastErr = err
			continue
		}
		for _, b := range beads {
			if b.Status == "hooked" || b.Status == "closed" {
				continue
			}
			if !seen[b.ID] {
				seen[b.ID] = true
				b.Blocked = !readyIDs[b.ID]
				result = append(result, b)
			}
		}
	}

	if failCount == len(dirs) && failCount > 0 {
		return nil, fmt.Errorf("all %d bead directories failed (last: %w)", failCount, lastErr)
	}
	return result, nil
}

// listScheduledBeadsFrom queries a single directory for beads with gt:queued label.
func listScheduledBeadsFrom(dir string) ([]scheduledBeadInfo, error) {
	listCmd := exec.Command("bd", "list", "--label="+capacity.LabelScheduled, "--json", "--limit=0")
	listCmd.Dir = dir
	var stdout strings.Builder
	listCmd.Stdout = &stdout

	if err := listCmd.Run(); err != nil {
		return nil, err
	}

	var raw []struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Status      string `json:"status"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(stdout.String()), &raw); err != nil {
		return nil, fmt.Errorf("parsing scheduled beads: %w", err)
	}

	result := make([]scheduledBeadInfo, 0, len(raw))
	for _, r := range raw {
		targetRig := ""
		meta := capacity.ParseMetadata(r.Description)
		if meta != nil {
			targetRig = meta.TargetRig
			if meta.DispatchFailures >= maxDispatchFailures {
				continue
			}
		}
		result = append(result, scheduledBeadInfo{
			ID:        r.ID,
			Title:     r.Title,
			Status:    r.Status,
			TargetRig: targetRig,
		})
	}
	return result, nil
}

// listAllScheduledBeadIDs returns the IDs of ALL beads with the gt:queued
// label across all rig DBs, without status or circuit-breaker filtering.
func listAllScheduledBeadIDs(townRoot string) ([]string, error) {
	var ids []string
	seen := make(map[string]bool)

	var lastErr error
	failCount := 0
	for _, dir := range beadsSearchDirs(townRoot) {
		listCmd := exec.Command("bd", "list", "--label="+capacity.LabelScheduled, "--json", "--limit=0")
		listCmd.Dir = dir
		var stdout strings.Builder
		listCmd.Stdout = &stdout
		if err := listCmd.Run(); err != nil {
			failCount++
			lastErr = err
			continue
		}
		var raw []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(stdout.String()), &raw); err != nil {
			failCount++
			lastErr = err
			continue
		}
		for _, r := range raw {
			if !seen[r.ID] {
				seen[r.ID] = true
				ids = append(ids, r.ID)
			}
		}
	}

	if failCount > 0 && len(ids) == 0 {
		return nil, fmt.Errorf("all directories failed (last: %w)", lastErr)
	}
	return ids, nil
}

// beadsSearchDirs returns directories to scan for scheduled beads:
// the town root plus any rig directories that have a .beads/ subdirectory.
func beadsSearchDirs(townRoot string) []string {
	dirs := []string{townRoot}
	seen := map[string]bool{townRoot: true}
	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return dirs
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || e.Name() == "mayor" || e.Name() == "settings" {
			continue
		}
		rigDir := filepath.Join(townRoot, e.Name())
		beadsDir := filepath.Join(rigDir, ".beads")
		if _, err := os.Stat(beadsDir); err == nil && !seen[rigDir] {
			dirs = append(dirs, rigDir)
			seen[rigDir] = true
		}
		mayorRigDir := filepath.Join(rigDir, "mayor", "rig")
		mayorBeadsDir := filepath.Join(mayorRigDir, ".beads")
		if _, err := os.Stat(mayorBeadsDir); err == nil && !seen[mayorRigDir] {
			dirs = append(dirs, mayorRigDir)
			seen[mayorRigDir] = true
		}
	}
	return dirs
}

// countActivePolecats counts all running polecats across all rigs in the town.
func countActivePolecats() int {
	listCmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	out, err := listCmd.Output()
	if err != nil {
		return 0
	}

	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		identity, err := session.ParseSessionName(line)
		if err != nil {
			continue
		}
		if identity.Role == session.RolePolecat {
			count++
		}
	}
	return count
}
