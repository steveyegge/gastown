package cmd

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/scheduler/capacity"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// shouldDeferDispatch checks the town config to decide dispatch mode.
// Returns (true, nil) when max_polecats > 0 (deferred dispatch).
// Returns (false, nil) when max_polecats <= 0 (direct dispatch).
func shouldDeferDispatch() (bool, error) {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return false, nil // No town — direct dispatch
	}

	settingsPath := config.TownSettingsPath(townRoot)
	settings, err := config.LoadOrCreateTownSettings(settingsPath)
	if err != nil {
		return false, fmt.Errorf("loading town settings: %w (dispatch blocked — fix config or use gt config set scheduler.max_polecats -1)", err)
	}

	schedulerCfg := settings.Scheduler
	if schedulerCfg == nil {
		return false, nil // No scheduler config — direct dispatch (default)
	}

	maxPol := schedulerCfg.GetMaxPolecats()
	if maxPol > 0 {
		return true, nil
	}
	return false, nil // -1 or 0 = direct dispatch
}

// ScheduleOptions holds options for scheduling a bead.
type ScheduleOptions struct {
	Formula     string   // Formula to apply at dispatch time (e.g., "mol-polecat-work")
	Args        string   // Natural language args for executor
	Vars        []string // Formula variables (key=value)
	Merge       string   // Merge strategy: direct/mr/local
	BaseBranch  string   // Override base branch for polecat worktree
	NoConvoy    bool     // Skip auto-convoy creation
	Owned       bool     // Mark auto-convoy as caller-managed lifecycle
	DryRun      bool     // Show what would be done without acting
	Force       bool     // Force schedule even if bead is hooked/in_progress
	NoMerge     bool     // Skip merge queue on completion
	Account     string   // Claude Code account handle
	Agent       string   // Agent override (e.g., "gemini", "codex")
	HookRawBead bool     // Hook raw bead without default formula
	Ralph       bool     // Ralph Wiggum loop mode
}

