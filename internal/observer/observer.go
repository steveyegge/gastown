// Package observer provides fire-and-forget event notifications to the
// tmux adapter's message bus. All notifications are non-blocking and
// fail-open: if the adapter is unreachable, events are silently dropped
// with zero impact on the delivery path.
//
// This implements the observer pattern from the Gas Town message bus design:
// tmux is the data plane (always works), the adapter is the control plane
// (optional enhancement layer for observability).
package observer

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"
)

// Event types emitted by gt nudge.
const (
	EventPreSend         = "pre-send"
	EventDeliverySuccess = "delivery-success"
	EventDeliveryFailure = "delivery-failure"
	EventQueueDrain      = "queue-drain"
)

// Default adapter endpoint.
const defaultEndpoint = "http://localhost:8080/api/events"

// envObserverURL is the environment variable to override the adapter endpoint.
// Set to empty string to disable observer notifications entirely.
const envObserverURL = "GT_OBSERVER_URL"

// Event is the JSON payload sent to the adapter's observer endpoint.
type Event struct {
	Type      string `json:"type"`
	From      string `json:"from"`
	To        string `json:"to"`
	Mode      string `json:"mode,omitempty"`
	MsgLength int    `json:"message_length,omitempty"`
	Timestamp string `json:"ts"`
	Error     string `json:"error,omitempty"`
	LatencyMs int64  `json:"latency_ms,omitempty"`

	// Queue drain fields
	DrainedCount  int `json:"drained_count,omitempty"`
	ExpiredCount  int `json:"expired_count,omitempty"`
	OrphanCount   int `json:"orphan_count,omitempty"`
}

var (
	client   *http.Client
	clientMu sync.Once
	endpoint string
	disabled bool
	inflight sync.WaitGroup
)

func init() {
	// Resolve endpoint once at init. If GT_OBSERVER_URL is set but empty,
	// observer is explicitly disabled.
	if v, ok := os.LookupEnv(envObserverURL); ok {
		if v == "" {
			disabled = true
		} else {
			endpoint = v
		}
	} else {
		endpoint = defaultEndpoint
	}
}

func getClient() *http.Client {
	clientMu.Do(func() {
		client = &http.Client{Timeout: 2 * time.Second}
	})
	return client
}

// Notify sends an event to the adapter. Fire-and-forget: errors are
// silently swallowed. The call is non-blocking — the HTTP request
// runs in a background goroutine.
func Notify(event Event) {
	if disabled || endpoint == "" {
		return
	}

	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	// Capture endpoint locally so the goroutine reads a stable value
	// even if the package-level var changes (e.g., during tests).
	url := endpoint

	inflight.Add(1)
	go func() {
		defer inflight.Done()
		req, err := http.NewRequest("POST", url, bytes.NewReader(data))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := getClient().Do(req)
		if err != nil {
			return // adapter down — silently drop
		}
		_ = resp.Body.Close()
	}()
}

// Flush blocks until all in-flight notifications complete or the timeout
// expires. Call this before process exit to ensure delivery-success/failure
// events aren't lost when the process terminates.
func Flush(timeout time.Duration) {
	if disabled {
		return
	}
	done := make(chan struct{})
	go func() {
		inflight.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

// PreSend emits a pre-send event before nudge delivery begins.
func PreSend(from, to, mode string, msgLength int) {
	Notify(Event{
		Type:      EventPreSend,
		From:      from,
		To:        to,
		Mode:      mode,
		MsgLength: msgLength,
	})
}

// DeliverySuccess emits a success event after nudge delivery completes.
func DeliverySuccess(from, to, mode string, latencyMs int64) {
	Notify(Event{
		Type:      EventDeliverySuccess,
		From:      from,
		To:        to,
		Mode:      mode,
		LatencyMs: latencyMs,
	})
}

// DeliveryFailure emits a failure event when nudge delivery fails.
func DeliveryFailure(from, to, mode string, deliveryErr error, latencyMs int64) {
	errStr := ""
	if deliveryErr != nil {
		errStr = deliveryErr.Error()
	}
	Notify(Event{
		Type:      EventDeliveryFailure,
		From:      from,
		To:        to,
		Mode:      mode,
		Error:     errStr,
		LatencyMs: latencyMs,
	})
}

// QueueDrain emits a drain event when queued nudges are picked up.
func QueueDrain(session string, drained, expired, orphaned int) {
	Notify(Event{
		Type:         EventQueueDrain,
		To:           session,
		DrainedCount: drained,
		ExpiredCount: expired,
		OrphanCount:  orphaned,
	})
}
