package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/nudge"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
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
			// Ack after output so message is delivered before being marked acked.
			if ackErr := mailbox.AcknowledgeDeliveries(address, messages); ackErr != nil {
				fmt.Fprintf(os.Stderr, "gt mail check: delivery ack update failed for %s: %v\n", address, ackErr)
			}
		}

		// Also drain queued nudges (from --mode=queue or --mode=wait-idle fallback).
		// The nudge queue is per-session; detect our session name.
		sessionName := tmux.CurrentSessionName()
		if sessionName != "" {
			queuedNudges, drainErr := nudge.Drain(workDir, sessionName)
			if drainErr != nil {
				fmt.Fprintf(os.Stderr, "gt mail check: nudge queue drain error: %v\n", drainErr)
			} else if len(queuedNudges) > 0 {
				fmt.Print(nudge.FormatForInjection(queuedNudges))
			}
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
		b.WriteString("<system-reminder>\n")
		fmt.Fprintf(&b, "URGENT: %d message(s) — read NOW with `gt mail read <id>`\n", len(urgent))
		for _, msg := range urgent {
			fmt.Fprintf(&b, "- %s from %s: %s\n", msg.ID, msg.From, msg.Subject)
		}
		if len(high) > 0 {
			fmt.Fprintf(&b, "\n+%d high-priority (process before idle):\n", len(high))
			for _, msg := range high {
				fmt.Fprintf(&b, "- %s from %s: %s\n", msg.ID, msg.From, msg.Subject)
			}
		}
		if len(normal) > 0 {
			fmt.Fprintf(&b, "\n+%d more in inbox\n", len(normal))
		}
		b.WriteString("</system-reminder>\n")
	} else if len(high) > 0 {
		b.WriteString("<system-reminder>\n")
		fmt.Fprintf(&b, "%d high-priority message(s) — process at task boundary:\n", len(high))
		for _, msg := range high {
			fmt.Fprintf(&b, "- %s from %s: %s\n", msg.ID, msg.From, msg.Subject)
		}
		if len(normal) > 0 {
			fmt.Fprintf(&b, "+%d more in inbox\n", len(normal))
		}
		b.WriteString("</system-reminder>\n")
	} else {
		b.WriteString("<system-reminder>\n")
		fmt.Fprintf(&b, "%d unread message(s) — check before idle:\n", len(normal))
		for _, msg := range normal {
			fmt.Fprintf(&b, "- %s from %s: %s\n", msg.ID, msg.From, msg.Subject)
		}
		b.WriteString("</system-reminder>\n")
	}

	return b.String()
}
