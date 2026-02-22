package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/scheduler/capacity"
	"github.com/steveyegge/gastown/internal/style"
)

// maxDispatchFailures is the maximum number of consecutive dispatch failures
// before a bead is marked as gt:dispatch-failed and removed from the scheduler.
const maxDispatchFailures = 3

// dispatchScheduledWork is the main dispatch loop for the capacity scheduler.
// Called by both `gt scheduler run` and the daemon heartbeat.
func dispatchScheduledWork(townRoot, actor string, batchOverride int, dryRun bool) (int, error) {
	// Acquire exclusive lock to prevent concurrent dispatch
	runtimeDir := filepath.Join(townRoot, ".runtime")
	_ = os.MkdirAll(runtimeDir, 0755)
	lockFile := filepath.Join(runtimeDir, "scheduler-dispatch.lock")
	fileLock := flock.New(lockFile)
	locked, err := fileLock.TryLock()
	if err != nil {
		return 0, fmt.Errorf("acquiring dispatch lock: %w", err)
	}
	if !locked {
		return 0, nil
	}
	defer func() { _ = fileLock.Unlock() }()

	// Load scheduler state
	state, err := capacity.LoadState(townRoot)
	if err != nil {
		return 0, fmt.Errorf("loading scheduler state: %w", err)
	}

	if state.Paused {
		if !dryRun {
			fmt.Printf("%s Scheduler is paused (by %s), skipping dispatch\n", style.Dim.Render("⏸"), state.PausedBy)
		}
		return 0, nil
	}

	// Load town settings for scheduler config
	settingsPath := config.TownSettingsPath(townRoot)
	settings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return 0, fmt.Errorf("loading town settings: %w", err)
	}

	schedulerCfg := settings.Scheduler
	if schedulerCfg == nil {
		schedulerCfg = capacity.DefaultSchedulerConfig()
	}

	// Nothing to dispatch when scheduler is in direct dispatch or disabled mode.
	// Early return before any bd commands to keep direct-mode overhead near zero.
	maxPolecats := schedulerCfg.GetMaxPolecats()
	if maxPolecats <= 0 {
		// Only check for stranded beads on manual invocations (not daemon heartbeats)
		// to avoid spawning bd processes every 3 minutes in direct mode.
		if !dryRun && !isDaemonDispatch() {
			staleBeads, _ := getReadyScheduledBeads(townRoot)
			if len(staleBeads) > 0 {
				fmt.Printf("%s %d bead(s) still carry gt:queued from a previous deferred mode\n",
					style.Warning.Render("⚠"), len(staleBeads))
				fmt.Printf("  Use: gt scheduler clear  (remove all scheduled labels)\n")
				fmt.Printf("  Or:  gt config set scheduler.max_polecats N  (re-enable deferred dispatch)\n")
			}
		}
		return 0, nil
	}

	// Determine limits
	batchSize := schedulerCfg.GetBatchSize()
	if batchOverride > 0 {
		batchSize = batchOverride
	}
	spawnDelay := schedulerCfg.GetSpawnDelay()

	// Count active polecats
	activePolecats := countActivePolecats()

	// Query ready scheduled beads (unblocked + has gt:queued label)
	readyBeads, err := getReadyScheduledBeads(townRoot)
	if err != nil {
		return 0, fmt.Errorf("querying ready beads: %w", err)
	}

	// Apply circuit breaker filter
	readyBeads, circuitBroken := capacity.FilterCircuitBroken(readyBeads, maxDispatchFailures)
	if circuitBroken > 0 && !dryRun {
		fmt.Printf("%s %d bead(s) circuit-broken (exceeded %d failures)\n",
			style.Dim.Render("⚠"), circuitBroken, maxDispatchFailures)
	}

	// Use pipeline to plan dispatch
	plan := capacity.PlanDispatch(maxPolecats, batchSize, activePolecats, readyBeads)

	if plan.Reason == "none" {
		if dryRun {
			fmt.Println("No ready beads scheduled for dispatch")
		}
		return 0, nil
	}

	if plan.Reason == "capacity" && len(plan.ToDispatch) == 0 {
		if dryRun {
			fmt.Printf("No capacity: %d/%d polecats active\n", activePolecats, maxPolecats)
		}
		return 0, nil
	}

	// Format capacity string for display
	capStr := "unlimited"
	if maxPolecats > 0 {
		cap := maxPolecats - activePolecats
		if cap < 0 {
			cap = 0
		}
		capStr = fmt.Sprintf("%d free of %d", cap, maxPolecats)
	}

	if dryRun {
		fmt.Printf("%s Would dispatch %d bead(s) (capacity: %s, batch: %d, ready: %d, reason: %s)\n",
			style.Bold.Render("📋"), len(plan.ToDispatch), capStr, batchSize, len(readyBeads), plan.Reason)
		for _, b := range plan.ToDispatch {
			fmt.Printf("  Would dispatch: %s → %s\n", b.ID, b.TargetRig)
		}
		return 0, nil
	}

	fmt.Printf("%s Dispatching %d bead(s) (capacity: %s, ready: %d)\n",
		style.Bold.Render("▶"), len(plan.ToDispatch), capStr, len(readyBeads))

	dispatched := 0
	successfulRigs := make(map[string]bool)
	for i, b := range plan.ToDispatch {
		fmt.Printf("\n[%d/%d] Dispatching %s → %s...\n", i+1, len(plan.ToDispatch), b.ID, b.TargetRig)

		if err := dispatchSingleBead(b, townRoot, actor); err != nil {
			fmt.Printf("  %s Failed: %v\n", style.Dim.Render("✗"), err)
			continue
		}
		dispatched++
		if b.TargetRig != "" {
			successfulRigs[b.TargetRig] = true
		}

		// Inter-spawn delay to avoid Dolt lock contention
		if i < len(plan.ToDispatch)-1 && spawnDelay > 0 {
			time.Sleep(spawnDelay)
		}
	}

	// Wake rig agents for each unique rig that had successful dispatches.
	for rig := range successfulRigs {
		wakeRigAgents(rig)
	}

	// Update runtime state with fresh read to avoid clobbering concurrent pause.
	if dispatched > 0 {
		freshState, err := capacity.LoadState(townRoot)
		if err != nil {
			fmt.Printf("%s Could not reload scheduler state: %v\n", style.Dim.Render("Warning:"), err)
		} else {
			freshState.RecordDispatch(dispatched)
			if err := capacity.SaveState(townRoot, freshState); err != nil {
				fmt.Printf("%s Could not save scheduler state: %v\n", style.Dim.Render("Warning:"), err)
			}
		}
	}

	fmt.Printf("\n%s Dispatched %d/%d bead(s)\n", style.Bold.Render("✓"), dispatched, len(plan.ToDispatch))
	return dispatched, nil
}

