package observer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestNotify_SendsCorrectJSON(t *testing.T) {
	var received Event
	var mu sync.Mutex
	done := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("failed to decode event: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		close(done)
	}))
	defer srv.Close()

	// Override globals for test
	oldEndpoint := endpoint
	oldDisabled := disabled
	endpoint = srv.URL
	disabled = false
	defer func() {
		endpoint = oldEndpoint
		disabled = oldDisabled
	}()

	PreSend("crew/dunks", "crew/timmy", "immediate", "normal", 42)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for notification")
	}

	mu.Lock()
	defer mu.Unlock()

	if received.Type != EventPreSend {
		t.Errorf("type = %q, want %q", received.Type, EventPreSend)
	}
	if received.From != "crew/dunks" {
		t.Errorf("from = %q, want %q", received.From, "crew/dunks")
	}
	if received.To != "crew/timmy" {
		t.Errorf("to = %q, want %q", received.To, "crew/timmy")
	}
	if received.Mode != "immediate" {
		t.Errorf("mode = %q, want %q", received.Mode, "immediate")
	}
	if received.Priority != "normal" {
		t.Errorf("priority = %q, want %q", received.Priority, "normal")
	}
	if received.MsgLength != 42 {
		t.Errorf("message_length = %d, want %d", received.MsgLength, 42)
	}
	if received.Timestamp == "" {
		t.Error("timestamp should be set")
	}
}

func TestNotify_FailOpen_AdapterDown(t *testing.T) {
	// Point to a closed server — should not block or error
	oldEndpoint := endpoint
	oldDisabled := disabled
	endpoint = "http://127.0.0.1:1" // port 1 — guaranteed refused
	disabled = false
	defer func() {
		endpoint = oldEndpoint
		disabled = oldDisabled
	}()

	start := time.Now()
	PreSend("a", "b", "immediate", "normal", 10)
	elapsed := time.Since(start)

	// Notify is non-blocking — should return nearly instantly
	if elapsed > 50*time.Millisecond {
		t.Errorf("Notify blocked for %v, should be non-blocking", elapsed)
	}
}

func TestNotify_Disabled(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	oldEndpoint := endpoint
	oldDisabled := disabled
	endpoint = srv.URL
	disabled = true
	defer func() {
		endpoint = oldEndpoint
		disabled = oldDisabled
	}()

	PreSend("a", "b", "immediate", "normal", 10)
	time.Sleep(200 * time.Millisecond) // give goroutine time to run if it leaked

	if calls != 0 {
		t.Errorf("expected 0 HTTP calls when disabled, got %d", calls)
	}
}

func TestDeliverySuccess_Payload(t *testing.T) {
	var received Event
	var mu sync.Mutex
	done := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		close(done)
	}))
	defer srv.Close()

	oldEndpoint := endpoint
	oldDisabled := disabled
	endpoint = srv.URL
	disabled = false
	defer func() {
		endpoint = oldEndpoint
		disabled = oldDisabled
	}()

	DeliverySuccess("crew/dunks", "crew/timmy", "wait-idle", "normal", 350)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for notification")
	}

	mu.Lock()
	defer mu.Unlock()

	if received.Type != EventDeliverySuccess {
		t.Errorf("type = %q, want %q", received.Type, EventDeliverySuccess)
	}
	if received.LatencyMs != 350 {
		t.Errorf("latency_ms = %d, want %d", received.LatencyMs, 350)
	}
}

func TestDeliveryFailure_Payload(t *testing.T) {
	var received Event
	var mu sync.Mutex
	done := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		close(done)
	}))
	defer srv.Close()

	oldEndpoint := endpoint
	oldDisabled := disabled
	endpoint = srv.URL
	disabled = false
	defer func() {
		endpoint = oldEndpoint
		disabled = oldDisabled
	}()

	DeliveryFailure("crew/dunks", "crew/timmy", "immediate", "urgent", fmt.Errorf("session not found"), 100)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for notification")
	}

	mu.Lock()
	defer mu.Unlock()

	if received.Type != EventDeliveryFailure {
		t.Errorf("type = %q, want %q", received.Type, EventDeliveryFailure)
	}
	if received.Error != "session not found" {
		t.Errorf("error = %q, want %q", received.Error, "session not found")
	}
}

func TestQueueDrain_Payload(t *testing.T) {
	var received Event
	var mu sync.Mutex
	done := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		close(done)
	}))
	defer srv.Close()

	oldEndpoint := endpoint
	oldDisabled := disabled
	endpoint = srv.URL
	disabled = false
	defer func() {
		endpoint = oldEndpoint
		disabled = oldDisabled
	}()

	QueueDrain("gt-sfgastown-crew-dunks", 3, 1, 2)

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for notification")
	}

	mu.Lock()
	defer mu.Unlock()

	if received.Type != EventQueueDrain {
		t.Errorf("type = %q, want %q", received.Type, EventQueueDrain)
	}
	if received.To != "gt-sfgastown-crew-dunks" {
		t.Errorf("to = %q, want %q", received.To, "gt-sfgastown-crew-dunks")
	}
	if received.DrainedCount != 3 {
		t.Errorf("drained_count = %d, want %d", received.DrainedCount, 3)
	}
	if received.ExpiredCount != 1 {
		t.Errorf("expired_count = %d, want %d", received.ExpiredCount, 1)
	}
	if received.OrphanCount != 2 {
		t.Errorf("orphan_count = %d, want %d", received.OrphanCount, 2)
	}
}
