// Package convoy provides shared convoy operations for redundant observers.
package convoy

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
)

// CheckConvoysForIssue finds any convoys tracking the given issue and triggers
// convoy completion checks. This enables redundant convoy observation from
// multiple agents (Witness, Refinery, Daemon).
//
// The check is idempotent - running it multiple times for the same issue is safe.
// The underlying `gt convoy check` handles already-closed convoys gracefully.
//
// Parameters:
//   - townRoot: path to the town root directory
//   - issueID: the issue ID that was just closed
//   - observer: identifier for logging (e.g., "witness", "refinery")
//   - logger: optional logger function (can be nil)
//
// Returns the convoy IDs that were checked (may be empty if issue isn't tracked).
func CheckConvoysForIssue(townRoot, issueID, observer string, logger func(format string, args ...interface{})) []string {
	if logger == nil {
		logger = func(format string, args ...interface{}) {} // no-op
	}

	// Find convoys tracking this issue
	convoyIDs := getTrackingConvoys(townRoot, issueID)
	if len(convoyIDs) == 0 {
		return nil
	}

	logger("%s: issue %s is tracked by %d convoy(s): %v", observer, issueID, len(convoyIDs), convoyIDs)

	// Run convoy check for each tracking convoy
	// Note: gt convoy check is idempotent and handles already-closed convoys
	for _, convoyID := range convoyIDs {
		if isConvoyClosed(townRoot, convoyID) {
			logger("%s: convoy %s already closed, skipping", observer, convoyID)
			continue
		}

		logger("%s: running convoy check for %s", observer, convoyID)
		if err := runConvoyCheck(townRoot, convoyID); err != nil {
			logger("%s: convoy check failed: %v", observer, err)
		}
	}

	return convoyIDs
}

// getTrackingConvoys returns convoy IDs that track the given issue.
// Uses beads.RunQuery to respect storage backend configuration (SQLite or Dolt).
func getTrackingConvoys(townRoot, issueID string) []string {
	townBeads := filepath.Join(townRoot, ".beads")

	// Query for convoys that track this issue
	// Handle both direct ID and external reference format
	safeIssueID := strings.ReplaceAll(issueID, "'", "''")

	// Query for dependencies where this issue is the target
	// Convoys use "tracks" type: convoy -> tracked issue (depends_on_id)
	query := fmt.Sprintf(`
		SELECT DISTINCT issue_id FROM dependencies
		WHERE type = 'tracks'
		AND (depends_on_id = '%s' OR depends_on_id LIKE '%%:%s')
	`, safeIssueID, safeIssueID)

	// Use beads.RunQuery to respect storage backend configuration (SQLite or Dolt)
	results, err := beads.RunQuery(townBeads, query)
	if err != nil {
		return nil
	}

	convoyIDs := make([]string, 0, len(results))
	for _, r := range results {
		if id, ok := r["issue_id"].(string); ok {
			convoyIDs = append(convoyIDs, id)
		}
	}
	return convoyIDs
}

// isConvoyClosed checks if a convoy is already closed.
func isConvoyClosed(townRoot, convoyID string) bool {
	townBeads := filepath.Join(townRoot, ".beads")

	safeConvoyID := strings.ReplaceAll(convoyID, "'", "''")
	query := fmt.Sprintf(`SELECT status FROM issues WHERE id = '%s'`, safeConvoyID)

	// Use beads.RunQuery to respect storage backend configuration (SQLite or Dolt)
	results, err := beads.RunQuery(townBeads, query)
	if err != nil || len(results) == 0 {
		return false
	}

	status, ok := results[0]["status"].(string)
	if !ok {
		return false
	}

	return status == "closed"
}

// runConvoyCheck runs `gt convoy check <convoy-id>` to check a specific convoy.
// This is idempotent and handles already-closed convoys gracefully.
func runConvoyCheck(townRoot, convoyID string) error {
	cmd := exec.Command("gt", "convoy", "check", convoyID)
	cmd.Dir = townRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%v: %s", err, stderr.String())
	}

	return nil
}
