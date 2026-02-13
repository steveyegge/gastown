// Package convoy provides shared convoy operations for redundant observers.
package convoy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
)

// CheckConvoysForIssue finds any convoys tracking the given issue and triggers
// convoy completion checks. If the convoy is not complete, it reactively feeds
// the next ready issue to keep the convoy progressing without waiting for
// polling-based patrol cycles.
//
// This enables redundant convoy observation from multiple agents (Witness,
// Refinery, Daemon).
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

		// Continuation feed: if convoy is still open after the completion check,
		// reactively dispatch the next ready issue. This makes convoy feeding
		// event-driven instead of relying on polling-based patrol cycles.
		if !isConvoyClosed(townRoot, convoyID) {
			feedNextReadyIssue(townRoot, convoyID, observer, logger)
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

// trackedIssue holds basic info about an issue tracked by a convoy.
type trackedIssue struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Assignee string `json:"assignee"`
	Priority int    `json:"priority"`
}

// feedNextReadyIssue finds the next ready issue in a convoy and dispatches it
// via gt sling. A ready issue is one that is open with no assignee. This
// provides reactive (event-driven) convoy feeding instead of waiting for
// polling-based patrol cycles.
//
// Only one issue is dispatched per call. When that issue completes, the
// observer fires again and feeds the next one.
func feedNextReadyIssue(townRoot, convoyID, observer string, logger func(format string, args ...interface{})) {
	tracked := getConvoyTrackedIssues(townRoot, convoyID)
	if len(tracked) == 0 {
		return
	}

	// Find the first ready issue (open, no assignee).
	// Issues are returned by bd dep list in dependency order, so we pick
	// the first match which is typically the highest priority.
	for _, issue := range tracked {
		if issue.Status != "open" || issue.Assignee != "" {
			continue
		}

		// Determine target rig from issue prefix
		rig := rigForIssue(townRoot, issue.ID)
		if rig == "" {
			logger("%s: convoy %s: cannot determine rig for issue %s, skipping", observer, convoyID, issue.ID)
			continue
		}

		logger("%s: convoy %s: feeding next ready issue %s to %s", observer, convoyID, issue.ID, rig)
		if err := dispatchIssue(townRoot, issue.ID, rig); err != nil {
			logger("%s: convoy %s: failed to dispatch %s: %v", observer, convoyID, issue.ID, err)
		}
		return // Feed one at a time
	}

	logger("%s: convoy %s: no ready issues to feed", observer, convoyID)
}

// getConvoyTrackedIssues returns issues tracked by a convoy with fresh status.
// Uses bd dep list for the tracking relations, then bd show for current status.
func getConvoyTrackedIssues(townRoot, convoyID string) []trackedIssue {
	// Get tracked issue IDs from dependency graph
	depCmd := exec.Command("bd", "dep", "list", convoyID, "--direction=down", "--type=tracks", "--json")
	depCmd.Dir = townRoot
	var stdout bytes.Buffer
	depCmd.Stdout = &stdout

	if err := depCmd.Run(); err != nil {
		return nil
	}

	var deps []struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Assignee string `json:"assignee"`
		Priority int    `json:"priority"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &deps); err != nil {
		return nil
	}

	if len(deps) == 0 {
		return nil
	}

	// Unwrap external:prefix:id format
	for i := range deps {
		deps[i].ID = extractIssueID(deps[i].ID)
	}

	// Refresh status via bd show for cross-rig accuracy.
	// bd dep list returns stale status from the HQ dependency record.
	ids := make([]string, len(deps))
	for i, d := range deps {
		ids[i] = d.ID
	}

	freshStatus := batchShowIssues(townRoot, ids)

	result := make([]trackedIssue, len(deps))
	for i, d := range deps {
		t := trackedIssue{
			ID:       d.ID,
			Status:   d.Status,
			Assignee: d.Assignee,
			Priority: d.Priority,
		}
		if fresh, ok := freshStatus[d.ID]; ok {
			t.Status = fresh.Status
			t.Assignee = fresh.Assignee
		}
		result[i] = t
	}

	return result
}

// batchShowIssues fetches fresh status for multiple issues via bd show.
func batchShowIssues(townRoot string, issueIDs []string) map[string]trackedIssue {
	result := make(map[string]trackedIssue)
	if len(issueIDs) == 0 {
		return result
	}

	args := append([]string{"show"}, issueIDs...)
	args = append(args, "--json")

	cmd := exec.Command("bd", args...)
	cmd.Dir = townRoot
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return result
	}

	var issues []struct {
		ID       string `json:"id"`
		Status   string `json:"status"`
		Assignee string `json:"assignee"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		return result
	}

	for _, issue := range issues {
		result[issue.ID] = trackedIssue{
			ID:       issue.ID,
			Status:   issue.Status,
			Assignee: issue.Assignee,
		}
	}

	return result
}

// extractIssueID strips the external:prefix:id wrapper from bead IDs.
func extractIssueID(id string) string {
	if strings.HasPrefix(id, "external:") {
		parts := strings.SplitN(id, ":", 3)
		if len(parts) == 3 {
			return parts[2]
		}
	}
	return id
}

// rigForIssue determines the rig name for an issue based on its ID prefix.
// Uses the beads routes to map prefixes to rigs.
func rigForIssue(townRoot, issueID string) string {
	prefix := beads.ExtractPrefix(issueID)
	if prefix == "" {
		return ""
	}
	return beads.GetRigNameForPrefix(townRoot, prefix)
}

// dispatchIssue dispatches an issue to a rig via gt sling.
func dispatchIssue(townRoot, issueID, rig string) error {
	cmd := exec.Command("gt", "sling", issueID, rig, "--no-boot")
	cmd.Dir = townRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}

	return nil
}
