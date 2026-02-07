package slackbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/steveyegge/gastown/internal/rpcclient"
)

// DecisionEventPayload is the JSON payload for decision bus events.
// Matches the format defined by od-k3o.15.1.
type DecisionEventPayload struct {
	DecisionID  string `json:"decision_id"`
	Question    string `json:"question"`
	Urgency     string `json:"urgency"`
	RequestedBy string `json:"requested_by"`
	OptionCount int    `json:"option_count"`
	ChosenIndex int    `json:"chosen_index,omitempty"`
	ChosenLabel string `json:"chosen_label,omitempty"`
	ResolvedBy  string `json:"resolved_by,omitempty"`
	Rationale   string `json:"rationale,omitempty"`
}

// BusListenerConfig configures the NATS-based event listener.
type BusListenerConfig struct {
	NatsURL      string   // NATS server URL (e.g., "nats://localhost:4222")
	AuthToken    string   // Authentication token (BD_DAEMON_TOKEN)
	Subjects     []string // NATS subjects to subscribe to (e.g., ["hooks.Decision>", "decisions.>"])
	ConsumerName string   // Durable consumer name for JetStream (default: "slack-bot")
	StreamName   string   // JetStream stream name (default: "HOOK_EVENTS")
}

// BusListener connects to NATS JetStream and forwards decision events to Slack.
// It replaces SSEListener for environments where the bd bus is available.
type BusListener struct {
	cfg       BusListenerConfig
	bot       *Bot
	rpcClient *rpcclient.Client
	seen      map[string]bool
	seenMu    sync.Mutex
	nc        *nats.Conn
	js        nats.JetStreamContext
}

// NewBusListener creates a new NATS-based event listener.
func NewBusListener(cfg BusListenerConfig, bot *Bot, rpcClient *rpcclient.Client) *BusListener {
	if cfg.ConsumerName == "" {
		cfg.ConsumerName = "slack-bot"
	}
	if cfg.StreamName == "" {
		cfg.StreamName = "HOOK_EVENTS"
	}
	if len(cfg.Subjects) == 0 {
		// Default: subscribe to all decision-related events on the hooks stream
		cfg.Subjects = []string{
			"hooks.DecisionCreated",
			"hooks.DecisionResponded",
			"hooks.DecisionEscalated",
			"hooks.DecisionExpired",
		}
	}
	return &BusListener{
		cfg:       cfg,
		bot:       bot,
		rpcClient: rpcClient,
		seen:      make(map[string]bool),
	}
}

// Run connects to NATS and subscribes to decision events.
// Blocks until ctx is canceled. Reconnects automatically with exponential backoff.
func (l *BusListener) Run(ctx context.Context) error {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := l.connectAndConsume(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("BusListener: connection error: %v, reconnecting in %v", err, backoff)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		} else {
			backoff = time.Second
		}
	}
}

func (l *BusListener) connectAndConsume(ctx context.Context) error {
	opts := []nats.Option{
		nats.Name("gt-slackbot"),
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(-1), // Unlimited reconnects
	}
	if l.cfg.AuthToken != "" {
		opts = append(opts, nats.Token(l.cfg.AuthToken))
	}

	nc, err := nats.Connect(l.cfg.NatsURL, opts...)
	if err != nil {
		return fmt.Errorf("connecting to NATS: %w", err)
	}
	defer nc.Close()
	l.nc = nc

	js, err := nc.JetStream()
	if err != nil {
		return fmt.Errorf("getting JetStream context: %w", err)
	}
	l.js = js

	log.Printf("BusListener: connected to NATS at %s", l.cfg.NatsURL)

	// Subscribe to each configured subject via JetStream pull consumer.
	// Use a durable consumer so we can replay missed events on reconnect.
	sub, err := js.PullSubscribe(
		l.cfg.Subjects[0], // Primary subject filter
		l.cfg.ConsumerName,
		nats.AckExplicit(),
		nats.DeliverAll(), // Replay all unacked messages on first connect
	)
	if err != nil {
		// JetStream may not have the stream yet (od-k3o.15 not landed).
		// Fall back to plain NATS subscription for forward compatibility.
		return l.consumePlainNATS(ctx, nc)
	}

	log.Printf("BusListener: subscribed to JetStream stream %s (consumer: %s)", l.cfg.StreamName, l.cfg.ConsumerName)

	// Pull messages in a loop
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msgs, err := sub.Fetch(10, nats.MaxWait(5*time.Second))
		if err != nil {
			if err == nats.ErrTimeout {
				continue // No messages, keep polling
			}
			return fmt.Errorf("fetching messages: %w", err)
		}

		for _, msg := range msgs {
			l.handleMessage(msg.Subject, msg.Data)
			if err := msg.Ack(); err != nil {
				log.Printf("BusListener: error acking message: %v", err)
			}
		}
	}
}

