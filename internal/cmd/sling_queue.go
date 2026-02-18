package cmd

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// EnqueueOptions holds options for enqueueing a bead.
type EnqueueOptions struct {
	Formula    string   // Formula to apply at dispatch time (e.g., "mol-polecat-work")
	Args       string   // Natural language args for executor
	Vars       []string // Formula variables (key=value)
	Merge      string   // Merge strategy: direct/mr/local
	BaseBranch string   // Override base branch for polecat worktree
	NoConvoy   bool     // Skip auto-convoy creation
	Owned      bool     // Mark auto-convoy as caller-managed lifecycle
	DryRun     bool     // Show what would be done without acting
	Force      bool     // Force enqueue even if bead is hooked/in_progress
}

const (
	// LabelQueued marks a bead as queued for dispatch.
	LabelQueued = "gt:queued"
	// LabelQueueRigPrefix is the prefix for the target rig label.
	LabelQueueRigPrefix = "gt:queue-rig:"
)

// enqueueBead queues a bead for deferred dispatch via the work queue.
// It adds labels, writes queue metadata to the description, and creates
// an auto-convoy. Does NOT spawn a polecat or hook the bead.
func enqueueBead(beadID, rigName string, opts EnqueueOptions) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Validate bead exists
	if err := verifyBeadExists(beadID); err != nil {
		return fmt.Errorf("bead '%s' not found", beadID)
	}

	// Validate rig exists
	if _, isRig := IsRigName(rigName); !isRig {
		return fmt.Errorf("'%s' is not a known rig", rigName)
	}

	// Get bead info for status/label checks
	info, err := getBeadInfo(beadID)
	if err != nil {
		return fmt.Errorf("checking bead status: %w", err)
	}

	// Idempotency: skip if already queued
	for _, label := range info.Labels {
		if label == LabelQueued {
			fmt.Printf("%s Bead %s is already queued, no-op\n", style.Dim.Render("○"), beadID)
			return nil
		}
	}

	// Check status: error if hooked/in_progress (unless --force)
	if (info.Status == "pinned" || info.Status == "hooked") && !opts.Force {
		return fmt.Errorf("bead %s is already %s to %s\nUse --force to override", beadID, info.Status, info.Assignee)
	}

	if opts.DryRun {
		fmt.Printf("Would queue %s → %s\n", beadID, rigName)
		fmt.Printf("  Would add labels: %s, %s%s\n", LabelQueued, LabelQueueRigPrefix, rigName)
		fmt.Printf("  Would append queue metadata to description\n")
		if !opts.NoConvoy {
			fmt.Printf("  Would create auto-convoy\n")
		}
		return nil
	}

	// Add labels: gt:queued + gt:queue-rig:<rigName>
	rigLabel := LabelQueueRigPrefix + rigName
	labelCmd := exec.Command("bd", "update", beadID,
		"--add-label="+LabelQueued,
		"--add-label="+rigLabel)
	labelCmd.Dir = townRoot
	var labelStderr bytes.Buffer
	labelCmd.Stderr = &labelStderr
	if err := labelCmd.Run(); err != nil {
		errMsg := strings.TrimSpace(labelStderr.String())
		if errMsg != "" {
			return fmt.Errorf("adding queue labels: %s", errMsg)
		}
		return fmt.Errorf("adding queue labels: %w", err)
	}

	// Build queue metadata
	meta := NewQueueMetadata(rigName)
	if opts.Formula != "" {
		meta.Formula = opts.Formula
	}
	if opts.Args != "" {
		meta.Args = opts.Args
	}
	if len(opts.Vars) > 0 {
		meta.Vars = strings.Join(opts.Vars, ",")
	}
	if opts.Merge != "" {
		meta.Merge = opts.Merge
	}
	if opts.BaseBranch != "" {
		meta.BaseBranch = opts.BaseBranch
	}

	// Append queue metadata to bead description
	metaBlock := FormatQueueMetadata(meta)
	newDesc := info.Description
	if newDesc != "" {
		newDesc += "\n"
	}
	newDesc += metaBlock

	descCmd := exec.Command("bd", "update", beadID, "--description="+newDesc)
	descCmd.Dir = townRoot
	if err := descCmd.Run(); err != nil {
		// Best effort: labels are set, metadata is nice-to-have
		fmt.Printf("%s Could not write queue metadata: %v\n", style.Dim.Render("Warning:"), err)
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
				// Store convoy in metadata for dispatch
				meta.Convoy = convoyID
			}
		} else {
			fmt.Printf("%s Already tracked by convoy %s\n", style.Dim.Render("○"), existingConvoy)
		}
	}

	// Log enqueue event
	actor := detectActor()
	_ = events.LogFeed(events.TypeQueueEnqueue, actor, events.QueueEnqueuePayload(beadID, rigName))

	fmt.Printf("%s Queued %s → %s\n", style.Bold.Render("✓"), beadID, rigName)
	return nil
}

// runBatchEnqueue enqueues multiple beads for deferred dispatch.
// Called from sling when --queue is set with multiple beads and a rig target.
func runBatchEnqueue(beadIDs []string, rigName string) error {
	if slingDryRun {
		fmt.Printf("%s Would queue %d beads to rig '%s':\n", style.Bold.Render("📋"), len(beadIDs), rigName)
		for _, beadID := range beadIDs {
			fmt.Printf("  Would queue: %s → %s\n", beadID, rigName)
		}
		return nil
	}

	fmt.Printf("%s Queuing %d beads to rig '%s'...\n", style.Bold.Render("📋"), len(beadIDs), rigName)

	successCount := 0
	for _, beadID := range beadIDs {
		err := enqueueBead(beadID, rigName, EnqueueOptions{
			Args:     slingArgs,
			Vars:     slingVars,
			NoConvoy: slingNoConvoy,
			Owned:    slingOwned,
			Merge:    slingMerge,
			DryRun:   false,
			Force:    slingForce,
		})
		if err != nil {
			fmt.Printf("  %s %s: %v\n", style.Dim.Render("✗"), beadID, err)
			continue
		}
		successCount++
	}

	fmt.Printf("\n%s Queued %d/%d beads\n", style.Bold.Render("📊"), successCount, len(beadIDs))
	return nil
}

// dequeueBeadLabels removes queue labels from a bead (claim for dispatch).
// Called by the dispatcher when a bead is about to be dispatched.
func dequeueBeadLabels(beadID, townRoot string) error {
	// First get the bead's labels to find the exact queue-rig label
	info, err := getBeadInfo(beadID)
	if err != nil {
		return fmt.Errorf("getting bead info: %w", err)
	}

	args := []string{"update", beadID, "--remove-label=" + LabelQueued}
	for _, label := range info.Labels {
		if strings.HasPrefix(label, LabelQueueRigPrefix) {
			args = append(args, "--remove-label="+label)
		}
	}

	cmd := exec.Command("bd", args...)
	cmd.Dir = townRoot
	return cmd.Run()
}

// getQueueRig extracts the target rig name from a bead's queue labels.
func getQueueRig(labels []string) string {
	for _, label := range labels {
		if strings.HasPrefix(label, LabelQueueRigPrefix) {
			return strings.TrimPrefix(label, LabelQueueRigPrefix)
		}
	}
	return ""
}

// hasQueuedLabel checks if a bead has the gt:queued label.
func hasQueuedLabel(labels []string) bool {
	for _, l := range labels {
		if l == LabelQueued {
			return true
		}
	}
	return false
}