// scheduleBead schedules a bead for deferred dispatch via the capacity scheduler.
func scheduleBead(beadID, rigName string, opts ScheduleOptions) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	if err := verifyBeadExists(beadID); err != nil {
		return fmt.Errorf("bead '%s' not found", beadID)
	}

	if _, isRig := IsRigName(rigName); !isRig {
		return fmt.Errorf("'%s' is not a known rig", rigName)
	}

	if !opts.Force {
		if err := checkCrossRigGuard(beadID, rigName+"/polecats/_", townRoot); err != nil {
			return err
		}
	}

	info, err := getBeadInfo(beadID)
	if err != nil {
		return fmt.Errorf("checking bead status: %w", err)
	}

	// Idempotency: skip if bead is actively scheduled (open + gt:queued label).
	isScheduled := false
	for _, label := range info.Labels {
		if label == capacity.LabelScheduled {
			isScheduled = true
			break
		}
	}
	if isScheduled && info.Status == "open" {
		fmt.Printf("%s Bead %s is already scheduled, no-op\n", style.Dim.Render("○"), beadID)
		return nil
	}

	if (info.Status == "pinned" || info.Status == "hooked") && !opts.Force {
		return fmt.Errorf("bead %s is already %s to %s\nUse --force to override", beadID, info.Status, info.Assignee)
	}

	if opts.Formula != "" {
		if err := verifyFormulaExists(opts.Formula); err != nil {
			return fmt.Errorf("formula %q not found: %w", opts.Formula, err)
		}
	}

	if opts.DryRun {
		fmt.Printf("Would schedule %s → %s\n", beadID, rigName)
		fmt.Printf("  Would add label: %s\n", capacity.LabelScheduled)
		fmt.Printf("  Would append scheduler metadata to description\n")
		if !opts.NoConvoy {
			fmt.Printf("  Would create auto-convoy\n")
		}
		return nil
	}

	// Cook formula after dry-run check to avoid side effects
	if opts.Formula != "" {
		workDir := beads.ResolveHookDir(townRoot, beadID, "")
		if err := CookFormula(opts.Formula, workDir, townRoot); err != nil {
			return fmt.Errorf("formula %q failed to cook: %w", opts.Formula, err)
		}
	}

	// Build scheduler metadata
	meta := capacity.NewMetadata(rigName)
	if opts.Formula != "" {
		meta.Formula = opts.Formula
	}
	if opts.Args != "" {
		meta.Args = opts.Args
	}
	if len(opts.Vars) > 0 {
		meta.Vars = strings.Join(opts.Vars, "\n")
	}
	if opts.Merge != "" {
		meta.Merge = opts.Merge
	}
	if opts.BaseBranch != "" {
		meta.BaseBranch = opts.BaseBranch
	}
	meta.NoMerge = opts.NoMerge
	if opts.Account != "" {
		meta.Account = opts.Account
	}
	if opts.Agent != "" {
		meta.Agent = opts.Agent
	}
	meta.HookRawBead = opts.HookRawBead
	if opts.Ralph {
		meta.Mode = "ralph"
	}
	meta.Owned = opts.Owned

	// Strip any existing metadata before appending new metadata
	baseDesc := capacity.StripMetadata(info.Description)

	metaBlock := capacity.FormatMetadata(meta)
	newDesc := baseDesc
	if newDesc != "" {
		newDesc += "\n"
	}
	newDesc += metaBlock

	// Write metadata FIRST, then add label (atomic activation)
	beadDir := resolveBeadDir(beadID)
	descCmd := exec.Command("bd", "update", beadID, "--description="+newDesc)
	descCmd.Dir = beadDir
	if err := descCmd.Run(); err != nil {
		return fmt.Errorf("writing scheduler metadata: %w", err)
	}

	labelCmd := exec.Command("bd", "update", beadID,
		"--add-label="+capacity.LabelScheduled)
	labelCmd.Dir = beadDir
	var labelStderr bytes.Buffer
	labelCmd.Stderr = &labelStderr
	if err := labelCmd.Run(); err != nil {
		// Roll back metadata
		rollbackCmd := exec.Command("bd", "update", beadID, "--description="+baseDesc)
		rollbackCmd.Dir = beadDir
		_ = rollbackCmd.Run()
		errMsg := strings.TrimSpace(labelStderr.String())
		if errMsg != "" {
			return fmt.Errorf("adding scheduled label: %s", errMsg)
		}
		return fmt.Errorf("adding scheduled label: %w", err)
	}

	// Auto-convoy (unless --no-convoy)
	if !opts.NoConvoy {
		existingConvoy := isTrackedByConvoy(beadID)
		if existingConvoy == "" {
			convoyID, err := createAutoConvoy(beadID, info.Title, opts.Owned, opts.Merge)
			if err != nil {
				fmt.Printf("%s Could not create auto-convoy: %v\n", style.Dim.Render("Warning:"), err)
			} else {
				fmt.Printf("%s Created convoy %s\n", style.Bold.Render("→"), convoyID)
				meta.Convoy = convoyID
				// Re-read the current description to avoid clobbering concurrent updates
				// that may have occurred between label activation and now.
				freshInfo, freshErr := getBeadInfo(beadID)
				currentBase := baseDesc
				if freshErr == nil {
					currentBase = capacity.StripMetadata(freshInfo.Description)
				}
				updatedBlock := capacity.FormatMetadata(meta)
				updatedDesc := currentBase
				if updatedDesc != "" {
					updatedDesc += "\n"
				}
				updatedDesc += updatedBlock
				convoyDescCmd := exec.Command("bd", "update", beadID, "--description="+updatedDesc)
				convoyDescCmd.Dir = beadDir
				if err := convoyDescCmd.Run(); err != nil {
					fmt.Printf("%s Could not update metadata with convoy: %v\n", style.Dim.Render("Warning:"), err)
				}
			}
		} else {
			fmt.Printf("%s Already tracked by convoy %s\n", style.Dim.Render("○"), existingConvoy)
		}
	}

	actor := detectActor()
	_ = events.LogFeed(events.TypeSchedulerEnqueue, actor, events.SchedulerEnqueuePayload(beadID, rigName))

	fmt.Printf("%s Scheduled %s → %s\n", style.Bold.Render("✓"), beadID, rigName)
	return nil
}

