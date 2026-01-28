package main

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/eventbus"
)

// setupSSETestTown creates a town for SSE handler tests.
func setupSSETestTown(t *testing.T) (string, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "sse-test-*")
	if err != nil {
		t.Fatal(err)
	}

	// Initialize beads repo from tmpDir (creates .beads subdirectory)
	b := beads.NewIsolated(tmpDir)
	if err := b.Init("test-"); err != nil {
		os.RemoveAll(tmpDir)
		t.Skipf("cannot initialize beads repo: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return tmpDir, cleanup
}

// TestSSEHandlerBasic tests basic SSE handler functionality.
func TestSSEHandlerBasic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	townRoot, cleanup := setupSSETestTown(t)
	defer cleanup()

	bus := eventbus.New()
	defer bus.Close()

	handler := NewSSEHandler(bus, townRoot)

	t.Run("SetCorrectHeaders", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/events", nil)
		w := httptest.NewRecorder()

		// Use a context with short timeout so handler exits
		ctx, cancel := context.WithTimeout(req.Context(), 200*time.Millisecond)
		defer cancel()
		req = req.WithContext(ctx)

		handler.ServeHTTP(w, req)

		resp := w.Result()
		if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
			t.Errorf("Content-Type = %q, want text/event-stream", ct)
		}
		if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("Cache-Control = %q, want no-cache", cc)
		}
		if conn := resp.Header.Get("Connection"); conn != "keep-alive" {
			t.Errorf("Connection = %q, want keep-alive", conn)
		}
		if cors := resp.Header.Get("Access-Control-Allow-Origin"); cors != "*" {
			t.Errorf("CORS header = %q, want *", cors)
		}
	})

	t.Run("SendsConnectedEvent", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/events", nil)
		w := httptest.NewRecorder()

		ctx, cancel := context.WithTimeout(req.Context(), 500*time.Millisecond)
		defer cancel()
		req = req.WithContext(ctx)

		handler.ServeHTTP(w, req)

		body := w.Body.String()
		if !strings.Contains(body, "event: connected") {
			t.Error("Response should contain 'event: connected'")
		}
		if !strings.Contains(body, `"status":"connected"`) {
			t.Error("Response should contain connected status in data")
		}
	})
}

// TestSSEHandlerWithDecisions tests SSE handler sends existing decisions.
func TestSSEHandlerWithDecisions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	townRoot, cleanup := setupSSETestTown(t)
	defer cleanup()

	// Create a decision before starting handler
	beadsPath := filepath.Join(townRoot, ".beads")
	b := beads.NewIsolated(beadsPath)
	fields := &beads.DecisionFields{
		Question:    "SSE test decision?",
		Options:     []beads.DecisionOption{{Label: "A"}, {Label: "B"}},
		Urgency:     beads.UrgencyHigh,
		RequestedBy: "test",
		RequestedAt: time.Now().Format(time.RFC3339),
	}
	_, err := b.CreateDecisionBead("SSE test decision?", fields)
	if err != nil {
		t.Fatalf("CreateDecisionBead failed: %v", err)
	}

	bus := eventbus.New()
	defer bus.Close()

	handler := NewSSEHandler(bus, townRoot)

	req := httptest.NewRequest("GET", "/events", nil)
	w := httptest.NewRecorder()

	ctx, cancel := context.WithTimeout(req.Context(), 500*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	handler.ServeHTTP(w, req)

	body := w.Body.String()

	// Should send the existing decision
	if !strings.Contains(body, "SSE test decision") {
		t.Logf("Response body:\n%s", body)
		t.Error("Response should contain existing decision")
	}
	if !strings.Contains(body, `"type":"pending"`) {
		t.Error("Response should contain pending type for existing decisions")
	}
}

