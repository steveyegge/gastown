package terminal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// StateChangeEvent is emitted when an agent transitions between states.
// Coop sends these as {"event":"transition",...} over WebSocket.
type StateChangeEvent struct {
	Event         string         `json:"event"`
	Prev          string         `json:"prev"`
	Next          string         `json:"next"`
	Seq           uint64         `json:"seq"`
	Prompt        *PromptContext `json:"prompt,omitempty"`
	ErrorDetail   *string        `json:"error_detail,omitempty"`
	ErrorCategory *string        `json:"error_category,omitempty"`
}

// ExitEvent is emitted when the agent process exits.
// Coop sends these as {"event":"exit",...} over WebSocket.
type ExitEvent struct {
	Event  string `json:"event"`
	Code   *int   `json:"code,omitempty"`
	Signal *int   `json:"signal,omitempty"`
}

// CoopStateWatcher subscribes to Coop's WebSocket state_change stream
// for a single session. It reconnects automatically on disconnection.
type CoopStateWatcher struct {
	baseURL string
	token   string

	// stateCh receives state change events from the WebSocket.
	stateCh chan StateChangeEvent

	// exitCh receives exit events (process exited).
	exitCh chan ExitEvent

	// errCh receives connection errors (for monitoring).
	errCh chan error

	// cancel stops the watcher goroutine.
	cancel context.CancelFunc

	// done is closed when the watcher goroutine exits.
	done chan struct{}

	mu   sync.Mutex
	conn *websocket.Conn
}

// CoopStateWatcherConfig configures a state watcher.
type CoopStateWatcherConfig struct {
	// BaseURL is the Coop HTTP base URL (e.g., "http://localhost:8080").
	BaseURL string

	// Token is the optional bearer token.
	Token string

	// BufferSize is the channel buffer for state events (default 64).
	BufferSize int

	// ReconnectDelay is the delay between reconnection attempts (default 2s).
	ReconnectDelay time.Duration
}

// WatchState starts a background goroutine that subscribes to Coop's
// WebSocket state_change stream. Returns a watcher that provides channels
// for state changes, exit events, and errors.
//
// Call watcher.Close() to stop the subscription.
func (b *CoopBackend) WatchState(session string, cfg CoopStateWatcherConfig) (*CoopStateWatcher, error) {
	base, err := b.baseURL(session)
	if err != nil {
		return nil, err
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = base
	}
	if cfg.Token == "" {
		cfg.Token = b.token
	}

	return newCoopStateWatcher(cfg)
}

// newCoopStateWatcher creates and starts a state watcher.
func newCoopStateWatcher(cfg CoopStateWatcherConfig) (*CoopStateWatcher, error) {
	bufSize := cfg.BufferSize
	if bufSize <= 0 {
		bufSize = 64
	}

	ctx, cancel := context.WithCancel(context.Background())
	w := &CoopStateWatcher{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		token:   cfg.Token,
		stateCh: make(chan StateChangeEvent, bufSize),
		exitCh:  make(chan ExitEvent, 4),
		errCh:   make(chan error, 8),
		cancel:  cancel,
		done:    make(chan struct{}),
	}

	reconnectDelay := cfg.ReconnectDelay
	if reconnectDelay <= 0 {
		reconnectDelay = 2 * time.Second
	}

	go w.run(ctx, reconnectDelay)
	return w, nil
}

// StateCh returns the channel that receives state change events.
func (w *CoopStateWatcher) StateCh() <-chan StateChangeEvent { return w.stateCh }

// ExitCh returns the channel that receives exit events.
func (w *CoopStateWatcher) ExitCh() <-chan ExitEvent { return w.exitCh }

// ErrCh returns the channel that receives connection errors.
func (w *CoopStateWatcher) ErrCh() <-chan error { return w.errCh }

// Close stops the watcher and closes the WebSocket connection.
func (w *CoopStateWatcher) Close() {
	w.cancel()
	w.mu.Lock()
	if w.conn != nil {
		w.conn.Close()
	}
	w.mu.Unlock()
	<-w.done
}

// run is the main loop: connect → read messages → reconnect on error.
func (w *CoopStateWatcher) run(ctx context.Context, reconnectDelay time.Duration) {
	defer close(w.done)
	defer close(w.stateCh)
	defer close(w.exitCh)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := w.connectAndRead(ctx)
		if err != nil && ctx.Err() == nil {
			select {
			case w.errCh <- fmt.Errorf("coop ws: %w", err):
			default:
			}
		}

		// Wait before reconnecting (unless canceled).
		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectDelay):
		}
	}
}

// connectAndRead dials the WebSocket, reads messages until disconnect.
func (w *CoopStateWatcher) connectAndRead(ctx context.Context) error {
	wsURL, err := w.wsURL()
	if err != nil {
		return err
	}

	header := http.Header{}
	if w.token != "" {
		header.Set("Authorization", "Bearer "+w.token)
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.DialContext(ctx, wsURL, header)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	w.mu.Lock()
	w.conn = conn
	w.mu.Unlock()

	defer func() {
		conn.Close()
		w.mu.Lock()
		w.conn = nil
		w.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		_, msg, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		w.dispatch(msg)
	}
}

// wsURL converts the HTTP base URL to a WebSocket URL.
func (w *CoopStateWatcher) wsURL() (string, error) {
	u, err := url.Parse(w.baseURL)
	if err != nil {
		return "", fmt.Errorf("parse base URL: %w", err)
	}

	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
		// already correct
	default:
		return "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}

	u.Path = "/ws"
	q := u.Query()
	q.Set("subscribe", "state")
	if w.token != "" {
		q.Set("token", w.token)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// dispatch routes a raw WebSocket message to the appropriate channel.
// Coop uses {"event": "<type>", ...} as its wire format.
func (w *CoopStateWatcher) dispatch(msg []byte) {
	// Peek at the "event" field (coop's discriminator tag).
	var envelope struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal(msg, &envelope); err != nil {
		return
	}

	switch envelope.Event {
	case "transition":
		var evt StateChangeEvent
		if err := json.Unmarshal(msg, &evt); err != nil {
			return
		}
		select {
		case w.stateCh <- evt:
		default:
			// Drop if consumer is slow — channel buffer full.
		}

	case "exit":
		var evt ExitEvent
		if err := json.Unmarshal(msg, &evt); err != nil {
			return
		}
		select {
		case w.exitCh <- evt:
		default:
		}

	// Ignore pong, screen, output, error, resize — we only subscribed to state.
	}
}
