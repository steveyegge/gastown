// Package convoy provides convoy tracking operations: finding tracking convoys,
// checking completion, feeding ready issues, and dispatching via gt sling.
package convoy

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	beadsdk "github.com/steveyegge/beads"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/util"
)

// CheckConvoysForIssue finds any convoys tracking the given issue and triggers
// convoy completion checks. If the convoy is not complete, it reactively feeds
// the next ready issue to keep the convoy progressing without waiting for
// polling-based patrol cycles.
//
// The check is idempotent - running it multiple times for the same issue is safe.
// The underlying `gt convoy check` handles already-closed convoys gracefully.
//
// Parameters:
//   - ctx: context for storage operations
//   - store: beads storage for dependency/issue queries (nil skips convoy checks)
//   - townRoot: path to the town root directory
//   - issueID: the issue ID that was just closed
//   - caller: identifier for logging (e.g., "Convoy")
//   - logger: optional logger function (can be nil)
//   - gtPath: resolved path to the gt binary (e.g. from exec.LookPath or daemon config)
//
// Returns the convoy IDs that were checked (may be empty if issue isn't tracked).
func CheckConvoysForIssue(ctx context.Context, store beadsdk.Storage, townRoot, issueID, caller string, logger func(format string, args ...interface{}), gtPath string, isRigParked func(string) bool) []string {
	if logger == nil {
		logger = func(format string, args ...interface{}) {} // no-op
	}
	if isRigParked == nil {
		isRigParked = func(string) bool { return false }
	}
	if store == nil {
		return nil
	}

	// Find convoys tracking this issue
	convoyIDs := getTrackingConvoys(ctx, store, issueID, logger)
	if len(convoyIDs) == 0 {
		return nil
	}

	logger("%s: %s tracked by %d convoy(s): %v", caller, issueID, len(convoyIDs), convoyIDs)

	// Run convoy check for each tracking convoy
	// Note: gt convoy check is idempotent and handles already-closed convoys
	for _, convoyID := range convoyIDs {
		if isConvoyClosed(ctx, store, convoyID) {
			logger("%s: convoy %s already closed, skipping", caller, convoyID)
			continue
		}

		logger("%s: checking convoy %s", caller, convoyID)
		if err := runConvoyCheck(ctx, townRoot, convoyID, gtPath); err != nil {
			logger("%s: convoy %s check failed: %s", caller, convoyID, util.FirstLine(err.Error()))
		}

		// Continuation feed: if convoy is still open after the completion check,
		// reactively dispatch the next ready issue. This makes convoy feeding
		// event-driven instead of relying on polling-based patrol cycles.
		if !isConvoyClosed(ctx, store, convoyID) {
			feedNextReadyIssue(ctx, store, townRoot, convoyID, caller, logger, gtPath, isRigParked)
		}
	}

	return convoyIDs
}

// getTrackingConvoys returns convoy IDs that track the given issue.
// Uses SDK GetDependentsWithMetadata filtered by type "tracks".
func getTrackingConvoys(ctx context.Context, store beadsdk.Storage, issueID string, logger func(format string, args ...interface{})) []string {
	dependents, err := store.GetDependentsWithMetadata(ctx, issueID)
	if err != nil {
		if logger != nil {
			logger("Convoy: getTrackingConvoys(%s) store error: %v", issueID, err)
		}
		return nil
	}

	convoyIDs := make([]string, 0)
	for _, d := range dependents {
		if string(d.DependencyType) == "tracks" {
			convoyIDs = append(convoyIDs, d.ID)
		}
	}
	return convoyIDs
}

// isConvoyClosed checks if a convoy is already closed.
func isConvoyClosed(ctx context.Context, store beadsdk.Storage, convoyID string) bool {
	issue, err := store.GetIssue(ctx, convoyID)
	if err != nil || issue == nil {
		return false
	}
	return string(issue.Status) == "closed"
}

