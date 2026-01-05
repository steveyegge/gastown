package cmd

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Mail command flags
var (
	mailSubject       string
	mailBody          string
	mailPriority      int
	mailUrgent        bool
	mailPinned        bool
	mailWisp          bool
	mailPermanent     bool
	mailType          string
	mailReplyTo       string
	mailNotify        bool
	mailSendSelf      bool
	mailCC            []string // CC recipients
	mailInboxJSON     bool
	mailReadJSON      bool
	mailInboxUnread   bool
	mailInboxIdentity string
	mailCheckInject   bool
	mailCheckJSON     bool
	mailCheckIdentity string
	mailThreadJSON    bool
	mailReplySubject  string
	mailReplyMessage  string

	// Search flags
	mailSearchFrom    string
	mailSearchSubject bool
	mailSearchBody    bool
	mailSearchArchive bool
	mailSearchJSON    bool

	// Announces flags
	mailAnnouncesJSON bool

	// Clear flags
	mailClearAll bool
)

var mailCmd = &cobra.Command{
	Use:     "mail",
	GroupID: GroupComm,
	Short:   "Agent messaging system",
	RunE:    requireSubcommand,
	Long: `Send and receive messages between agents.

The mail system allows Mayor, polecats, and the Refinery to communicate.
Messages are stored in beads as issues with type=message.

MAIL ROUTING:
  ┌─────────────────────────────────────────────────────┐
  │                    Town (.beads/)                   │
  │  ┌─────────────────────────────────────────────┐   │
  │  │                 Mayor Inbox                 │   │
  │  │  └── mayor/                                 │   │
  │  └─────────────────────────────────────────────┘   │
  │                                                     │
  │  ┌─────────────────────────────────────────────┐   │
  │  │           gastown/ (rig mailboxes)          │   │
  │  │  ├── witness      ← greenplace/witness         │   │
  │  │  ├── refinery     ← greenplace/refinery        │   │
  │  │  ├── Toast        ← greenplace/Toast           │   │
  │  │  └── crew/max     ← greenplace/crew/max        │   │
  │  └─────────────────────────────────────────────┘   │
  └─────────────────────────────────────────────────────┘

ADDRESS FORMATS:
  mayor/              → Mayor inbox
  <rig>/witness       → Rig's Witness
  <rig>/refinery      → Rig's Refinery
  <rig>/<polecat>     → Polecat (e.g., greenplace/Toast)
  <rig>/crew/<name>   → Crew worker (e.g., greenplace/crew/max)
  --human             → Special: human overseer

COMMANDS:
  inbox     View your inbox
  send      Send a message
  read      Read a specific message
  mark      Mark messages read/unread`,
}

var mailSendCmd = &cobra.Command{
	Use:   "send <address>",
	Short: "Send a message",
	Long: `Send a message to an agent.

Addresses:
  mayor/           - Send to Mayor
  <rig>/refinery   - Send to a rig's Refinery
  <rig>/<polecat>  - Send to a specific polecat
  <rig>/           - Broadcast to a rig
  list:<name>      - Send to a mailing list (fans out to all members)

Mailing lists are defined in ~/gt/config/messaging.json and allow
sending to multiple recipients at once. Each recipient gets their
own copy of the message.

Message types:
  task          - Required processing
  scavenge      - Optional first-come work
  notification  - Informational (default)
  reply         - Response to message

Priority levels:
  0 - urgent/critical
  1 - high
  2 - normal (default)
  3 - low
  4 - backlog

Use --urgent as shortcut for --priority 0.

Examples:
  gt mail send greenplace/Toast -s "Status check" -m "How's that bug fix going?"
  gt mail send mayor/ -s "Work complete" -m "Finished gt-abc"
  gt mail send gastown/ -s "All hands" -m "Swarm starting" --notify
  gt mail send greenplace/Toast -s "Task" -m "Fix bug" --type task --priority 1
  gt mail send greenplace/Toast -s "Urgent" -m "Help!" --urgent
  gt mail send mayor/ -s "Re: Status" -m "Done" --reply-to msg-abc123
  gt mail send --self -s "Handoff" -m "Context for next session"
  gt mail send greenplace/Toast -s "Update" -m "Progress report" --cc overseer
  gt mail send list:oncall -s "Alert" -m "System down"`,
	Args: cobra.MaximumNArgs(1),
	RunE: runMailSend,
}

var mailInboxCmd = &cobra.Command{
	Use:   "inbox [address]",
	Short: "Check inbox",
	Long: `Check messages in an inbox.

If no address is specified, shows the current context's inbox.
Use --identity for polecats to explicitly specify their identity.

Examples:
  gt mail inbox                       # Current context (auto-detected)
  gt mail inbox mayor/                # Mayor's inbox
  gt mail inbox greenplace/Toast         # Polecat's inbox
  gt mail inbox --identity greenplace/Toast  # Explicit polecat identity`,
	Args: cobra.MaximumNArgs(1),
	RunE: runMailInbox,
}

var mailReadCmd = &cobra.Command{
	Use:   "read <message-id>",
	Short: "Read a message",
	Long: `Read a specific message and mark it as read.

The message ID can be found from 'gt mail inbox'.`,
	Args: cobra.ExactArgs(1),
	RunE: runMailRead,
}

var mailPeekCmd = &cobra.Command{
	Use:   "peek",
	Short: "Show preview of first unread message",
	Long: `Display a compact preview of the first unread message.

Useful for status bar popups - shows subject, sender, and body preview.
Exits silently with code 1 if no unread messages.`,
	RunE: runMailPeek,
}

var mailDeleteCmd = &cobra.Command{
	Use:   "delete <message-id>",
	Short: "Delete a message",
	Long: `Delete (acknowledge) a message.

This closes the message in beads.`,
	Args: cobra.ExactArgs(1),
	RunE: runMailDelete,
}

