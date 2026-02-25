package cmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/telemetry"
	"github.com/steveyegge/gastown/internal/workspace"
)

// slingGenerateShortID generates a short random ID (5 lowercase chars).
func slingGenerateShortID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return strings.ToLower(base32.StdEncoding.EncodeToString(b)[:5])
}

// isTrackedByConvoy checks if an issue is already being tracked by a convoy.
// Returns the convoy ID if tracked, empty string otherwise.
func isTrackedByConvoy(beadID string) string {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return ""
	}

	// Primary: Use bd dep list to find what tracks this issue (direction=up)
	// This is authoritative when cross-rig routing works
	depCmd := exec.Command("bd", "dep", "list", beadID, "--direction=up", "--type=tracks", "--json")
	depCmd.Dir = townRoot

	out, err := depCmd.Output()
	if err == nil {
		var trackers []struct {
			ID        string `json:"id"`
			IssueType string `json:"issue_type"`
			Status    string `json:"status"`
		}
		if err := json.Unmarshal(out, &trackers); err == nil {
			for _, tracker := range trackers {
				if tracker.IssueType == "convoy" && tracker.Status == "open" {
					return tracker.ID
				}
			}
		}
	}

	// Fallback: Query convoys directly by description pattern
	// This is more robust when cross-rig routing has issues (G19, G21)
	// Auto-convoys have description "Auto-created convoy tracking <beadID>"
	return findConvoyByDescription(townRoot, beadID)
}

// findConvoyByDescription searches open convoys for one tracking the given beadID.
// Checks both convoy descriptions (for auto-created convoys) and tracked deps
// (for manually-created convoys where the description won't match).
// Returns convoy ID if found, empty string otherwise.
func findConvoyByDescription(townRoot, beadID string) string {
	townBeads := filepath.Join(townRoot, ".beads")

	// Query all open convoys from HQ
	listCmd := exec.Command("bd", "list", "--type=convoy", "--status=open", "--json")
	listCmd.Dir = townBeads

	out, err := listCmd.Output()
	if err != nil {
		return ""
	}

	var convoys []struct {
		ID          string `json:"id"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(out, &convoys); err != nil {
		return ""
	}

	// Check if any convoy's description mentions tracking this beadID
	// (matches auto-created convoys with "Auto-created convoy tracking <beadID>")
	trackingPattern := fmt.Sprintf("tracking %s", beadID)
	for _, convoy := range convoys {
		if strings.Contains(convoy.Description, trackingPattern) {
			return convoy.ID
		}
	}

	// Check tracked deps of each convoy (for manually-created convoys).
	// This handles the case where cross-rig dep resolution (direction=up) fails
	// but the convoy does have a tracks dependency on the bead.
	for _, convoy := range convoys {
		if convoyTracksBead(townBeads, convoy.ID, beadID) {
			return convoy.ID
		}
	}

	return ""
}

// convoyTracksBead checks if a convoy has a tracks dependency on the given beadID.
// Handles both raw bead IDs and external-formatted references (e.g., "external:gt-mol:gt-mol-xyz").
func convoyTracksBead(beadsDir, convoyID, beadID string) bool {
	depCmd := exec.Command("bd", "dep", "list", convoyID, "--direction=down", "--type=tracks", "--json")
	depCmd.Dir = beadsDir

	out, err := depCmd.Output()
	if err != nil {
		return false
	}

	var tracked []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &tracked); err != nil {
		return false
	}

	for _, t := range tracked {
		// Exact match (raw beadID stored as-is)
		if t.ID == beadID {
			return true
		}
		// External reference match: unwrap "external:prefix:beadID" format
		if strings.HasPrefix(t.ID, "external:") {
			parts := strings.SplitN(t.ID, ":", 3)
			if len(parts) == 3 && parts[2] == beadID {
				return true
			}
		}
	}

	return false
}

// ConvoyInfo holds convoy details for an issue's tracking convoy.
type ConvoyInfo struct {
	ID            string // Convoy bead ID (e.g., "hq-cv-abc")
	Owned         bool   // true if convoy has gt:owned label
	MergeStrategy string // "direct", "mr", "local", or "" (default = mr)
}

// IsOwnedDirect returns true if the convoy is owned with direct merge strategy.
// This is the key check for skipping witness/refinery merge pipeline.
func (c *ConvoyInfo) IsOwnedDirect() bool {
	return c != nil && c.Owned && c.MergeStrategy == "direct"
}

// getConvoyInfoForIssue checks if an issue is tracked by a convoy and returns its info.
// Returns nil if not tracked by any convoy.
func getConvoyInfoForIssue(issueID string) *ConvoyInfo {
	convoyID := isTrackedByConvoy(issueID)
	if convoyID == "" {
		return nil
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return nil
	}
	townBeads := filepath.Join(townRoot, ".beads")

	// Get convoy details (labels + description) for ownership and merge strategy
	showCmd := exec.Command("bd", "show", convoyID, "--json")
	showCmd.Dir = townBeads
	var stdout, stderr bytes.Buffer
	showCmd.Stdout = &stdout
	showCmd.Stderr = &stderr

	if err := showCmd.Run(); err != nil {
		// Check if this is a "not found" error (phantom convoy) vs transient error.
		// Phantom convoys occur when a convoy bead is deleted from HQ but tracking
		// deps still exist in local beads DB (gt-9xum2). Return nil to treat as
		// untracked, allowing normal MR flow to proceed.
		stderrStr := stderr.String()
		if strings.Contains(stderrStr, "not found") ||
			strings.Contains(stderrStr, "Issue not found") ||
			strings.Contains(stderrStr, "no issue found") {
			return nil // Phantom convoy - proceed without convoy context
		}
		// Other error (transient) - return basic info as fallback
		return &ConvoyInfo{ID: convoyID}
	}

	var convoys []struct {
		Labels      []string `json:"labels"`
		Description string   `json:"description"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &convoys); err != nil || len(convoys) == 0 {
		return &ConvoyInfo{ID: convoyID}
	}

	info := &ConvoyInfo{ID: convoyID}

	// Check for gt:owned label
	for _, label := range convoys[0].Labels {
		if label == "gt:owned" {
			info.Owned = true
			break
		}
	}

	// Parse merge strategy from description
	info.MergeStrategy = parseConvoyMergeStrategy(convoys[0].Description)

	return info
}

