package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

func runEscalate(cmd *cobra.Command, args []string) error {
	// Require at least a description when creating an escalation
	if len(args) == 0 {
		return cmd.Help()
	}

	description := strings.Join(args, " ")

	// Validate severity
	severity := strings.ToLower(escalateSeverity)
	validSeverities := map[string]bool{
		config.SeverityCritical: true,
		config.SeverityHigh:     true,
		config.SeverityNormal:   true,
		config.SeverityLow:      true,
	}
	if !validSeverities[severity] {
		return fmt.Errorf("invalid severity '%s': must be critical, high, normal, or low", escalateSeverity)
	}

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load escalation config
	escalationConfig, err := config.LoadOrCreateEscalationConfig(config.EscalationConfigPath(townRoot))
	if err != nil {
		return fmt.Errorf("loading escalation config: %w", err)
	}

	if !escalationConfig.Enabled {
		return fmt.Errorf("escalation system is disabled in config")
	}

	// Detect agent identity
	agentID := detectSender()
	if agentID == "" {
		agentID = "unknown"
	}

	// Dry run mode
	if escalateDryRun {
		route := escalationConfig.GetRouteForSeverity(severity)
		fmt.Printf("Would create escalation:\n")
		fmt.Printf("  Severity: %s\n", severity)
		fmt.Printf("  Description: %s\n", description)
		if escalateReason != "" {
			fmt.Printf("  Reason: %s\n", escalateReason)
		}
		fmt.Printf("  Targets: %s\n", strings.Join(route.Targets, ", "))
		if route.UseExternal {
			fmt.Printf("  External: enabled\n")
		}
		return nil
	}

	// Create escalation bead
	bd := beads.New(beads.ResolveBeadsDir(townRoot))
	fields := &beads.EscalationFields{
		Severity:    severity,
		Reason:      escalateReason,
		EscalatedBy: agentID,
		EscalatedAt: time.Now().Format(time.RFC3339),
		RelatedBead: escalateRelatedBead,
	}

	issue, err := bd.CreateEscalationBead(description, fields)
	if err != nil {
		return fmt.Errorf("creating escalation bead: %w", err)
	}

	// Get routing for this severity
	route := escalationConfig.GetRouteForSeverity(severity)

	// Send mail to each target
	router := mail.NewRouter(townRoot)
	for _, target := range route.Targets {
		msg := &mail.Message{
			From:    agentID,
			To:      target,
			Subject: fmt.Sprintf("[%s] %s", strings.ToUpper(severity), description),
			Body:    formatEscalationMailBody(issue.ID, severity, escalateReason, agentID, escalateRelatedBead),
			Type:    mail.TypeTask,
		}

		// Set priority based on severity
		switch severity {
		case config.SeverityCritical:
			msg.Priority = mail.PriorityUrgent
		case config.SeverityHigh:
			msg.Priority = mail.PriorityHigh
		case config.SeverityNormal:
			msg.Priority = mail.PriorityNormal
		default:
			msg.Priority = mail.PriorityLow
		}

		if err := router.Send(msg); err != nil {
			style.PrintWarning("failed to send to %s: %v", target, err)
		}
	}

	// Log to activity feed
	payload := events.EscalationPayload(issue.ID, agentID, strings.Join(route.Targets, ","), description)
	payload["severity"] = severity
	_ = events.LogFeed(events.TypeEscalationSent, agentID, payload)

	// Output
	if escalateJSON {
		out, _ := json.MarshalIndent(map[string]interface{}{
			"id":       issue.ID,
			"severity": severity,
			"targets":  route.Targets,
		}, "", "  ")
		fmt.Println(string(out))
	} else {
		emoji := severityEmoji(severity)
		fmt.Printf("%s Escalation created: %s\n", emoji, issue.ID)
		fmt.Printf("  Severity: %s\n", severity)
		fmt.Printf("  Routed to: %s\n", strings.Join(route.Targets, ", "))
	}

	return nil
}

