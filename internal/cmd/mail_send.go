package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

func runMailSend(cmd *cobra.Command, args []string) error {
	var to string

	if mailSendSelf {
		// Auto-detect identity from cwd
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}
		townRoot, err := workspace.FindFromCwd()
		if err != nil || townRoot == "" {
			return fmt.Errorf("not in a Gas Town workspace")
		}
		roleInfo, err := GetRoleWithContext(cwd, townRoot)
		if err != nil {
			return fmt.Errorf("detecting role: %w", err)
		}
		ctx := RoleContext{
			Role:     roleInfo.Role,
			Rig:      roleInfo.Rig,
			Polecat:  roleInfo.Polecat,
			TownRoot: townRoot,
			WorkDir:  cwd,
		}
		to = buildAgentIdentity(ctx)
		if to == "" {
			return fmt.Errorf("cannot determine identity (role: %s)", ctx.Role)
		}
	} else if len(args) > 0 {
		to = args[0]
	} else {
		return fmt.Errorf("address required (or use --self)")
	}

	// Expand role shortcuts to addresses (consistent with gt nudge)
	to = expandMailShortcut(to)

	// All mail uses town beads (two-level architecture)
	workDir, err := findMailWorkDir()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Determine sender
	from := detectSender()

	// Create message
	msg := &mail.Message{
		From:    from,
		To:      to,
		Subject: mailSubject,
		Body:    mailBody,
	}

	// Set priority (--urgent overrides --priority)
	if mailUrgent {
		msg.Priority = mail.PriorityUrgent
	} else {
		msg.Priority = mail.PriorityFromInt(mailPriority)
	}
	if mailNotify && msg.Priority == mail.PriorityNormal {
		msg.Priority = mail.PriorityHigh
	}

	// Set message type
	msg.Type = mail.ParseMessageType(mailType)

	// Set pinned flag
	msg.Pinned = mailPinned

	// Set wisp flag (ephemeral message) - default true, --permanent overrides
	msg.Wisp = mailWisp && !mailPermanent

	// Set CC recipients
	msg.CC = mailCC

	// Handle reply-to: auto-set type to reply and look up thread
	if mailReplyTo != "" {
		msg.ReplyTo = mailReplyTo
		if msg.Type == mail.TypeNotification {
			msg.Type = mail.TypeReply
		}

		// Look up original message to get thread ID
		router := mail.NewRouter(workDir)
		mailbox, err := router.GetMailbox(from)
		if err == nil {
			if original, err := mailbox.Get(mailReplyTo); err == nil {
				msg.ThreadID = original.ThreadID
			}
		}
	}

	// Generate thread ID for new threads
	if msg.ThreadID == "" {
		msg.ThreadID = generateThreadID()
	}

	// Use address resolver for new address types
	townRoot, _ := workspace.FindFromCwd()
	b := beads.New(townRoot)
	resolver := mail.NewResolver(b, townRoot)

	recipients, err := resolver.Resolve(to)
	if err != nil {
		// Fall back to legacy routing if resolver fails
		router := mail.NewRouter(workDir)
		if err := router.Send(msg); err != nil {
			return fmt.Errorf("sending message: %w", err)
		}
		_ = events.LogFeed(events.TypeMail, from, events.MailPayload(to, mailSubject))
		fmt.Printf("%s Message sent to %s\n", style.Bold.Render("✓"), to)
		fmt.Printf("  Subject: %s\n", mailSubject)
		return nil
	}

	// Route based on recipient type
	router := mail.NewRouter(workDir)
	var recipientAddrs []string

	for _, rec := range recipients {
		switch rec.Type {
		case mail.RecipientQueue:
			// Queue messages: single message, workers claim
			msg.To = rec.Address
			if err := router.Send(msg); err != nil {
				return fmt.Errorf("sending to queue: %w", err)
			}
			recipientAddrs = append(recipientAddrs, rec.Address)

		case mail.RecipientChannel:
			// Channel messages: single message, broadcast
			msg.To = rec.Address
			if err := router.Send(msg); err != nil {
				return fmt.Errorf("sending to channel: %w", err)
			}
			recipientAddrs = append(recipientAddrs, rec.Address)

		default:
			// Direct/agent messages: fan out to each recipient
			msgCopy := *msg
			msgCopy.To = rec.Address
			if err := router.Send(&msgCopy); err != nil {
				return fmt.Errorf("sending to %s: %w", rec.Address, err)
			}
			recipientAddrs = append(recipientAddrs, rec.Address)
		}
	}

	// Log mail event to activity feed
	_ = events.LogFeed(events.TypeMail, from, events.MailPayload(to, mailSubject))

	fmt.Printf("%s Message sent to %s\n", style.Bold.Render("✓"), to)
	fmt.Printf("  Subject: %s\n", mailSubject)

	// Show resolved recipients if fan-out occurred
	if len(recipientAddrs) > 1 || (len(recipientAddrs) == 1 && recipientAddrs[0] != to) {
		fmt.Printf("  Recipients: %s\n", strings.Join(recipientAddrs, ", "))
	}

	if len(msg.CC) > 0 {
		fmt.Printf("  CC: %s\n", strings.Join(msg.CC, ", "))
	}
	if msg.Type != mail.TypeNotification {
		fmt.Printf("  Type: %s\n", msg.Type)
	}

	return nil
}

// generateThreadID creates a random thread ID for new message threads.
func generateThreadID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b) // crypto/rand.Read only fails on broken system
	return "thread-" + hex.EncodeToString(b)
}

// expandMailShortcut expands role shortcuts to full mail addresses.
// This provides consistency with gt nudge which accepts the same shortcuts.
func expandMailShortcut(address string) string {
	switch address {
	case "mayor":
		return "mayor/"
	case "witness", "refinery":
		// These need the current rig context
		roleInfo, err := GetRole()
		if err != nil || roleInfo.Rig == "" {
			return address // Can't expand without rig context
		}
		return fmt.Sprintf("%s/%s", roleInfo.Rig, address)
	case "deacon":
		// Deacon doesn't have a mail inbox, return as-is to get proper error
		return address
	default:
		return address
	}
}