// getConvoyInfoFromIssue reads convoy info directly from the issue's attachment fields.
// This is the primary lookup method (gt-7b6wf fix): gt sling stores convoy_id and
// merge_strategy on the issue when dispatching, avoiding unreliable cross-rig dep
// resolution. Returns nil if the issue has no convoy fields in its description.
func getConvoyInfoFromIssue(issueID, cwd string) *ConvoyInfo {
	if issueID == "" {
		return nil
	}

	bd := beads.New(beads.ResolveBeadsDir(cwd))
	issue, err := bd.Show(issueID)
	if err != nil {
		return nil
	}

	attachment := beads.ParseAttachmentFields(issue)
	if attachment == nil || attachment.ConvoyID == "" {
		return nil
	}

	return &ConvoyInfo{
		ID:            attachment.ConvoyID,
		MergeStrategy: attachment.MergeStrategy,
		Owned:         attachment.ConvoyOwned,
	}
}

// printConvoyConflict prints detailed information about a bead that is already
// tracked by another convoy, including all beads in that convoy with their
// statuses, and recommended actions the user can take.
func printConvoyConflict(beadID, convoyID string) {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		fmt.Printf("\n  %s is already tracked by convoy %s\n", beadID, convoyID)
		return
	}
	townBeads := filepath.Join(townRoot, ".beads")

	// Get convoy title
	var convoyTitle string
	showCmd := exec.Command("bd", "show", convoyID, "--json")
	showCmd.Dir = townBeads
	var showOut bytes.Buffer
	showCmd.Stdout = &showOut
	if err := showCmd.Run(); err == nil {
		var items []struct {
			Title string `json:"title"`
		}
		if json.Unmarshal(showOut.Bytes(), &items) == nil && len(items) > 0 {
			convoyTitle = items[0].Title
		}
	}

	fmt.Printf("\n  Conflict: %s is already tracked by convoy %s", beadID, convoyID)
	if convoyTitle != "" {
		fmt.Printf(" (%s)", convoyTitle)
	}
	fmt.Println()

	// Get all beads in the conflicting convoy
	tracked, err := getTrackedIssues(townBeads, convoyID)
	if err == nil && len(tracked) > 0 {
		fmt.Printf("\n  Beads in convoy %s:\n", convoyID)
		for _, t := range tracked {
			marker := " "
			if t.ID == beadID {
				marker = "→"
			}
			statusIcon := "○"
			switch t.Status {
			case "open":
				statusIcon = "●"
			case "closed":
				statusIcon = "✓"
			case "hooked", "pinned":
				statusIcon = "◆"
			}
			title := t.Title
			if title == "" {
				title = "(no title)"
			}
			suffix := ""
			if t.ID == beadID {
				suffix = "  ← conflict"
			}
			fmt.Printf("    %s %s %s  %s [%s]%s\n", marker, statusIcon, t.ID, title, t.Status, suffix)
		}
	}

	fmt.Printf("\n  Options:\n")
	fmt.Printf("    1. Remove the bead from this batch:\n")
	fmt.Printf("         gt sling <other-beads...> <rig>   (without %s)\n", beadID)
	fmt.Printf("    2. Move the bead to the new batch (remove from existing convoy first):\n")
	fmt.Printf("         bd dep remove %s %s --type=tracks\n", convoyID, beadID)
	fmt.Printf("         gt sling <all-beads...> <rig>\n")
	fmt.Printf("    3. Close the existing convoy and re-sling all beads together:\n")
	fmt.Printf("         gt convoy close %s --reason \"re-batching\"\n", convoyID)
	fmt.Printf("         gt sling <all-beads...> <rig>\n")
	fmt.Printf("    4. Add the other beads to the existing convoy instead:\n")
	fmt.Printf("         gt convoy add %s <other-beads...>\n", convoyID)
	fmt.Println()
}

