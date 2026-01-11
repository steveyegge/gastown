package cmd

import (
	"github.com/spf13/cobra"
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
	Aliases: []string{"show"},
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

var mailMarkReadCmd = &cobra.Command{
	Use:   "mark-read <message-id> [message-id...]",
	Short: "Mark messages as read without archiving",
	Long: `Mark one or more messages as read without removing them from inbox.

This adds a 'read' label to the message, which is reflected in the inbox display.
The message remains in your inbox (unlike archive which closes/removes it).

Use case: You've read a message but want to keep it visible in your inbox
for reference or follow-up.

Examples:
  gt mail mark-read hq-abc123
  gt mail mark-read hq-abc123 hq-def456`,
	Args: cobra.MinimumNArgs(1),
	RunE: runMailMarkRead,
}

var mailMarkUnreadCmd = &cobra.Command{
	Use:   "mark-unread <message-id> [message-id...]",
	Short: "Mark messages as unread",
	Long: `Mark one or more messages as unread.

This removes the 'read' label from the message.

Examples:
  gt mail mark-unread hq-abc123
  gt mail mark-unread hq-abc123 hq-def456`,
	Args: cobra.MinimumNArgs(1),
	RunE: runMailMarkUnread,
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
	mailCmd.AddCommand(mailMarkReadCmd)
	mailCmd.AddCommand(mailMarkUnreadCmd)
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
