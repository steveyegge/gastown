package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/circuit"
	"github.com/steveyegge/gastown/internal/scheduler/capacity"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	schedulerStatusJSON    bool
	schedulerListJSON      bool
	schedulerClearBead     string
	schedulerRunBatch      int
	schedulerRunDryRun     bool
	schedulerRunBead       string
	schedulerReorderBy     string
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
  gt scheduler promote   # Move bead to front of queue
  gt scheduler demote    # Move bead to back of queue
  gt scheduler reorder   # Reorder queue by priority

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
	Use:   "clear [bead-id]",
	Short: "Remove beads from the scheduler",
	Long: `Remove beads from the scheduler by closing sling context beads.

Without a bead ID, removes ALL beads from the scheduler.
With a bead ID (positional or --bead), removes only that bead.

  gt scheduler clear              # Remove all beads
  gt scheduler clear be-abc123    # Remove one bead (positional)
  gt scheduler clear --bead be-abc123  # Remove one bead (flag)`,
	Args: cobra.MaximumNArgs(1),
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
  gt scheduler run --dry-run        # Preview what would dispatch
  gt scheduler run --bead gt-abc    # Dispatch only this specific bead`,
	RunE: runSchedulerRun,
}

var schedulerPromoteCmd = &cobra.Command{
	Use:   "promote <bead-id>",
	Short: "Move a bead to the front of the dispatch queue",
	Long: `Move a scheduled bead to the front of the queue by setting its
EnqueuedAt timestamp to epoch. The bead will be dispatched first on
the next scheduler cycle (subject to readiness and capacity).`,
	Args: cobra.ExactArgs(1),
	RunE: runSchedulerPromote,
}

var schedulerDemoteCmd = &cobra.Command{
	Use:   "demote <bead-id>",
	Short: "Move a bead to the back of the dispatch queue",
	Long: `Move a scheduled bead to the back of the queue by setting its
EnqueuedAt timestamp to now. The bead will be dispatched after all
currently queued beads.`,
	Args: cobra.ExactArgs(1),
	RunE: runSchedulerDemote,
}

var schedulerReorderCmd = &cobra.Command{
	Use:   "reorder",
	Short: "Reorder the dispatch queue by a field",
	Long: `Reorder all scheduled beads by the specified field.

  gt scheduler reorder --by priority    # P0 first, then P1, P2, etc.

This reassigns EnqueuedAt timestamps so that higher-priority beads
are dispatched before lower-priority ones. Within the same priority
level, original FIFO order is preserved.`,
	RunE: runSchedulerReorder,
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
	schedulerRunCmd.Flags().StringVar(&schedulerRunBead, "bead", "", "Dispatch only this specific bead")

	// Reorder flags
	schedulerReorderCmd.Flags().StringVar(&schedulerReorderBy, "by", "", "Field to reorder by (currently: priority)")
	_ = schedulerReorderCmd.MarkFlagRequired("by")

	// Build command tree (flat — no intermediary "capacity" level)
	schedulerCmd.AddCommand(schedulerStatusCmd)
	schedulerCmd.AddCommand(schedulerListCmd)
	schedulerCmd.AddCommand(schedulerPauseCmd)
	schedulerCmd.AddCommand(schedulerResumeCmd)
	schedulerCmd.AddCommand(schedulerClearCmd)
	schedulerCmd.AddCommand(schedulerRunCmd)
	schedulerCmd.AddCommand(schedulerPromoteCmd)
	schedulerCmd.AddCommand(schedulerDemoteCmd)
	schedulerCmd.AddCommand(schedulerReorderCmd)

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

	// Resolve target bead: positional arg takes precedence over --bead flag.
	// This matches the UX of `gt scheduler promote/demote` (positional args).
	targetBead := schedulerClearBead
	if len(args) > 0 {
		targetBead = args[0]
	}

	if targetBead != "" {
		// Close ALL sling contexts for this specific work bead (there may be
		// duplicates if concurrent scheduleBead calls raced past idempotency).
		// Scan all rig dirs since contexts live in target rig beads. (GH#3468)
		contexts, listErr := listAllSlingContexts(townRoot)
		if listErr != nil {
			return fmt.Errorf("listing contexts: %w", listErr)
		}

		closed := 0
		for _, ctx := range contexts {
			fields := beads.ParseSlingContextFields(ctx.Description)
			if fields != nil && fields.WorkBeadID == targetBead {
				b := beadsForContext(townRoot, fields)
				if err := b.CloseSlingContext(ctx.ID, "cleared"); err != nil {
					fmt.Printf("  %s Could not close context %s: %v\n", style.Dim.Render("Warning:"), ctx.ID, err)
					continue
				}
				closed++
			}
		}

		if closed == 0 {
			fmt.Printf("%s No sling context found for %s\n", style.Dim.Render("○"), targetBead)
		} else {
			fmt.Printf("%s Removed %s from scheduler (closed %d context(s))\n",
				style.Bold.Render("✓"), targetBead, closed)
		}
		return nil
	}

	// Close all open sling contexts across all dirs
	allContexts, err := listAllSlingContexts(townRoot)
	if err != nil {
		return fmt.Errorf("listing sling contexts: %w", err)
	}

	if len(allContexts) == 0 {
		fmt.Println("Scheduler is already empty.")
		return nil
	}

	cleared := 0
	for _, ctx := range allContexts {
		fields := beads.ParseSlingContextFields(ctx.Description)
		b := beadsForContext(townRoot, fields)
		if err := b.CloseSlingContext(ctx.ID, "cleared"); err != nil {
			fmt.Printf("  %s Could not close context %s: %v\n", style.Dim.Render("Warning:"), ctx.ID, err)
			continue
		}
		cleared++
	}

	fmt.Printf("%s Cleared %d context bead(s) from scheduler\n", style.Bold.Render("✓"), cleared)
	return nil
}

