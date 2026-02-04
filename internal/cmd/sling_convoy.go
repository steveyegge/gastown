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
	"github.com/steveyegge/gastown/internal/style"
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
	depCmd := exec.Command("bd", "--no-daemon", "dep", "list", beadID, "--direction=up", "--type=tracks", "--json")
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
// Returns convoy ID if found, empty string otherwise.
func findConvoyByDescription(townRoot, beadID string) string {
	// Query all open convoys from HQ
	listCmd := exec.Command("bd", "--no-daemon", "list", "--type=convoy", "--status=open", "--json")
	listCmd.Dir = filepath.Join(townRoot, ".beads")

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
	trackingPattern := fmt.Sprintf("tracking %s", beadID)
	for _, convoy := range convoys {
		if strings.Contains(convoy.Description, trackingPattern) {
			return convoy.ID
		}
	}

	return ""
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

	createCmd := exec.Command("bd", append([]string{"--no-daemon"}, createArgs...)...)
	createCmd.Dir = townBeads
	createCmd.Stderr = os.Stderr

	if err := createCmd.Run(); err != nil {
		return "", fmt.Errorf("creating convoy: %w", err)
	}

	// Add tracking relation: convoy tracks the issue
	trackBeadID := formatTrackBeadID(beadID)
	depArgs := []string{"--no-daemon", "dep", "add", convoyID, trackBeadID, "--type=tracks"}
	depCmd := exec.Command("bd", depArgs...)
	depCmd.Dir = townBeads
	depCmd.Stderr = os.Stderr

	if err := depCmd.Run(); err != nil {
		// Convoy was created but tracking failed - log warning but continue
		fmt.Printf("%s Could not add tracking relation: %v\n", style.Dim.Render("Warning:"), err)
	}

	return convoyID, nil
}

// formatTrackBeadID formats a bead ID for use in convoy tracking dependencies.
// Cross-rig beads (non-hq- prefixed) are formatted as external references
// so the bd tool can resolve them when running from HQ context.
//
// Examples:
//   - "hq-abc123" -> "hq-abc123" (HQ beads unchanged)
//   - "gt-mol-xyz" -> "external:gt-mol:gt-mol-xyz"
//   - "beads-task-123" -> "external:beads-task:beads-task-123"
func formatTrackBeadID(beadID string) string {
	if strings.HasPrefix(beadID, "hq-") {
		return beadID
	}
	parts := strings.SplitN(beadID, "-", 3)
	if len(parts) >= 2 {
		rigPrefix := parts[0] + "-" + parts[1]
		return fmt.Sprintf("external:%s:%s", rigPrefix, beadID)
	}
	// Fallback for malformed IDs (single segment)
	return beadID
}
