package telegram

import (
	"context"
	"log"
)

// InboundMessage is a normalized representation of a message received from Telegram.
type InboundMessage struct {
	ChatID       int64
	MessageID    int64
	Text         string
	Username     string
	UserID       int64
	ReplyToMsgID *int
}

// InboundRelay converts incoming Telegram messages into gt mail and nudges.
type InboundRelay struct {
	sender Sender
	msgMap *MessageMap
	target string // mail recipient, e.g. "mayor/"
}

// NewInboundRelay creates an InboundRelay that delivers mail to target and
// records thread context in msgMap.
func NewInboundRelay(sender Sender, msgMap *MessageMap, target string) *InboundRelay {
	return &InboundRelay{
		sender: sender,
		msgMap: msgMap,
		target: target,
	}
}

// Relay processes an inbound Telegram message: it sends mail to the configured
// target and nudges the mayor session. Empty messages are silently skipped.
// A nudge failure is non-fatal; it is logged but does not return an error.
func (r *InboundRelay) Relay(ctx context.Context, msg InboundMessage) error {
	if msg.Text == "" {
		return nil
	}

	// If this message is a reply, look up the existing thread context for future use.
	if msg.ReplyToMsgID != nil {
		if threadID, ok := r.msgMap.ThreadID(msg.ChatID, *msg.ReplyToMsgID); ok {
			_ = threadID // thread context reserved for future reply-chaining
		}
	}

	if err := r.sender.SendMail(ctx, r.target, "Telegram", msg.Text); err != nil {
		return err
	}

	if err := r.sender.Nudge(ctx, "hq-mayor", "New Telegram message from overseer"); err != nil {
		log.Printf("telegram inbound: nudge failed (non-fatal): %v", err)
	}

	return nil
}