func runEscalateList(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	bd := beads.New(beads.ResolveBeadsDir(townRoot))

	var issues []*beads.Issue
	if escalateListAll {
		// List all (open and closed)
		out, err := bd.Run("list", "--label=gt:escalation", "--status=all", "--json")
		if err != nil {
			return fmt.Errorf("listing escalations: %w", err)
		}
		if err := json.Unmarshal(out, &issues); err != nil {
			return fmt.Errorf("parsing escalations: %w", err)
		}
	} else {
		issues, err = bd.ListEscalations()
		if err != nil {
			return fmt.Errorf("listing escalations: %w", err)
		}
	}

	if escalateListJSON {
		out, _ := json.MarshalIndent(issues, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	if len(issues) == 0 {
		fmt.Println("No escalations found")
		return nil
	}

	fmt.Printf("Escalations (%d):\n\n", len(issues))
	for _, issue := range issues {
		fields := beads.ParseEscalationFields(issue.Description)
		emoji := severityEmoji(fields.Severity)

		status := issue.Status
		if beads.HasLabel(issue, "acked") {
			status = "acked"
		}

		fmt.Printf("  %s %s [%s] %s\n", emoji, issue.ID, status, issue.Title)
		fmt.Printf("     Severity: %s | From: %s | %s\n",
			fields.Severity, fields.EscalatedBy, formatRelativeTime(issue.CreatedAt))
		if fields.AckedBy != "" {
			fmt.Printf("     Acked by: %s\n", fields.AckedBy)
		}
		fmt.Println()
	}

	return nil
}

func runEscalateAck(cmd *cobra.Command, args []string) error {
	escalationID := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Detect who is acknowledging
	ackedBy := detectSender()
	if ackedBy == "" {
		ackedBy = "unknown"
	}

	bd := beads.New(beads.ResolveBeadsDir(townRoot))
	if err := bd.AckEscalation(escalationID, ackedBy); err != nil {
		return fmt.Errorf("acknowledging escalation: %w", err)
	}

	// Log to activity feed
	_ = events.LogFeed(events.TypeEscalationAcked, ackedBy, map[string]interface{}{
		"escalation_id": escalationID,
		"acked_by":      ackedBy,
	})

	fmt.Printf("%s Escalation acknowledged: %s\n", style.Bold.Render("✓"), escalationID)
	return nil
}

func runEscalateClose(cmd *cobra.Command, args []string) error {
	escalationID := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Detect who is closing
	closedBy := detectSender()
	if closedBy == "" {
		closedBy = "unknown"
	}

	bd := beads.New(beads.ResolveBeadsDir(townRoot))
	if err := bd.CloseEscalation(escalationID, closedBy, escalateCloseReason); err != nil {
		return fmt.Errorf("closing escalation: %w", err)
	}

	// Log to activity feed
	_ = events.LogFeed(events.TypeEscalationClosed, closedBy, map[string]interface{}{
		"escalation_id": escalationID,
		"closed_by":     closedBy,
		"reason":        escalateCloseReason,
	})

	fmt.Printf("%s Escalation closed: %s\n", style.Bold.Render("✓"), escalationID)
	fmt.Printf("  Reason: %s\n", escalateCloseReason)
	return nil
}

func runEscalateStale(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load escalation config for threshold
	escalationConfig, err := config.LoadOrCreateEscalationConfig(config.EscalationConfigPath(townRoot))
	if err != nil {
		return fmt.Errorf("loading escalation config: %w", err)
	}

	threshold := escalationConfig.GetStaleThreshold()

	bd := beads.New(beads.ResolveBeadsDir(townRoot))
	stale, err := bd.ListStaleEscalations(threshold)
	if err != nil {
		return fmt.Errorf("listing stale escalations: %w", err)
	}

	if escalateStaleJSON {
		out, _ := json.MarshalIndent(stale, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	if len(stale) == 0 {
		fmt.Printf("No stale escalations (threshold: %s)\n", threshold)
		return nil
	}

	fmt.Printf("Stale escalations (%d, threshold: %s):\n\n", len(stale), threshold)
	for _, issue := range stale {
		fields := beads.ParseEscalationFields(issue.Description)
		emoji := severityEmoji(fields.Severity)

		fmt.Printf("  %s %s %s\n", emoji, issue.ID, issue.Title)
		fmt.Printf("     Severity: %s | From: %s | %s\n",
			fields.Severity, fields.EscalatedBy, formatRelativeTime(issue.CreatedAt))
		fmt.Println()
	}

	return nil
}

func runEscalateShow(cmd *cobra.Command, args []string) error {
	escalationID := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	bd := beads.New(beads.ResolveBeadsDir(townRoot))
	issue, fields, err := bd.GetEscalationBead(escalationID)
	if err != nil {
		return fmt.Errorf("getting escalation: %w", err)
	}
	if issue == nil {
		return fmt.Errorf("escalation not found: %s", escalationID)
	}

	if escalateJSON {
		data := map[string]interface{}{
			"id":          issue.ID,
			"title":       issue.Title,
			"status":      issue.Status,
			"created_at":  issue.CreatedAt,
			"severity":    fields.Severity,
			"reason":      fields.Reason,
			"escalatedBy": fields.EscalatedBy,
			"escalatedAt": fields.EscalatedAt,
			"ackedBy":     fields.AckedBy,
			"ackedAt":     fields.AckedAt,
			"closedBy":    fields.ClosedBy,
			"closedReason": fields.ClosedReason,
			"relatedBead": fields.RelatedBead,
		}
		out, _ := json.MarshalIndent(data, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	emoji := severityEmoji(fields.Severity)
	fmt.Printf("%s Escalation: %s\n", emoji, issue.ID)
	fmt.Printf("  Title: %s\n", issue.Title)
	fmt.Printf("  Status: %s\n", issue.Status)
	fmt.Printf("  Severity: %s\n", fields.Severity)
	fmt.Printf("  Created: %s\n", formatRelativeTime(issue.CreatedAt))
	fmt.Printf("  Escalated by: %s\n", fields.EscalatedBy)
	if fields.Reason != "" {
		fmt.Printf("  Reason: %s\n", fields.Reason)
	}
	if fields.AckedBy != "" {
		fmt.Printf("  Acknowledged by: %s at %s\n", fields.AckedBy, fields.AckedAt)
	}
	if fields.ClosedBy != "" {
		fmt.Printf("  Closed by: %s\n", fields.ClosedBy)
		fmt.Printf("  Resolution: %s\n", fields.ClosedReason)
	}
	if fields.RelatedBead != "" {
		fmt.Printf("  Related: %s\n", fields.RelatedBead)
	}

	return nil
}

// Helper functions

func formatEscalationMailBody(beadID, severity, reason, from, related string) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("Escalation ID: %s", beadID))
	lines = append(lines, fmt.Sprintf("Severity: %s", severity))
	lines = append(lines, fmt.Sprintf("From: %s", from))
	if reason != "" {
		lines = append(lines, "")
		lines = append(lines, "Reason:")
		lines = append(lines, reason)
	}
	if related != "" {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("Related: %s", related))
	}
	lines = append(lines, "")
	lines = append(lines, "---")
	lines = append(lines, "To acknowledge: gt escalate ack "+beadID)
	lines = append(lines, "To close: gt escalate close "+beadID+" --reason \"resolution\"")
	return strings.Join(lines, "\n")
}

func severityEmoji(severity string) string {
	switch severity {
	case config.SeverityCritical:
		return "🚨"
	case config.SeverityHigh:
		return "⚠️"
	case config.SeverityNormal:
		return "📢"
	case config.SeverityLow:
		return "ℹ️"
	default:
		return "📋"
	}
}

func formatRelativeTime(timestamp string) string {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return timestamp
	}

	duration := time.Since(t)
	if duration < time.Minute {
		return "just now"
	}
	if duration < time.Hour {
		mins := int(duration.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	}
	if duration < 24*time.Hour {
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}
	days := int(duration.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}

// detectSender is defined in mail_send.go - we reuse it here
// If it's not accessible, we fall back to environment variables
func detectSenderFallback() string {
	// Try BD_ACTOR first (most common in agent context)
	if actor := os.Getenv("BD_ACTOR"); actor != "" {
		return actor
	}
	// Try GT_ROLE
	if role := os.Getenv("GT_ROLE"); role != "" {
		return role
	}
	return ""
}
