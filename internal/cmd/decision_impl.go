package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/inject"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/rpcclient"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/style"
	decisionTUI "github.com/steveyegge/gastown/internal/tui/decision"
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

	// Validate FILE option when failure context detected (Fail-then-File principle)
	if !decisionNoFileCheck && hasFailureContext(decisionPrompt, decisionContext) {
		if !hasFileOption(options) {
			return fmt.Errorf("failure context detected but no FILE option provided.\n\n"+
				"The prompt mentions an error/failure. Per the 'Fail then File' principle,\n"+
				"decisions about failures should include an option to file a tracking bug.\n\n"+
				"Suggested option:\n  --option \"%s\"\n\n"+
				"Use --no-file-check to skip this validation.", suggestFileOption())
		}
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

	// Try RPC first if gtmobile is available (enables real-time event bus notifications)
	var issue *beads.Issue
	rpcUsed := false

	rpcClient := rpcclient.NewClient("http://localhost:8443")
	if rpcClient.IsAvailable(context.Background()) {
		// Convert options for RPC
		var rpcOptions []rpcclient.DecisionOption
		for _, opt := range options {
			rpcOptions = append(rpcOptions, rpcclient.DecisionOption{
				Label:       opt.Label,
				Description: opt.Description,
				Recommended: opt.Recommended,
			})
		}

		decision, rpcErr := rpcClient.CreateDecision(context.Background(), rpcclient.CreateDecisionRequest{
			Question:    decisionPrompt,
			Context:     decisionContext,
			Options:     rpcOptions,
			RequestedBy: agentID,
			Urgency:     urgency,
			Blockers:    fields.Blockers,
		})
		if rpcErr == nil {
			// RPC succeeded - use the returned decision
			issue = &beads.Issue{ID: decision.ID}
			rpcUsed = true
		}
		// If RPC fails, fall back to direct beads
	}

	// Fall back to direct beads if RPC not available or failed
	if !rpcUsed {
		bd := beads.New(beads.ResolveBeadsDir(townRoot))
		var err error
		issue, err = bd.CreateDecisionBead(decisionPrompt, fields)
		if err != nil {
			return fmt.Errorf("creating decision bead: %w", err)
		}
	}

	// Add blocker dependency if specified (only if we used direct beads, RPC handles this)
	if decisionBlocks != "" && !rpcUsed {
		bd := beads.New(beads.ResolveBeadsDir(townRoot))
		if err := bd.AddDecisionBlocker(issue.ID, decisionBlocks); err != nil {
			style.PrintWarning("failed to add blocker dependency: %v", err)
		}
	}

	// Send notification mail to human/overseer
	router := mail.NewRouter(townRoot)
	msg := &mail.Message{
		From:    agentID,
		To:      "overseer",
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
		// Use ListAllPendingDecisions to include both gt and bd decisions
		issues, err = bd.ListAllPendingDecisions()
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
		} else if beads.HasLabel(issue, "decision:canceled") {
			status = "CANCELED"
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

	// Use option description as fallback rationale if none provided
	effectiveRationale := decisionRationale
	if effectiveRationale == "" && chosenOption.Description != "" {
		effectiveRationale = chosenOption.Description
	}

	// Resolve the decision
	if err := bd.ResolveDecision(decisionID, decisionChoice, effectiveRationale, resolvedBy); err != nil {
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

		// Build subject - include rationale so agents see it in mail notifications
		subject := fmt.Sprintf("[DECISION RESOLVED] %s → %s", truncateString(fields.Question, 30), chosenOption.Label)
		if effectiveRationale != "" {
			subject += fmt.Sprintf(": %s", truncateString(effectiveRationale, 40))
		}

		msg := &mail.Message{
			From:     resolvedBy,
			To:       fields.RequestedBy,
			Subject:  subject,
			Body:     formatResolutionMailBody(decisionID, fields.Question, chosenOption.Label, effectiveRationale, resolvedBy),
			Type:     mail.TypeTask,
			Priority: mail.PriorityNormal,
		}

		if err := router.Send(msg); err != nil {
			style.PrintWarning("failed to notify requestor: %v", err)
		}

		// Nudge the requesting agent so they see the resolution immediately
		nudgeMsg := fmt.Sprintf("[DECISION RESOLVED] %s: Chose \"%s\"", decisionID, chosenOption.Label)
		if effectiveRationale != "" {
			nudgeMsg += fmt.Sprintf(" - %s", effectiveRationale)
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
	if effectiveRationale != "" {
		payload["rationale"] = effectiveRationale
	}
	_ = events.LogFeed(events.TypeDecisionResolved, resolvedBy, payload)

	// Output
	if decisionJSON {
		result := map[string]interface{}{
			"id":           decisionID,
			"chosen_index": decisionChoice,
			"chosen_label": chosenOption.Label,
			"rationale":    effectiveRationale,
			"resolved_by":  resolvedBy,
			"unblocked":    fields.Blockers,
		}
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
	} else {
		fmt.Printf("✓ Resolved %s: %s\n", decisionID, chosenOption.Label)
		if effectiveRationale != "" {
			fmt.Printf("  Rationale: %s\n", effectiveRationale)
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

func runDecisionWatch(cmd *cobra.Command, args []string) error {
	// Handle RPC mode - simple streaming watcher for testing the RPC layer
	if decisionWatchRPC {
		return runDecisionWatchRPC()
	}

	// Verify we're in a Gas Town workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Try to infer current rig
	currentRig, _ := inferRigFromCwd(townRoot)
	if currentRig == "" {
		currentRig = "gastown" // fallback
	}

	// Create the TUI model
	m := decisionTUI.New()

	// Set workspace info for crew creation
	m.SetWorkspace(townRoot, currentRig)

	// Apply flags
	if decisionWatchUrgentOnly {
		m.SetFilter("high")
	}
	if decisionWatchNotify {
		m.SetNotify(true)
	}

	// Run the TUI
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running decision watch: %w", err)
	}

	return nil
}

// runDecisionWatchRPC runs a simple RPC-based decision watcher.
// This serves as a test harness for the mobile RPC server.
func runDecisionWatchRPC() error {
	fmt.Printf("Connecting to RPC server at %s...\n", decisionWatchRPCAddr)

	client := rpcclient.NewClient(decisionWatchRPCAddr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nStopping...")
		cancel()
	}()

	fmt.Println("Watching for decisions (Ctrl+C to stop)...")
	fmt.Println()

	seen := make(map[string]bool)
	err := client.WatchDecisions(ctx, func(d rpcclient.Decision) error {
		if seen[d.ID] {
			return nil
		}
		seen[d.ID] = true

		// Print decision info
		urgencyIcon := "●"
		switch d.Urgency {
		case "high":
			urgencyIcon = "🔴"
		case "medium":
			urgencyIcon = "🟡"
		case "low":
			urgencyIcon = "🟢"
		}

		fmt.Printf("%s [%s] %s\n", urgencyIcon, d.ID, d.Question)
		fmt.Printf("   Requested by: %s\n", d.RequestedBy)
		for i, opt := range d.Options {
			rec := ""
			if opt.Recommended {
				rec = " (recommended)"
			}
			fmt.Printf("   %d. %s%s\n", i+1, opt.Label, rec)
		}
		fmt.Println()
		return nil
	})

	if err != nil && err != context.Canceled {
		return fmt.Errorf("watch error: %w", err)
	}
	return nil
}

// --- Turn enforcement functions ---

// turnMarkerPath returns the path to the turn marker file for a session.
func turnMarkerPath(sessionID string) string {
	return fmt.Sprintf("/tmp/.decision-offered-%s", sessionID)
}

// turnMarkerExists checks if the turn marker exists.
func turnMarkerExists(sessionID string) bool {
	_, err := os.Stat(turnMarkerPath(sessionID))
	return err == nil
}

// createTurnMarker creates the turn marker file.
func createTurnMarker(sessionID string) error {
	f, err := os.Create(turnMarkerPath(sessionID))
	if err != nil {
		return err
	}
	return f.Close()
}

// clearTurnMarker removes the turn marker file.
func clearTurnMarker(sessionID string) {
	_ = os.Remove(turnMarkerPath(sessionID))
}

// isDecisionCommand checks if a command creates a decision.
func isDecisionCommand(command string) bool {
	return strings.Contains(command, "gt decision request") ||
		strings.Contains(command, "bd decision create")
}

// TurnBlockResult is the JSON response for blocking a turn.
type TurnBlockResult struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// checkTurnMarker checks if a decision was offered this turn.
// Returns nil if allowed, or a TurnBlockResult if blocked.
// If soft is true, never blocks (just returns nil).
// NOTE: This does NOT clear the marker - that's done by turn-clear at the
// start of the next turn. This allows Stop hook to fire multiple times
// without incorrectly blocking on subsequent checks.
func checkTurnMarker(sessionID string, soft bool) *TurnBlockResult {
	if turnMarkerExists(sessionID) {
		// Decision was offered - allow (don't clear; turn-clear handles that)
		return nil
	}

	// No decision offered
	if soft {
		// Soft mode - don't block
		return nil
	}

	// Strict mode - block
	return &TurnBlockResult{
		Decision: "block",
		Reason:   "You must offer a formal decision point using 'gt decision request' before ending this turn. This ensures humans stay informed about progress and can provide guidance.",
	}
}

// turnHookInput represents the JSON input from Claude Code hooks for turn enforcement.
type turnHookInput struct {
	SessionID string `json:"session_id"`
	ToolInput struct {
		Command string `json:"command"`
	} `json:"tool_input"`
}

// readTurnHookInput reads and parses hook JSON from stdin.
func readTurnHookInput() (*turnHookInput, error) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, err
	}
	var input turnHookInput
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, err
	}
	return &input, nil
}

func runDecisionTurnClear(cmd *cobra.Command, args []string) error {
	input, err := readTurnHookInput()
	if err != nil {
		// Hooks should never fail - just exit cleanly
		return nil
	}

	if input.SessionID != "" {
		clearTurnMarker(input.SessionID)
	}
	return nil
}

func runDecisionTurnMark(cmd *cobra.Command, args []string) error {
	input, err := readTurnHookInput()
	if err != nil {
		// Hooks should never fail
		return nil
	}

	if input.SessionID == "" {
		return nil
	}

	// Check if this is a decision command
	if isDecisionCommand(input.ToolInput.Command) {
		_ = createTurnMarker(input.SessionID)
	}

	return nil
}

func runDecisionTurnCheck(cmd *cobra.Command, args []string) error {
	input, err := readTurnHookInput()
	if err != nil {
		// Hooks should never fail
		return nil
	}

	if input.SessionID == "" {
		return nil
	}

	result := checkTurnMarker(input.SessionID, decisionTurnCheckSoft)

	if result != nil {
		// Output block JSON
		out, _ := json.Marshal(result)
		fmt.Println(string(out))
		// Exit non-zero to fail the hook (unless soft mode)
		if !decisionTurnCheckSoft {
			return NewSilentExit(1)
		}
	}

	return nil
}

func runDecisionCancel(cmd *cobra.Command, args []string) error {
	decisionID := args[0]

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a workspace: %w", err)
	}

	// Connect to beads
	bd := beads.New(beads.ResolveBeadsDir(townRoot))

	// Get the decision to verify it exists
	issue, _, err := bd.GetDecisionBead(decisionID)
	if err != nil {
		return fmt.Errorf("failed to get decision %s: %w", decisionID, err)
	}
	if issue == nil {
		return fmt.Errorf("decision not found: %s", decisionID)
	}

	// Verify it's a pending decision (has decision:pending label)
	isPending := false
	for _, label := range issue.Labels {
		if label == "decision:pending" {
			isPending = true
			break
		}
	}

	if !isPending {
		return fmt.Errorf("%s is not a pending decision (may already be resolved)", decisionID)
	}

	// Build new labels: remove decision:pending, add decision:canceled
	newLabels := []string{}
	for _, label := range issue.Labels {
		if label != "decision:pending" {
			newLabels = append(newLabels, label)
		}
	}
	newLabels = append(newLabels, "decision:canceled")

	// Close the decision with cancellation reason
	reason := decisionCancelReason
	if reason == "" {
		reason = "Canceled"
	}

	if err := bd.CloseWithReason(reason, decisionID); err != nil {
		return fmt.Errorf("failed to cancel decision: %w", err)
	}

	// Update labels
	if err := bd.Update(decisionID, beads.UpdateOptions{SetLabels: newLabels}); err != nil {
		// Non-fatal, decision is already closed
		fmt.Fprintf(os.Stderr, "Warning: failed to update labels: %v\n", err)
	}

	fmt.Printf("✓ Canceled %s: %s\n", decisionID, reason)
	return nil
}

func runDecisionCheck(cmd *cobra.Command, args []string) error {
	// Determine identity (priority: --identity flag, auto-detect)
	identity := ""
	if decisionCheckIdentity != "" {
		identity = decisionCheckIdentity
	} else {
		identity = detectSender()
	}

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		if decisionCheckInject {
			// Inject mode: always exit 0, silent on error
			return nil
		}
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	bd := beads.New(beads.ResolveBeadsDir(townRoot))

	// Get pending decisions
	issues, err := bd.ListDecisions()
	if err != nil {
		if decisionCheckInject {
			return nil
		}
		return fmt.Errorf("listing decisions: %w", err)
	}

	// Filter to decisions requested by this identity (or all if identity is overseer/human)
	var relevant []*beads.Issue
	for _, issue := range issues {
		fields := beads.ParseDecisionFields(issue.Description)
		// Include if:
		// 1. Requested by this identity
		// 2. Identity is "overseer" or "human" (sees all)
		// 3. Has this identity in blockers (waiting on decision to unblock)
		if fields.RequestedBy == identity ||
			identity == "overseer" ||
			identity == "human" ||
			containsString(fields.Blockers, identity) {
			relevant = append(relevant, issue)
		}
	}

	// JSON output
	if decisionCheckJSON {
		result := map[string]interface{}{
			"identity": identity,
			"pending":  len(relevant),
			"has_new":  len(relevant) > 0,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	// Inject mode: queue system-reminder if decisions exist
	if decisionCheckInject {
		if len(relevant) > 0 {
			// Build the system-reminder content
			var buf bytes.Buffer
			buf.WriteString("<system-reminder>\n")
			buf.WriteString(fmt.Sprintf("You have %d pending decision(s) awaiting resolution.\n\n", len(relevant)))

			for _, issue := range relevant {
				fields := beads.ParseDecisionFields(issue.Description)
				emoji := urgencyEmoji(fields.Urgency)
				buf.WriteString(fmt.Sprintf("- %s %s [%s]: %s\n", emoji, issue.ID, strings.ToUpper(fields.Urgency), truncateString(fields.Question, 50)))
			}

			buf.WriteString("\n")
			buf.WriteString("Run 'gt decision list' to see details, or 'gt decision resolve <id> --choice N' to resolve.\n")
			buf.WriteString("</system-reminder>\n")

			// Check if we should queue or output directly
			sessionID := runtime.SessionIDFromEnv()
			if sessionID != "" {
				// Session ID available - use queue
				queue := inject.NewQueue(townRoot, sessionID)
				if err := queue.Enqueue(inject.TypeDecision, buf.String()); err != nil {
					// Fall back to direct output on queue error
					fmt.Print(buf.String())
				}
			} else {
				// No session ID - output directly (legacy behavior)
				fmt.Print(buf.String())
			}
		}
		return nil
	}

	// Normal mode
	if len(relevant) > 0 {
		fmt.Printf("%s %d pending decision(s)\n", style.Bold.Render("📋"), len(relevant))
		return NewSilentExit(0)
	}
	fmt.Println("No pending decisions")
	return NewSilentExit(1)
}

// containsString checks if a slice contains a string.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// --- Fail-then-File validation ---

// failureKeywords are patterns that indicate a failure context in the prompt or context.
var failureKeywords = []string{
	"error", "fail", "bug", "broke", "broken", "issue", "problem",
	"stuck", "crash", "exception", "panic", "fatal", "cannot", "unable",
	"doesn't work", "does not work", "not working", "won't", "will not",
	"400", "401", "403", "404", "500", "502", "503", "504", // HTTP error codes
}

// fileKeywords are patterns that indicate an option mentions filing/tracking.
var fileKeywords = []string{
	"file", "bug", "track", "bd create", "investigate", "report",
	"create issue", "create bead", "open issue", "log issue",
}

// hasFailureContext checks if the prompt or context contains failure indicators.
func hasFailureContext(prompt, context string) bool {
	combined := strings.ToLower(prompt + " " + context)
	for _, keyword := range failureKeywords {
		if strings.Contains(combined, keyword) {
			return true
		}
	}
	return false
}

// hasFileOption checks if any option mentions filing/tracking bugs.
func hasFileOption(options []beads.DecisionOption) bool {
	for _, opt := range options {
		combined := strings.ToLower(opt.Label + " " + opt.Description)
		for _, keyword := range fileKeywords {
			if strings.Contains(combined, keyword) {
				return true
			}
		}
	}
	return false
}

// suggestFileOption returns a suggested option text for filing a bug.
func suggestFileOption() string {
	return "File bug: Create tracking bead to investigate root cause"
}

