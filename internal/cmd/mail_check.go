package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/inject"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/runtime"
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
			// Inject mode: always exit 0, silent on error
			return nil
		}
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Get mailbox
	router := mail.NewRouter(workDir)
	mailbox, err := router.GetMailbox(address)
	if err != nil {
		if mailCheckInject {
			return nil
		}
		return fmt.Errorf("getting mailbox: %w", err)
	}

	// Count unread
	_, unread, err := mailbox.Count()
	if err != nil {
		if mailCheckInject {
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

	// Inject mode: queue system-reminder if mail exists
	if mailCheckInject {
		if unread > 0 {
			// Get subjects for context
			messages, _ := mailbox.ListUnread()
			var subjects []string
			for _, msg := range messages {
				subjects = append(subjects, fmt.Sprintf("- %s from %s: %s", msg.ID, msg.From, msg.Subject))
			}

			// Build the system-reminder content
			var buf bytes.Buffer
			buf.WriteString("<system-reminder>\n")
			buf.WriteString(fmt.Sprintf("You have %d unread message(s) in your inbox.\n\n", unread))
			for _, s := range subjects {
				buf.WriteString(s + "\n")
			}
			buf.WriteString("\n")
			buf.WriteString("Run 'gt mail inbox' to see your messages, or 'gt mail read <id>' for a specific message.\n")
			buf.WriteString("</system-reminder>\n")

			// Check if we should queue or output directly
			sessionID := runtime.SessionIDFromEnv()
			if sessionID != "" {
				// Session ID available - use queue
				queue := inject.NewQueue(workDir, sessionID)
				if err := queue.Enqueue(inject.TypeMail, buf.String()); err != nil {
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
	if unread > 0 {
		fmt.Printf("%s %d unread message(s)\n", style.Bold.Render("📬"), unread)
		return NewSilentExit(0)
	}
	fmt.Println("No new mail")
	return NewSilentExit(1)
}
