// Package refinery provides semantic conflict detection and escalation.
package refinery

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/mail"
)

// SemanticConflict represents a detected semantic conflict between Polecat modifications.
type SemanticConflict struct {
	BeadID   string              // The bead that has conflicting changes
	Field    string              // The field with conflicting values
	Changes  []BeadFieldChange   // All changes to this field
	Detected time.Time           // When the conflict was detected
}

// BeadFieldChange represents a single modification to a bead field.
type BeadFieldChange struct {
	BeadID     string    // The bead being modified
	Field      string    // The field being modified
	Polecat    string    // Which polecat made this change
	OldValue   string    // Previous value
	NewValue   string    // New value
	Timestamp  time.Time // When the change was made
	CommitSHA  string    // Git commit that made the change
	Reasoning  string    // Why this change was made (if provided)
	Confidence float64   // Confidence score 0.0-1.0 (if provided)
}

// detectSemanticConflicts analyzes the MR branch for semantic conflicts.
// It examines git commits to find bead modifications where different Polecats
// changed the same field to different values.
func (e *Engineer) detectSemanticConflicts(mr *beads.Issue) ([]SemanticConflict, error) {
	// Skip if semantic conflict detection is disabled
	if !e.config.SemanticConflicts.Enabled {
		return nil, nil
	}

	// Get branch name from MR metadata
	branch, ok := mr.Metadata["branch"].(string)
	if !ok || branch == "" {
		return nil, fmt.Errorf("MR missing branch in metadata")
	}

	target := e.config.TargetBranch
	if targetOverride, ok := mr.Metadata["target"].(string); ok && targetOverride != "" {
		target = targetOverride
	}

	// Get commits in this MR (commits not in target branch)
	commits, err := e.getCommitsInBranch(branch, target)
	if err != nil {
		return nil, fmt.Errorf("getting commits for semantic conflict detection: %w", err)
	}

	// Extract bead modifications from commits
	beadChanges, err := e.extractBeadChanges(commits)
	if err != nil {
		return nil, fmt.Errorf("extracting bead changes: %w", err)
	}

	// Group changes by bead ID and field
	grouped := e.groupBeadChanges(beadChanges)

	// Detect conflicts in grouped changes
	conflicts := []SemanticConflict{}
	for key, changes := range grouped {
		parts := strings.SplitN(key, ":", 2)
		beadID := parts[0]
		field := parts[1]

		// Only check fields that should be escalated
		if !e.shouldEscalateField(field) {
			continue
		}

		if e.isSemanticConflict(changes) {
			conflicts = append(conflicts, SemanticConflict{
				BeadID:   beadID,
				Field:    field,
				Changes:  changes,
				Detected: time.Now(),
			})
		}
	}

	return conflicts, nil
}

// getCommitsInBranch returns commits in branch that are not in target.
func (e *Engineer) getCommitsInBranch(branch, target string) ([]git.Commit, error) {
	// Use git log to get commits in branch..target range
	revRange := "origin/" + target + "..origin/" + branch
	commits, err := e.git.LogRange(revRange)
	if err != nil {
		return nil, fmt.Errorf("git log %s: %w", revRange, err)
	}
	return commits, nil
}

// extractBeadChanges analyzes commits to find bead field modifications.
// This looks for commits that modified beads (via bd commands or direct JSON edits).
func (e *Engineer) extractBeadChanges(commits []git.Commit) ([]BeadFieldChange, error) {
	changes := []BeadFieldChange{}

	for _, commit := range commits {
		// Parse commit message to extract bead modifications
		// Expected format:
		//   bd update <bead-id> --field=value
		//   or commit body with JSON metadata
		beadChanges := e.parseCommitForBeadChanges(commit)
		changes = append(changes, beadChanges...)
	}

	return changes, nil
}

