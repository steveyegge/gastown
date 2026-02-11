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
	// Urgent mail interrupts (agent should act now). Normal mail is delivered
	// as background context that does NOT interrupt the current task.
	if mailCheckInject {
		if unread > 0 {
			messages, listErr := mailbox.ListUnread()
			if listErr != nil {
				fmt.Fprintf(os.Stderr, "gt mail check: could not list unread for %s: %v\n", address, listErr)
				return nil
			}

			// Separate urgent from non-urgent
			var urgent, normal []*mail.Message
			for _, msg := range messages {
				if msg.Priority == mail.PriorityUrgent {
					urgent = append(urgent, msg)
				} else {
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
				if len(normal) > 0 {
					fmt.Printf("\n(Plus %d non-urgent message(s) — read after current task.)\n", len(normal))
				}
				fmt.Println()
				fmt.Println("Run 'gt mail read <id>' to read urgent messages.")
				fmt.Println("</system-reminder>")
			} else {
				// Non-urgent mail only: deliver as background notification.
				// Explicitly tell the agent NOT to interrupt current work.
				fmt.Println("<system-reminder>")
				fmt.Printf("You have %d unread message(s) in your inbox.\n\n", len(normal))
				for _, msg := range normal {
					fmt.Printf("- %s from %s: %s\n", msg.ID, msg.From, msg.Subject)
				}
				fmt.Println()
				fmt.Println("This is a background notification. Do NOT stop or interrupt your current task.")
				fmt.Println("Read these messages when your current work is complete: 'gt mail inbox'")
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
