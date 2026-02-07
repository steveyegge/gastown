// Package convoy provides shared convoy operations for redundant observers.
package convoy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
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
// Uses bd dep list to query the dependency graph.
func getTrackingConvoys(townRoot, issueID string) []string {
	// Query for convoys that track this issue (direction=up finds dependents)
	cmd := exec.Command("bd", "dep", "list", issueID, "--direction=up", "-t", "tracks", "--json")
	cmd.Dir = townRoot
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil
	}

	var results []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		return nil
	}

	convoyIDs := make([]string, 0, len(results))
	for _, r := range results {
		convoyIDs = append(convoyIDs, r.ID)
	}
	return convoyIDs
}

// isConvoyClosed checks if a convoy is already closed.
func isConvoyClosed(townRoot, convoyID string) bool {
	cmd := exec.Command("bd", "show", convoyID, "--json")
	cmd.Dir = townRoot
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return false
	}

	var results []struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil || len(results) == 0 {
		return false
	}

	return results[0].Status == "closed"
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