// parseCommitForBeadChanges extracts bead modifications from a commit message.
// This is a simple parser - in production, you might want to analyze actual file diffs.
func (e *Engineer) parseCommitForBeadChanges(commit git.Commit) []BeadFieldChange {
	changes := []BeadFieldChange{}

	// Look for patterns like:
	//   "bd update gt-abc123 --priority=0"
	//   "Update bead gt-abc123: priority 0 -> 2"
	//
	// For now, we'll look for a JSON block in the commit body that
	// Polecats can use to report their changes in a structured way:
	//
	// BEAD_CHANGES:
	// {
	//   "bead_id": "gt-abc123",
	//   "changes": [
	//     {"field": "priority", "old": "2", "new": "0", "confidence": 0.95, "reasoning": "CVE detected"}
	//   ]
	// }

	body := commit.Message
	if idx := strings.Index(body, "BEAD_CHANGES:"); idx != -1 {
		jsonStart := idx + len("BEAD_CHANGES:")
		jsonStr := strings.TrimSpace(body[jsonStart:])

		var changeData struct {
			BeadID  string `json:"bead_id"`
			Polecat string `json:"polecat"`
			Changes []struct {
				Field      string  `json:"field"`
				OldValue   string  `json:"old_value"`
				NewValue   string  `json:"new_value"`
				Confidence float64 `json:"confidence"`
				Reasoning  string  `json:"reasoning"`
			} `json:"changes"`
		}

		if err := json.Unmarshal([]byte(jsonStr), &changeData); err == nil {
			for _, ch := range changeData.Changes {
				changes = append(changes, BeadFieldChange{
					BeadID:     changeData.BeadID,
					Field:      ch.Field,
					Polecat:    changeData.Polecat,
					OldValue:   ch.OldValue,
					NewValue:   ch.NewValue,
					Timestamp:  commit.Timestamp,
					CommitSHA:  commit.SHA,
					Reasoning:  ch.Reasoning,
					Confidence: ch.Confidence,
				})
			}
		}
	}

	return changes
}

// groupBeadChanges groups changes by "beadID:field" key.
func (e *Engineer) groupBeadChanges(changes []BeadFieldChange) map[string][]BeadFieldChange {
	grouped := make(map[string][]BeadFieldChange)
	for _, change := range changes {
		key := change.BeadID + ":" + change.Field
		grouped[key] = append(grouped[key], change)
	}
	return grouped
}

// isSemanticConflict checks if changes represent a semantic conflict.
// A semantic conflict occurs when:
// 1. Multiple different values were set for the same field
// 2. Multiple different polecats made the changes
func (e *Engineer) isSemanticConflict(changes []BeadFieldChange) bool {
	if len(changes) < 2 {
		return false // Need at least 2 changes to conflict
	}

	// Get unique values and unique polecats
	uniqueValues := make(map[string]bool)
	uniquePolecats := make(map[string]bool)

	for _, change := range changes {
		uniqueValues[change.NewValue] = true
		uniquePolecats[change.Polecat] = true
	}

	return len(uniqueValues) > 1 && len(uniquePolecats) > 1
}

// shouldEscalateField checks if a field requires Mayor decision on conflict.
func (e *Engineer) shouldEscalateField(field string) bool {
	for _, f := range e.config.SemanticConflicts.EscalateFields {
		if f == field {
			return true
		}
	}
	return false
}

// shouldAutoResolveField checks if a field uses automatic resolution.
func (e *Engineer) shouldAutoResolveField(field string) bool {
	for _, f := range e.config.SemanticConflicts.AutoResolveFields {
		if f == field {
			return true
		}
	}
	return false
}

