package cmd

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

func runDecisionRequest(cmd *cobra.Command, args []string) error {
	// Validate prompt
	if decisionPrompt == "" {
		return fmt.Errorf("--prompt is required")
	}

	// Validate options (2-4 required)
	if len(decisionOptions) < 2 {
		return fmt.Errorf("at least 2 options required (use --option multiple times)")
	}
	if len(decisionOptions) > 4 {
		return fmt.Errorf("at most 4 options allowed")
	}

	// Validate urgency
	urgency := strings.ToLower(decisionUrgency)
	if !beads.IsValidUrgency(urgency) {
		return fmt.Errorf("invalid urgency '%s': must be high, medium, or low", decisionUrgency)
	}

	// Validate recommend index
	if decisionRecommend < 0 || decisionRecommend > len(decisionOptions) {
		return fmt.Errorf("--recommend must be between 1 and %d", len(decisionOptions))
	}

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Detect agent identity
	agentID := detectSender()
	if agentID == "" {
		agentID = "unknown"
	}

	// Parse options
	var options []beads.DecisionOption
	for i, optStr := range decisionOptions {
		opt := beads.DecisionOption{}

		// Parse "Label: Description" format
		if colonIdx := strings.Index(optStr, ":"); colonIdx != -1 {
			opt.Label = strings.TrimSpace(optStr[:colonIdx])
			opt.Description = strings.TrimSpace(optStr[colonIdx+1:])
		} else {
			opt.Label = strings.TrimSpace(optStr)
		}

		// Mark as recommended if specified
		if decisionRecommend == i+1 {
			opt.Recommended = true
		}

		options = append(options, opt)
	}

	// Build decision fields
	fields := &beads.DecisionFields{
		Question:    decisionPrompt,
		Context:     decisionContext,
		Options:     options,
		ChosenIndex: 0, // Pending
		RequestedBy: agentID,
		RequestedAt: time.Now().Format(time.RFC3339),
		Urgency:     urgency,
	}

	// Add blocker if specified
	if decisionBlocks != "" {
		fields.Blockers = []string{decisionBlocks}
	}

	// Create decision bead
	bd := beads.New(beads.ResolveBeadsDir(townRoot))
	issue, err := bd.CreateDecisionBead(decisionPrompt, fields)
	if err != nil {
		return fmt.Errorf("creating decision bead: %w", err)
	}

	// Add blocker dependency if specified
	if decisionBlocks != "" {
		if err := bd.AddDecisionBlocker(issue.ID, decisionBlocks); err != nil {
			style.PrintWarning("failed to add blocker dependency: %v", err)
		}
	}

	// Send notification mail to human/overseer
	router := mail.NewRouter(townRoot)
	msg := &mail.Message{
		From:    agentID,
		To:      "human",
		Subject: fmt.Sprintf("[DECISION] %s", decisionPrompt),
		Body:    formatDecisionMailBody(issue.ID, fields),
		Type:    mail.TypeTask,
	}

	// Set priority based on urgency
	switch urgency {
	case beads.UrgencyHigh:
		msg.Priority = mail.PriorityHigh
	case beads.UrgencyMedium:
		msg.Priority = mail.PriorityNormal
	default:
		msg.Priority = mail.PriorityLow
	}

	if err := router.Send(msg); err != nil {
		style.PrintWarning("failed to send notification: %v", err)
	}

	// Log to activity feed
	payload := map[string]interface{}{
		"decision_id":  issue.ID,
		"question":     decisionPrompt,
		"urgency":      urgency,
		"option_count": len(options),
		"requested_by": agentID,
	}
	if decisionBlocks != "" {
		payload["blocking"] = decisionBlocks
	}
	_ = events.LogFeed(events.TypeDecisionRequested, agentID, payload)

	// Output
	if decisionJSON {
		result := map[string]interface{}{
			"id":           issue.ID,
			"question":     decisionPrompt,
			"urgency":      urgency,
			"options":      options,
			"requested_by": agentID,
		}
		if decisionBlocks != "" {
			result["blocking"] = decisionBlocks
		}
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Printf("📋 Decision requested: %s\n", issue.ID)
		fmt.Printf("   Question: %s\n", decisionPrompt)
		fmt.Printf("   Options: %s\n", formatOptionsSummary(options))
		if decisionBlocks != "" {
			fmt.Printf("   Blocking: %s\n", decisionBlocks)
		}
		fmt.Printf("\n→ Notified human (overseer)\n")
		fmt.Printf("\nTo resolve: gt decision resolve %s --choice N --rationale \"...\"\n", issue.ID)
	}

	return nil
}