// getReadyScheduledBeads queries for beads that are both scheduled and unblocked.
func getReadyScheduledBeads(townRoot string) ([]capacity.PendingBead, error) {
	var result []capacity.PendingBead
	seen := make(map[string]bool)

	dirs := beadsSearchDirs(townRoot)
	var lastErr error
	failCount := 0

	for _, dir := range dirs {
		beads, err := getReadyScheduledBeadsFrom(dir)
		if err != nil {
			failCount++
			lastErr = err
			fmt.Printf("%s bd ready failed in %s: %v\n", style.Dim.Render("Warning:"), dir, err)
			continue
		}
		for _, b := range beads {
			if !seen[b.ID] {
				seen[b.ID] = true
				result = append(result, b)
			}
		}
	}

	if failCount == len(dirs) && failCount > 0 {
		return nil, fmt.Errorf("all %d bead directories failed (last: %w)", failCount, lastErr)
	}
	return result, nil
}

// getReadyScheduledBeadsFrom queries a single directory for ready scheduled beads.
func getReadyScheduledBeadsFrom(dir string) ([]capacity.PendingBead, error) {
	cmd := exec.Command("bd", "ready", "--label", capacity.LabelScheduled, "--json", "--limit=0")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bd ready failed in %s: %w", dir, err)
	}

	var raw []struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Labels      []string `json:"labels"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing ready beads: %w", err)
	}

	result := make([]capacity.PendingBead, 0, len(raw))
	for _, r := range raw {
		meta := capacity.ParseMetadata(r.Description)
		targetRig := ""
		if meta != nil {
			targetRig = meta.TargetRig
		}
		result = append(result, capacity.PendingBead{
			ID:          r.ID,
			Title:       r.Title,
			TargetRig:   targetRig,
			Description: r.Description,
			Labels:      r.Labels,
			Meta:        meta,
		})
	}
	return result, nil
}

// dispatchSingleBead dispatches one scheduled bead via executeSling.
func dispatchSingleBead(b capacity.PendingBead, townRoot, actor string) error {
	meta := b.Meta
	if meta == nil {
		meta = capacity.ParseMetadata(b.Description)
	}

	// Validate metadata exists
	if meta == nil || meta.TargetRig == "" {
		quarantineErr := fmt.Errorf("missing scheduler metadata or target_rig")
		beadDir := resolveBeadDir(b.ID)
		failCmd := exec.Command("bd", "update", b.ID, "--add-label=gt:dispatch-failed", "--remove-label="+capacity.LabelScheduled)
		failCmd.Dir = beadDir
		if qErr := failCmd.Run(); qErr != nil {
			fmt.Printf("  %s Could not quarantine %s: %v\n", style.Warning.Render("⚠"), b.ID, qErr)
		}
		return quarantineErr
	}

	rigName := b.TargetRig
	if rigName == "" {
		rigName = meta.TargetRig
	}

	// Reconstruct SlingParams from scheduler metadata
	dp := capacity.ReconstructDispatchParams(meta, b.ID)
	params := SlingParams{
		BeadID:           dp.BeadID,
		RigName:          dp.RigName,
		FormulaName:      dp.FormulaName,
		Args:             dp.Args,
		Vars:             dp.Vars,
		Merge:            dp.Merge,
		BaseBranch:       dp.BaseBranch,
		NoMerge:          dp.NoMerge,
		Account:          dp.Account,
		Agent:            dp.Agent,
		HookRawBead:      dp.HookRawBead,
		Mode:             dp.Mode,
		FormulaFailFatal: true,
		CallerContext:    "scheduler-dispatch",
		NoConvoy:         true,
		NoBoot:           true,
		TownRoot:         townRoot,
		BeadsDir:         filepath.Join(townRoot, ".beads"),
	}

	result, err := executeSling(params)
	if err != nil {
		_ = events.LogFeed(events.TypeSchedulerDispatchFailed, actor,
			events.SchedulerDispatchFailedPayload(b.ID, rigName, err.Error()))
		recordDispatchFailure(b, err)
		return fmt.Errorf("sling failed: %w", err)
	}

	// Post-dispatch cleanup: strip metadata and swap labels
	beadDir := resolveBeadDir(b.ID)
	freshInfo, err := getBeadInfo(b.ID)
	if err == nil {
		cleanDesc := capacity.StripMetadata(freshInfo.Description)
		if cleanDesc != freshInfo.Description {
			descCmd := exec.Command("bd", "update", b.ID, "--description="+cleanDesc)
			descCmd.Dir = beadDir
			if descErr := descCmd.Run(); descErr != nil {
				fmt.Printf("  %s Failed to strip metadata from %s: %v\n", style.Warning.Render("⚠"), b.ID, descErr)
			}
		}
	}
	// Label swap with retry to prevent double-dispatch. If executeSling succeeded
	// but the label swap fails, the bead stays gt:queued and the next dispatch cycle
	// would spawn a second polecat for the same work. Retry once, then force-remove
	// gt:queued as a last resort to prevent re-dispatch.
	swapCmd := exec.Command("bd", "update", b.ID,
		"--remove-label="+capacity.LabelScheduled, "--add-label=gt:queue-dispatched")
	swapCmd.Dir = beadDir
	if swapErr := swapCmd.Run(); swapErr != nil {
		// Retry once after brief delay
		time.Sleep(500 * time.Millisecond)
		retryCmd := exec.Command("bd", "update", b.ID,
			"--remove-label="+capacity.LabelScheduled, "--add-label=gt:queue-dispatched")
		retryCmd.Dir = beadDir
		if retryErr := retryCmd.Run(); retryErr != nil {
			// Last resort: force-remove gt:queued to prevent double-dispatch.
			// The bead won't get the gt:queue-dispatched label, but it also won't
			// be re-dispatched on the next cycle.
			stripCmd := exec.Command("bd", "update", b.ID,
				"--remove-label="+capacity.LabelScheduled)
			stripCmd.Dir = beadDir
			if stripErr := stripCmd.Run(); stripErr != nil {
				fmt.Printf("  %s CRITICAL: could not remove %s from %s after successful dispatch — risk of double-dispatch: %v\n",
					style.Warning.Render("⚠"), capacity.LabelScheduled, b.ID, stripErr)
			} else {
				fmt.Printf("  %s Removed %s from %s (label swap failed, gt:queue-dispatched not set): %v\n",
					style.Warning.Render("⚠"), capacity.LabelScheduled, b.ID, retryErr)
			}
		}
	}

	polecatName := ""
	if result != nil && result.SpawnInfo != nil {
		polecatName = result.SpawnInfo.PolecatName
	}
	_ = events.LogFeed(events.TypeSchedulerDispatch, actor,
		events.SchedulerDispatchPayload(b.ID, rigName, polecatName))

	return nil
}

// isDaemonDispatch returns true when dispatch is triggered by the daemon heartbeat.
func isDaemonDispatch() bool {
	return os.Getenv("GT_DAEMON") == "1"
}

// recordDispatchFailure increments the dispatch failure counter in the bead's
// scheduler metadata. This function does a read-modify-write on the description
// which is NOT independently serialized. It is safe because:
//   - The only caller is dispatchSingleBead, which runs inside the dispatch flock
//   - The flock prevents concurrent dispatch of the same bead
//   - Manual operations (gt scheduler clear) are user-initiated and acceptable race
func recordDispatchFailure(b capacity.PendingBead, dispatchErr error) {
	currentDesc := b.Description
	if freshInfo, err := getBeadInfo(b.ID); err == nil {
		currentDesc = freshInfo.Description
	}

	meta := capacity.ParseMetadata(currentDesc)
	if meta == nil {
		meta = &capacity.SchedulerMetadata{}
	}
	meta.DispatchFailures++
	meta.LastFailure = dispatchErr.Error()

	baseDesc := capacity.StripMetadata(currentDesc)
	metaBlock := capacity.FormatMetadata(meta)
	newDesc := baseDesc
	if newDesc != "" {
		newDesc += "\n"
	}
	newDesc += metaBlock

	beadDir := resolveBeadDir(b.ID)
	descCmd := exec.Command("bd", "update", b.ID, "--description="+newDesc)
	descCmd.Dir = beadDir
	if descErr := descCmd.Run(); descErr != nil {
		fmt.Printf("  %s Failed to record dispatch failure metadata for %s: %v\n", style.Warning.Render("⚠"), b.ID, descErr)
	}

	if meta.DispatchFailures >= maxDispatchFailures {
		failCmd := exec.Command("bd", "update", b.ID,
			"--add-label=gt:dispatch-failed", "--remove-label="+capacity.LabelScheduled)
		failCmd.Dir = beadDir
		if labelErr := failCmd.Run(); labelErr != nil {
			fmt.Printf("  %s Failed to apply gt:dispatch-failed label to %s: %v\n", style.Warning.Render("⚠"), b.ID, labelErr)
		}
		fmt.Printf("  %s Bead %s failed %d times, marked gt:dispatch-failed\n",
			style.Warning.Render("⚠"), b.ID, meta.DispatchFailures)
	}
}