// createBatchConvoy creates a single auto-convoy that tracks all beads in a batch sling.
// Returns the convoy ID and the list of bead IDs that were successfully tracked.
// Callers should only stamp ConvoyID on beads in the tracked set — a bead whose
// dep add failed should not reference a convoy that has no knowledge of it.
// If owned is true, the convoy is marked with gt:owned label.
// beadIDs must be non-empty. The convoy title uses the rig name and bead count.
func createBatchConvoy(beadIDs []string, rigName string, owned bool, mergeStrategy string) (string, []string, error) {
	if len(beadIDs) == 0 {
		return "", nil, fmt.Errorf("no beads to track")
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return "", nil, fmt.Errorf("finding town root: %w", err)
	}

	townBeads := filepath.Join(townRoot, ".beads")

	convoyID := fmt.Sprintf("hq-cv-%s", slingGenerateShortID())

	convoyTitle := fmt.Sprintf("Batch: %d beads to %s", len(beadIDs), rigName)
	description := fmt.Sprintf("Auto-created convoy tracking %d beads", len(beadIDs))
	if mergeStrategy != "" {
		description += fmt.Sprintf("\nMerge: %s", mergeStrategy)
	}

	createArgs := []string{
		"create",
		"--type=convoy",
		"--id=" + convoyID,
		"--title=" + convoyTitle,
		"--description=" + description,
	}
	if owned {
		createArgs = append(createArgs, "--labels=gt:owned")
	}
	if beads.NeedsForceForID(convoyID) {
		createArgs = append(createArgs, "--force")
	}

	// Use BdCmd with WithAutoCommit to ensure convoy is persisted even when
	// gt sling has set BD_DOLT_AUTO_COMMIT=off globally (gt-9xum2 root cause fix).
	if out, err := BdCmd(createArgs...).Dir(townBeads).WithAutoCommit().CombinedOutput(); err != nil {
		return "", nil, fmt.Errorf("creating batch convoy: %w\noutput: %s", err, out)
	}

	// Add tracking relations for all beads, recording which succeed.
	// Use WithAutoCommit for the same reason as above.
	var tracked []string
	for _, beadID := range beadIDs {
		depArgs := []string{"dep", "add", convoyID, beadID, "--type=tracks"}
		if out, err := BdCmd(depArgs...).Dir(townRoot).WithAutoCommit().CombinedOutput(); err != nil {
			// Log but continue — partial tracking is better than no tracking
			fmt.Printf("  Warning: could not track %s in convoy: %v\nOutput: %s\n", beadID, err, out)
		} else {
			tracked = append(tracked, beadID)
		}
	}

	return convoyID, tracked, nil
}

