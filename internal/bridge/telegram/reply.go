package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

// InboxMessage represents a mail message from the overseer's inbox.
type InboxMessage struct {
	ID       string
	From     string
	Subject  string
	Body     string
	ThreadID string
}

// InboxReader abstracts reading the overseer's mail inbox.
type InboxReader interface {
	UnreadMessages(ctx context.Context) ([]InboxMessage, error)
	MarkRead(ctx context.Context, id string) error
}

// CLIInboxReader reads the overseer inbox via bd list.
//
// The mail system auto-closes and acks messages to "overseer" within seconds,
// so we can't rely on gt mail inbox --unread. Instead we query bd list directly
// for all messages (open or closed) assigned to overseer with the gt:message
// label, and track which IDs we've already forwarded to avoid duplicates.
type CLIInboxReader struct {
	townRoot  string
	forwarded map[string]bool // IDs already forwarded to Telegram
}

// NewCLIInboxReader creates a CLIInboxReader rooted at townRoot.
func NewCLIInboxReader(townRoot string) *CLIInboxReader {
	return &CLIInboxReader{
		townRoot:  townRoot,
		forwarded: make(map[string]bool),
	}
}

// bdIssue is the JSON structure returned by `bd list --json`.
type bdIssue struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Assignee    string   `json:"assignee"`
	Labels      []string `json:"labels"`
}

// UnreadMessages queries bd list for messages assigned to overseer that
// haven't been forwarded yet. Catches both open and recently-closed messages.
func (r *CLIInboxReader) UnreadMessages(ctx context.Context) ([]InboxMessage, error) {
	cmd := exec.CommandContext(ctx, "bd", "list",
		"--assignee", "overseer",
		"--label", "gt:message",
		"--include-infra", // messages are infra beads
		"--json",
		"--no-pager",
	)
	cmd.Dir = r.townRoot
	cmd.Env = append(os.Environ(), "BD_ACTOR=overseer")

	out, err := cmd.Output()
	if err != nil {
		// bd list returns exit 0 even with no results, but just in case
		return nil, fmt.Errorf("bd list: %w", err)
	}
	if len(out) == 0 {
		return nil, nil
	}

	var issues []bdIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("bd list: parse JSON: %w", err)
	}

	var msgs []InboxMessage
	for _, iss := range issues {
		// Skip already-forwarded messages
		if r.forwarded[iss.ID] {
			continue
		}

		// Extract from and thread from labels
		from, threadID := "", ""
		for _, l := range iss.Labels {
			if strings.HasPrefix(l, "from:") {
				from = l[5:]
			}
			if strings.HasPrefix(l, "thread:") {
				threadID = l[7:]
			}
		}

		// Skip messages FROM overseer (those are our outbound, not replies to us)
		if from == "overseer" {
			r.forwarded[iss.ID] = true
			continue
		}

		msgs = append(msgs, InboxMessage{
			ID:       iss.ID,
			From:     from,
			Subject:  iss.Title,
			Body:     iss.Description,
			ThreadID: threadID,
		})
	}

	// Cap forwarded map to prevent unbounded growth
	if len(r.forwarded) > 10000 {
		r.forwarded = make(map[string]bool)
	}

	return msgs, nil
}

// SeedForwarded queries all existing messages and marks them as already-forwarded.
// Call this once at startup so that only messages arriving after the bridge starts
// get forwarded to Telegram.
func (r *CLIInboxReader) SeedForwarded(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "bd", "list",
		"--assignee", "overseer",
		"--label", "gt:message",
		"--all",
		"--include-infra",
		"--json",
		"--no-pager",
	)
	cmd.Dir = r.townRoot
	cmd.Env = append(os.Environ(), "BD_ACTOR=overseer")

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("seed forwarded: bd list: %w", err)
	}
	if len(out) == 0 {
		return nil
	}

	var issues []bdIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return fmt.Errorf("seed forwarded: parse JSON: %w", err)
	}

	for _, iss := range issues {
		r.forwarded[iss.ID] = true
	}
	log.Printf("reply forwarder: seeded %d existing messages as already-forwarded", len(issues))
	return nil
}