func runDecisionList(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	bd := beads.New(beads.ResolveBeadsDir(townRoot))

	var issues []*beads.Issue
	if decisionListAll {
		issues, err = bd.ListAllDecisions()
	} else {
		issues, err = bd.ListDecisions()
	}
	if err != nil {
		return fmt.Errorf("listing decisions: %w", err)
	}

	if decisionListJSON {
		out, _ := json.MarshalIndent(issues, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	if len(issues) == 0 {
		fmt.Println("No pending decisions")
		return nil
	}

	statusLabel := "Pending"
	if decisionListAll {
		statusLabel = "All"
	}
	fmt.Printf("📋 %s Decisions (%d):\n\n", statusLabel, len(issues))

	for _, issue := range issues {
		fields := beads.ParseDecisionFields(issue.Description)
		emoji := urgencyEmoji(fields.Urgency)

		status := "PENDING"
		if beads.HasLabel(issue, "decision:resolved") {
			status = "RESOLVED"
		}

		fmt.Printf("  %s %s [%s] %s\n", emoji, issue.ID, status, truncateString(fields.Question, 50))
		fmt.Printf("     Requested by: %s | %s\n", fields.RequestedBy, formatRelativeTimeSimple(issue.CreatedAt))
		fmt.Printf("     Options: %s\n", formatOptionsSummary(fields.Options))
		if len(fields.Blockers) > 0 {
			fmt.Printf("     Blocking: %s\n", strings.Join(fields.Blockers, ", "))
		}
		if fields.ChosenIndex > 0 && fields.ChosenIndex <= len(fields.Options) {
			fmt.Printf("     → Chose: %s\n", fields.Options[fields.ChosenIndex-1].Label)
		}
		fmt.Println()
	}

	return nil
}

func runDecisionShow(cmd *cobra.Command, args []string) error {
	decisionID := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	bd := beads.New(beads.ResolveBeadsDir(townRoot))
	issue, fields, err := bd.GetDecisionBead(decisionID)
	if err != nil {
		return fmt.Errorf("getting decision: %w", err)
	}
	if issue == nil {
		return fmt.Errorf("decision not found: %s", decisionID)
	}

	if decisionJSON {
		data := map[string]interface{}{
			"id":           issue.ID,
			"question":     fields.Question,
			"context":      fields.Context,
			"options":      fields.Options,
			"chosen_index": fields.ChosenIndex,
			"rationale":    fields.Rationale,
			"urgency":      fields.Urgency,
			"requested_by": fields.RequestedBy,
			"requested_at": fields.RequestedAt,
			"resolved_by":  fields.ResolvedBy,
			"resolved_at":  fields.ResolvedAt,
			"blockers":     fields.Blockers,
			"status":       issue.Status,
		}
		out, _ := json.MarshalIndent(data, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	emoji := urgencyEmoji(fields.Urgency)
	status := "PENDING"
	if beads.HasLabel(issue, "decision:resolved") {
		status = "RESOLVED"
	}

	fmt.Printf("%s Decision: %s [%s]\n\n", emoji, issue.ID, status)
	fmt.Printf("Question: %s\n\n", fields.Question)

	if fields.Context != "" {
		fmt.Printf("Context:\n  %s\n\n", strings.ReplaceAll(fields.Context, "\n", "\n  "))
	}

	fmt.Printf("Options:\n")
	for i, opt := range fields.Options {
		num := i + 1
		marker := ""
		if opt.Recommended {
			marker = " (Recommended)"
		}
		if fields.ChosenIndex == num {
			marker += " ✓ CHOSEN"
		}
		fmt.Printf("  %d. %s%s\n", num, opt.Label, marker)
		if opt.Description != "" {
			fmt.Printf("     %s\n", opt.Description)
		}
	}
	fmt.Println()

	fmt.Printf("Requested by: %s\n", fields.RequestedBy)
	fmt.Printf("Requested at: %s\n", formatRelativeTimeSimple(fields.RequestedAt))
	fmt.Printf("Urgency: %s\n", fields.Urgency)
	if len(fields.Blockers) > 0 {
		fmt.Printf("Blocking: %s\n", strings.Join(fields.Blockers, ", "))
	}

	if fields.ChosenIndex > 0 {
		fmt.Println()
		fmt.Printf("Resolution:\n")
		fmt.Printf("  Chosen: %s\n", fields.Options[fields.ChosenIndex-1].Label)
		if fields.Rationale != "" {
			fmt.Printf("  Rationale: %s\n", fields.Rationale)
		}
		fmt.Printf("  Resolved by: %s\n", fields.ResolvedBy)
		fmt.Printf("  Resolved at: %s\n", formatRelativeTimeSimple(fields.ResolvedAt))
	} else {
		fmt.Printf("\nTo resolve: gt decision resolve %s --choice N --rationale \"...\"\n", issue.ID)
	}

	return nil
}

func runDecisionResolve(cmd *cobra.Command, args []string) error {
	decisionID := args[0]

	if decisionChoice < 1 {
		return fmt.Errorf("--choice is required and must be >= 1")
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Detect who is resolving
	resolvedBy := detectSender()
	if resolvedBy == "" {
		resolvedBy = "human"
	}

	bd := beads.New(beads.ResolveBeadsDir(townRoot))

	// Get the decision first to validate and get info for notifications
	issue, fields, err := bd.GetDecisionBead(decisionID)
	if err != nil {
		return fmt.Errorf("getting decision: %w", err)
	}
	if issue == nil {
		return fmt.Errorf("decision not found: %s", decisionID)
	}

	// Validate choice
	if decisionChoice > len(fields.Options) {
		return fmt.Errorf("invalid choice %d: only %d options available", decisionChoice, len(fields.Options))
	}

	chosenOption := fields.Options[decisionChoice-1]

	// Resolve the decision
	if err := bd.ResolveDecision(decisionID, decisionChoice, decisionRationale, resolvedBy); err != nil {
		return fmt.Errorf("resolving decision: %w", err)
	}

	// Remove blocker dependencies
	for _, blockerID := range fields.Blockers {
		if err := bd.RemoveDecisionBlocker(decisionID, blockerID); err != nil {
			style.PrintWarning("failed to remove blocker dependency %s: %v", blockerID, err)
		}
	}

	// Send notification to requestor
	if fields.RequestedBy != "" && fields.RequestedBy != "unknown" {
		router := mail.NewRouter(townRoot)
		msg := &mail.Message{
			From:     resolvedBy,
			To:       fields.RequestedBy,
			Subject:  fmt.Sprintf("[DECISION RESOLVED] %s → %s", truncateString(fields.Question, 30), chosenOption.Label),
			Body:     formatResolutionMailBody(decisionID, fields.Question, chosenOption.Label, decisionRationale, resolvedBy),
			Type:     mail.TypeTask,
			Priority: mail.PriorityNormal,
		}

		if err := router.Send(msg); err != nil {
			style.PrintWarning("failed to notify requestor: %v", err)
		}

		// Nudge the requesting agent so they see the resolution immediately
		nudgeMsg := fmt.Sprintf("[DECISION RESOLVED] %s: Chose \"%s\"", decisionID, chosenOption.Label)
		if decisionRationale != "" {
			nudgeMsg += fmt.Sprintf(" - %s", decisionRationale)
		}
		nudgeCmd := execCommand("gt", "nudge", fields.RequestedBy, nudgeMsg)
		if err := nudgeCmd.Run(); err != nil {
			// Don't fail resolve, just warn
			style.PrintWarning("failed to nudge requestor: %v", err)
		}
	}

	// Log to activity feed
	payload := map[string]interface{}{
		"decision_id":  decisionID,
		"question":     fields.Question,
		"chosen_index": decisionChoice,
		"chosen_label": chosenOption.Label,
		"resolved_by":  resolvedBy,
	}
	if decisionRationale != "" {
		payload["rationale"] = decisionRationale
	}
	_ = events.LogFeed(events.TypeDecisionResolved, resolvedBy, payload)

	// Output
	if decisionJSON {
		result := map[string]interface{}{
			"id":           decisionID,
			"chosen_index": decisionChoice,
			"chosen_label": chosenOption.Label,
			"rationale":    decisionRationale,
			"resolved_by":  resolvedBy,
			"unblocked":    fields.Blockers,
		}
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Printf("✓ Resolved %s: %s\n", decisionID, chosenOption.Label)
		if decisionRationale != "" {
			fmt.Printf("  Rationale: %s\n", decisionRationale)
		}
		if len(fields.Blockers) > 0 {
			fmt.Printf("\n→ Unblocked: %s\n", strings.Join(fields.Blockers, ", "))
		}
		if fields.RequestedBy != "" && fields.RequestedBy != "unknown" {
			fmt.Printf("→ Notified: %s\n", fields.RequestedBy)
		}
	}

	return nil
}

func runDecisionDashboard(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	bd := beads.New(beads.ResolveBeadsDir(townRoot))

	// Get pending decisions grouped by urgency
	pendingHigh, _ := bd.ListDecisionsByUrgency(beads.UrgencyHigh)
	pendingMedium, _ := bd.ListDecisionsByUrgency(beads.UrgencyMedium)
	pendingLow, _ := bd.ListDecisionsByUrgency(beads.UrgencyLow)

	// Get recently resolved (last 7 days)
	recentlyResolved, _ := bd.ListRecentlyResolvedDecisions(7 * 24 * time.Hour)

	// Get stale decisions (older than 24 hours)
	staleDecisions, _ := bd.ListStaleDecisions(24 * time.Hour)

	totalPending := len(pendingHigh) + len(pendingMedium) + len(pendingLow)

	if decisionDashboardJSON {
		result := map[string]interface{}{
			"pending": map[string]interface{}{
				"high":   formatDecisionsList(pendingHigh),
				"medium": formatDecisionsList(pendingMedium),
				"low":    formatDecisionsList(pendingLow),
				"total":  totalPending,
			},
			"recently_resolved": formatDecisionsList(recentlyResolved),
			"stale":             formatDecisionsList(staleDecisions),
		}
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	fmt.Println("📋 Decision Dashboard")
	fmt.Println()

	// Pending section
	fmt.Printf("Pending (%d)\n", totalPending)
	if totalPending == 0 {
		fmt.Println("  (none)")
	} else {
		// High urgency first
		for _, issue := range pendingHigh {
			fields := beads.ParseDecisionFields(issue.Description)
			age := formatDecisionAge(issue.CreatedAt)
			fmt.Printf("  🔴 [HIGH] %s: %s (%s)\n", issue.ID, truncateString(fields.Question, 40), age)
		}
		// Medium urgency
		for _, issue := range pendingMedium {
			fields := beads.ParseDecisionFields(issue.Description)
			age := formatDecisionAge(issue.CreatedAt)
			fmt.Printf("  🟡 [MEDIUM] %s: %s (%s)\n", issue.ID, truncateString(fields.Question, 40), age)
		}
		// Low urgency
		for _, issue := range pendingLow {
			fields := beads.ParseDecisionFields(issue.Description)
			age := formatDecisionAge(issue.CreatedAt)
			fmt.Printf("  🟢 [LOW] %s: %s (%s)\n", issue.ID, truncateString(fields.Question, 40), age)
		}
	}
	fmt.Println()

	// Recently resolved section
	fmt.Printf("Recently Resolved (%d)\n", len(recentlyResolved))
	if len(recentlyResolved) == 0 {
		fmt.Println("  (none in last 7 days)")
	} else {
		for i, issue := range recentlyResolved {
			if i >= 5 {
				fmt.Printf("  ... and %d more\n", len(recentlyResolved)-5)
				break
			}
			fields := beads.ParseDecisionFields(issue.Description)
			chosen := "?"
			if fields.ChosenIndex > 0 && fields.ChosenIndex <= len(fields.Options) {
				chosen = fields.Options[fields.ChosenIndex-1].Label
			}
			age := formatDecisionAge(issue.ClosedAt)
			fmt.Printf("  ✓ %s: %s → \"%s\" (%s)\n", issue.ID, truncateString(fields.Question, 30), chosen, age)
		}
	}
	fmt.Println()

	// Stale section
	if len(staleDecisions) > 0 {
		fmt.Printf("⚠️  Stale (unresolved > 24h): %d\n", len(staleDecisions))
		for _, issue := range staleDecisions {
			fields := beads.ParseDecisionFields(issue.Description)
			age := formatDecisionAge(issue.CreatedAt)
			fmt.Printf("  ⚠️  %s: %s (%s old)\n", issue.ID, truncateString(fields.Question, 40), age)
		}
		fmt.Println()
	}

	fmt.Println("Run 'gt decision list' for details")

	return nil
}

func formatDecisionsList(issues []*beads.Issue) []map[string]interface{} {
	var result []map[string]interface{}
	for _, issue := range issues {
		fields := beads.ParseDecisionFields(issue.Description)
		item := map[string]interface{}{
			"id":           issue.ID,
			"question":     fields.Question,
			"urgency":      fields.Urgency,
			"requested_by": fields.RequestedBy,
			"created_at":   issue.CreatedAt,
		}
		if fields.ChosenIndex > 0 && fields.ChosenIndex <= len(fields.Options) {
			item["chosen"] = fields.Options[fields.ChosenIndex-1].Label
		}
		result = append(result, item)
	}
	return result
}

func formatDecisionAge(timestamp string) string {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return "?"
	}
	d := time.Since(t)
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// Helper functions

func formatDecisionMailBody(beadID string, fields *beads.DecisionFields) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("Decision ID: %s", beadID))
	lines = append(lines, fmt.Sprintf("Urgency: %s", fields.Urgency))
	lines = append(lines, fmt.Sprintf("From: %s", fields.RequestedBy))
	lines = append(lines, "")
	lines = append(lines, "Question:")
	lines = append(lines, fields.Question)

	if fields.Context != "" {
		lines = append(lines, "")
		lines = append(lines, "Context:")
		lines = append(lines, fields.Context)
	}

	lines = append(lines, "")
	lines = append(lines, "Options:")
	for i, opt := range fields.Options {
		marker := ""
		if opt.Recommended {
			marker = " (Recommended)"
		}
		lines = append(lines, fmt.Sprintf("  %d. %s%s", i+1, opt.Label, marker))
		if opt.Description != "" {
			lines = append(lines, fmt.Sprintf("     %s", opt.Description))
		}
	}

	if len(fields.Blockers) > 0 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("Blocking: %s", strings.Join(fields.Blockers, ", ")))
	}

	lines = append(lines, "")
	lines = append(lines, "---")
	lines = append(lines, fmt.Sprintf("To resolve: gt decision resolve %s --choice N --rationale \"...\"", beadID))
	return strings.Join(lines, "\n")
}

func formatResolutionMailBody(beadID, question, chosen, rationale, resolvedBy string) string {
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

func formatOptionsSummary(options []beads.DecisionOption) string {
	var labels []string
	for _, opt := range options {
		label := opt.Label
		if opt.Recommended {
			label += "*"
		}
		labels = append(labels, label)
	}
	return strings.Join(labels, ", ")
}

func urgencyEmoji(urgency string) string {
	switch urgency {
	case beads.UrgencyHigh:
		return "🔴"
	case beads.UrgencyMedium:
		return "🟡"
	case beads.UrgencyLow:
		return "🟢"
	default:
		return "📋"
	}
}

func runDecisionAwait(cmd *cobra.Command, args []string) error {
	decisionID := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Parse timeout if specified
	var timeout time.Duration
	if decisionAwaitTimeout != "" {
		timeout, err = time.ParseDuration(decisionAwaitTimeout)
		if err != nil {
			return fmt.Errorf("invalid timeout format: %w", err)
		}
	}

	bd := beads.New(beads.ResolveBeadsDir(townRoot))

	// Check if already resolved
	issue, fields, err := bd.GetDecisionBead(decisionID)
	if err != nil {
		return fmt.Errorf("getting decision: %w", err)
	}
	if issue == nil {
		return fmt.Errorf("decision not found: %s", decisionID)
	}

	startTime := time.Now()
	pollInterval := 5 * time.Second

	for {
		// Check if resolved
		if fields.ChosenIndex > 0 {
			// Decision is resolved
			if decisionJSON {
				result := map[string]interface{}{
					"id":           decisionID,
					"resolved":     true,
					"chosen_index": fields.ChosenIndex,
					"chosen_label": fields.Options[fields.ChosenIndex-1].Label,
					"rationale":    fields.Rationale,
					"resolved_by":  fields.ResolvedBy,
					"resolved_at":  fields.ResolvedAt,
				}
				out, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(out))
			} else {
				fmt.Printf("✓ Decision %s resolved: %s\n", decisionID, fields.Options[fields.ChosenIndex-1].Label)
				if fields.Rationale != "" {
					fmt.Printf("  Rationale: %s\n", fields.Rationale)
				}
			}
			return nil
		}

		// Check timeout
		if timeout > 0 && time.Since(startTime) > timeout {
			if decisionJSON {
				result := map[string]interface{}{
					"id":       decisionID,
					"resolved": false,
					"error":    "timeout waiting for decision",
				}
				out, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(out))
			}
			return fmt.Errorf("timeout waiting for decision %s to be resolved", decisionID)
		}

		// Wait and poll again
		time.Sleep(pollInterval)

		// Refresh decision state
		issue, fields, err = bd.GetDecisionBead(decisionID)
		if err != nil {
			return fmt.Errorf("getting decision: %w", err)
		}
		if issue == nil {
			return fmt.Errorf("decision not found: %s", decisionID)
		}
	}
}

func runDecisionRemind(cmd *cobra.Command, args []string) error {
	// Detect if there's work in the session that warrants a decision
	hasWork := false
	var workIndicators []string

	// Check 1: Git status - uncommitted changes
	gitChanges := checkGitChanges()
	if gitChanges != "" {
		hasWork = true
		workIndicators = append(workIndicators, gitChanges)
	}

	// Check 2: Hooked work
	hookedWork := checkHookedWork()
	if hookedWork != "" {
		hasWork = true
		workIndicators = append(workIndicators, hookedWork)
	}

	// Check 3: In-progress beads
	inProgressBeads := checkInProgressBeads()
	if inProgressBeads != "" {
		hasWork = true
		workIndicators = append(workIndicators, inProgressBeads)
	}

	if !hasWork {
		if decisionRemindInject || decisionRemindNudge {
			// Silent exit - no reminder needed
			return nil
		}
		fmt.Println("No session work detected - no decision reminder needed")
		return nil
	}

	// Format the reminder
	reminderText := formatDecisionReminder(workIndicators)

	if decisionRemindNudge {
		// Send reminder as nudge to current agent's session
		agent := detectSender()
		if agent == "" {
			agent = "gastown/crew/decision_point" // fallback
		}
		nudgeMsg := "DECISION REMINDER: Session work detected. Consider offering the user a decision point about next steps before ending this session."
		nudgeCmd := execCommand("gt", "nudge", agent, nudgeMsg)
		if err := nudgeCmd.Run(); err != nil {
			// Don't fail the hook, just log
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to send decision nudge: %v\n", err)
		}
		return nil
	}

	if decisionRemindInject {
		// Output as system-reminder for Claude Code hooks
		fmt.Printf("<system-reminder>\n%s\n</system-reminder>\n", reminderText)
		return nil
	}

	// Human-readable output
	fmt.Println(reminderText)
	return nil
}

func checkGitChanges() string {
	// Check for uncommitted changes using git status
	out, err := runGitCommand("status", "--porcelain")
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return ""
	}
	return fmt.Sprintf("uncommitted changes (%d files)", len(lines))
}

