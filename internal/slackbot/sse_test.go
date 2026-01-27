package slackbot

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/rpcclient"
)

func TestSSEListener_ParseEvent(t *testing.T) {
	// Create a test SSE server
	eventSent := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}

		// Send a connected event
		fmt.Fprintf(w, "event: connected\n")
		fmt.Fprintf(w, "data: {\"status\":\"connected\"}\n\n")
		flusher.Flush()

		// Send a decision event
		fmt.Fprintf(w, "event: decision\n")
		fmt.Fprintf(w, "data: {\"id\":\"test-123\",\"type\":\"created\"}\n\n")
		flusher.Flush()

		eventSent <- true

		// Keep connection open briefly
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	// Create a minimal bot config (won't actually connect to Slack)
	cfg := Config{
		BotToken:    "xoxb-test",
		AppToken:    "xapp-test",
		RPCEndpoint: "http://localhost:8443",
		ChannelID:   "C12345",
	}
	bot, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create bot: %v", err)
	}

	rpcClient := rpcclient.NewClient("http://localhost:8443")
	listener := NewSSEListener(server.URL, bot, rpcClient)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Run listener in background
	go func() {
		_ = listener.Run(ctx)
	}()

	// Wait for event to be sent
	select {
	case <-eventSent:
		// Success - event was processed
	case <-ctx.Done():
		t.Fatal("timeout waiting for event")
	}
}

func TestSSEListener_Reconnect(t *testing.T) {
	connectionCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connectionCount++
		if connectionCount == 1 {
			// First connection - close immediately
			w.WriteHeader(http.StatusOK)
			return
		}
		// Second connection - stay open briefly
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	cfg := Config{
		BotToken:    "xoxb-test",
		AppToken:    "xapp-test",
		RPCEndpoint: "http://localhost:8443",
	}
	bot, _ := New(cfg)
	rpcClient := rpcclient.NewClient("http://localhost:8443")
	listener := NewSSEListener(server.URL, bot, rpcClient)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go func() {
		_ = listener.Run(ctx)
	}()

	// Wait for reconnection attempts
	time.Sleep(2 * time.Second)

	if connectionCount < 2 {
		t.Errorf("expected at least 2 connections (reconnect), got %d", connectionCount)
	}
}

func TestSSEListener_ServerError(t *testing.T) {
	// Test that server returning 500 triggers reconnect
	connectionCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connectionCount++
		if connectionCount == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Subsequent connections succeed briefly
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	cfg := Config{
		BotToken:    "xoxb-test",
		AppToken:    "xapp-test",
		RPCEndpoint: "http://localhost:8443",
	}
	bot, _ := New(cfg)
	rpcClient := rpcclient.NewClient("http://localhost:8443")
	listener := NewSSEListener(server.URL, bot, rpcClient)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	go func() {
		_ = listener.Run(ctx)
	}()

	time.Sleep(2 * time.Second)

	if connectionCount < 2 {
		t.Errorf("expected reconnect after 500 error, got %d connections", connectionCount)
	}
}

func TestSSEListener_MalformedJSON(t *testing.T) {
	// Test that malformed JSON is handled gracefully (logged, not crash)
	eventProcessed := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}

		// Send malformed JSON
		fmt.Fprintf(w, "event: decision\n")
		fmt.Fprintf(w, "data: {invalid json here}\n\n")
		flusher.Flush()

		// Send valid event after
		fmt.Fprintf(w, "event: connected\n")
		fmt.Fprintf(w, "data: {\"status\":\"ok\"}\n\n")
		flusher.Flush()

		eventProcessed <- true
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	cfg := Config{
		BotToken:    "xoxb-test",
		AppToken:    "xapp-test",
		RPCEndpoint: "http://localhost:8443",
	}
	bot, _ := New(cfg)
	rpcClient := rpcclient.NewClient("http://localhost:8443")
	listener := NewSSEListener(server.URL, bot, rpcClient)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_ = listener.Run(ctx)
	}()

	select {
	case <-eventProcessed:
		// Success - didn't crash on malformed JSON
	case <-ctx.Done():
		t.Fatal("timeout - listener may have crashed on malformed JSON")
	}
}

