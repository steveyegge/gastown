package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Escalation severity levels.
// These map to mail priorities and indicate urgency for human attention.
const (
	// SeverityCritical (P0) - System-threatening issues requiring immediate human attention.
	// Examples: data corruption, security breach, complete system failure.
	SeverityCritical = "CRITICAL"

	// SeverityHigh (P1) - Important blockers that need human attention soon.
	// Examples: unresolvable merge conflicts, critical blocking bugs, ambiguous requirements.
	SeverityHigh = "HIGH"

	// SeverityMedium (P2) - Standard escalations for human attention at convenience.
	// Examples: unclear requirements, design decisions needed, non-blocking issues.
	SeverityMedium = "MEDIUM"
)

var escalateCmd = &cobra.Command{
	Use:     "escalate <topic>",
	GroupID: GroupComm,
	Short:   "Escalate an issue to the human overseer",
	Long: `Escalate an issue to the human overseer for attention.

This is the structured escalation channel for Gas Town. Any agent can use this
to request human intervention when automated resolution isn't possible.

Severity levels:
  CRITICAL (P0) - System-threatening, immediate attention required
                  Examples: data corruption, security breach, system down
  HIGH     (P1) - Important blocker, needs human soon
                  Examples: unresolvable conflict, critical bug, ambiguous spec
  MEDIUM   (P2) - Standard escalation, human attention at convenience
                  Examples: design decision needed, unclear requirements

The escalation creates an audit trail bead and sends mail to the overseer
with appropriate priority. All molecular algebra edge cases should escalate
here rather than failing silently.

Examples:
  gt escalate "Database migration failed"
  gt escalate -s CRITICAL "Data corruption detected in user table"
  gt escalate -s HIGH "Merge conflict cannot be resolved automatically"
  gt escalate -s MEDIUM "Need clarification on API design" -m "Details here..."`,
	Args: cobra.MinimumNArgs(1),
	RunE: runEscalate,
}

var (
	escalateSeverity string
	escalateMessage  string
	escalateDryRun   bool
)

func init() {
	escalateCmd.Flags().StringVarP(&escalateSeverity, "severity", "s", SeverityMedium,
		"Severity level: CRITICAL, HIGH, or MEDIUM")
	escalateCmd.Flags().StringVarP(&escalateMessage, "message", "m", "",
		"Additional details about the escalation")
	escalateCmd.Flags().BoolVarP(&escalateDryRun, "dry-run", "n", false,
		"Show what would be done without executing")
	rootCmd.AddCommand(escalateCmd)
}

func runEscalate(cmd *cobra.Command, args []string) error {
	topic := strings.Join(args, " ")

	// Validate severity
	severity := strings.ToUpper(escalateSeverity)
	if severity != SeverityCritical && severity != SeverityHigh && severity != SeverityMedium {
		return fmt.Errorf("invalid severity '%s': must be CRITICAL, HIGH, or MEDIUM", escalateSeverity)
	}

	// Map severity to mail priority
	var priority mail.Priority
	switch severity {
	case SeverityCritical:
		priority = mail.PriorityUrgent
	case SeverityHigh:
		priority = mail.PriorityHigh
	default:
		priority = mail.PriorityNormal
	}

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Detect agent identity
	agentID, err := detectAgentIdentity()
	if err != nil {
		agentID = "unknown"
	}

	// Build mail subject with severity tag
	subject := fmt.Sprintf("[%s] %s", severity, topic)

	// Build mail body
	var bodyParts []string
	bodyParts = append(bodyParts, fmt.Sprintf("Escalated by: %s", agentID))
	bodyParts = append(bodyParts, fmt.Sprintf("Severity: %s", severity))
	if escalateMessage != "" {
		bodyParts = append(bodyParts, "")
		bodyParts = append(bodyParts, escalateMessage)
	}
	body := strings.Join(bodyParts, "\n")

	// Dry run mode
	if escalateDryRun {
		fmt.Printf("Would create escalation:\n")
		fmt.Printf("  Severity: %s\n", severity)
		fmt.Printf("  Priority: %s\n", priority)
		fmt.Printf("  Subject:  %s\n", subject)
		fmt.Printf("  Body:\n%s\n", indentText(body, "    "))
		fmt.Printf("Would send mail to: overseer\n")
		return nil
	}

	// Create escalation bead for audit trail
	beadID, err := createEscalationBead(topic, severity, agentID, escalateMessage)
	if err != nil {
		// Non-fatal - escalation mail is more important
		style.PrintWarning("could not create escalation bead: %v", err)
	} else {
		fmt.Printf("%s Created escalation bead: %s\n", style.Bold.Render("📋"), beadID)
	}

	// Send mail to overseer
	router := mail.NewRouter(townRoot)
	msg := &mail.Message{
		From:     agentID,
		To:       "overseer",
		Subject:  subject,
		Body:     body,
		Priority: priority,
	}

	if err := router.Send(msg); err != nil {
		return fmt.Errorf("sending escalation mail: %w", err)
	}

	// Log to activity feed
	payload := events.EscalationPayload("", agentID, "overseer", topic)
	payload["severity"] = severity
	if beadID != "" {
		payload["bead"] = beadID
	}
	_ = events.LogFeed(events.TypeEscalationSent, agentID, payload)

	// Print confirmation with severity-appropriate styling
	var emoji string
	switch severity {
	case SeverityCritical:
		emoji = "🚨"
	case SeverityHigh:
		emoji = "⚠️"
	default:
		emoji = "📢"
	}

	fmt.Printf("%s Escalation sent to overseer [%s]\n", emoji, severity)
	fmt.Printf("   Topic: %s\n", topic)
	if beadID != "" {
		fmt.Printf("   Bead:  %s\n", beadID)
	}

	return nil
}

// detectAgentIdentity returns the current agent's identity string.
func detectAgentIdentity() (string, error) {
	// Try GT_ROLE first
	if role := os.Getenv("GT_ROLE"); role != "" {
		return role, nil
	}

	// Try to detect from cwd
	agentID, _, _, err := resolveSelfTarget()
	if err != nil {
		return "", err
	}
	return agentID, nil
}

// createEscalationBead creates a bead to track the escalation.
func createEscalationBead(topic, severity, from, details string) (string, error) {
	// Use bd create to make the escalation bead
	args := []string{
		"create",
		"--title", fmt.Sprintf("[ESCALATION] %s", topic),
		"--type", "task", // Use task type since escalation isn't a standard type
		"--priority", severityToBeadsPriority(severity),
	}

	// Add description with escalation metadata
	desc := fmt.Sprintf("Escalation from: %s\nSeverity: %s\n", from, severity)
	if details != "" {
		desc += "\n" + details
	}
	args = append(args, "--description", desc)

	// Add tag for filtering
	args = append(args, "--tag", "escalation")

	cmd := exec.Command("bd", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("bd create: %w", err)
	}

	// Parse bead ID from output (bd create outputs: "Created bead: gt-xxxxx")
	output := strings.TrimSpace(string(out))
	parts := strings.Split(output, ": ")
	if len(parts) >= 2 {
		return strings.TrimSpace(parts[len(parts)-1]), nil
	}
	return "", fmt.Errorf("could not parse bead ID from: %s", output)
}

// severityToBeadsPriority converts severity to beads priority string.
func severityToBeadsPriority(severity string) string {
	switch severity {
	case SeverityCritical:
		return "0" // P0
	case SeverityHigh:
		return "1" // P1
	default:
		return "2" // P2
	}
}

// indentText indents each line of text with the given prefix.
func indentText(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