func checkHookedWork() string {
	// Check for hooked work via gt hook
	out, err := runCommand("gt", "hook", "--json")
	if err != nil {
		return ""
	}
	// Parse JSON to check if there's a hooked bead
	var hookData map[string]interface{}
	if err := json.Unmarshal([]byte(out), &hookData); err != nil {
		return ""
	}
	if beadID, ok := hookData["bead_id"].(string); ok && beadID != "" {
		return fmt.Sprintf("hooked work: %s", beadID)
	}
	return ""
}

func checkInProgressBeads() string {
	// Check for in-progress beads
	out, err := runCommand("bd", "list", "--status=in_progress", "--json")
	if err != nil {
		return ""
	}
	var beadsList []interface{}
	if err := json.Unmarshal([]byte(out), &beadsList); err != nil {
		return ""
	}
	if len(beadsList) > 0 {
		return fmt.Sprintf("in-progress beads (%d)", len(beadsList))
	}
	return ""
}

func runGitCommand(args ...string) (string, error) {
	cmd := execCommand("git", args...)
	out, err := cmd.Output()
	return string(out), err
}

func runCommand(name string, args ...string) (string, error) {
	cmd := execCommand(name, args...)
	out, err := cmd.Output()
	return string(out), err
}

// execCommand is a wrapper for os/exec.Command
func execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...) //nolint:gosec // trusted internal commands
}

func formatDecisionReminder(workIndicators []string) string {
	var sb strings.Builder
	sb.WriteString("DECISION OFFERING REMINDER\n")
	sb.WriteString("\n")
	sb.WriteString("Session work detected:\n")
	for _, indicator := range workIndicators {
		sb.WriteString(fmt.Sprintf("  - %s\n", indicator))
	}
	sb.WriteString("\n")
	sb.WriteString("Before ending this session, consider offering a decision point:\n")
	sb.WriteString("1. What was accomplished in this session?\n")
	sb.WriteString("2. What are the next steps or options?\n")
	sb.WriteString("3. Are there architectural choices or scope decisions needed?\n")
	sb.WriteString("\n")
	sb.WriteString("Use 'gt decision request' to create a decision, or proceed with handoff if\n")
	sb.WriteString("the work is self-contained and no human input is needed.\n")
	return sb.String()
}

