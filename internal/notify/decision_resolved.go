// Package notify provides shared notification functions for agent communication.
package notify

import (
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/util"
)

// DecisionResolved notifies the requesting agent that their decision was resolved.
// It sends mail (persistent), nudges (immediate), removes blockers, and logs the event.
// Errors are logged but do not cause failure - notification is best-effort.
func DecisionResolved(townRoot, decisionID string, fields beads.DecisionFields, chosenLabel, rationale, resolvedBy string) {
	// 1. Remove blocker dependencies
	bd := beads.New(beads.GetTownBeadsPath(townRoot))
	for _, blockerID := range fields.Blockers {
		if err := bd.RemoveDecisionBlocker(decisionID, blockerID); err != nil {
			log.Printf("notify: failed to remove blocker dependency %s: %v", blockerID, err)
		}
	}

	// 2. Determine notification target
	router := mail.NewRouter(townRoot)
	subject := fmt.Sprintf("[DECISION RESOLVED] %s → %s", truncate(fields.Question, 30), chosenLabel)
	if rationale != "" {
		subject += fmt.Sprintf(": %s", truncate(rationale, 40))
	}
	body := formatResolutionBody(decisionID, fields.Question, chosenLabel, rationale, resolvedBy)

	// 3. Send mail to requestor (persistent notification)
	if fields.RequestedBy != "" && fields.RequestedBy != "unknown" {
		msg := &mail.Message{
			From:       resolvedBy,
			To:         fields.RequestedBy,
			Subject:    subject,
			Body:       body,
			Type:       mail.TypeTask,
			Priority:   mail.PriorityNormal,
			SkipNotify: true,  // We send an explicit nudge below - skip mail notification to avoid double-nudge (hq-t1wcr5)
			PreRead:    true,  // Mark as read on delivery - informational, agent already gets nudge (bd-bug-mail_inbox_shows_decision_resolutions)
		}

		if err := router.Send(msg); err != nil {
			log.Printf("notify: FAILED to mail requestor %q: %v", fields.RequestedBy, err)
		}

		// 4. Nudge the requesting agent (immediate, best-effort).
		// Direct tmux send is now the default behavior for gt nudge (no --direct needed).
		// This is critical because the requesting agent is likely idle (blocked waiting
		// for this decision), so queued nudges would never be drained (no hooks fire
		// in idle sessions). Direct send wakes up the session immediately.
		semanticSlug := util.GenerateDecisionSlug(decisionID, fields.Question)
		nudgeMsg := fmt.Sprintf("[DECISION RESOLVED] %s: Chose \"%s\"", semanticSlug, chosenLabel)
		if rationale != "" {
			nudgeMsg += fmt.Sprintf(" - %s", rationale)
		}
		// Add actionable instructions so the agent knows to continue working
		nudgeMsg += " → Continue working - check inbox"
		nudgeCmd := exec.Command("gt", "nudge", fields.RequestedBy, nudgeMsg) //nolint:gosec // trusted internal command
		if err := nudgeCmd.Run(); err != nil {
			log.Printf("notify: failed to nudge requestor %q: %v", fields.RequestedBy, err)
		}
	} else {
		// RequestedBy is empty or unknown - log this and send fallback notification to overseer
		log.Printf("notify: decision %s has no requestor (RequestedBy=%q), sending fallback notification to overseer", decisionID, fields.RequestedBy)

		// Fallback: notify overseer so they're aware of the resolution
		msg := &mail.Message{
			From:     resolvedBy,
			To:       "overseer",
			Subject:  subject + " [NO REQUESTOR]",
			Body:     body + "\n\n---\nNote: Original requestor unknown, sending to overseer as fallback.",
			Type:     mail.TypeTask,
			Priority: mail.PriorityNormal,
		}

		if err := router.Send(msg); err != nil {
			log.Printf("notify: FAILED to mail overseer (fallback): %v", err)
		}
	}

	// 5. Log to activity feed
	payload := map[string]interface{}{
		"decision_id":  decisionID,
		"question":     fields.Question,
		"chosen_label": chosenLabel,
		"resolved_by":  resolvedBy,
	}
	if rationale != "" {
		payload["rationale"] = rationale
	}
	_ = events.LogFeed(events.TypeDecisionResolved, resolvedBy, payload)
}

func formatResolutionBody(beadID, question, chosen, rationale, resolvedBy string) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("Decision ID: %s", beadID))
	lines = append(lines, fmt.Sprintf("Question: %s", question))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Chosen: %s", chosen))
	if rationale != "" {
		lines = append(lines, fmt.Sprintf("Rationale: %s", rationale))
	}
	lines = append(lines, fmt.Sprintf("Resolved by: %s", resolvedBy))
	lines = append(lines, "")
	lines = append(lines, "---")
	lines = append(lines, "This decision has been resolved. Any blocked work should now be unblocked.")
	return strings.Join(lines, "\n")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