// runConvoyCheck runs `gt convoy check <convoy-id>` to check a specific convoy.
// This is idempotent and handles already-closed convoys gracefully.
// The context parameter enables cancellation on daemon shutdown.
// gtPath is the resolved path to the gt binary.
func runConvoyCheck(ctx context.Context, townRoot, convoyID, gtPath string) error {
	cmd := exec.CommandContext(ctx, gtPath, "convoy", "check", convoyID)
	cmd.Dir = townRoot
	util.SetProcessGroup(cmd)
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
// next close event triggers another feed cycle.
// gtPath is the resolved path to the gt binary.
func feedNextReadyIssue(ctx context.Context, store beadsdk.Storage, townRoot, convoyID, caller string, logger func(format string, args ...interface{}), gtPath string, isRigParked func(string) bool) {
	tracked := getConvoyTrackedIssues(ctx, store, convoyID)
	if len(tracked) == 0 {
		return
	}

	// Find the first ready issue (open, no assignee).
	// Pick the first match, which is typically the highest priority.
	for _, issue := range tracked {
		if issue.Status != "open" || issue.Assignee != "" {
			continue
		}

		// Determine target rig from issue prefix
		rig := rigForIssue(townRoot, issue.ID)
		if rig == "" {
			logger("%s: convoy %s: cannot determine rig for issue %s, skipping", caller, convoyID, issue.ID)
			continue
		}

		if isRigParked(rig) {
			logger("%s: convoy %s: rig %s is parked, skipping %s", caller, convoyID, rig, issue.ID)
			continue
		}

		logger("%s: convoy %s: feeding next ready issue %s to %s", caller, convoyID, issue.ID, rig)
		if err := dispatchIssue(ctx, townRoot, issue.ID, rig, gtPath); err != nil {
			logger("%s: convoy %s: dispatch %s failed: %s", caller, convoyID, issue.ID, util.FirstLine(err.Error()))
		}
		return // Feed one at a time
	}

	logger("%s: convoy %s: no ready issues to feed", caller, convoyID)
}

// getConvoyTrackedIssues returns issues tracked by a convoy with fresh status.
// Uses SDK GetDependenciesWithMetadata filtered by tracks, then GetIssuesByIDs for current status.
func getConvoyTrackedIssues(ctx context.Context, store beadsdk.Storage, convoyID string) []trackedIssue {
	deps, err := store.GetDependenciesWithMetadata(ctx, convoyID)
	if err != nil || len(deps) == 0 {
		return nil
	}

	// Filter by tracks type and collect IDs
	var ids []string
	type depMeta struct {
		status   string
		assignee string
		priority int
	}
	metaByID := make(map[string]depMeta)
	for _, d := range deps {
		if string(d.DependencyType) == "tracks" {
			id := extractIssueID(d.ID)
			ids = append(ids, id)
			metaByID[id] = depMeta{
				status:   string(d.Status),
				assignee: d.Assignee,
				priority: d.Priority,
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}

	// Refresh status via GetIssuesByIDs for cross-rig accuracy
	freshIssues, err := store.GetIssuesByIDs(ctx, ids)
	if err != nil {
		freshIssues = nil
	}

	freshMap := make(map[string]*beadsdk.Issue)
	for _, iss := range freshIssues {
		if iss != nil {
			freshMap[iss.ID] = iss
		}
	}

	result := make([]trackedIssue, 0, len(ids))
	for _, id := range ids {
		t := trackedIssue{ID: id}
		if fresh := freshMap[id]; fresh != nil {
			t.Status = string(fresh.Status)
			t.Assignee = fresh.Assignee
			t.Priority = fresh.Priority
		} else if meta, ok := metaByID[id]; ok {
			t.Status = meta.status
			t.Assignee = meta.assignee
			t.Priority = meta.priority
		}
		result = append(result, t)
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
// The context parameter enables cancellation on daemon shutdown.
// gtPath is the resolved path to the gt binary.
func dispatchIssue(ctx context.Context, townRoot, issueID, rig, gtPath string) error {
	cmd := exec.CommandContext(ctx, gtPath, "sling", issueID, rig, "--no-boot")
	cmd.Dir = townRoot
	util.SetProcessGroup(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}

	return nil
}
