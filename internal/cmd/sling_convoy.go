package cmd

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
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

// createAutoConvoy creates an auto-convoy for a single issue and tracks it.
// Returns the created convoy ID.
func createAutoConvoy(beadID, beadTitle string) (string, error) {
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

	createArgs := []string{
		"create",
		"--type=convoy",
		"--id=" + convoyID,
		"--title=" + convoyTitle,
		"--description=" + description,
	}
	if beads.NeedsForceForID(convoyID) {
		createArgs = append(createArgs, "--force")
	}

	createCmd := exec.Command("bd", createArgs...)
	createCmd.Dir = townBeads
	createCmd.Stderr = os.Stderr

	if err := createCmd.Run(); err != nil {
		return "", fmt.Errorf("creating convoy: %w", err)
	}

	// Add tracking relation: convoy tracks the issue.
	// Pass the raw beadID and let bd handle cross-rig resolution via routes.jsonl,
	// matching what gt convoy create/add already do (convoy.go:368, convoy.go:464).
	depArgs := []string{"dep", "add", convoyID, beadID, "--type=tracks"}
	depCmd := exec.Command("bd", depArgs...)
	depCmd.Dir = townRoot
	depCmd.Stderr = os.Stderr

	if err := depCmd.Run(); err != nil {
		// Tracking failed — delete the orphan convoy to prevent accumulation
		delCmd := exec.Command("bd", "close", convoyID, "-r", "tracking dep failed")
		delCmd.Dir = townRoot
		_ = delCmd.Run()
		return "", fmt.Errorf("adding tracking relation for %s: %w", beadID, err)
	}

	return convoyID, nil
}