// MarkRead records the message as forwarded and closes the bead so it won't
// be re-forwarded if the bridge restarts (the in-memory forwarded set is lost
// on restart, so we need durable state via bd close).
func (r *CLIInboxReader) MarkRead(ctx context.Context, id string) error {
	r.forwarded[id] = true

	// Close the bead so it leaves the open query results. Best-effort:
	// if this fails, the in-memory forwarded set still prevents duplicates
	// within the current process lifetime.
	cmd := exec.CommandContext(ctx, "bd", "close", id)
	cmd.Dir = r.townRoot
	cmd.Env = append(os.Environ(), "BD_ACTOR=overseer")
	if out, err := cmd.CombinedOutput(); err != nil {
		// This goes to default logger since CLIInboxReader doesn't have the bridge logger.
		// Best-effort close — the in-memory forwarded set still prevents duplicates.
		log.Printf("reply forwarder: bd close %s: %v: %s", id, err, out)
	} else {
		log.Printf("reply forwarder: closed %s", id)
	}
	return nil
}

// ReplyForwarder polls the overseer's mail inbox and forwards Mayor replies
// to Telegram.
type ReplyForwarder struct {
	bot    BotSender
	inbox  InboxReader
	msgMap *MessageMap
	logger *log.Logger
}

// NewReplyForwarder creates a ReplyForwarder.
func NewReplyForwarder(bot BotSender, inbox InboxReader, msgMap *MessageMap, logger *log.Logger) *ReplyForwarder {
	if logger == nil {
		logger = log.Default()
	}
	return &ReplyForwarder{
		bot:    bot,
		inbox:  inbox,
		msgMap: msgMap,
		logger: logger,
	}
}

// Run polls the inbox every 3 seconds, forwarding new messages to Telegram.
// It blocks until ctx is cancelled.
func (r *ReplyForwarder) Run(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.PollOnce(ctx)
		}
	}
}

// PollOnce performs a single poll cycle: reads unread messages and forwards
// each to Telegram. Forward-first ordering ensures failed sends are retried
// on the next cycle (the message stays unread until Telegram delivery succeeds).
func (r *ReplyForwarder) PollOnce(ctx context.Context) {
	msgs, err := r.inbox.UnreadMessages(ctx)
	if err != nil {
		r.logger.Printf("reply forwarder: UnreadMessages: %v", err)
		return
	}

	if len(msgs) > 0 {
		r.logger.Printf("reply forwarder: found %d message(s) to forward", len(msgs))
	}

	for _, msg := range msgs {
		r.logger.Printf("reply forwarder: forwarding %s from=%s subject=%q", msg.ID, msg.From, msg.Subject)
		text := fmt.Sprintf("@%s: %s", msg.From, msg.Body)

		// Look up reply threading via the message map.
		var replyTo *int
		if msg.ThreadID != "" {
			if _, msgID, ok := r.msgMap.TelegramID(msg.ThreadID); ok {
				id := msgID
				replyTo = &id
			}
		}

		// Forward to Telegram FIRST. If this fails, leave the message unread
		// so it will be retried on the next cycle.
		sentID, err := r.bot.SendMessage(text, replyTo)
		if err != nil {
			r.logger.Printf("reply forwarder: SendMessage: %v (will retry)", err)
			continue
		}
		r.logger.Printf("reply forwarder: sent %s to Telegram (msgID=%d), closing bead", msg.ID, sentID)

		// Store the outbound Telegram message ID for future threading.
		// We don't have a chatID here, so we use 0 as a placeholder.
		// The mapping is keyed by threadID so lookup still works.
		if msg.ThreadID != "" {
			r.msgMap.Store(0, sentID, msg.ThreadID)
		}

		// Mark read only after successful Telegram delivery.
		if err := r.inbox.MarkRead(ctx, msg.ID); err != nil {
			r.logger.Printf("reply forwarder: MarkRead %s: %v", msg.ID, err)
		}
	}
}
