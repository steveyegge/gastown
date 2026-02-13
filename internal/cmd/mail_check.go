package cmd

import (
	"encoding/json"
	"fmt"
	"os"

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

			// Separate by priority: urgent interrupts, high is actionable, rest is informational.
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

			if len(urgent) > 0 {
				// Urgent mail: interrupt — agent should stop and read
				fmt.Println("<system-reminder>")
				fmt.Printf("URGENT: %d urgent message(s) require immediate attention.\n\n", len(urgent))
				for _, msg := range urgent {
					fmt.Printf("- %s from %s: %s\n", msg.ID, msg.From, msg.Subject)
				}
				other := len(high) + len(normal)
				if other > 0 {
					fmt.Printf("\n(Plus %d non-urgent message(s) — read after current task.)\n", other)
				}
				fmt.Println()
				fmt.Println("Run 'gt mail read <id>' to read urgent messages.")
				fmt.Println("</system-reminder>")
			} else if len(high) > 0 {
				// High-priority mail: don't interrupt, but process promptly at task boundary.
				fmt.Println("<system-reminder>")
				fmt.Printf("You have %d high-priority message(s) in your inbox.\n\n", len(high))
				for _, msg := range high {
					fmt.Printf("- %s from %s: %s\n", msg.ID, msg.From, msg.Subject)
				}
				if len(normal) > 0 {
					fmt.Printf("\n(Plus %d normal-priority message(s).)\n", len(normal))
				}
				fmt.Println()
				fmt.Println("Continue your current task. When it completes, process these messages")
				fmt.Println("before going idle: 'gt mail inbox'")
				fmt.Println("</system-reminder>")
			} else {
				// Normal/low mail: informational, process at next task boundary.
				fmt.Println("<system-reminder>")
				fmt.Printf("You have %d unread message(s) in your inbox.\n\n", len(normal))
				for _, msg := range normal {
					fmt.Printf("- %s from %s: %s\n", msg.ID, msg.From, msg.Subject)
				}
				fmt.Println()
				fmt.Println("Continue your current task. When it completes, check these messages")
				fmt.Println("before going idle: 'gt mail inbox'")
				fmt.Println("</system-reminder>")
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
