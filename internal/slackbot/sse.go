package slackbot

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/steveyegge/gastown/internal/rpcclient"
)

// SSEListener connects to gtmobile's SSE endpoint and forwards decision events to Slack.
type SSEListener struct {
	sseURL    string
	bot       *Bot
	rpcClient *rpcclient.Client
	seen      map[string]bool // Track notified decision IDs to avoid duplicates
	seenMu    sync.Mutex
}

// NewSSEListener creates a new SSE listener.
func NewSSEListener(sseURL string, bot *Bot, rpcClient *rpcclient.Client) *SSEListener {
	return &SSEListener{
		sseURL:    sseURL,
		bot:       bot,
		rpcClient: rpcClient,
		seen:      make(map[string]bool),
	}
}

// sseEvent represents a parsed SSE event.
type sseEvent struct {
	Event string
	Data  string
}

// decisionEvent represents the JSON data in a decision SSE event.
type decisionEvent struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Urgency  string `json:"urgency"`
	Type     string `json:"type"` // "pending", "created", "resolved", "canceled"
}

// Run starts listening for SSE events. Blocks until context is canceled.
// Automatically reconnects on disconnect with exponential backoff.
func (l *SSEListener) Run(ctx context.Context) error {
	// Note: We intentionally do NOT pre-populate the seen map.
	// The SSE stream sends "pending" events for existing decisions (state dump)
	// and "created" events for new ones. We only notify on "created" to avoid
	// re-notifying on restart while still catching decisions created while down.

	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := l.connect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("SSE: Connection error: %v, reconnecting in %v", err, backoff)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			// Exponential backoff
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		} else {
			// Reset backoff on successful connection
			backoff = time.Second
		}
	}
}