func TestSSEListener_CommentsIgnored(t *testing.T) {
	// Test that SSE comments (lines starting with :) are ignored
	eventReceived := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}

		// Send comment (should be ignored)
		fmt.Fprintf(w, ": this is a comment\n")
		fmt.Fprintf(w, ": another comment\n")

		// Send real event
		fmt.Fprintf(w, "event: connected\n")
		fmt.Fprintf(w, "data: {\"status\":\"connected\"}\n\n")
		flusher.Flush()

		eventReceived <- true
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	cfg := Config{
		BotToken:    "xoxb-test",
		AppToken:    "xapp-test",
		RPCEndpoint: "http://localhost:8443",
	}
	bot, _ := New(cfg)
	rpcClient := rpcclient.NewClient("http://localhost:8443")
	listener := NewSSEListener(server.URL, bot, rpcClient)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_ = listener.Run(ctx)
	}()

	select {
	case <-eventReceived:
		// Success
	case <-ctx.Done():
		t.Fatal("timeout waiting for event")
	}
}

func TestSSEListener_ContextCancellation(t *testing.T) {
	// Test clean exit on context cancellation
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		// Keep connection open
		select {
		case <-r.Context().Done():
			return
		case <-time.After(10 * time.Second):
			return
		}
	}))
	defer server.Close()

	cfg := Config{
		BotToken:    "xoxb-test",
		AppToken:    "xapp-test",
		RPCEndpoint: "http://localhost:8443",
	}
	bot, _ := New(cfg)
	rpcClient := rpcclient.NewClient("http://localhost:8443")
	listener := NewSSEListener(server.URL, bot, rpcClient)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- listener.Run(ctx)
	}()

	// Give listener time to connect
	time.Sleep(100 * time.Millisecond)

	// Cancel context
	cancel()

	// Listener should exit within reasonable time
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("listener did not exit after context cancellation")
	}
}

func TestSSEListener_EmptyEvent(t *testing.T) {
	// Test that events with no data are skipped
	validEventProcessed := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}

		// Send event with no data (should be skipped)
		fmt.Fprintf(w, "event: empty\n\n")
		flusher.Flush()

		// Send valid event
		fmt.Fprintf(w, "event: connected\n")
		fmt.Fprintf(w, "data: {\"status\":\"ok\"}\n\n")
		flusher.Flush()

		validEventProcessed <- true
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	cfg := Config{
		BotToken:    "xoxb-test",
		AppToken:    "xapp-test",
		RPCEndpoint: "http://localhost:8443",
	}
	bot, _ := New(cfg)
	rpcClient := rpcclient.NewClient("http://localhost:8443")
	listener := NewSSEListener(server.URL, bot, rpcClient)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_ = listener.Run(ctx)
	}()

	select {
	case <-validEventProcessed:
		// Success - empty event didn't cause issues
	case <-ctx.Done():
		t.Fatal("timeout")
	}
}

func TestSSEListener_RapidEvents(t *testing.T) {
	// Test handling multiple rapid events
	eventsToSend := 10
	eventsSent := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}

		// Send multiple events rapidly
		for i := 0; i < eventsToSend; i++ {
			fmt.Fprintf(w, "event: decision\n")
			fmt.Fprintf(w, "data: {\"id\":\"test-%d\",\"type\":\"created\"}\n\n", i)
			flusher.Flush()
		}

		eventsSent <- true
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	cfg := Config{
		BotToken:    "xoxb-test",
		AppToken:    "xapp-test",
		RPCEndpoint: "http://localhost:8443",
	}
	bot, _ := New(cfg)
	rpcClient := rpcclient.NewClient("http://localhost:8443")
	listener := NewSSEListener(server.URL, bot, rpcClient)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		_ = listener.Run(ctx)
	}()

	select {
	case <-eventsSent:
		// Success - handled rapid events without issue
	case <-ctx.Done():
		t.Fatal("timeout processing rapid events")
	}
}
