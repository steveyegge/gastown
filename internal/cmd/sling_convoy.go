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
// Supports both SQLite and Dolt backends via beads.RunQuery.
func isTrackedByConvoy(beadID string) string {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return ""
	}

	// Query town beads for any convoy that tracks this issue
	// Convoys use "tracks" dependency type: convoy -> tracked issue
	townBeads := filepath.Join(townRoot, ".beads")

	// Query dependencies where this bead is being tracked
	// Also check for external reference format: external:rig:issue-id
	query := fmt.Sprintf(`
		SELECT d.issue_id
		FROM dependencies d
		JOIN issues i ON d.issue_id = i.id
		WHERE d.type = 'tracks'
		AND i.issue_type = 'convoy'
		AND i.status = 'open'
		AND (d.depends_on_id = '%s' OR d.depends_on_id LIKE '%%:%s')
		LIMIT 1
	`, beadID, beadID)

	// Use backend-aware query (works with SQLite or Dolt)
	results, err := beads.RunQuery(townBeads, query)
	if err != nil || len(results) == 0 {
		return ""
	}

	convoyID, _ := results[0]["issue_id"].(string)
	return strings.TrimSpace(convoyID)
}

// createAutoConvoy creates an auto-convoy for a single issue and tracks it.
// The convoy is assigned to the same agent that is working on the tracked bead.
// Returns the created convoy ID.
func createAutoConvoy(beadID, beadTitle, assignee string) (string, error) {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return "", fmt.Errorf("finding town root: %w", err)
	}

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
	if assignee != "" {
		createArgs = append(createArgs, "--assignee="+assignee)
	}
	if beads.NeedsForceForID(convoyID) {
		createArgs = append(createArgs, "--force")
	}

	createCmd := exec.Command("bd", append([]string{"--no-daemon"}, createArgs...)...)
	createCmd.Dir = townRoot // Run from town root so bd can find .beads/config.yaml
	createCmd.Stderr = os.Stderr

	if err := createCmd.Run(); err != nil {
		return "", fmt.Errorf("creating convoy: %w", err)
	}

	// Add tracking relation: convoy tracks the issue
	trackBeadID := formatTrackBeadID(beadID)
	depArgs := []string{"--no-daemon", "dep", "add", convoyID, trackBeadID, "--type=tracks"}
	depCmd := exec.Command("bd", depArgs...)
	depCmd.Dir = townRoot // Run from town root so bd can find .beads/config.yaml
	depCmd.Stderr = os.Stderr

	if err := depCmd.Run(); err != nil {
		// Convoy was created but tracking failed - log warning but continue
		fmt.Printf("%s Could not add tracking relation: %v\n", style.Dim.Render("Warning:"), err)
	}

	return convoyID, nil
}

// addToConvoy adds a bead to an existing convoy.
// If the convoy is closed, it will be reopened.
func addToConvoy(convoyID, beadID string) error {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Check if convoy exists and get its status
	showArgs := []string{"--no-daemon", "show", convoyID, "--json"}
	showCmd := exec.Command("bd", showArgs...)
	showCmd.Dir = townRoot
	out, err := showCmd.Output()
	if err != nil {
		return fmt.Errorf("convoy '%s' not found", convoyID)
	}

	// Parse convoy data to check status
	var convoys []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Type   string `json:"issue_type"`
	}
	if err := parseJSON(out, &convoys); err != nil || len(convoys) == 0 {
		return fmt.Errorf("convoy '%s' not found", convoyID)
	}

	convoy := convoys[0]
	if convoy.Type != "convoy" {
		return fmt.Errorf("'%s' is not a convoy (type: %s)", convoyID, convoy.Type)
	}

	// If convoy is closed, reopen it
	if convoy.Status == "closed" {
		reopenArgs := []string{"--no-daemon", "update", convoyID, "--status=open"}
		reopenCmd := exec.Command("bd", reopenArgs...)
		reopenCmd.Dir = townRoot
		if err := reopenCmd.Run(); err != nil {
			return fmt.Errorf("couldn't reopen convoy: %w", err)
		}
		fmt.Printf("%s Reopened convoy %s\n", style.Bold.Render("↺"), convoyID)
	}

	// Add tracking relation: convoy tracks the issue
	trackBeadID := formatTrackBeadID(beadID)
	depArgs := []string{"--no-daemon", "dep", "add", convoyID, trackBeadID, "--type=tracks"}
	depCmd := exec.Command("bd", depArgs...)
	depCmd.Dir = townRoot
	depCmd.Stderr = os.Stderr

	if err := depCmd.Run(); err != nil {
		return fmt.Errorf("adding tracking relation: %w", err)
	}

	return nil
}

// parseJSON is a helper to unmarshal JSON into a target.
func parseJSON(data []byte, target interface{}) error {
	if len(data) == 0 {
		return fmt.Errorf("empty data")
	}
	return json.Unmarshal(data, target)
}

// formatTrackBeadID formats a bead ID for use in convoy tracking dependencies.
// Cross-rig beads (non-hq- prefixed) are formatted as external references
// so the bd tool can resolve them when running from HQ context.
//
// The external ref format is "external:<project>:<bead-id>" where project
// is derived from routes.jsonl (e.g., "gastown", "beads"), not from the
// bead ID prefix. This aligns with bd's routing.ResolveToExternalRef().
//
// Examples (with routes {"prefix":"gt-","path":"gastown/mayor/rig"}):
//   - "hq-abc123" -> "hq-abc123" (HQ beads unchanged)
//   - "gt-mol-abc123" -> "external:gastown:gt-mol-abc123"
//   - "bd-xyz" -> "external:beads:bd-xyz"
//
// Falls back to legacy prefix-based format if routes lookup fails.
func formatTrackBeadID(beadID string) string {
	if strings.HasPrefix(beadID, "hq-") {
		return beadID
	}

	// Try to resolve via routes.jsonl for proper external ref format
	townRoot, err := workspace.FindFromCwd()
	if err == nil {
		if extRef := beads.ResolveToExternalRef(townRoot, beadID); extRef != "" {
			return extRef
		}
	}

	// Fallback: if no route found, use legacy prefix-based format
	// This maintains backwards compatibility for repos without routes.jsonl
	parts := strings.SplitN(beadID, "-", 3)
	switch len(parts) {
	case 0, 1:
		// Single segment or empty - return as-is (fallback)
		return beadID
	case 2:
		// Two segments - use the whole ID as the prefix
		return fmt.Sprintf("external:%s:%s", beadID, beadID)
	default:
		// Three+ segments - use first two as the prefix
		rigPrefix := parts[0] + "-" + parts[1]
		return fmt.Sprintf("external:%s:%s", rigPrefix, beadID)
	}
}
