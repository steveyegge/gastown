package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/style"
)

// getMailbox returns the mailbox for the given address.
func getMailbox(address string) (*mail.Mailbox, error) {
	// All mail uses town beads (two-level architecture)
	workDir, err := findMailWorkDir()
	if err != nil {
		return nil, fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Get mailbox
	router := mail.NewRouter(workDir)
	mailbox, err := router.GetMailbox(address)
	if err != nil {
		return nil, fmt.Errorf("getting mailbox: %w", err)
	}
	return mailbox, nil
}

func runMailInbox(cmd *cobra.Command, args []string) error {
	// Determine which inbox to check (priority: --identity flag, positional arg, auto-detect)
	address := ""
	if mailInboxIdentity != "" {
		address = mailInboxIdentity
	} else if len(args) > 0 {
		address = args[0]
	} else {
		address = detectSender()
	}

	mailbox, err := getMailbox(address)
	if err != nil {
		return err
	}

	// Get messages
	var messages []*mail.Message
	if mailInboxUnread {
		messages, err = mailbox.ListUnread()
	} else {
		messages, err = mailbox.List()
	}
	if err != nil {
		return fmt.Errorf("listing messages: %w", err)
	}

	// JSON output
	if mailInboxJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(messages)
	}

	// Human-readable output
	total, unread, _ := mailbox.Count()
	fmt.Printf("%s Inbox: %s (%d messages, %d unread)\n\n",
		style.Bold.Render("📬"), address, total, unread)

	if len(messages) == 0 {
		fmt.Printf("  %s\n", style.Dim.Render("(no messages)"))
		return nil
	}

	for _, msg := range messages {
		readMarker := "●"
		if msg.Read {
			readMarker = "○"
		}
		typeMarker := ""
		if msg.Type != "" && msg.Type != mail.TypeNotification {
			typeMarker = fmt.Sprintf(" [%s]", msg.Type)
		}
		priorityMarker := ""
		if msg.Priority == mail.PriorityHigh || msg.Priority == mail.PriorityUrgent {
			priorityMarker = " " + style.Bold.Render("!")
		}
		wispMarker := ""
		if msg.Wisp {
			wispMarker = " " + style.Dim.Render("(wisp)")
		}

		fmt.Printf("  %s %s%s%s%s\n", readMarker, msg.Subject, typeMarker, priorityMarker, wispMarker)
		fmt.Printf("    %s from %s\n",
			style.Dim.Render(msg.ID),
			msg.From)
		fmt.Printf("    %s\n",
			style.Dim.Render(msg.Timestamp.Format("2006-01-02 15:04")))
	}

	return nil
}

func runMailRead(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return errors.New("msgID argument required")
	}
	msgID := args[0]

	// Determine which inbox
	address := detectSender()

	mailbox, err := getMailbox(address)
	if err != nil {
		return err
	}

	msg, err := mailbox.Get(msgID)
	if err != nil {
		return fmt.Errorf("getting message: %w", err)
	}

	// Note: We intentionally do NOT mark as read/ack on read.
	// User must explicitly delete/ack the message.
	// This preserves handoff messages for reference.

	// JSON output
	if mailReadJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(msg)
	}

	// Human-readable output
	priorityStr := ""
	if msg.Priority == mail.PriorityUrgent {
		priorityStr = " " + style.Bold.Render("[URGENT]")
	} else if msg.Priority == mail.PriorityHigh {
		priorityStr = " " + style.Bold.Render("[HIGH PRIORITY]")
	}

	typeStr := ""
	if msg.Type != "" && msg.Type != mail.TypeNotification {
		typeStr = fmt.Sprintf(" [%s]", msg.Type)
	}

	fmt.Printf("%s %s%s%s\n\n", style.Bold.Render("Subject:"), msg.Subject, typeStr, priorityStr)
	fmt.Printf("From: %s\n", msg.From)
	fmt.Printf("To: %s\n", msg.To)
	fmt.Printf("Date: %s\n", msg.Timestamp.Format("2006-01-02 15:04:05"))
	fmt.Printf("ID: %s\n", style.Dim.Render(msg.ID))

	if msg.ThreadID != "" {
		fmt.Printf("Thread: %s\n", style.Dim.Render(msg.ThreadID))
	}
	if msg.ReplyTo != "" {
		fmt.Printf("Reply-To: %s\n", style.Dim.Render(msg.ReplyTo))
	}

	if msg.Body != "" {
		fmt.Printf("\n%s\n", msg.Body)
	}

	return nil
}

func runMailPeek(cmd *cobra.Command, args []string) error {
	// Determine which inbox
	address := detectSender()

	mailbox, err := getMailbox(address)
	if err != nil {
		return NewSilentExit(1) // Silent exit - can't access mailbox
	}

	// Get unread messages
	messages, err := mailbox.ListUnread()
	if err != nil || len(messages) == 0 {
		return NewSilentExit(1) // Silent exit - no unread
	}

	// Show first unread message
	msg := messages[0]

	// Header with priority indicator
	priorityStr := ""
	if msg.Priority == mail.PriorityUrgent {
		priorityStr = " [URGENT]"
	} else if msg.Priority == mail.PriorityHigh {
		priorityStr = " [!]"
	}

	fmt.Printf("📬 %s%s\n", msg.Subject, priorityStr)
	fmt.Printf("From: %s\n", msg.From)
	fmt.Printf("ID: %s\n\n", msg.ID)

	// Body preview (truncate long bodies)
	if msg.Body != "" {
		body := msg.Body
		// Truncate to ~500 chars for popup display
		if len(body) > 500 {
			body = body[:500] + "\n..."
		}
		fmt.Print(body)
		if !strings.HasSuffix(body, "\n") {
			fmt.Println()
		}
	}

	// Show count if more messages
	if len(messages) > 1 {
		fmt.Printf("\n%s\n", style.Dim.Render(fmt.Sprintf("(+%d more unread)", len(messages)-1)))
	}

	return nil
}

