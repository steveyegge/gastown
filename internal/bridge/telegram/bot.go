package telegram

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// pollTimeout is the Telegram long-poll timeout in seconds.
const pollTimeout = 30

// httpClientTimeout is the HTTP client timeout for the Telegram API.
// It must be longer than pollTimeout to allow the long-poll to complete,
// but short enough to detect silently-dropped TCP connections quickly
// (e.g., cloud NAT/firewall idle timeouts).
const httpClientTimeout = (pollTimeout + 10) * time.Second

// BotSender is satisfied by any type that can send a Telegram message.
// It is used by OutboundNotifier for testability.
type BotSender interface {
	SendMessage(text string, replyToMsgID *int) (int, error)
}

// Bot wraps the Telegram Bot API for polling and sending messages.
type Bot struct {
	api      *tgbotapi.BotAPI
	cfg      Config
	gate     *AccessGate
	limiter  *RateLimiter
	messages chan InboundMessage
}

// NewBot creates a new Bot, connecting to the Telegram API to verify the token.
// It configures an HTTP client timeout slightly longer than the long-poll timeout
// to detect silently-dropped TCP connections (common on cloud servers with
// NAT/firewall idle connection timeouts).
func NewBot(cfg Config) (*Bot, error) {
	client := &http.Client{Timeout: httpClientTimeout}
	api, err := tgbotapi.NewBotAPIWithClient(cfg.Token, tgbotapi.APIEndpoint, client)
	if err != nil {
		return nil, fmt.Errorf("telegram: connect: %w", err)
	}

	window := time.Minute
	limit := cfg.RateLimit
	if limit <= 0 {
		limit = 30
	}

	return &Bot{
		api:      api,
		cfg:      cfg,
		gate:     NewAccessGate(cfg),
		limiter:  NewRateLimiter(limit, window),
		messages: make(chan InboundMessage, 32),
	}, nil
}

// Messages returns the channel on which inbound messages are delivered.
func (b *Bot) Messages() <-chan InboundMessage {
	return b.messages
}

// Poll long-polls the Telegram Bot API, running the access gate and rate
// limiter before forwarding messages to the Messages() channel. It blocks
// until ctx is cancelled.
func (b *Bot) Poll(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = pollTimeout

	updates := b.api.GetUpdatesChan(u)
	for {
		select {
		case <-ctx.Done():
			b.api.StopReceivingUpdates()
			return
		case update, ok := <-updates:
			if !ok {
				return
			}
			if update.Message == nil {
				continue
			}
			msg := update.Message

			// Access gate: reject bots, then check allow list.
			if !b.gate.Check(msg.From.ID, msg.From.IsBot) {
				continue
			}

			// Rate limiter: per-user sliding window.
			if !b.limiter.Allow(msg.From.ID) {
				continue
			}

			inbound := InboundMessage{
				ChatID:    msg.Chat.ID,
				MessageID: int64(msg.MessageID),
				Text:      msg.Text,
				Username:  msg.From.UserName,
				UserID:    msg.From.ID,
			}
			if msg.ReplyToMessage != nil {
				id := msg.ReplyToMessage.MessageID
				inbound.ReplyToMsgID = &id
			}

			select {
			case b.messages <- inbound:
			case <-ctx.Done():
				b.api.StopReceivingUpdates()
				return
			default:
				log.Printf("telegram: inbound message channel full, dropping message from user %d", msg.From.ID)
			}
		}
	}
}

// SendMessage sends text to the configured chat. If replyToMsgID is non-nil
// the message is sent as a reply. It returns the sent message's Telegram ID.
func (b *Bot) SendMessage(text string, replyToMsgID *int) (int, error) {
	msg := tgbotapi.NewMessage(b.cfg.ChatID, text)
	if replyToMsgID != nil {
		msg.ReplyToMessageID = *replyToMsgID //nolint:staticcheck // tgbotapi v5 field
	}
	sent, err := b.api.Send(msg)
	if err != nil {
		return 0, fmt.Errorf("telegram: send message: %w", err)
	}
	return sent.MessageID, nil
}

// --- AccessGate ---

// AccessGate checks bot/allow_from before processing a message.
type AccessGate struct {
	cfg Config
}

// NewAccessGate creates an AccessGate from the given config.
func NewAccessGate(cfg Config) *AccessGate {
	return &AccessGate{cfg: cfg}
}

// Check returns true if the message from this user should be processed.
// Bots are always blocked. Then cfg.IsAllowed is consulted (fail-closed).
func (g *AccessGate) Check(userID int64, isBot bool) bool {
	if isBot {
		return false
	}
	return g.cfg.IsAllowed(userID)
}

// --- RateLimiter ---

// RateLimiter implements per-user sliding window rate limiting.
type RateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	// timestamps holds the call times for each user within the current window.
	timestamps map[int64][]time.Time
}

// NewRateLimiter creates a RateLimiter with the given per-user limit and window.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		limit:      limit,
		window:     window,
		timestamps: make(map[int64][]time.Time),
	}
}

// Allow returns true if the user is within the rate limit. It prunes expired
// timestamps on each call and records the current call time.
func (r *RateLimiter) Allow(userID int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-r.window)

	// Prune entries outside the sliding window.
	ts := r.timestamps[userID]
	valid := ts[:0]
	for _, t := range ts {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= r.limit {
		r.timestamps[userID] = valid
		return false
	}

	r.timestamps[userID] = append(valid, now)
	return true
}