// runBatchSchedule schedules multiple beads for deferred dispatch.
// Returns error when all schedule attempts fail.
func runBatchSchedule(beadIDs []string, rigName string) error {
	if slingDryRun {
		fmt.Printf("%s Would schedule %d beads to rig '%s':\n", style.Bold.Render("📋"), len(beadIDs), rigName)
		for _, beadID := range beadIDs {
			fmt.Printf("  Would schedule: %s → %s\n", beadID, rigName)
		}
		return nil
	}

	fmt.Printf("%s Scheduling %d beads to rig '%s'...\n", style.Bold.Render("📋"), len(beadIDs), rigName)

	successCount := 0
	for _, beadID := range beadIDs {
		formula := resolveFormula(slingFormula, slingHookRawBead)
		err := scheduleBead(beadID, rigName, ScheduleOptions{
			Formula:     formula,
			Args:        slingArgs,
			Vars:        slingVars,
			NoConvoy:    slingNoConvoy,
			Owned:       slingOwned,
			Merge:       slingMerge,
			BaseBranch:  slingBaseBranch,
			DryRun:      false,
			Force:       slingForce,
			NoMerge:     slingNoMerge,
			Account:     slingAccount,
			Agent:       slingAgent,
			HookRawBead: slingHookRawBead,
			Ralph:       slingRalph,
		})
		if err != nil {
			fmt.Printf("  %s %s: %v\n", style.Dim.Render("✗"), beadID, err)
			continue
		}
		successCount++
	}

	fmt.Printf("\n%s Scheduled %d/%d beads\n", style.Bold.Render("📊"), successCount, len(beadIDs))
	if successCount == 0 {
		return fmt.Errorf("all %d schedule attempts failed", len(beadIDs))
	}
	return nil
}

// unscheduleBeadLabels removes the gt:queued label and strips scheduler metadata.
func unscheduleBeadLabels(beadID string) error {
	beadDir := resolveBeadDir(beadID)

	info, err := getBeadInfo(beadID)
	if err == nil {
		stripped := capacity.StripMetadata(info.Description)
		if stripped != info.Description {
			cmd := exec.Command("bd", "update", beadID,
				"--description="+stripped,
				"--remove-label="+capacity.LabelScheduled)
			cmd.Dir = beadDir
			return cmd.Run()
		}
	}

	cmd := exec.Command("bd", "update", beadID, "--remove-label="+capacity.LabelScheduled)
	cmd.Dir = beadDir
	return cmd.Run()
}

// resolveRigForBead determines the rig that owns a bead from its ID prefix.
func resolveRigForBead(townRoot, beadID string) string {
	prefix := beads.ExtractPrefix(beadID)
	if prefix == "" {
		return ""
	}
	return beads.GetRigNameForPrefix(townRoot, prefix)
}

// resolveFormula determines the formula name from user flags.
func resolveFormula(explicit string, hookRawBead bool) string {
	if hookRawBead {
		return ""
	}
	if explicit != "" {
		return explicit
	}
	return "mol-polecat-work"
}

// hasScheduledLabel checks if a bead has the gt:queued label.
func hasScheduledLabel(labels []string) bool {
	for _, l := range labels {
		if l == capacity.LabelScheduled {
			return true
		}
	}
	return false
}

// detectSchedulerIDType determines what kind of ID was passed for scheduling.
// Returns "convoy", "epic", or "task".
func detectSchedulerIDType(id string) (string, error) {
	// Fast path: hq-cv-* is always a convoy
	if strings.HasPrefix(id, "hq-cv-") {
		return "convoy", nil
	}

	info, err := getBeadInfo(id)
	if err != nil {
		return "", fmt.Errorf("cannot resolve bead '%s': %w", id, err)
	}

	switch info.IssueType {
	case "epic":
		return "epic", nil
	case "convoy":
		return "convoy", nil
	}

	for _, label := range info.Labels {
		switch label {
		case "gt:epic":
			return "epic", nil
		case "gt:convoy":
			return "convoy", nil
		}
	}

	return "task", nil
}

// schedulerTaskOnlyFlagNames lists flags that only apply to task bead scheduling,
// not convoy or epic mode.
var schedulerTaskOnlyFlagNames = []string{
	"account", "agent", "ralph", "args", "var",
	"merge", "base-branch", "no-convoy", "owned", "no-merge",
}

// validateNoTaskOnlySchedulerFlags checks that no task-only flags were set.
func validateNoTaskOnlySchedulerFlags(cmd *cobra.Command, mode string) error {
	var used []string
	for _, name := range schedulerTaskOnlyFlagNames {
		if f := cmd.Flags().Lookup(name); f != nil && f.Changed {
			used = append(used, "--"+name)
		}
	}
	if len(used) > 0 {
		return fmt.Errorf("%s mode does not support: %s\nThese flags only apply to task bead scheduling",
			mode, strings.Join(used, ", "))
	}
	return nil
}
