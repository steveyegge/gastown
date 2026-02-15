package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/style"
)

func runMailCheck(cmd *cobra.Command, args []string) error {
	// Determine which inbox (priority: --identity flag, auto-detect)
	address := ""
	if mailCheckIdentity != "" {
		address = mailCheckIdentity
	} else {
		address = detectSender()
	}

	// All mail uses town beads (two-level architecture)
	workDir, err := findMailWorkDir()
	if err != nil {
		if mailCheckInject {
			fmt.Fprintf(os.Stderr, "gt mail check: workspace lookup failed: %v\n", err)
			return nil
		}
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Get mailbox
	router := mail.NewRouter(workDir)
	mailbox, err := router.GetMailbox(address)
	if err != nil {
		if mailCheckInject {
			fmt.Fprintf(os.Stderr, "gt mail check: mailbox error for %s: %v\n", address, err)
			return nil
		}
		return fmt.Errorf("getting mailbox: %w", err)
	}

	// Count unread
	_, unread, err := mailbox.Count()
	if err != nil {
		if mailCheckInject {
			fmt.Fprintf(os.Stderr, "gt mail check: count error for %s: %v\n", address, err)
			return nil
		}
		return fmt.Errorf("counting messages: %w", err)
	}

	// JSON output
	if mailCheckJSON {
		result := map[string]interface{}{
			"address": address,
			"unread":  unread,
			"has_new": unread > 0,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	// Inject mode: notify agent of mail with priority-appropriate framing.
	// Three tiers: urgent interrupts immediately, high-priority is processed
	// at the next task boundary, normal/low is informational but still
	// checked before going idle (prevents mail from sitting unread).
	if mailCheckInject {
		if unread > 0 {
			messages, listErr := mailbox.ListUnread()
			if listErr != nil {
				fmt.Fprintf(os.Stderr, "gt mail check: could not list unread for %s: %v\n", address, listErr)
				return nil
			}

			fmt.Print(formatInjectOutput(messages))
		}
		return nil
	}

	// Normal mode
	if unread > 0 {
		fmt.Printf("%s %d unread message(s)\n", style.Bold.Render("📬"), unread)
		return NewSilentExit(0)
	}
	fmt.Println("No new mail")
	return NewSilentExit(1)
}

// formatInjectOutput builds the system-reminder text for inject mode.
// It separates messages into three tiers (urgent, high, normal/low) and
// formats them with priority-appropriate framing for the agent.
func formatInjectOutput(messages []*mail.Message) string {
	var urgent, high, normal []*mail.Message
	for _, msg := range messages {
		switch msg.Priority {
		case mail.PriorityUrgent:
			urgent = append(urgent, msg)
		case mail.PriorityHigh:
			high = append(high, msg)
		default:
			normal = append(normal, msg)
		}
	}

	var b strings.Builder

	if len(urgent) > 0 {
		// Urgent mail: interrupt — agent should stop and read.
		b.WriteString("<system-reminder>\n")
		fmt.Fprintf(&b, "URGENT: %d urgent message(s) require immediate attention.\n\n", len(urgent))
		for _, msg := range urgent {
			fmt.Fprintf(&b, "- %s from %s: %s\n", msg.ID, msg.From, msg.Subject)
		}
		// Show high-priority messages separately so their "process before idle"
		// framing is preserved even when urgent messages are present.
		if len(high) > 0 {
			fmt.Fprintf(&b, "\nAlso %d high-priority message(s) — process before going idle:\n", len(high))
			for _, msg := range high {
				fmt.Fprintf(&b, "- %s from %s: %s\n", msg.ID, msg.From, msg.Subject)
			}
		}
		if len(normal) > 0 {
			fmt.Fprintf(&b, "\n(Plus %d additional message(s) — check after current task.)\n", len(normal))
		}
		b.WriteString("\nRun 'gt mail read <id>' to read urgent messages.\n")
		b.WriteString("</system-reminder>\n")
	} else if len(high) > 0 {
		// High-priority mail: don't interrupt, but process promptly at task boundary.
		b.WriteString("<system-reminder>\n")
		fmt.Fprintf(&b, "You have %d high-priority message(s) in your inbox.\n\n", len(high))
		for _, msg := range high {
			fmt.Fprintf(&b, "- %s from %s: %s\n", msg.ID, msg.From, msg.Subject)
		}
		if len(normal) > 0 {
			fmt.Fprintf(&b, "\n(Plus %d additional message(s).)\n", len(normal))
		}
		b.WriteString("\nContinue your current task. When it completes, process these messages\n")
		b.WriteString("before going idle: 'gt mail inbox'\n")
		b.WriteString("</system-reminder>\n")
	} else {
		// Normal/low mail: informational, process at next task boundary.
		b.WriteString("<system-reminder>\n")
		fmt.Fprintf(&b, "You have %d unread message(s) in your inbox.\n\n", len(normal))
		for _, msg := range normal {
			fmt.Fprintf(&b, "- %s from %s: %s\n", msg.ID, msg.From, msg.Subject)
		}
		b.WriteString("\nContinue your current task. When it completes, check these messages\n")
		b.WriteString("before going idle: 'gt mail inbox'\n")
		b.WriteString("</system-reminder>\n")
	}

	return b.String()
}