// consumePlainNATS subscribes using plain NATS (no JetStream) as a fallback
// when the JetStream stream doesn't exist yet.
func (l *BusListener) consumePlainNATS(ctx context.Context, nc *nats.Conn) error {
	log.Printf("BusListener: JetStream unavailable, falling back to plain NATS subscriptions")

	msgCh := make(chan *nats.Msg, 100)

	var subs []*nats.Subscription
	for _, subject := range l.cfg.Subjects {
		sub, err := nc.ChanSubscribe(subject, msgCh)
		if err != nil {
			// Clean up already-created subscriptions
			for _, s := range subs {
				_ = s.Unsubscribe()
			}
			return fmt.Errorf("subscribing to %s: %w", subject, err)
		}
		subs = append(subs, sub)
		log.Printf("BusListener: subscribed to %s (plain NATS)", subject)
	}
	defer func() {
		for _, s := range subs {
			_ = s.Unsubscribe()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg := <-msgCh:
			l.handleMessage(msg.Subject, msg.Data)
		}
	}
}

// handleMessage parses a NATS message and dispatches to the appropriate handler.
func (l *BusListener) handleMessage(subject string, data []byte) {
	var payload DecisionEventPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		log.Printf("BusListener: error parsing event from %s: %v", subject, err)
		return
	}

	if payload.DecisionID == "" {
		log.Printf("BusListener: ignoring event with empty decision_id from %s", subject)
		return
	}

	log.Printf("BusListener: received event %s for decision %s", subject, payload.DecisionID)

	// Determine event type from subject suffix
	// Subjects: hooks.DecisionCreated, hooks.DecisionResponded, etc.
	switch {
	case contains(subject, "DecisionCreated") || contains(subject, "decision.created") || contains(subject, "decisions.created"):
		l.handleDecisionCreated(payload)
	case contains(subject, "DecisionResponded") || contains(subject, "decision.responded") || contains(subject, "decisions.responded"):
		l.handleDecisionResponded(payload)
	case contains(subject, "DecisionExpired") || contains(subject, "decision.expired") || contains(subject, "decisions.expired"):
		l.handleDecisionCanceled(payload)
	case contains(subject, "DecisionEscalated") || contains(subject, "decision.escalated") || contains(subject, "decisions.escalated"):
		l.handleDecisionEscalated(payload)
	default:
		log.Printf("BusListener: unhandled event subject: %s", subject)
	}
}

func (l *BusListener) handleDecisionCreated(payload DecisionEventPayload) {
	l.seenMu.Lock()
	if l.seen[payload.DecisionID] {
		l.seenMu.Unlock()
		return
	}
	l.seen[payload.DecisionID] = true
	l.seenMu.Unlock()

	if l.rpcClient == nil || l.bot == nil {
		log.Printf("BusListener: skipping notification for %s (no rpc/bot)", payload.DecisionID)
		return
	}

	// Fetch full decision details from RPC
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	decision, err := l.rpcClient.GetDecision(ctx, payload.DecisionID)
	if err != nil {
		log.Printf("BusListener: error fetching decision %s: %v", payload.DecisionID, err)
		// Fall back to basic notification from payload
		if payload.Question == "" {
			log.Printf("BusListener: no question in payload, skipping %s", payload.DecisionID)
			return
		}
		fallback := rpcclient.Decision{
			ID:       payload.DecisionID,
			Question: payload.Question,
			Urgency:  payload.Urgency,
		}
		if err := l.bot.NotifyNewDecision(fallback); err != nil {
			log.Printf("BusListener: error notifying Slack: %v", err)
		}
		return
	}

	if decision.Resolved {
		log.Printf("BusListener: decision %s already resolved, skipping", payload.DecisionID)
		return
	}

	log.Printf("BusListener: sending notification for decision %s", payload.DecisionID)
	if err := l.bot.NotifyNewDecision(*decision); err != nil {
		log.Printf("BusListener: error notifying Slack: %v", err)
	} else {
		log.Printf("BusListener: successfully notified Slack for decision %s", payload.DecisionID)
		l.addSlackNotifiedLabel(payload.DecisionID)
	}
}

