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
			SkipNotify: true, // We send an explicit nudge below - skip mail notification to avoid double-nudge (hq-t1wcr5)
		}

		if err := router.Send(msg); err != nil {
			log.Printf("notify: FAILED to mail requestor %q: %v", fields.RequestedBy, err)
		}

		// 4. Nudge the requesting agent (immediate, best-effort).
		// Use --direct to send via tmux immediately rather than queuing to NudgeQueue.
		// This is critical because the requesting agent is likely idle (blocked waiting
		// for this decision), so queued nudges would never be drained (no hooks fire
		// in idle sessions). Direct tmux send wakes up the session.
		semanticSlug := generateSemanticSlug(decisionID, fields.Question)
		nudgeMsg := fmt.Sprintf("[DECISION RESOLVED] %s: Chose \"%s\"", semanticSlug, chosenLabel)
		if rationale != "" {
			nudgeMsg += fmt.Sprintf(" - %s", rationale)
		}
		// Add actionable instructions so the agent knows to continue working
		nudgeMsg += " → Continue working - check inbox"
		nudgeCmd := exec.Command("gt", "nudge", "--direct", fields.RequestedBy, nudgeMsg) //nolint:gosec // trusted internal command
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

// generateSemanticSlug creates a human-readable semantic slug from a decision.
// Format: prefix-dec-title_slugrandom (e.g., gt-dec-cache_strategyzfyl8)
func generateSemanticSlug(id, question string) string {
	// Extract prefix and random from ID (e.g., "gt-zfyl8" -> prefix="gt", random="zfyl8")
	parts := strings.SplitN(id, "-", 2)
	if len(parts) != 2 {
		return id // Can't parse, return original
	}
	prefix := parts[0]
	random := parts[1]

	// Strip child suffix if present (e.g., "zfyl8.1" -> "zfyl8")
	if dotIdx := strings.Index(random, "."); dotIdx > 0 {
		random = random[:dotIdx]
	}

	// Generate slug from question
	slug := generateSlug(question)
	if slug == "" {
		slug = "decision"
	}

	// Decision type code is "dec"
	return fmt.Sprintf("%s-dec-%s%s", prefix, slug, random)
}

// generateSlug converts a title/question to a slug.
func generateSlug(title string) string {
	if title == "" {
		return "untitled"
	}

	// Lowercase
	slug := strings.ToLower(title)

	// Stop words to remove
	stopWords := map[string]bool{
		"a": true, "an": true, "the": true,
		"in": true, "on": true, "at": true, "to": true, "for": true,
		"of": true, "with": true, "by": true, "from": true, "as": true,
		"and": true, "or": true, "but": true, "nor": true,
		"is": true, "are": true, "was": true, "were": true,
		"be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true,
		"do": true, "does": true, "did": true,
		"this": true, "that": true, "these": true, "those": true,
		"it": true, "its": true,
		"should": true, "would": true, "could": true,
		"how": true, "what": true, "which": true, "who": true,
		"we": true, "i": true, "you": true, "they": true,
	}

	// Replace non-alphanumeric with spaces
	var result []rune
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			result = append(result, r)
		} else {
			result = append(result, ' ')
		}
	}
	slug = string(result)

	// Split and filter stop words
	words := strings.Fields(slug)
	var filtered []string
	for _, word := range words {
		if !stopWords[word] && len(word) > 0 {
			filtered = append(filtered, word)
		}
	}

	// Fallback if all words were filtered
	if len(filtered) == 0 && len(words) > 0 {
		filtered = []string{words[0]}
	}

	// Join with underscores
	slug = strings.Join(filtered, "_")

	// Ensure starts with letter
	if len(slug) > 0 && (slug[0] >= '0' && slug[0] <= '9') {
		slug = "n" + slug
	}

	// Truncate to 40 chars at word boundary
	if len(slug) > 40 {
		truncated := slug[:40]
		if lastUnderscore := strings.LastIndex(truncated, "_"); lastUnderscore > 20 {
			truncated = truncated[:lastUnderscore]
		}
		slug = truncated
	}

	// Ensure minimum length
	if len(slug) < 3 {
		slug = slug + strings.Repeat("x", 3-len(slug))
	}

	// Clean up
	slug = strings.Trim(slug, "_")

	return slug
}