var mailArchiveCmd = &cobra.Command{
	Use:   "archive <message-id> [message-id...]",
	Short: "Archive messages",
	Long: `Archive one or more messages.

Removes the messages from your inbox by closing them in beads.

Examples:
  gt mail archive hq-abc123
  gt mail archive hq-abc123 hq-def456 hq-ghi789`,
	Args: cobra.MinimumNArgs(1),
	RunE: runMailArchive,
}

var mailCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for new mail (for hooks)",
	Long: `Check for new mail - useful for Claude Code hooks.

Exit codes (normal mode):
  0 - New mail available
  1 - No new mail

Exit codes (--inject mode):
  0 - Always (hooks should never block)
  Output: system-reminder if mail exists, silent if no mail

Use --identity for polecats to explicitly specify their identity.

Examples:
  gt mail check                           # Simple check (auto-detect identity)
  gt mail check --inject                  # For hooks
  gt mail check --identity greenplace/Toast  # Explicit polecat identity`,
	RunE: runMailCheck,
}

var mailThreadCmd = &cobra.Command{
	Use:   "thread <thread-id>",
	Short: "View a message thread",
	Long: `View all messages in a conversation thread.

Shows messages in chronological order (oldest first).

Examples:
  gt mail thread thread-abc123`,
	Args: cobra.ExactArgs(1),
	RunE: runMailThread,
}

var mailReplyCmd = &cobra.Command{
	Use:   "reply <message-id>",
	Short: "Reply to a message",
	Long: `Reply to a specific message.

This is a convenience command that automatically:
- Sets the reply-to field to the original message
- Prefixes the subject with "Re: " (if not already present)
- Sends to the original sender

Examples:
  gt mail reply msg-abc123 -m "Thanks, working on it now"
  gt mail reply msg-abc123 -s "Custom subject" -m "Reply body"`,
	Args: cobra.ExactArgs(1),
	RunE: runMailReply,
}

var mailClaimCmd = &cobra.Command{
	Use:   "claim <queue-name>",
	Short: "Claim a message from a queue",
	Long: `Claim the oldest unclaimed message from a work queue.

SYNTAX:
  gt mail claim <queue-name>

BEHAVIOR:
1. List unclaimed messages in the queue
2. Pick the oldest unclaimed message
3. Set assignee to caller identity
4. Set status to in_progress
5. Print claimed message details

ELIGIBILITY:
The caller must match a pattern in the queue's workers list
(defined in ~/gt/config/messaging.json).

Examples:
  gt mail claim work/gastown    # Claim from gastown work queue`,
	Args: cobra.ExactArgs(1),
	RunE: runMailClaim,
}

var mailReleaseCmd = &cobra.Command{
	Use:   "release <message-id>",
	Short: "Release a claimed queue message",
	Long: `Release a previously claimed message back to its queue.

SYNTAX:
  gt mail release <message-id>

BEHAVIOR:
1. Find the message by ID
2. Verify caller is the one who claimed it (assignee matches)
3. Set assignee back to queue:<name> (from message labels)
4. Set status back to open
5. Message returns to queue for others to claim

ERROR CASES:
- Message not found
- Message not claimed (still assigned to queue)
- Caller did not claim this message

Examples:
  gt mail release hq-abc123    # Release a claimed message`,
	Args: cobra.ExactArgs(1),
	RunE: runMailRelease,
}

var mailClearCmd = &cobra.Command{
	Use:   "clear [target]",
	Short: "Clear all messages from an inbox",
	Long: `Clear (delete) all messages from an inbox.

SYNTAX:
  gt mail clear              # Clear your own inbox
  gt mail clear <target>     # Clear another agent's inbox

BEHAVIOR:
1. List all messages in the target inbox
2. Delete each message
3. Print count of deleted messages

Use case: Town quiescence - reset all inboxes across workers efficiently.

Examples:
  gt mail clear                      # Clear your inbox
  gt mail clear gastown/polecats/joe # Clear joe's inbox
  gt mail clear mayor/               # Clear mayor's inbox`,
	Args: cobra.MaximumNArgs(1),
	RunE: runMailClear,
}

var mailSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search messages by content",
	Long: `Search inbox for messages matching a pattern.

SYNTAX:
  gt mail search <query> [flags]

The query is a regular expression pattern. Search is case-insensitive by default.

FLAGS:
  --from <sender>   Filter by sender address (substring match)
  --subject         Only search subject lines
  --body            Only search message body
  --archive         Include archived (closed) messages
  --json            Output as JSON

By default, searches both subject and body text.

Examples:
  gt mail search "urgent"                    # Find messages with "urgent"
  gt mail search "status.*check" --subject   # Regex in subjects only
  gt mail search "error" --from witness      # From witness, containing "error"
  gt mail search "handoff" --archive         # Include archived messages
  gt mail search "" --from mayor/            # All messages from mayor`,
	Args: cobra.ExactArgs(1),
	RunE: runMailSearch,
}

var mailAnnouncesCmd = &cobra.Command{
	Use:   "announces [channel]",
	Short: "List or read announce channels",
	Long: `List available announce channels or read messages from a channel.

SYNTAX:
  gt mail announces              # List all announce channels
  gt mail announces <channel>    # Read messages from a channel

Announce channels are bulletin boards defined in ~/gt/config/messaging.json.
Messages are broadcast to readers and persist until retention limit is reached.
Unlike regular mail, announce messages are NOT removed when read.

BEHAVIOR for 'gt mail announces':
- Loads messaging.json
- Lists all announce channel names
- Shows reader patterns and retain_count for each

BEHAVIOR for 'gt mail announces <channel>':
- Validates channel exists
- Queries beads for messages with announce_channel=<channel>
- Displays in reverse chronological order (newest first)
- Does NOT mark as read or remove messages

Examples:
  gt mail announces              # List all channels
  gt mail announces alerts       # Read messages from 'alerts' channel
  gt mail announces --json       # List channels as JSON`,
	Args: cobra.MaximumNArgs(1),
	RunE: runMailAnnounces,
}