func (l *BusListener) handleDecisionResponded(payload DecisionEventPayload) {
	resolvedKey := "resolved:" + payload.DecisionID
	l.seenMu.Lock()
	if l.seen[resolvedKey] {
		l.seenMu.Unlock()
		log.Printf("BusListener: already notified resolution for %s", payload.DecisionID)
		return
	}
	l.seen[resolvedKey] = true
	l.seenMu.Unlock()

	if l.rpcClient == nil || l.bot == nil {
		log.Printf("BusListener: skipping resolution notification for %s (no rpc/bot)", payload.DecisionID)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	decision, err := l.rpcClient.GetDecision(ctx, payload.DecisionID)
	if err != nil {
		log.Printf("BusListener: error fetching resolved decision %s: %v", payload.DecisionID, err)
		return
	}

	if err := l.bot.NotifyResolution(*decision); err != nil {
		log.Printf("BusListener: error notifying resolution: %v", err)
	}
}

func (l *BusListener) handleDecisionCanceled(payload DecisionEventPayload) {
	if l.bot == nil {
		log.Printf("BusListener: skipping cancel notification for %s (no bot)", payload.DecisionID)
		return
	}
	if l.bot.DismissDecisionByID(payload.DecisionID) {
		log.Printf("BusListener: auto-dismissed expired/canceled decision %s", payload.DecisionID)
	} else {
		log.Printf("BusListener: no tracked message for decision %s", payload.DecisionID)
	}
}

func (l *BusListener) handleDecisionEscalated(payload DecisionEventPayload) {
	// Escalated decisions are treated like new decisions for notification purposes
	log.Printf("BusListener: decision %s escalated, treating as new notification", payload.DecisionID)
	l.handleDecisionCreated(payload)
}

// EmitDecisionResponse publishes a decision response event to the bus.
// Called when a user responds to a decision via Slack buttons.
func (l *BusListener) EmitDecisionResponse(decisionID string, chosenIndex int, chosenLabel string, slackUserID string) error {
	payload := DecisionEventPayload{
		DecisionID:  decisionID,
		ChosenIndex: chosenIndex,
		ChosenLabel: chosenLabel,
		ResolvedBy:  "slack:" + slackUserID,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling response event: %w", err)
	}

	// Try direct NATS publish first
	if l.js != nil {
		_, err := l.js.Publish("hooks.DecisionResponded", data)
		if err == nil {
			log.Printf("BusListener: emitted DecisionResponded for %s via NATS", decisionID)
			return nil
		}
		log.Printf("BusListener: NATS publish failed, falling back to bd bus emit: %v", err)
	}

	// Fallback: shell out to bd bus emit
	cmd := exec.Command("bd", "bus", "emit", "--hook=DecisionResponded")
	cmd.Stdin = bytes.NewReader(data)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("bd bus emit failed: %v: %s", err, string(output))
	}

	log.Printf("BusListener: emitted DecisionResponded for %s via bd bus emit", decisionID)
	return nil
}

// addSlackNotifiedLabel adds the slack_notified label to a decision bead.
func (l *BusListener) addSlackNotifiedLabel(decisionID string) {
	if err := exec.Command("bd", "label", "add", decisionID, "slack_notified").Run(); err != nil {
		log.Printf("BusListener: warning: failed to add slack_notified label to %s: %v", decisionID, err)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