// escalateSemanticConflict sends escalation mail to Mayor and blocks the MR.
// Returns ProcessResult indicating the MR should be blocked pending Mayor decision.
func (e *Engineer) escalateSemanticConflict(mr *beads.Issue, conflicts []SemanticConflict) ProcessResult {
	// Acquire merge slot to serialize decision-making
	holder := e.rig.Name + "/refinery"
	status, err := e.beads.MergeSlotAcquire(holder, true /* addWaiter */)
	if err != nil {
		return ProcessResult{
			Success:  false,
			Conflict: true,
			Error:    fmt.Sprintf("failed to acquire merge slot for semantic conflict: %v", err),
		}
	}
	if !status.Available && status.Holder != holder {
		// Another conflict resolution is in progress - defer this one
		_, _ = fmt.Fprintf(e.output, "[Engineer] Merge slot held by %s - deferring semantic conflict escalation\n", status.Holder)
		return ProcessResult{
			Success:  false,
			Conflict: true,
			Error:    "deferred pending mayor decision on another semantic conflict",
		}
	}

	_, _ = fmt.Fprintf(e.output, "[Engineer] Semantic conflict detected - escalating to Mayor\n")

	// Compose escalation mail to Mayor
	subject := fmt.Sprintf("Decision needed: %s conflicts on %s", conflicts[0].Field, conflicts[0].BeadID)
	body := e.composeEscalationMail(mr, conflicts)

	// Create mail message
	msg := mail.NewMessage(
		e.rig.Name+"/refinery",
		"mayor/",
		subject,
		body,
	)
	msg.Type = mail.TypeTask
	msg.Priority = mail.PriorityHigh
	msg.ThreadID = "semantic-conflict-" + mr.ID

	// Send mail to Mayor
	router := mail.NewRouter(e.workDir)
	if err := router.Send(msg); err != nil {
		// Release merge slot on send failure
		_ = e.beads.MergeSlotRelease(holder)
		return ProcessResult{
			Success:  false,
			Conflict: true,
			Error:    fmt.Sprintf("failed to send escalation mail: %v", err),
		}
	}

	_, _ = fmt.Fprintf(e.output, "[Engineer] Escalation mail sent to Mayor: %s\n", msg.ID)

	// The MR stays blocked until Mayor responds
	// Merge slot is held until Witness applies Mayor's decision
	// (Witness will release the slot after applying resolution)

	return ProcessResult{
		Success:  false,
		Conflict: true,
		Error:    fmt.Sprintf("escalated to Mayor for semantic conflict decision (mail: %s)", msg.ID),
	}
}

// composeEscalationMail creates the mail body for Mayor escalation.
func (e *Engineer) composeEscalationMail(mr *beads.Issue, conflicts []SemanticConflict) string {
	var body strings.Builder

	body.WriteString(fmt.Sprintf("Semantic conflicts detected in MR: %s\n\n", mr.ID))
	body.WriteString(fmt.Sprintf("Title: %s\n", mr.Title))
	body.WriteString(fmt.Sprintf("Branch: %s\n", mr.Metadata["branch"]))
	body.WriteString("\n")

	for i, conflict := range conflicts {
		body.WriteString(fmt.Sprintf("## Conflict %d: %s.%s\n\n", i+1, conflict.BeadID, conflict.Field))

		for j, change := range conflict.Changes {
			body.WriteString(fmt.Sprintf("**Change %d** (by %s):\n", j+1, change.Polecat))
			body.WriteString(fmt.Sprintf("- Value: %s -> %s\n", change.OldValue, change.NewValue))
			if change.Confidence > 0 {
				body.WriteString(fmt.Sprintf("- Confidence: %.2f\n", change.Confidence))
			}
			if change.Reasoning != "" {
				body.WriteString(fmt.Sprintf("- Reasoning: %s\n", change.Reasoning))
			}
			body.WriteString(fmt.Sprintf("- Commit: %s\n", change.CommitSHA[:8]))
			body.WriteString(fmt.Sprintf("- Timestamp: %s\n", change.Timestamp.Format(time.RFC3339)))
			body.WriteString("\n")
		}
	}

	body.WriteString("---\n")
	body.WriteString("Please review the conflicting changes and provide a resolution.\n\n")
	body.WriteString("To resolve:\n")
	body.WriteString("1. Review the changes and their reasoning\n")
	body.WriteString("2. Decide which value to accept (or provide a new value)\n")
	body.WriteString("3. Reply to this mail with your decision\n\n")
	body.WriteString("[Automated escalation from Refinery]\n")

	return body.String()
}
