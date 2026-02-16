package cmd

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/nudge"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// mailNudgeIdleTimeout is the maximum time to wait for a recipient's session to
// become idle before falling back to a queued nudge. Kept as a var (not const)
// so tests can shorten it.
var mailNudgeIdleTimeout = 1 * time.Second

func runMailSend(cmd *cobra.Command, args []string) error {
	// Handle --stdin: read message body from stdin (avoids shell quoting issues)
	if mailStdin {
		if mailBody != "" {
			return fmt.Errorf("cannot use --stdin with --message/-m")
		}
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		mailBody = strings.TrimRight(string(data), "\n")
	}

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

	// All mail uses town beads (two-level architecture)
	workDir, err := findMailWorkDir()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Determine sender
	from := detectSender()

	// Create message with auto-generated ID and thread ID
	msg := mail.NewMessage(from, to, mailSubject, mailBody)

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

	// Always suppress the router's built-in notification — the CLI owns
	// the notification decision via --no-notify / --notify flags.
	msg.SuppressNotify = true

	// Handle reply-to: auto-set type to reply and look up thread
	if mailReplyTo != "" {
		msg.ReplyTo = mailReplyTo
		if msg.Type == mail.TypeNotification {
			msg.Type = mail.TypeReply
		}

		// Look up original message in current user's mailbox to get thread ID.
		// The message we're replying to lives in our inbox (we received it),
		// so we look it up via our own identity (from), not the recipient (to).
		router := mail.NewRouter(workDir)
		mailbox, err := router.GetMailbox(from)
		if err != nil {
			style.PrintWarning("could not open mailbox for thread lookup: %v", err)
		} else {
			original, err := mailbox.Get(mailReplyTo)
			if err != nil {
				style.PrintWarning("could not find original message %s for threading (new thread will be created)", mailReplyTo)
			} else {
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

		// CLI-side notification for legacy path
		if !mailNoNotify {
			notifyRecipients(townRoot, from, mailSubject, []string{to}, router)
		}
		return nil
	}

	// Route based on recipient type, collecting errors instead of failing early
	router := mail.NewRouter(workDir)
	var recipientAddrs []string
	var sendErrs []string

	for _, rec := range recipients {
		switch rec.Type {
		case mail.RecipientQueue:
			// Queue messages: single message, workers claim
			msg.To = rec.Address
			if err := router.Send(msg); err != nil {
				sendErrs = append(sendErrs, fmt.Sprintf("queue %s: %v", rec.Address, err))
				continue
			}
			recipientAddrs = append(recipientAddrs, rec.Address)

		case mail.RecipientChannel:
			// Channel messages: single message, broadcast
			msg.To = rec.Address
			if err := router.Send(msg); err != nil {
				sendErrs = append(sendErrs, fmt.Sprintf("channel %s: %v", rec.Address, err))
				continue
			}
			recipientAddrs = append(recipientAddrs, rec.Address)

		default:
			// Direct/agent messages: fan out to each recipient
			msgCopy := *msg
			msgCopy.To = rec.Address
			msgCopy.ID = "" // Each fan-out copy gets its own unique ID
			if err := router.Send(&msgCopy); err != nil {
				sendErrs = append(sendErrs, fmt.Sprintf("%s: %v", rec.Address, err))
				continue
			}
			recipientAddrs = append(recipientAddrs, rec.Address)
		}
	}

	if len(sendErrs) > 0 {
		if len(recipientAddrs) == 0 {
			return fmt.Errorf("all sends failed: %s", strings.Join(sendErrs, "; "))
		}
		fmt.Fprintf(os.Stderr, "⚠ Some deliveries failed: %s\n", strings.Join(sendErrs, "; "))
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

	// CLI-side notification: idle → immediate nudge, busy → queued nudge
	if !mailNoNotify {
		notifyRecipients(townRoot, from, mailSubject, recipientAddrs, router)
	}

	return nil
}

// notifyRecipients sends notifications to each recipient address.
// For idle sessions it sends an immediate nudge via tmux; for busy sessions
// it enqueues a nudge for cooperative delivery at the next turn boundary.
func notifyRecipients(townRoot, from, subject string, addrs []string, router *mail.Router) {
	t := tmux.NewTmux()
	fromIdentity := mail.AddressToIdentity(from)
	notification := fmt.Sprintf(
		"📬 You have new mail from %s. Subject: %s. Run 'gt mail inbox' to read.",
		from, subject,
	)

	for _, addr := range addrs {
		// Skip self-mail
		if mail.AddressToIdentity(addr) == fromIdentity {
			continue
		}

		// Skip DND/muted recipients
		if router.IsRecipientMuted(addr) {
			continue
		}

		sessionIDs := mail.AddressToSessionIDs(addr)
		if len(sessionIDs) == 0 {
			continue
		}

		notified := false
		isOverseer := mail.AddressToIdentity(addr) == "overseer"

		for _, sessionID := range sessionIDs {
			hasSession, err := t.HasSession(sessionID)
			if err != nil || !hasSession {
				continue
			}

			// Overseer is a human operator — use a visible banner instead of
			// NudgeSession, which would type into the human's terminal input.
			if isOverseer {
				if err := t.SendNotificationBanner(sessionID, from, subject); err == nil {
					fmt.Printf("  ↳ Notified overseer %s\n", addr)
					notified = true
				}
				break
			}

			// Session exists — check if idle
			if err := t.WaitForIdle(sessionID, mailNudgeIdleTimeout); err == nil {
				// Idle → send immediate nudge
				if err := t.NudgeSession(sessionID, notification); err == nil {
					fmt.Printf("  ↳ Nudged idle recipient %s\n", addr)
					notified = true
					break
				}
			}

			// Busy or nudge failed → enqueue for cooperative delivery
			if townRoot != "" {
				if err := nudge.Enqueue(townRoot, sessionID, nudge.QueuedNudge{
					Sender:  from,
					Message: notification,
				}); err != nil {
					style.PrintWarning("failed to enqueue notification for %s: %v", addr, err)
				} else {
					fmt.Printf("  ↳ Queued notification for %s\n", addr)
				}
				notified = true
				break
			}
		}

		// If no session found at all, skip silently (agent is offline)
		_ = notified
	}
}

// generateThreadID creates a random thread ID for new message threads.
func generateThreadID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b) // crypto/rand.Read only fails on broken system
	return "thread-" + hex.EncodeToString(b)
}