func runMailDelete(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return errors.New("msgID argument required")
	}
	msgID := args[0]

	// Determine which inbox
	address := detectSender()

	mailbox, err := getMailbox(address)
	if err != nil {
		return err
	}

	if err := mailbox.Delete(msgID); err != nil {
		return fmt.Errorf("deleting message: %w", err)
	}

	fmt.Printf("%s Message deleted\n", style.Bold.Render("✓"))
	return nil
}

func runMailArchive(cmd *cobra.Command, args []string) error {
	// Determine which inbox
	address := detectSender()

	mailbox, err := getMailbox(address)
	if err != nil {
		return err
	}

	// Archive all specified messages
	archived := 0
	var errors []string
	for _, msgID := range args {
		if err := mailbox.Delete(msgID); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", msgID, err))
		} else {
			archived++
		}
	}

	// Report results
	if len(errors) > 0 {
		fmt.Printf("%s Archived %d/%d messages\n",
			style.Bold.Render("⚠"), archived, len(args))
		for _, e := range errors {
			fmt.Printf("  Error: %s\n", e)
		}
		return fmt.Errorf("failed to archive %d messages", len(errors))
	}

	if len(args) == 1 {
		fmt.Printf("%s Message archived\n", style.Bold.Render("✓"))
	} else {
		fmt.Printf("%s Archived %d messages\n", style.Bold.Render("✓"), archived)
	}
	return nil
}

func runMailMarkRead(cmd *cobra.Command, args []string) error {
	// Determine which inbox
	address := detectSender()

	mailbox, err := getMailbox(address)
	if err != nil {
		return err
	}

	// Mark all specified messages as read
	marked := 0
	var errors []string
	for _, msgID := range args {
		if err := mailbox.MarkReadOnly(msgID); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", msgID, err))
		} else {
			marked++
		}
	}

	// Report results
	if len(errors) > 0 {
		fmt.Printf("%s Marked %d/%d messages as read\n",
			style.Bold.Render("⚠"), marked, len(args))
		for _, e := range errors {
			fmt.Printf("  Error: %s\n", e)
		}
		return fmt.Errorf("failed to mark %d messages", len(errors))
	}

	if len(args) == 1 {
		fmt.Printf("%s Message marked as read\n", style.Bold.Render("✓"))
	} else {
		fmt.Printf("%s Marked %d messages as read\n", style.Bold.Render("✓"), marked)
	}
	return nil
}

func runMailMarkUnread(cmd *cobra.Command, args []string) error {
	// Determine which inbox
	address := detectSender()

	mailbox, err := getMailbox(address)
	if err != nil {
		return err
	}

	// Mark all specified messages as unread
	marked := 0
	var errors []string
	for _, msgID := range args {
		if err := mailbox.MarkUnreadOnly(msgID); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", msgID, err))
		} else {
			marked++
		}
	}

	// Report results
	if len(errors) > 0 {
		fmt.Printf("%s Marked %d/%d messages as unread\n",
			style.Bold.Render("⚠"), marked, len(args))
		for _, e := range errors {
			fmt.Printf("  Error: %s\n", e)
		}
		return fmt.Errorf("failed to mark %d messages", len(errors))
	}

	if len(args) == 1 {
		fmt.Printf("%s Message marked as unread\n", style.Bold.Render("✓"))
	} else {
		fmt.Printf("%s Marked %d messages as unread\n", style.Bold.Render("✓"), marked)
	}
	return nil
}

func runMailClear(cmd *cobra.Command, args []string) error {
	// Determine which inbox to clear (target arg or auto-detect)
	address := ""
	if len(args) > 0 {
		address = args[0]
	} else {
		address = detectSender()
	}

	mailbox, err := getMailbox(address)
	if err != nil {
		return err
	}

	// List all messages
	messages, err := mailbox.List()
	if err != nil {
		return fmt.Errorf("listing messages: %w", err)
	}

	if len(messages) == 0 {
		fmt.Printf("%s Inbox %s is already empty\n", style.Dim.Render("○"), address)
		return nil
	}

	// Delete each message
	deleted := 0
	var errors []string
	for _, msg := range messages {
		if err := mailbox.Delete(msg.ID); err != nil {
			// If file is already gone (race condition), ignore it and count as success
			if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file") {
				continue
			}
			errors = append(errors, fmt.Sprintf("%s: %v", msg.ID, err))
		} else {
			deleted++
		}
	}

	// Report results
	if len(errors) > 0 {
		fmt.Printf("%s Cleared %d/%d messages from %s\n",
			style.Bold.Render("⚠"), deleted, len(messages), address)
		for _, e := range errors {
			fmt.Printf("  Error: %s\n", e)
		}
		return fmt.Errorf("failed to clear %d messages", len(errors))
	}

	fmt.Printf("%s Cleared %d messages from %s\n",
		style.Bold.Render("✓"), deleted, address)
	return nil
}