func runSchedulerRun(cmd *cobra.Command, args []string) error {
	if schedulerRunBead != "" && schedulerRunBatch > 0 {
		return fmt.Errorf("--bead and --batch are mutually exclusive")
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	// --bead: dispatch a single specific bead directly
	if schedulerRunBead != "" {
		return runSchedulerRunBead(townRoot, schedulerRunBead, schedulerRunDryRun)
	}

	_, err = dispatchScheduledWork(townRoot, detectActor(), schedulerRunBatch, schedulerRunDryRun)
	return err
}

// runSchedulerRunBead dispatches a single specific bead from the scheduler queue.
func runSchedulerRunBead(townRoot, beadID string, dryRun bool) error {
	townBeads := beads.NewWithBeadsDir(townRoot, filepath.Join(townRoot, ".beads"))

	ctx, fields, err := townBeads.FindOpenSlingContext(beadID)
	if err != nil {
		return fmt.Errorf("finding sling context: %w", err)
	}
	if ctx == nil || fields == nil {
		return fmt.Errorf("no sling context found for %s\nUse 'gt scheduler list' to see scheduled beads", beadID)
	}

	if fields.DispatchFailures >= maxDispatchFailures {
		return fmt.Errorf("bead %s is circuit-broken (%d failures) — use 'gt scheduler clear %s' to reset",
			beadID, fields.DispatchFailures, beadID)
	}

	// Check rig-level circuit breaker
	if fields.TargetRig != "" {
		if err := circuit.CheckDispatchForRig(townRoot, fields.TargetRig); err != nil {
			return err
		}
	}

	pending := capacity.PendingBead{
		ID:          ctx.ID,
		WorkBeadID:  fields.WorkBeadID,
		Title:       ctx.Title,
		TargetRig:   fields.TargetRig,
		Description: ctx.Description,
		Context:     fields,
	}

	if dryRun {
		fmt.Printf("%s Would dispatch %s → %s\n", style.Bold.Render("→"), beadID, fields.TargetRig)
		return nil
	}

	result, err := dispatchSingleBead(pending, townRoot, detectActor())
	if err != nil {
		return fmt.Errorf("dispatching %s: %w", beadID, err)
	}

	// Close the context after successful dispatch
	if closeErr := townBeads.CloseSlingContext(ctx.ID, "dispatched-manual"); closeErr != nil {
		style.PrintWarning("dispatch succeeded but context close failed: %v", closeErr)
	}

	polecatName := ""
	if result != nil {
		polecatName = result.PolecatName
	}

	fmt.Printf("%s Dispatched %s → %s", style.Bold.Render("✓"), beadID, fields.TargetRig)
	if polecatName != "" {
		fmt.Printf(" (polecat: %s)", polecatName)
	}
	fmt.Println()

	// Wake rig agents
	if fields.TargetRig != "" {
		wakeRigAgents(fields.TargetRig)
	}

	return nil
}

// listScheduledBeads returns info about all scheduled beads for display.
// Reconciles sling context beads with work bead readiness to mark blocked status.
// Uses batch fetch for work bead info to avoid N+1 subprocess spawns.
func listScheduledBeads(townRoot string) ([]scheduledBeadInfo, error) {
	allContexts, err := listAllSlingContexts(townRoot)
	if err != nil {
		return nil, err
	}

	if len(allContexts) == 0 {
		return nil, nil
	}

	// Collect work bead IDs from contexts for targeted fetch
	var workBeadIDs []string
	for _, ctx := range allContexts {
		fields := beads.ParseSlingContextFields(ctx.Description)
		if fields != nil && fields.WorkBeadID != "" {
			workBeadIDs = append(workBeadIDs, fields.WorkBeadID)
		}
	}

	// Build readyIDs set and batch-fetch work bead info for specific IDs
	readyWorkIDs := listReadyWorkBeadIDs(townRoot)
	workBeadInfo := batchFetchBeadInfoByIDs(townRoot, workBeadIDs)

	// Supplement readyWorkIDs with beads whose custom status is "ready".
	// bd ready only returns built-in "open" status; the crew pipeline uses
	// custom "ready" status for refined, dispatchable beads.
	for _, id := range workBeadIDs {
		if info, found := workBeadInfo[id]; found && info.Status == "ready" {
			readyWorkIDs[id] = true
		}
	}

	seenWork := make(map[string]bool)
	var result []scheduledBeadInfo
	for _, ctx := range allContexts {
		fields := beads.ParseSlingContextFields(ctx.Description)
		if fields == nil {
			continue
		}

		// Exclude circuit-broken
		if fields.DispatchFailures >= maxDispatchFailures {
			continue
		}

		// Dedup by WorkBeadID (mirrors getReadySlingContexts logic)
		if seenWork[fields.WorkBeadID] {
			continue
		}
		seenWork[fields.WorkBeadID] = true

		// Get work bead info for title/status from batch-fetched map
		title := ctx.Title
		status := "open"
		if info, found := workBeadInfo[fields.WorkBeadID]; found {
			title = info.Title
			status = info.Status
			// Skip if work bead is hooked/closed
			if status == "hooked" || status == "closed" || status == "tombstone" {
				continue
			}
		}

		result = append(result, scheduledBeadInfo{
			ID:        fields.WorkBeadID,
			Title:     title,
			Status:    status,
			TargetRig: fields.TargetRig,
			Blocked:   !readyWorkIDs[fields.WorkBeadID],
		})
	}

	return result, nil
}

// listAllScheduledBeadIDs returns the work bead IDs of all scheduled beads.
func listAllScheduledBeadIDs(townRoot string) ([]string, error) {
	allContexts, err := listAllSlingContexts(townRoot)
	if err != nil {
		return nil, err
	}

	var ids []string
	seen := make(map[string]bool)
	for _, ctx := range allContexts {
		fields := beads.ParseSlingContextFields(ctx.Description)
		if fields == nil {
			continue
		}
		if !seen[fields.WorkBeadID] {
			seen[fields.WorkBeadID] = true
			ids = append(ids, fields.WorkBeadID)
		}
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

// runSchedulerPromote moves a bead to the front of the dispatch queue.
func runSchedulerPromote(cmd *cobra.Command, args []string) error {
	beadID := args[0]
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	townBeads := beads.NewWithBeadsDir(townRoot, filepath.Join(townRoot, ".beads"))
	updated, err := updateEnqueuedAt(townBeads, beadID, "0001-01-01T00:00:00Z")
	if err != nil {
		return err
	}

	state, _ := capacity.LoadState(townRoot)

	fmt.Printf("%s Promoted %s to front of queue (%d context(s) updated)\n",
		style.Bold.Render("⬆"), beadID, updated)
	if state != nil && state.Paused {
		fmt.Printf("  %s Scheduler is paused — bead will dispatch when resumed\n", style.Dim.Render("ℹ"))
	}
	return nil
}

// runSchedulerDemote moves a bead to the back of the dispatch queue.
func runSchedulerDemote(cmd *cobra.Command, args []string) error {
	beadID := args[0]
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	townBeads := beads.NewWithBeadsDir(townRoot, filepath.Join(townRoot, ".beads"))
	updated, err := updateEnqueuedAt(townBeads, beadID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}

	state, _ := capacity.LoadState(townRoot)

	fmt.Printf("%s Demoted %s to back of queue (%d context(s) updated)\n",
		style.Bold.Render("⬇"), beadID, updated)
	if state != nil && state.Paused {
		fmt.Printf("  %s Scheduler is paused — bead will dispatch when resumed\n", style.Dim.Render("ℹ"))
	}
	return nil
}

// updateEnqueuedAt finds all open sling contexts for a work bead and updates
// their EnqueuedAt timestamp. Returns the number of contexts updated.
func updateEnqueuedAt(townBeads *beads.Beads, workBeadID, newTimestamp string) (int, error) {
	contexts, err := townBeads.ListOpenSlingContexts()
	if err != nil {
		return 0, fmt.Errorf("listing sling contexts: %w", err)
	}

	updated := 0
	for _, ctx := range contexts {
		fields := beads.ParseSlingContextFields(ctx.Description)
		if fields == nil || fields.WorkBeadID != workBeadID {
			continue
		}
		if fields.DispatchFailures >= maxDispatchFailures {
			fmt.Printf("  %s Skipping circuit-broken context %s\n", style.Dim.Render("⚠"), ctx.ID)
			continue
		}
		fields.EnqueuedAt = newTimestamp
		if err := townBeads.UpdateSlingContextFields(ctx.ID, fields); err != nil {
			fmt.Printf("  %s Could not update context %s: %v\n", style.Dim.Render("Warning:"), ctx.ID, err)
			continue
		}
		updated++
	}

	if updated == 0 {
		return 0, fmt.Errorf("no sling context found for %s\nUse 'gt scheduler list' to see scheduled beads", workBeadID)
	}

	return updated, nil
}

// runSchedulerReorder reorders the entire queue by the specified field.
func runSchedulerReorder(cmd *cobra.Command, args []string) error {
	if schedulerReorderBy != "priority" {
		return fmt.Errorf("unsupported reorder field %q (supported: priority)", schedulerReorderBy)
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	townBeads := beads.NewWithBeadsDir(townRoot, filepath.Join(townRoot, ".beads"))
	contexts, err := townBeads.ListOpenSlingContexts()
	if err != nil {
		return fmt.Errorf("listing sling contexts: %w", err)
	}

	if len(contexts) == 0 {
		fmt.Println("No beads scheduled — nothing to reorder.")
		return nil
	}

	// Parse all contexts and collect work bead IDs
	type contextEntry struct {
		ctx    *beads.Issue
		fields *capacity.SlingContextFields
	}
	var entries []contextEntry
	var workBeadIDs []string

	for _, ctx := range contexts {
		fields := beads.ParseSlingContextFields(ctx.Description)
		if fields == nil {
			continue
		}
		if fields.DispatchFailures >= maxDispatchFailures {
			continue // Skip circuit-broken
		}
		entries = append(entries, contextEntry{ctx: ctx, fields: fields})
		workBeadIDs = append(workBeadIDs, fields.WorkBeadID)
	}

	if len(entries) == 0 {
		fmt.Println("No active contexts to reorder.")
		return nil
	}

	// Batch-fetch priorities for work beads
	priorities := batchFetchBeadPriorities(townRoot, workBeadIDs)

	// Sort entries by priority (P0=0 first), preserving FIFO within same priority
	sort.SliceStable(entries, func(i, j int) bool {
		pi := priorities[entries[i].fields.WorkBeadID]
		pj := priorities[entries[j].fields.WorkBeadID]
		return pi < pj
	})

	// Reassign EnqueuedAt timestamps: start from a base time, increment 1s each
	baseTime, _ := time.Parse(time.RFC3339, "2000-01-01T00:00:00Z")
	updated := 0
	for i, entry := range entries {
		newTime := baseTime.Add(time.Duration(i) * time.Second).Format(time.RFC3339)
		entry.fields.EnqueuedAt = newTime
		if err := townBeads.UpdateSlingContextFields(entry.ctx.ID, entry.fields); err != nil {
			fmt.Printf("  %s Could not update %s: %v\n", style.Dim.Render("Warning:"), entry.fields.WorkBeadID, err)
			continue
		}
		updated++
	}

	fmt.Printf("%s Reordered %d beads by priority\n", style.Bold.Render("↕"), updated)

	// Show new order
	for i, entry := range entries {
		p := priorities[entry.fields.WorkBeadID]
		fmt.Printf("  %d. P%d %s\n", i+1, p, entry.fields.WorkBeadID)
	}

	return nil
}

// batchFetchBeadPriorities returns a map of bead ID → priority (int, 0=P0).
// Beads not found default to priority 2 (P2).
func batchFetchBeadPriorities(townRoot string, ids []string) map[string]int {
	result := make(map[string]int)
	for _, id := range ids {
		result[id] = 2 // Default P2
	}
	if len(ids) == 0 {
		return result
	}

	for _, dir := range beadsSearchDirs(townRoot) {
		b := beads.New(dir)
		args := append([]string{"show", "--json"}, ids...)
		out, err := b.Run(args...)
		if err != nil {
			continue
		}
		var items []struct {
			ID       string `json:"id"`
			Priority int    `json:"priority"`
		}
		if err := json.Unmarshal(out, &items); err == nil {
			for _, item := range items {
				result[item.ID] = item.Priority
			}
		}
	}
	return result
}

// countActivePolecats counts all running polecat tmux sessions across all rigs.
// This includes idle polecats (completed work, no hook bead) which still occupy
// tmux sessions under the persistent polecat model. For capacity gating, use
// countWorkingPolecats which excludes idle sessions.
func countActivePolecats() int {
	return countActivePolecatsForRig("")
}

// countActivePolecatsForRig counts working polecats, optionally filtered by rig name.
// If rigName is empty, counts all polecats across all rigs.
//
// A polecat is "active" (consuming a capacity slot) only if it is working.
// Idle and done polecats have live tmux sessions (persistent model) but are
// NOT counted — they are available for reuse via FindIdlePolecat.
func countActivePolecatsForRig(rigName string) int {
	listCmd := tmux.BuildCommand("list-sessions", "-F", "#{session_name}")
	out, err := listCmd.Output()
	if err != nil {
		return 0
	}

	// Build set of idle/done polecat names by checking agent beads.
	// This prevents idle polecats from consuming capacity slots.
	idlePolecats := idlePolecatNames()

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
			if rigName == "" || identity.Rig == rigName {
				// Skip idle/done polecats — they don't consume capacity
				key := identity.Rig + "/" + identity.Name
				if idlePolecats[key] {
					continue
				}
				count++
			}
		}
	}
	return count
}

// idlePolecatNames returns a set of "rig/name" strings for polecats whose
// agent beads indicate idle or done state with a completed exit_type.
// These polecats have finished work and are available for reuse.
func idlePolecatNames() map[string]bool {
	result := make(map[string]bool)

	townRoot, err := workspace.FindFromCwd()
	if err != nil || townRoot == "" {
		return result
	}

	// Query all agent beads across all rig directories
	for _, dir := range beadsSearchDirs(townRoot) {
		b := beads.New(dir)
		out, err := b.Run("list", "--type=agent", "--json", "--limit=0")
		if err != nil {
			continue
		}
		var agents []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
			AgentState  string `json:"agent_state,omitempty"`
		}
		if err := json.Unmarshal(out, &agents); err != nil {
			continue
		}
		for _, agent := range agents {
			// Parse the bead ID to get rig and polecat name
			rig, role, name, ok := beads.ParseAgentBeadID(agent.ID)
			if !ok || role != "polecat" || name == "" {
				continue
			}

			fields := beads.ParseAgentFields(agent.Description)
			if fields == nil {
				continue
			}
			// Prefer the structured agent_state column over description-parsed
			// state. `bd agent state` updates the DB column directly without
			// rewriting the description, so the description can be stale.
			// This matches GetAgentBead() behavior (beads_agent.go:621-623).
			if agent.AgentState != "" {
				fields.AgentState = agent.AgentState
			}
			agentState := beads.AgentState(fields.AgentState)
			if (agentState == beads.AgentStateIdle || agentState == beads.AgentStateDone) &&
				fields.ExitType != "" {
				key := rig + "/" + name
				result[key] = true
			}
		}
	}
	return result
}

// countWorkingPolecats counts polecat sessions that are actively working.
// A polecat is "working" if its agent bead has a non-null hook_bead.
// Idle polecats (completed work, hook_bead=null) don't count toward capacity
// since they're available for re-sling under the persistent polecat model.
func countWorkingPolecats() int {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return countActivePolecats() // Fallback to total count
	}

	listCmd := tmux.BuildCommand("list-sessions", "-F", "#{session_name}")
	out, err := listCmd.Output()
	if err != nil {
		return 0
	}

	bd := beads.New(townRoot)
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		identity, err := session.ParseSessionName(line)
		if err != nil || identity.Role != session.RolePolecat {
			continue
		}

		// Check if this polecat has hooked work
		prefix := identity.Prefix
		if prefix == "" {
			prefix = session.PrefixFor(identity.Rig)
		}
		agentBeadID := beads.PolecatBeadIDWithPrefix(prefix, identity.Rig, identity.Name)
		issue, err := bd.Show(agentBeadID)
		if err != nil || issue == nil {
			count++ // Can't verify — count conservatively
			continue
		}

		fields := beads.ParseAgentFields(issue.Description)
		if fields.HookBead == "" {
			continue // Idle — don't count toward cap
		}
		count++
	}
	return count
}
