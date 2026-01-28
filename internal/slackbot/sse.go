package slackbot

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	log.Println("SSE: Connected to decision events stream")

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
		case "created", "pending":
			l.notifyNewDecision(de)
		case "resolved":
			l.notifyResolvedDecision(de)
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	decision, err := l.rpcClient.GetDecision(ctx, de.ID)
	if err != nil {
		log.Printf("SSE: Error fetching decision %s: %v", de.ID, err)
		// Fall back to basic notification with what we have from the event
		if de.Question == "" {
			log.Printf("SSE: No question in event data, skipping notification for %s", de.ID)
			return
		}
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
	}
}

func (l *SSEListener) notifyResolvedDecision(de decisionEvent) {
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
