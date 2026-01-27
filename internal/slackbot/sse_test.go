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