func init() {
	// Send flags
	mailSendCmd.Flags().StringVarP(&mailSubject, "subject", "s", "", "Message subject (required)")
	mailSendCmd.Flags().StringVarP(&mailBody, "message", "m", "", "Message body")
	mailSendCmd.Flags().IntVar(&mailPriority, "priority", 2, "Message priority (0=urgent, 1=high, 2=normal, 3=low, 4=backlog)")
	mailSendCmd.Flags().BoolVar(&mailUrgent, "urgent", false, "Set priority=0 (urgent)")
	mailSendCmd.Flags().StringVar(&mailType, "type", "notification", "Message type (task, scavenge, notification, reply)")
	mailSendCmd.Flags().StringVar(&mailReplyTo, "reply-to", "", "Message ID this is replying to")
	mailSendCmd.Flags().BoolVarP(&mailNotify, "notify", "n", false, "Send tmux notification to recipient")
	mailSendCmd.Flags().BoolVar(&mailPinned, "pinned", false, "Pin message (for handoff context that persists)")
	mailSendCmd.Flags().BoolVar(&mailWisp, "wisp", true, "Send as wisp (ephemeral, default)")
	mailSendCmd.Flags().BoolVar(&mailPermanent, "permanent", false, "Send as permanent (not ephemeral, synced to remote)")
	mailSendCmd.Flags().BoolVar(&mailSendSelf, "self", false, "Send to self (auto-detect from cwd)")
	mailSendCmd.Flags().StringArrayVar(&mailCC, "cc", nil, "CC recipients (can be used multiple times)")
	_ = mailSendCmd.MarkFlagRequired("subject") // cobra flags: error only at runtime if missing

	// Inbox flags
	mailInboxCmd.Flags().BoolVar(&mailInboxJSON, "json", false, "Output as JSON")
	mailInboxCmd.Flags().BoolVarP(&mailInboxUnread, "unread", "u", false, "Show only unread messages")
	mailInboxCmd.Flags().StringVar(&mailInboxIdentity, "identity", "", "Explicit identity for inbox (e.g., greenplace/Toast)")
	mailInboxCmd.Flags().StringVar(&mailInboxIdentity, "address", "", "Alias for --identity")

	// Read flags
	mailReadCmd.Flags().BoolVar(&mailReadJSON, "json", false, "Output as JSON")

	// Check flags
	mailCheckCmd.Flags().BoolVar(&mailCheckInject, "inject", false, "Output format for Claude Code hooks")
	mailCheckCmd.Flags().BoolVar(&mailCheckJSON, "json", false, "Output as JSON")
	mailCheckCmd.Flags().StringVar(&mailCheckIdentity, "identity", "", "Explicit identity for inbox (e.g., greenplace/Toast)")
	mailCheckCmd.Flags().StringVar(&mailCheckIdentity, "address", "", "Alias for --identity")

	// Thread flags
	mailThreadCmd.Flags().BoolVar(&mailThreadJSON, "json", false, "Output as JSON")

	// Reply flags
	mailReplyCmd.Flags().StringVarP(&mailReplySubject, "subject", "s", "", "Override reply subject (default: Re: <original>)")
	mailReplyCmd.Flags().StringVarP(&mailReplyMessage, "message", "m", "", "Reply message body (required)")
	_ = mailReplyCmd.MarkFlagRequired("message")

	// Search flags
	mailSearchCmd.Flags().StringVar(&mailSearchFrom, "from", "", "Filter by sender address")
	mailSearchCmd.Flags().BoolVar(&mailSearchSubject, "subject", false, "Only search subject lines")
	mailSearchCmd.Flags().BoolVar(&mailSearchBody, "body", false, "Only search message body")
	mailSearchCmd.Flags().BoolVar(&mailSearchArchive, "archive", false, "Include archived messages")
	mailSearchCmd.Flags().BoolVar(&mailSearchJSON, "json", false, "Output as JSON")

	// Announces flags
	mailAnnouncesCmd.Flags().BoolVar(&mailAnnouncesJSON, "json", false, "Output as JSON")

	// Clear flags
	mailClearCmd.Flags().BoolVar(&mailClearAll, "all", false, "Clear all messages (default behavior)")

	// Add subcommands
	mailCmd.AddCommand(mailSendCmd)
	mailCmd.AddCommand(mailInboxCmd)
	mailCmd.AddCommand(mailReadCmd)
	mailCmd.AddCommand(mailPeekCmd)
	mailCmd.AddCommand(mailDeleteCmd)
	mailCmd.AddCommand(mailArchiveCmd)
	mailCmd.AddCommand(mailCheckCmd)
	mailCmd.AddCommand(mailThreadCmd)
	mailCmd.AddCommand(mailReplyCmd)
	mailCmd.AddCommand(mailClaimCmd)
	mailCmd.AddCommand(mailReleaseCmd)
	mailCmd.AddCommand(mailClearCmd)
	mailCmd.AddCommand(mailSearchCmd)
	mailCmd.AddCommand(mailAnnouncesCmd)

	rootCmd.AddCommand(mailCmd)
}

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

	// Send via router
	router := mail.NewRouter(workDir)

	// Check if this is a list address to show fan-out details
	var listRecipients []string
	if strings.HasPrefix(to, "list:") {
		var err error
		listRecipients, err = router.ExpandListAddress(to)
		if err != nil {
			return fmt.Errorf("sending message: %w", err)
		}
	}

	if err := router.Send(msg); err != nil {
		return fmt.Errorf("sending message: %w", err)
	}

	// Log mail event to activity feed
	_ = events.LogFeed(events.TypeMail, from, events.MailPayload(to, mailSubject))

	fmt.Printf("%s Message sent to %s\n", style.Bold.Render("✓"), to)
	fmt.Printf("  Subject: %s\n", mailSubject)

	// Show fan-out recipients for list addresses
	if len(listRecipients) > 0 {
		fmt.Printf("  Recipients: %s\n", strings.Join(listRecipients, ", "))
	}

	if len(msg.CC) > 0 {
		fmt.Printf("  CC: %s\n", strings.Join(msg.CC, ", "))
	}
	if msg.Type != mail.TypeNotification {
		fmt.Printf("  Type: %s\n", msg.Type)
	}

	return nil
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

	// All mail uses town beads (two-level architecture)
	workDir, err := findMailWorkDir()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Get mailbox
	router := mail.NewRouter(workDir)
	mailbox, err := router.GetMailbox(address)
	if err != nil {
		return fmt.Errorf("getting mailbox: %w", err)
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
	msgID := args[0]

	// Determine which inbox
	address := detectSender()

	// All mail uses town beads (two-level architecture)
	workDir, err := findMailWorkDir()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Get mailbox and message
	router := mail.NewRouter(workDir)
	mailbox, err := router.GetMailbox(address)
	if err != nil {
		return fmt.Errorf("getting mailbox: %w", err)
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

	// All mail uses town beads (two-level architecture)
	workDir, err := findMailWorkDir()
	if err != nil {
		return NewSilentExit(1) // Silent exit - no workspace
	}

	// Get mailbox
	router := mail.NewRouter(workDir)
	mailbox, err := router.GetMailbox(address)
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
	msgID := args[0]

	// Determine which inbox
	address := detectSender()

	// All mail uses town beads (two-level architecture)
	workDir, err := findMailWorkDir()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Get mailbox
	router := mail.NewRouter(workDir)
	mailbox, err := router.GetMailbox(address)
	if err != nil {
		return fmt.Errorf("getting mailbox: %w", err)
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

	// All mail uses town beads (two-level architecture)
	workDir, err := findMailWorkDir()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Get mailbox
	router := mail.NewRouter(workDir)
	mailbox, err := router.GetMailbox(address)
	if err != nil {
		return fmt.Errorf("getting mailbox: %w", err)
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

func runMailClear(cmd *cobra.Command, args []string) error {
	// Determine which inbox to clear (target arg or auto-detect)
	address := ""
	if len(args) > 0 {
		address = args[0]
	} else {
		address = detectSender()
	}

	// All mail uses town beads (two-level architecture)
	workDir, err := findMailWorkDir()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Get mailbox
	router := mail.NewRouter(workDir)
	mailbox, err := router.GetMailbox(address)
	if err != nil {
		return fmt.Errorf("getting mailbox: %w", err)
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

// findMailWorkDir returns the town root for all mail operations.
//
// Two-level beads architecture:
// - Town beads (~/gt/.beads/): ALL mail and coordination
// - Clone beads (<rig>/crew/*/.beads/): Project issues only
//
// Mail ALWAYS uses town beads, regardless of sender or recipient address.
// This ensures messages are visible to all agents in the town.
func findMailWorkDir() (string, error) {
	return workspace.FindFromCwdOrError()
}

// findLocalBeadsDir finds the nearest .beads directory by walking up from CWD.
// Used for project work (molecules, issue creation) that uses clone beads.
//
// Priority:
//  1. BEADS_DIR environment variable (set by session manager for polecats)
//  2. Walk up from CWD looking for .beads directory
//
// Polecats use redirect-based beads access, so their worktree doesn't have a full
// .beads directory. The session manager sets BEADS_DIR to the correct location.
func findLocalBeadsDir() (string, error) {
	// Check BEADS_DIR environment variable first (set by session manager for polecats).
	// This is important for polecats that use redirect-based beads access.
	if beadsDir := os.Getenv("BEADS_DIR"); beadsDir != "" {
		// BEADS_DIR points directly to the .beads directory, return its parent
		if _, err := os.Stat(beadsDir); err == nil {
			return filepath.Dir(beadsDir), nil
		}
	}

	// Fallback: walk up from CWD
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	path := cwd
	for {
		if _, err := os.Stat(filepath.Join(path, ".beads")); err == nil {
			return path, nil
		}

		parent := filepath.Dir(path)
		if parent == path {
			break // Reached root
		}
		path = parent
	}

	return "", fmt.Errorf("no .beads directory found")
}

// detectSender determines the current context's address.
// Priority:
//  1. GT_ROLE env var → use the role-based identity (agent session)
//  2. No GT_ROLE → try cwd-based detection (witness/refinery/polecat/crew directories)
//  3. No match → return "overseer" (human at terminal)
//
// All Gas Town agents run in tmux sessions with GT_ROLE set at spawn.
// However, cwd-based detection is also tried to support running commands
// from agent directories without GT_ROLE set (e.g., debugging sessions).
func detectSender() string {
	// Check GT_ROLE first (authoritative for agent sessions)
	role := os.Getenv("GT_ROLE")
	if role != "" {
		// Agent session - build address from role and context
		return detectSenderFromRole(role)
	}

	// No GT_ROLE - try cwd-based detection, defaults to overseer if not in agent directory
	return detectSenderFromCwd()
}

// detectSenderFromRole builds an address from the GT_ROLE and related env vars.
// GT_ROLE can be either a simple role name ("crew", "polecat") or a full address
// ("greenplace/crew/joe") depending on how the session was started.
//
// If GT_ROLE is a simple name but required env vars (GT_RIG, GT_POLECAT, etc.)
// are missing, falls back to cwd-based detection. This could return "overseer"
// if cwd doesn't match any known agent path - a misconfigured agent session.
func detectSenderFromRole(role string) string {
	rig := os.Getenv("GT_RIG")

	// Check if role is already a full address (contains /)
	if strings.Contains(role, "/") {
		// GT_ROLE is already a full address, use it directly
		return role
	}

	// GT_ROLE is a simple role name, build the full address
	switch role {
	case "mayor":
		return "mayor/"
	case "deacon":
		return "deacon/"
	case "polecat":
		polecat := os.Getenv("GT_POLECAT")
		if rig != "" && polecat != "" {
			return fmt.Sprintf("%s/%s", rig, polecat)
		}
		// Fallback to cwd detection for polecats
		return detectSenderFromCwd()
	case "crew":
		crew := os.Getenv("GT_CREW")
		if rig != "" && crew != "" {
			return fmt.Sprintf("%s/crew/%s", rig, crew)
		}
		// Fallback to cwd detection for crew
		return detectSenderFromCwd()
	case "witness":
		if rig != "" {
			return fmt.Sprintf("%s/witness", rig)
		}
		return detectSenderFromCwd()
	case "refinery":
		if rig != "" {
			return fmt.Sprintf("%s/refinery", rig)
		}
		return detectSenderFromCwd()
	default:
		// Unknown role, try cwd detection
		return detectSenderFromCwd()
	}
}

// detectSenderFromCwd is the legacy cwd-based detection for edge cases.
func detectSenderFromCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "overseer"
	}

	// If in a rig's polecats directory, extract address (format: rig/polecats/name)
	if strings.Contains(cwd, "/polecats/") {
		parts := strings.Split(cwd, "/polecats/")
		if len(parts) >= 2 {
			rigPath := parts[0]
			polecatPath := strings.Split(parts[1], "/")[0]
			rigName := filepath.Base(rigPath)
			return fmt.Sprintf("%s/polecats/%s", rigName, polecatPath)
		}
	}

	// If in a rig's crew directory, extract address (format: rig/crew/name)
	if strings.Contains(cwd, "/crew/") {
		parts := strings.Split(cwd, "/crew/")
		if len(parts) >= 2 {
			rigPath := parts[0]
			crewName := strings.Split(parts[1], "/")[0]
			rigName := filepath.Base(rigPath)
			return fmt.Sprintf("%s/crew/%s", rigName, crewName)
		}
	}

	// If in a rig's refinery directory, extract address (format: rig/refinery)
	if strings.Contains(cwd, "/refinery") {
		parts := strings.Split(cwd, "/refinery")
		if len(parts) >= 1 {
			rigName := filepath.Base(parts[0])
			return fmt.Sprintf("%s/refinery", rigName)
		}
	}

	// If in a rig's witness directory, extract address (format: rig/witness)
	if strings.Contains(cwd, "/witness") {
		parts := strings.Split(cwd, "/witness")
		if len(parts) >= 1 {
			rigName := filepath.Base(parts[0])
			return fmt.Sprintf("%s/witness", rigName)
		}
	}

	// Default to overseer (human)
	return "overseer"
}

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

	// Inject mode: output system-reminder if mail exists
	if mailCheckInject {
		if unread > 0 {
			// Get subjects for context
			messages, _ := mailbox.ListUnread()
			var subjects []string
			for _, msg := range messages {
				subjects = append(subjects, fmt.Sprintf("- %s from %s: %s", msg.ID, msg.From, msg.Subject))
			}

			fmt.Println("<system-reminder>")
			fmt.Printf("You have %d unread message(s) in your inbox.\n\n", unread)
			for _, s := range subjects {
				fmt.Println(s)
			}
			fmt.Println()
			fmt.Println("Run 'gt mail inbox' to see your messages, or 'gt mail read <id>' for a specific message.")
			fmt.Println("</system-reminder>")
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

func runMailThread(cmd *cobra.Command, args []string) error {
	threadID := args[0]

	// All mail uses town beads (two-level architecture)
	workDir, err := findMailWorkDir()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Determine which inbox
	address := detectSender()

	// Get mailbox and thread messages
	router := mail.NewRouter(workDir)
	mailbox, err := router.GetMailbox(address)
	if err != nil {
		return fmt.Errorf("getting mailbox: %w", err)
	}

	messages, err := mailbox.ListByThread(threadID)
	if err != nil {
		return fmt.Errorf("getting thread: %w", err)
	}

	// JSON output
	if mailThreadJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(messages)
	}

	// Human-readable output
	fmt.Printf("%s Thread: %s (%d messages)\n\n",
		style.Bold.Render("🧵"), threadID, len(messages))

	if len(messages) == 0 {
		fmt.Printf("  %s\n", style.Dim.Render("(no messages in thread)"))
		return nil
	}

	for i, msg := range messages {
		typeMarker := ""
		if msg.Type != "" && msg.Type != mail.TypeNotification {
			typeMarker = fmt.Sprintf(" [%s]", msg.Type)
		}
		priorityMarker := ""
		if msg.Priority == mail.PriorityHigh || msg.Priority == mail.PriorityUrgent {
			priorityMarker = " " + style.Bold.Render("!")
		}

		if i > 0 {
			fmt.Printf("  %s\n", style.Dim.Render("│"))
		}
		fmt.Printf("  %s %s%s%s\n", style.Bold.Render("●"), msg.Subject, typeMarker, priorityMarker)
		fmt.Printf("    %s from %s to %s\n",
			style.Dim.Render(msg.ID),
			msg.From, msg.To)
		fmt.Printf("    %s\n",
			style.Dim.Render(msg.Timestamp.Format("2006-01-02 15:04")))

		if msg.Body != "" {
			fmt.Printf("    %s\n", msg.Body)
		}
	}

	return nil
}

func runMailReply(cmd *cobra.Command, args []string) error {
	msgID := args[0]

	// All mail uses town beads (two-level architecture)
	workDir, err := findMailWorkDir()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Determine current address
	from := detectSender()

	// Get the original message
	router := mail.NewRouter(workDir)
	mailbox, err := router.GetMailbox(from)
	if err != nil {
		return fmt.Errorf("getting mailbox: %w", err)
	}

	original, err := mailbox.Get(msgID)
	if err != nil {
		return fmt.Errorf("getting message: %w", err)
	}

	// Build reply subject
	subject := mailReplySubject
	if subject == "" {
		if strings.HasPrefix(original.Subject, "Re: ") {
			subject = original.Subject
		} else {
			subject = "Re: " + original.Subject
		}
	}

	// Create reply message
	reply := &mail.Message{
		From:     from,
		To:       original.From, // Reply to sender
		Subject:  subject,
		Body:     mailReplyMessage,
		Type:     mail.TypeReply,
		Priority: mail.PriorityNormal,
		ReplyTo:  msgID,
		ThreadID: original.ThreadID,
	}

	// If original has no thread ID, create one
	if reply.ThreadID == "" {
		reply.ThreadID = generateThreadID()
	}

	// Send the reply
	if err := router.Send(reply); err != nil {
		return fmt.Errorf("sending reply: %w", err)
	}

	fmt.Printf("%s Reply sent to %s\n", style.Bold.Render("✓"), original.From)
	fmt.Printf("  Subject: %s\n", subject)
	if original.ThreadID != "" {
		fmt.Printf("  Thread: %s\n", style.Dim.Render(original.ThreadID))
	}

	return nil
}

// generateThreadID creates a random thread ID for new message threads.
func generateThreadID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b) // crypto/rand.Read only fails on broken system
	return "thread-" + hex.EncodeToString(b)
}

// runMailClaim claims the oldest unclaimed message from a work queue.
func runMailClaim(cmd *cobra.Command, args []string) error {
	queueName := args[0]

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load queue config from messaging.json
	configPath := config.MessagingConfigPath(townRoot)
	cfg, err := config.LoadMessagingConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading messaging config: %w", err)
	}

	queueCfg, ok := cfg.Queues[queueName]
	if !ok {
		return fmt.Errorf("unknown queue: %s", queueName)
	}

	// Get caller identity
	caller := detectSender()

	// Check if caller is eligible (matches any pattern in workers list)
	if !isEligibleWorker(caller, queueCfg.Workers) {
		return fmt.Errorf("not eligible to claim from queue %s (caller: %s, workers: %v)",
			queueName, caller, queueCfg.Workers)
	}

	// List unclaimed messages in the queue
	// Queue messages have assignee=queue:<name> and status=open
	queueAssignee := "queue:" + queueName
	messages, err := listQueueMessages(townRoot, queueAssignee)
	if err != nil {
		return fmt.Errorf("listing queue messages: %w", err)
	}

	if len(messages) == 0 {
		fmt.Printf("%s No messages to claim in queue %s\n", style.Dim.Render("○"), queueName)
		return nil
	}

	// Pick the oldest unclaimed message (first in list, sorted by created)
	oldest := messages[0]

	// Claim the message: set assignee to caller and status to in_progress
	if err := claimMessage(townRoot, oldest.ID, caller); err != nil {
		return fmt.Errorf("claiming message: %w", err)
	}

	// Print claimed message details
	fmt.Printf("%s Claimed message from queue %s\n", style.Bold.Render("✓"), queueName)
	fmt.Printf("  ID: %s\n", oldest.ID)
	fmt.Printf("  Subject: %s\n", oldest.Title)
	if oldest.Description != "" {
		// Show first line of description
		lines := strings.SplitN(oldest.Description, "\n", 2)
		preview := lines[0]
		if len(preview) > 80 {
			preview = preview[:77] + "..."
		}
		fmt.Printf("  Preview: %s\n", style.Dim.Render(preview))
	}
	fmt.Printf("  From: %s\n", oldest.From)
	fmt.Printf("  Created: %s\n", oldest.Created.Format("2006-01-02 15:04"))

	return nil
}

// queueMessage represents a message in a queue.
type queueMessage struct {
	ID          string
	Title       string
	Description string
	From        string
	Created     time.Time
	Priority    int
}

// isEligibleWorker checks if the caller matches any pattern in the workers list.
// Patterns support wildcards: "gastown/polecats/*" matches "gastown/polecats/capable".
func isEligibleWorker(caller string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchWorkerPattern(pattern, caller) {
			return true
		}
	}
	return false
}

// matchWorkerPattern checks if caller matches the pattern.
// Supports simple wildcards: * matches a single path segment (no slashes).
func matchWorkerPattern(pattern, caller string) bool {
	// Handle exact match
	if pattern == caller {
		return true
	}

	// Handle wildcard patterns
	if strings.Contains(pattern, "*") {
		// Convert to simple glob matching
		// "gastown/polecats/*" should match "gastown/polecats/capable"
		// but NOT "gastown/polecats/sub/capable"
		parts := strings.Split(pattern, "*")
		if len(parts) == 2 {
			prefix := parts[0]
			suffix := parts[1]
			if strings.HasPrefix(caller, prefix) && strings.HasSuffix(caller, suffix) {
				// Check that the middle part doesn't contain path separators
				middle := caller[len(prefix) : len(caller)-len(suffix)]
				if !strings.Contains(middle, "/") {
					return true
				}
			}
		}
	}

	return false
}

// listQueueMessages lists unclaimed messages in a queue.
func listQueueMessages(townRoot, queueAssignee string) ([]queueMessage, error) {
	// Use bd list to find messages with assignee=queue:<name> and status=open
	beadsDir := filepath.Join(townRoot, ".beads")

	args := []string{"list",
		"--assignee", queueAssignee,
		"--status", "open",
		"--type", "message",
		"--sort", "created",
		"--limit", "0", // No limit
		"--json",
	}

	cmd := exec.Command("bd", args...)
	cmd.Env = append(os.Environ(), "BEADS_DIR="+beadsDir)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("%s", errMsg)
		}
		return nil, err
	}

	// Parse JSON output
	var issues []struct {
		ID          string    `json:"id"`
		Title       string    `json:"title"`
		Description string    `json:"description"`
		Labels      []string  `json:"labels"`
		CreatedAt   time.Time `json:"created_at"`
		Priority    int       `json:"priority"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		// If no messages, bd might output empty or error
		if strings.TrimSpace(stdout.String()) == "" || strings.TrimSpace(stdout.String()) == "[]" {
			return nil, nil
		}
		return nil, fmt.Errorf("parsing bd output: %w", err)
	}

	// Convert to queueMessage, extracting 'from' from labels
	var messages []queueMessage
	for _, issue := range issues {
		msg := queueMessage{
			ID:          issue.ID,
			Title:       issue.Title,
			Description: issue.Description,
			Created:     issue.CreatedAt,
			Priority:    issue.Priority,
		}

		// Extract 'from' from labels (format: "from:address")
		for _, label := range issue.Labels {
			if strings.HasPrefix(label, "from:") {
				msg.From = strings.TrimPrefix(label, "from:")
				break
			}
		}

		messages = append(messages, msg)
	}

	// Sort by created time (oldest first)
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Created.Before(messages[j].Created)
	})

	return messages, nil
}

// claimMessage claims a message by setting assignee and status.
func claimMessage(townRoot, messageID, claimant string) error {
	beadsDir := filepath.Join(townRoot, ".beads")

	args := []string{"update", messageID,
		"--assignee", claimant,
		"--status", "in_progress",
	}

	cmd := exec.Command("bd", args...)
	cmd.Env = append(os.Environ(),
		"BEADS_DIR="+beadsDir,
		"BD_ACTOR="+claimant,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return fmt.Errorf("%s", errMsg)
		}
		return err
	}

	return nil
}

// runMailRelease releases a claimed queue message back to its queue.
func runMailRelease(cmd *cobra.Command, args []string) error {
	messageID := args[0]

	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Get caller identity
	caller := detectSender()

	// Get message details to verify ownership and find queue
	msgInfo, err := getMessageInfo(townRoot, messageID)
	if err != nil {
		return fmt.Errorf("getting message: %w", err)
	}

	// Verify message exists and is a queue message
	if msgInfo.QueueName == "" {
		return fmt.Errorf("message %s is not a queue message (no queue label)", messageID)
	}

	// Verify caller is the one who claimed it
	if msgInfo.Assignee != caller {
		if strings.HasPrefix(msgInfo.Assignee, "queue:") {
			return fmt.Errorf("message %s is not claimed (still in queue)", messageID)
		}
		return fmt.Errorf("message %s was claimed by %s, not %s", messageID, msgInfo.Assignee, caller)
	}

	// Release the message: set assignee back to queue and status to open
	queueAssignee := "queue:" + msgInfo.QueueName
	if err := releaseMessage(townRoot, messageID, queueAssignee, caller); err != nil {
		return fmt.Errorf("releasing message: %w", err)
	}

	fmt.Printf("%s Released message back to queue %s\n", style.Bold.Render("✓"), msgInfo.QueueName)
	fmt.Printf("  ID: %s\n", messageID)
	fmt.Printf("  Subject: %s\n", msgInfo.Title)

	return nil
}

// messageInfo holds details about a queue message.
type messageInfo struct {
	ID        string
	Title     string
	Assignee  string
	QueueName string
	Status    string
}

// getMessageInfo retrieves information about a message.
func getMessageInfo(townRoot, messageID string) (*messageInfo, error) {
	beadsDir := filepath.Join(townRoot, ".beads")

	args := []string{"show", messageID, "--json"}

	cmd := exec.Command("bd", args...)
	cmd.Env = append(os.Environ(), "BEADS_DIR="+beadsDir)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if strings.Contains(errMsg, "not found") {
			return nil, fmt.Errorf("message not found: %s", messageID)
		}
		if errMsg != "" {
			return nil, fmt.Errorf("%s", errMsg)
		}
		return nil, err
	}

	// Parse JSON output - bd show --json returns an array
	var issues []struct {
		ID       string   `json:"id"`
		Title    string   `json:"title"`
		Assignee string   `json:"assignee"`
		Labels   []string `json:"labels"`
		Status   string   `json:"status"`
	}

	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		return nil, fmt.Errorf("parsing message: %w", err)
	}

	if len(issues) == 0 {
		return nil, fmt.Errorf("message not found: %s", messageID)
	}

	issue := issues[0]
	info := &messageInfo{
		ID:       issue.ID,
		Title:    issue.Title,
		Assignee: issue.Assignee,
		Status:   issue.Status,
	}

	// Extract queue name from labels (format: "queue:<name>")
	for _, label := range issue.Labels {
		if strings.HasPrefix(label, "queue:") {
			info.QueueName = strings.TrimPrefix(label, "queue:")
			break
		}
	}

	return info, nil
}

// releaseMessage releases a claimed message back to its queue.
func releaseMessage(townRoot, messageID, queueAssignee, actor string) error {
	beadsDir := filepath.Join(townRoot, ".beads")

	args := []string{"update", messageID,
		"--assignee", queueAssignee,
		"--status", "open",
	}

	cmd := exec.Command("bd", args...)
	cmd.Env = append(os.Environ(),
		"BEADS_DIR="+beadsDir,
		"BD_ACTOR="+actor,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return fmt.Errorf("%s", errMsg)
		}
		return err
	}

	return nil
}

// runMailSearch searches for messages matching a pattern.
func runMailSearch(cmd *cobra.Command, args []string) error {
	query := args[0]

	// Determine which inbox to search
	address := detectSender()

	// Get workspace for mail operations
	workDir, err := findMailWorkDir()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Get mailbox
	router := mail.NewRouter(workDir)
	mailbox, err := router.GetMailbox(address)
	if err != nil {
		return fmt.Errorf("getting mailbox: %w", err)
	}

	// Build search options
	opts := mail.SearchOptions{
		Query:       query,
		FromFilter:  mailSearchFrom,
		SubjectOnly: mailSearchSubject,
		BodyOnly:    mailSearchBody,
	}

	// Execute search
	messages, err := mailbox.Search(opts)
	if err != nil {
		return fmt.Errorf("searching messages: %w", err)
	}

	// JSON output
	if mailSearchJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(messages)
	}

	// Human-readable output
	fmt.Printf("%s Search results for %s: %d message(s)\n\n",
		style.Bold.Render("🔍"), address, len(messages))

	if len(messages) == 0 {
		fmt.Printf("  %s\n", style.Dim.Render("(no matches)"))
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

// runMailAnnounces lists announce channels or reads messages from a channel.
func runMailAnnounces(cmd *cobra.Command, args []string) error {
	// Find workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Load messaging config
	configPath := config.MessagingConfigPath(townRoot)
	cfg, err := config.LoadMessagingConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading messaging config: %w", err)
	}

	// If no channel specified, list all channels
	if len(args) == 0 {
		return listAnnounceChannels(cfg)
	}

	// Read messages from specified channel
	channelName := args[0]
	return readAnnounceChannel(townRoot, cfg, channelName)
}

// listAnnounceChannels lists all announce channels and their configuration.
func listAnnounceChannels(cfg *config.MessagingConfig) error {
	if cfg.Announces == nil || len(cfg.Announces) == 0 {
		if mailAnnouncesJSON {
			fmt.Println("[]")
			return nil
		}
		fmt.Printf("%s No announce channels configured\n", style.Dim.Render("○"))
		return nil
	}

	// JSON output
	if mailAnnouncesJSON {
		type channelInfo struct {
			Name        string   `json:"name"`
			Readers     []string `json:"readers"`
			RetainCount int      `json:"retain_count"`
		}
		var channels []channelInfo
		for name, annCfg := range cfg.Announces {
			channels = append(channels, channelInfo{
				Name:        name,
				Readers:     annCfg.Readers,
				RetainCount: annCfg.RetainCount,
			})
		}
		// Sort by name for consistent output
		sort.Slice(channels, func(i, j int) bool {
			return channels[i].Name < channels[j].Name
		})
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(channels)
	}

	// Human-readable output
	fmt.Printf("%s Announce Channels (%d)\n\n", style.Bold.Render("📢"), len(cfg.Announces))

	// Sort channel names for consistent output
	var names []string
	for name := range cfg.Announces {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		annCfg := cfg.Announces[name]
		retainStr := "unlimited"
		if annCfg.RetainCount > 0 {
			retainStr = fmt.Sprintf("%d messages", annCfg.RetainCount)
		}
		fmt.Printf("  %s %s\n", style.Bold.Render("●"), name)
		fmt.Printf("    Readers: %s\n", strings.Join(annCfg.Readers, ", "))
		fmt.Printf("    Retain: %s\n", style.Dim.Render(retainStr))
	}

	return nil
}

// readAnnounceChannel reads messages from an announce channel.
func readAnnounceChannel(townRoot string, cfg *config.MessagingConfig, channelName string) error {
	// Validate channel exists
	if cfg.Announces == nil {
		return fmt.Errorf("no announce channels configured")
	}
	_, ok := cfg.Announces[channelName]
	if !ok {
		return fmt.Errorf("unknown announce channel: %s", channelName)
	}

	// Query beads for messages with announce_channel=<channel>
	messages, err := listAnnounceMessages(townRoot, channelName)
	if err != nil {
		return fmt.Errorf("listing announce messages: %w", err)
	}

	// JSON output
	if mailAnnouncesJSON {
		// Ensure empty array instead of null for JSON
		if messages == nil {
			messages = []announceMessage{}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(messages)
	}

	// Human-readable output
	fmt.Printf("%s Channel: %s (%d messages)\n\n",
		style.Bold.Render("📢"), channelName, len(messages))

	if len(messages) == 0 {
		fmt.Printf("  %s\n", style.Dim.Render("(no messages)"))
		return nil
	}

	for _, msg := range messages {
		priorityMarker := ""
		if msg.Priority <= 1 {
			priorityMarker = " " + style.Bold.Render("!")
		}

		fmt.Printf("  %s %s%s\n", style.Bold.Render("●"), msg.Title, priorityMarker)
		fmt.Printf("    %s from %s\n",
			style.Dim.Render(msg.ID),
			msg.From)
		fmt.Printf("    %s\n",
			style.Dim.Render(msg.Created.Format("2006-01-02 15:04")))
		if msg.Description != "" {
			// Show first line of description as preview
			lines := strings.SplitN(msg.Description, "\n", 2)
			preview := lines[0]
			if len(preview) > 80 {
				preview = preview[:77] + "..."
			}
			fmt.Printf("    %s\n", style.Dim.Render(preview))
		}
	}

	return nil
}

// announceMessage represents a message in an announce channel.
type announceMessage struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	From        string    `json:"from"`
	Created     time.Time `json:"created"`
	Priority    int       `json:"priority"`
}

// listAnnounceMessages lists messages from an announce channel.
func listAnnounceMessages(townRoot, channelName string) ([]announceMessage, error) {
	beadsDir := filepath.Join(townRoot, ".beads")

	// Query for messages with label announce_channel:<channel>
	// Messages are stored with this label when sent via sendToAnnounce()
	args := []string{"list",
		"--type", "message",
		"--label", "announce_channel:" + channelName,
		"--sort", "-created", // Newest first
		"--limit", "0",       // No limit
		"--json",
	}

	cmd := exec.Command("bd", args...)
	cmd.Env = append(os.Environ(), "BEADS_DIR="+beadsDir)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return nil, fmt.Errorf("%s", errMsg)
		}
		return nil, err
	}

	// Parse JSON output
	var issues []struct {
		ID          string    `json:"id"`
		Title       string    `json:"title"`
		Description string    `json:"description"`
		Labels      []string  `json:"labels"`
		CreatedAt   time.Time `json:"created_at"`
		Priority    int       `json:"priority"`
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" || output == "[]" {
		return nil, nil
	}

	if err := json.Unmarshal(stdout.Bytes(), &issues); err != nil {
		return nil, fmt.Errorf("parsing bd output: %w", err)
	}

	// Convert to announceMessage, extracting 'from' from labels
	var messages []announceMessage
	for _, issue := range issues {
		msg := announceMessage{
			ID:          issue.ID,
			Title:       issue.Title,
			Description: issue.Description,
			Created:     issue.CreatedAt,
			Priority:    issue.Priority,
		}

		// Extract 'from' from labels (format: "from:address")
		for _, label := range issue.Labels {
			if strings.HasPrefix(label, "from:") {
				msg.From = strings.TrimPrefix(label, "from:")
				break
			}
		}

		messages = append(messages, msg)
	}

	return messages, nil
}