// createAutoConvoy creates an auto-convoy for a single issue and tracks it.
// If owned is true, the convoy is marked with the gt:owned label for caller-managed lifecycle.
// mergeStrategy is optional: "direct", "mr", or "local" (empty = default mr).
// Returns the created convoy ID.
func createAutoConvoy(beadID, beadTitle string, owned bool, mergeStrategy string) (_ string, retErr error) {
	defer func() { telemetry.RecordConvoyCreate(context.Background(), beadID, retErr) }()
	// Guard against flag-like titles propagating into convoy names (gt-e0kx5)
	if beads.IsFlagLikeTitle(beadTitle) {
		return "", fmt.Errorf("refusing to create convoy: bead title %q looks like a CLI flag", beadTitle)
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return "", fmt.Errorf("finding town root: %w", err)
	}

	townBeads := filepath.Join(townRoot, ".beads")

	// Generate convoy ID with hq-cv- prefix for visual distinction
	// The hq-cv- prefix is registered in routes during gt install
	convoyID := fmt.Sprintf("hq-cv-%s", slingGenerateShortID())

	// Create convoy with title "Work: <issue-title>"
	convoyTitle := fmt.Sprintf("Work: %s", beadTitle)
	description := fmt.Sprintf("Auto-created convoy tracking %s", beadID)
	if mergeStrategy != "" {
		description += fmt.Sprintf("\nMerge: %s", mergeStrategy)
	}

	createArgs := []string{
		"create",
		"--type=convoy",
		"--id=" + convoyID,
		"--title=" + convoyTitle,
		"--description=" + description,
	}
	if owned {
		createArgs = append(createArgs, "--labels=gt:owned")
	}
	if beads.NeedsForceForID(convoyID) {
		createArgs = append(createArgs, "--force")
	}

	// Use BdCmd with WithAutoCommit to ensure convoy is persisted even when
	// gt sling has set BD_DOLT_AUTO_COMMIT=off globally (gt-9xum2 root cause fix).
	if out, err := BdCmd(createArgs...).Dir(townBeads).WithAutoCommit().CombinedOutput(); err != nil {
		return "", fmt.Errorf("creating convoy: %w\noutput: %s", err, out)
	}

	// Add tracking relation: convoy tracks the issue.
	// Pass the raw beadID and let bd handle cross-rig resolution via routes.jsonl,
	// matching what gt convoy create/add already do (convoy.go:368, convoy.go:464).
	// Use WithAutoCommit for the same reason as above.
	depArgs := []string{"dep", "add", convoyID, beadID, "--type=tracks"}
	if out, err := BdCmd(depArgs...).Dir(townRoot).WithAutoCommit().CombinedOutput(); err != nil {
		// Tracking failed — delete the orphan convoy to prevent accumulation
		_ = BdCmd("close", convoyID, "-r", "tracking dep failed").Dir(townRoot).Run()
		return "", fmt.Errorf("adding tracking relation for %s: %w\noutput: %s", beadID, err, out)
	}

	return convoyID, nil
}
