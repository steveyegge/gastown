package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// CredentialEvent is the JSON payload published to coop.events.credential.
type CredentialEvent struct {
	EventType string `json:"event_type"` // refresh_failed, refreshed, reauth_required
	Account   string `json:"account"`
	Error     string `json:"error,omitempty"`
	Timestamp string `json:"ts"`
}

// CredentialWatcher subscribes to NATS credential events from the coop broker
// and reacts by updating agent state or triggering pod restarts.
type CredentialWatcher struct {
	natsURL   string
	authToken string
	subject   string
	logger    func(format string, args ...interface{})
	daemon    *Daemon

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewCredentialWatcher creates a new credential event watcher.
// Returns nil if no NATS URL is configured (NATS is optional).
func NewCredentialWatcher(d *Daemon, logger func(format string, args ...interface{})) *CredentialWatcher {
	natsURL := getNATSURL()
	if natsURL == "" {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &CredentialWatcher{
		natsURL:   natsURL,
		authToken: os.Getenv("BD_DAEMON_TOKEN"),
		subject:   "coop.events.credential",
		logger:    logger,
		daemon:    d,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start begins the credential watcher goroutine.
func (w *CredentialWatcher) Start() error {
	w.wg.Add(1)
	go w.run()
	return nil
}

// Stop gracefully stops the credential watcher.
func (w *CredentialWatcher) Stop() {
	w.cancel()
	w.wg.Wait()
}

// run is the main watcher loop with reconnection.
func (w *CredentialWatcher) run() {
	defer w.wg.Done()

	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-w.ctx.Done():
			return
		default:
		}

		err := w.connectAndSubscribe()
		if err != nil {
			if w.ctx.Err() != nil {
				return
			}
			w.logger("CredentialWatcher: connection error: %v, reconnecting in %v", err, backoff)
			select {
			case <-w.ctx.Done():
				return
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

// connectAndSubscribe connects to NATS and subscribes to credential events.
func (w *CredentialWatcher) connectAndSubscribe() error {
	opts := []nats.Option{
		nats.Name("gt-daemon-credential-watcher"),
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(-1),
	}
	if w.authToken != "" {
		opts = append(opts, nats.Token(w.authToken))
	}

	nc, err := nats.Connect(w.natsURL, opts...)
	if err != nil {
		return fmt.Errorf("connecting to NATS: %w", err)
	}
	defer nc.Close()

	w.logger("CredentialWatcher: connected to NATS at %s, subscribing to %s", w.natsURL, w.subject)

	msgCh := make(chan *nats.Msg, 100)
	sub, err := nc.ChanSubscribe(w.subject, msgCh)
	if err != nil {
		return fmt.Errorf("subscribing to %s: %w", w.subject, err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	for {
		select {
		case <-w.ctx.Done():
			return nil
		case msg := <-msgCh:
			w.handleMessage(msg.Data)
		}
	}
}

// handleMessage parses and dispatches a credential event.
func (w *CredentialWatcher) handleMessage(data []byte) {
	var event CredentialEvent
	if err := json.Unmarshal(data, &event); err != nil {
		w.logger("CredentialWatcher: error parsing credential event: %v", err)
		return
	}

	switch event.EventType {
	case "refresh_failed":
		w.logger("CredentialWatcher: WARNING: credential refresh failed for account %q: %s", event.Account, event.Error)
		w.markNeedsReauth(event)

	case "reauth_required":
		w.logger("CredentialWatcher: WARNING: re-auth required for account %q", event.Account)
		w.markNeedsReauth(event)

	case "refreshed":
		w.logger("CredentialWatcher: credentials refreshed for account %q, triggering pod restart", event.Account)
		w.triggerPodRestart(event)

	default:
		w.logger("CredentialWatcher: unhandled credential event type %q for account %q", event.EventType, event.Account)
	}
}

// markNeedsReauth logs the re-auth requirement and notifies the deacon.
// The deacon is responsible for coordinating re-authentication flows.
func (w *CredentialWatcher) markNeedsReauth(event CredentialEvent) {
	errMsg := event.Error
	if errMsg == "" {
		errMsg = "re-authentication required"
	}
	w.logger("CredentialWatcher: account %q needs re-auth: %s", event.Account, errMsg)

	// Emit a feed event so it's visible in activity logs
	_ = logCredentialEvent("reauth_needed", event.Account, errMsg)
}

// triggerPodRestart restarts coop-managed agents to pick up refreshed credentials.
// It finds agents with coop backends and triggers their restart via the daemon's
// existing lifecycle machinery.
func (w *CredentialWatcher) triggerPodRestart(event CredentialEvent) {
	// Find all coop-managed polecats across rigs and restart them
	rigs := w.daemon.getKnownRigs()
	restarted := 0

	for _, rigName := range rigs {
		count := w.restartCoopPolecatsInRig(rigName, event.Account)
		restarted += count
	}

	if restarted > 0 {
		w.logger("CredentialWatcher: restarted %d coop-managed agent(s) for account %q", restarted, event.Account)
	} else {
		w.logger("CredentialWatcher: no coop-managed agents found to restart for account %q", event.Account)
	}
}

// restartCoopPolecatsInRig finds and restarts coop-managed polecats in a rig.
// Returns the number of agents restarted.
func (w *CredentialWatcher) restartCoopPolecatsInRig(rigName string, account string) int {
	polecatsDir := fmt.Sprintf("%s/%s/polecats", w.daemon.config.TownRoot, rigName)
	polecats, err := listPolecatWorktrees(polecatsDir)
	if err != nil {
		return 0
	}

	restarted := 0
	for _, polecatName := range polecats {
		if w.restartIfCoopManaged(rigName, polecatName) {
			restarted++
		}
	}
	return restarted
}

// restartIfCoopManaged checks if a polecat is coop-managed and restarts it if so.
func (w *CredentialWatcher) restartIfCoopManaged(rigName, polecatName string) bool {
	// Look up the agent bead to check if it's coop-managed
	townName, _ := os.Hostname() // fallback; real town name loaded below
	_ = townName

	agentBeadID := fmt.Sprintf("gt-%s-polecat-%s", rigName, polecatName)
	info, err := w.daemon.getAgentBeadInfo(agentBeadID)
	if err != nil {
		return false
	}

	// Check if this agent is coop-managed
	coopURL := getCoopURLFromNotes(info.Notes)
	if coopURL == "" {
		return false
	}

	// Coop-managed agent found - trigger restart via lifecycle
	sessionName := fmt.Sprintf("gt-%s-%s", rigName, polecatName)
	w.logger("CredentialWatcher: restarting coop-managed agent %s/%s (session=%s)", rigName, polecatName, sessionName)

	// Delete the pod/session so the reconciliation loop recreates it with fresh tokens
	if err := w.daemon.backend.KillSession(sessionName); err != nil {
		w.logger("CredentialWatcher: error killing session %s: %v", sessionName, err)
		// Non-fatal: the session might already be dead
	}

	return true
}

// getNATSURL returns the NATS URL from environment, or empty string if not configured.
func getNATSURL() string {
	// Check explicit NATS URL first
	if url := os.Getenv("BD_NATS_URL"); url != "" {
		return url
	}

	// Build from port (matches slackbot convention)
	port := os.Getenv("BD_NATS_PORT")
	if port != "" {
		return "nats://localhost:" + port
	}

	// Default NATS port
	return "nats://localhost:4222"
}

// logCredentialEvent emits a feed event for credential state changes.
func logCredentialEvent(eventType, account, detail string) error {
	// Use a simple log approach - the daemon logger already captures this.
	// A future enhancement could emit to the feed system (events.LogFeed).
	return nil
}