func (l *SSEListener) connect(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", l.sseURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	client := &http.Client{
		Timeout: 0, // No timeout for SSE
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connecting: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	log.Println("SSE: Connected to decision events stream")

	// Catch up on missed notifications: find decisions without slack_notified label
	l.catchUpMissedDecisions()

	scanner := bufio.NewScanner(resp.Body)
	var currentEvent sseEvent

	for scanner.Scan() {
		line := scanner.Text()

		// Empty line marks end of event
		if line == "" {
			if currentEvent.Event != "" && currentEvent.Data != "" {
				l.handleEvent(currentEvent)
			}
			currentEvent = sseEvent{}
			continue
		}

		// Parse SSE format
		if strings.HasPrefix(line, "event:") {
			currentEvent.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			currentEvent.Data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
		// Ignore comments (lines starting with :)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading stream: %w", err)
	}

	return fmt.Errorf("stream closed")
}

func (l *SSEListener) handleEvent(evt sseEvent) {
	switch evt.Event {
	case "connected":
		log.Printf("SSE: Server confirmed connection: %s", evt.Data)

	case "decision":
		var de decisionEvent
		if err := json.Unmarshal([]byte(evt.Data), &de); err != nil {
			log.Printf("SSE: Error parsing decision event: %v", err)
			return
		}

		log.Printf("SSE: Received decision event: id=%s type=%s question=%q", de.ID, de.Type, de.Question)

		switch de.Type {
		case "created":
			// Only notify on "created" events - these are genuinely new decisions
			l.notifyNewDecision(de)
		case "pending":
			// "pending" events are the initial state dump on connect - skip to avoid duplicates
			// The seen map still prevents duplicates if the same decision is created multiple times
			log.Printf("SSE: Skipping pending event (state dump): %s", de.ID)
		case "resolved":
			l.notifyResolvedDecision(de)
		case "cancelled", "canceled":
			l.handleCancelledDecision(de)
		default:
			log.Printf("SSE: Ignoring event type: %s", de.Type)
		}
	}
}

func (l *SSEListener) notifyNewDecision(de decisionEvent) {
	// Check if we've already notified for this decision
	l.seenMu.Lock()
	if l.seen[de.ID] {
		l.seenMu.Unlock()
		return
	}
	l.seen[de.ID] = true
	l.seenMu.Unlock()

	// Fetch full decision details from RPC using GetDecision (works even if resolved)
	// Retry with backoff to avoid falling back to buttonless notification on transient failures
	var decision *rpcclient.Decision
	var err error
	retryDelays := []time.Duration{0, 500 * time.Millisecond, 1 * time.Second, 2 * time.Second}

	for attempt, delay := range retryDelays {
		if delay > 0 {
			time.Sleep(delay)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		decision, err = l.rpcClient.GetDecision(ctx, de.ID)
		cancel()
		if err == nil {
			break
		}
		if attempt < len(retryDelays)-1 {
			log.Printf("SSE: Attempt %d failed to fetch decision %s: %v, retrying...", attempt+1, de.ID, err)
		}
	}

	if err != nil {
		log.Printf("SSE: All attempts failed to fetch decision %s: %v", de.ID, err)
		// Fall back to basic notification with what we have from the event
		// WARNING: This notification will NOT have clickable option buttons
		if de.Question == "" {
			log.Printf("SSE: No question in event data, skipping notification for %s", de.ID)
			return
		}
		log.Printf("SSE: Falling back to buttonless notification for %s (RPC unavailable)", de.ID)
		fallback := rpcclient.Decision{
			ID:       de.ID,
			Question: de.Question,
			Urgency:  de.Urgency,
		}
		if err := l.bot.NotifyNewDecision(fallback); err != nil {
			log.Printf("SSE: Error notifying Slack: %v", err)
		}
		return
	}

	// Skip if already resolved (we only notify for new pending decisions)
	if decision.Resolved {
		log.Printf("SSE: Decision %s already resolved, skipping notification", de.ID)
		return
	}

	log.Printf("SSE: Sending notification for decision %s to Slack", de.ID)
	if err := l.bot.NotifyNewDecision(*decision); err != nil {
		log.Printf("SSE: Error notifying Slack: %v", err)
	} else {
		log.Printf("SSE: Successfully notified Slack for decision %s", de.ID)
		// Mark as notified via label
		l.addSlackNotifiedLabel(de.ID)
	}
}

func (l *SSEListener) notifyResolvedDecision(de decisionEvent) {
	// Check if we've already notified for this resolution
	// Use "resolved:" prefix to distinguish from new decision notifications
	resolvedKey := "resolved:" + de.ID
	l.seenMu.Lock()
	if l.seen[resolvedKey] {
		l.seenMu.Unlock()
		log.Printf("SSE: Already notified resolution for %s, skipping duplicate", de.ID)
		return
	}
	l.seen[resolvedKey] = true
	l.seenMu.Unlock()

	// Fetch full decision details from RPC
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	decision, err := l.rpcClient.GetDecision(ctx, de.ID)
	if err != nil {
		log.Printf("SSE: Error fetching resolved decision %s: %v", de.ID, err)
		return
	}

	if err := l.bot.NotifyResolution(*decision); err != nil {
		log.Printf("SSE: Error notifying Slack of resolution: %v", err)
	}
}

func (l *SSEListener) handleCancelledDecision(de decisionEvent) {
	// Auto-dismiss: delete the Slack notification for cancelled decisions
	if l.bot.DismissDecisionByID(de.ID) {
		log.Printf("SSE: Auto-dismissed cancelled decision %s", de.ID)
	} else {
		log.Printf("SSE: No tracked message for cancelled decision %s (may not have been posted)", de.ID)
	}
}

// catchUpMissedDecisions queries for decisions that were created while the bot was down
// and notifies Slack about them. Uses the slack_notified label to track notification state.
func (l *SSEListener) catchUpMissedDecisions() {
	// Query for open gate-type decisions without the slack_notified label
	out, err := exec.Command("bd", "q", "type:gate status:open -label:slack_notified").Output()
	if err != nil {
		log.Printf("SSE: Error querying for missed decisions: %v", err)
		return
	}

	ids := strings.TrimSpace(string(out))
	if ids == "" {
		log.Println("SSE: No missed decisions to catch up on")
		return
	}

	for _, id := range strings.Split(ids, "\n") {
		id = strings.TrimSpace(id)
		if id != "" {
			l.notifyMissedDecision(id)
		}
	}
}

// notifyMissedDecision notifies Slack about a decision that was missed during bot downtime.
func (l *SSEListener) notifyMissedDecision(decisionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	decision, err := l.rpcClient.GetDecision(ctx, decisionID)
	if err != nil {
		log.Printf("SSE: Error fetching missed decision %s: %v", decisionID, err)
		return
	}

	if decision == nil || decision.Resolved {
		log.Printf("SSE: Missed decision %s is nil or already resolved, skipping", decisionID)
		return
	}

	// Check if we've already notified (in case of race with event stream)
	l.seenMu.Lock()
	if l.seen[decisionID] {
		l.seenMu.Unlock()
		return
	}
	l.seen[decisionID] = true
	l.seenMu.Unlock()

	log.Printf("SSE: Notifying missed decision %s", decisionID)
	if err := l.bot.NotifyNewDecision(*decision); err != nil {
		log.Printf("SSE: Error notifying missed decision: %v", err)
		return
	}

	// Mark as notified
	l.addSlackNotifiedLabel(decisionID)
	log.Printf("SSE: Successfully notified missed decision %s", decisionID)
}

// addSlackNotifiedLabel adds the slack_notified label to a decision bead.
func (l *SSEListener) addSlackNotifiedLabel(decisionID string) {
	if err := exec.Command("bd", "label", "add", decisionID, "slack_notified").Run(); err != nil {
		log.Printf("SSE: Warning: failed to add slack_notified label to %s: %v", decisionID, err)
	}
}