// TestSSEHandlerStreamEvents tests SSE handler streams live events.
func TestSSEHandlerStreamEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	townRoot, cleanup := setupSSETestTown(t)
	defer cleanup()

	bus := eventbus.New()
	defer bus.Close()

	handler := NewSSEHandler(bus, townRoot)

	// Create test server
	server := httptest.NewServer(handler)
	defer server.Close()

	// Connect to SSE endpoint
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Read events in background
	eventsChan := make(chan string, 10)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "data:") {
				eventsChan <- line
			}
		}
		close(eventsChan)
	}()

	// Wait a moment for connection to establish
	time.Sleep(100 * time.Millisecond)

	// Publish events
	bus.PublishDecisionCreated("stream-dec-1", nil)
	bus.PublishDecisionResolved("stream-dec-2", nil)
	bus.PublishDecisionCanceled("stream-dec-3")

	// Collect events
	var receivedEvents []string
	timeout := time.After(1 * time.Second)
	foundConnected := false
	foundCreated := false
	foundResolved := false
	foundCanceled := false

	for {
		select {
		case line, ok := <-eventsChan:
			if !ok {
				goto done
			}
			receivedEvents = append(receivedEvents, line)
			if strings.Contains(line, "connected") {
				foundConnected = true
			}
			if strings.Contains(line, "created") {
				foundCreated = true
			}
			if strings.Contains(line, "resolved") {
				foundResolved = true
			}
			if strings.Contains(line, "canceled") {
				foundCanceled = true
			}
		case <-timeout:
			goto done
		}
	}
done:

	if !foundConnected {
		t.Error("Did not receive connected event")
	}
	if !foundCreated {
		t.Error("Did not receive created event")
	}
	if !foundResolved {
		t.Error("Did not receive resolved event")
	}
	if !foundCanceled {
		t.Error("Did not receive canceled event")
	}

	t.Logf("Received %d event lines", len(receivedEvents))
}

// TestEscapeJSON tests JSON escaping for SSE data.
func TestEscapeJSON(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{`with "quotes"`, `with \"quotes\"`},
		{"with\nnewline", `with\nnewline`},
		{"with\ttab", `with\ttab`},
		{`back\slash`, `back\\slash`},        // Single backslash becomes escaped
		{"with\rcarriage", `with\rcarriage`},
		{"mixed\"\n\\", `mixed\"\n\\`},       // Mixed escaping
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := escapeJSON(tt.input)
			if got != tt.want {
				t.Errorf("escapeJSON(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestSSEHandlerContextCancellation tests handler respects context cancellation.
func TestSSEHandlerContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	townRoot, cleanup := setupSSETestTown(t)
	defer cleanup()

	bus := eventbus.New()
	defer bus.Close()

	handler := NewSSEHandler(bus, townRoot)

	req := httptest.NewRequest("GET", "/events", nil)
	w := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(w, req)
		close(done)
	}()

	// Cancel context after short delay
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Handler should exit
	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Error("Handler did not exit after context cancellation")
	}
}

// TestSSEHandlerNoFlusher tests graceful handling when ResponseWriter doesn't support flushing.
func TestSSEHandlerNoFlusher(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	townRoot, cleanup := setupSSETestTown(t)
	defer cleanup()

	bus := eventbus.New()
	defer bus.Close()

	handler := NewSSEHandler(bus, townRoot)

	// Create a ResponseWriter that doesn't implement http.Flusher
	req := httptest.NewRequest("GET", "/events", nil)
	w := &nonFlushingResponseWriter{header: make(http.Header)}

	handler.ServeHTTP(w, req)

	if w.statusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want %d", w.statusCode, http.StatusInternalServerError)
	}
}

// nonFlushingResponseWriter is a ResponseWriter that doesn't implement Flusher.
type nonFlushingResponseWriter struct {
	header     http.Header
	statusCode int
	body       []byte
}

func (w *nonFlushingResponseWriter) Header() http.Header {
	return w.header
}

func (w *nonFlushingResponseWriter) Write(b []byte) (int, error) {
	w.body = append(w.body, b...)
	return len(b), nil
}

func (w *nonFlushingResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}
