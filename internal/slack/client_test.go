package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	c := NewClient("https://hooks.slack.com/test")

	if c.webhookURL != "https://hooks.slack.com/test" {
		t.Errorf("webhookURL = %q, want %q", c.webhookURL, "https://hooks.slack.com/test")
	}
	if c.maxRetries != 3 {
		t.Errorf("maxRetries = %d, want 3", c.maxRetries)
	}
}

func TestNewClientWithOptions(t *testing.T) {
	c := NewClient("https://hooks.slack.com/test",
		WithTimeout(10*time.Second),
		WithMaxRetries(5),
		WithBackoff(2*time.Second, 60*time.Second),
	)

	if c.httpClient.Timeout != 10*time.Second {
		t.Errorf("timeout = %v, want 10s", c.httpClient.Timeout)
	}
	if c.maxRetries != 5 {
		t.Errorf("maxRetries = %d, want 5", c.maxRetries)
	}
	if c.initialBackoff != 2*time.Second {
		t.Errorf("initialBackoff = %v, want 2s", c.initialBackoff)
	}
	if c.maxBackoff != 60*time.Second {
		t.Errorf("maxBackoff = %v, want 60s", c.maxBackoff)
	}
}

func TestBuildDecisionBlocks(t *testing.T) {
	c := NewClient("https://hooks.slack.com/test")

	d := &Decision{
		ID:       "dec-123",
		Question: "Which database should we use?",
		Context:  "We need a database for storing user data.",
		Options: []DecisionOption{
			{Label: "PostgreSQL", Description: "Relational database", Recommended: true},
			{Label: "MongoDB", Description: "Document database"},
		},
		Urgency:     "high",
		RequestedBy: "agent/witness",
		Blockers:    []string{"task-456", "task-789"},
		ResolveURL:  "https://example.com/resolve/dec-123",
	}

	blocks := c.buildDecisionBlocks(d)

	// Verify we have blocks
	if len(blocks) == 0 {
		t.Fatal("expected blocks, got none")
	}

	// Convert to JSON for easier inspection
	jsonBlocks, err := json.MarshalIndent(blocks, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal blocks: %v", err)
	}

	jsonStr := string(jsonBlocks)

	// Check header contains urgency emoji
	if !strings.Contains(jsonStr, ":red_circle:") {
		t.Error("expected :red_circle: for high urgency")
	}

	// Check question is present
	if !strings.Contains(jsonStr, "Which database should we use?") {
		t.Error("expected question in blocks")
	}

	// Check context is present
	if !strings.Contains(jsonStr, "storing user data") {
		t.Error("expected context in blocks")
	}

	// Check options are present
	if !strings.Contains(jsonStr, "PostgreSQL") {
		t.Error("expected PostgreSQL option")
	}
	if !strings.Contains(jsonStr, "MongoDB") {
		t.Error("expected MongoDB option")
	}

	// Check recommended marker
	if !strings.Contains(jsonStr, ":star:") {
		t.Error("expected :star: for recommended option")
	}

	// Check blockers
	if !strings.Contains(jsonStr, "task-456") {
		t.Error("expected blocker task-456")
	}

	// Check resolve URL
	if !strings.Contains(jsonStr, "https://example.com/resolve/dec-123") {
		t.Error("expected resolve URL")
	}
}

func TestBuildDecisionBlocksUrgencyLevels(t *testing.T) {
	c := NewClient("https://hooks.slack.com/test")

	tests := []struct {
		urgency string
		emoji   string
	}{
		{"high", ":red_circle:"},
		{"medium", ":large_yellow_circle:"},
		{"low", ":large_green_circle:"},
		{"unknown", ":white_circle:"},
		{"", ":white_circle:"},
	}

	for _, tt := range tests {
		t.Run(tt.urgency, func(t *testing.T) {
			d := &Decision{
				ID:       "dec-123",
				Question: "Test question",
				Options: []DecisionOption{
					{Label: "Option 1"},
					{Label: "Option 2"},
				},
				Urgency: tt.urgency,
			}

			blocks := c.buildDecisionBlocks(d)
			jsonBlocks, _ := json.Marshal(blocks)

			if !strings.Contains(string(jsonBlocks), tt.emoji) {
				t.Errorf("expected %s for urgency %q", tt.emoji, tt.urgency)
			}
		})
	}
}

func TestBuildDecisionBlocksLongContext(t *testing.T) {
	c := NewClient("https://hooks.slack.com/test")

	// Create a context longer than 2500 chars
	longContext := strings.Repeat("x", 3000)

	d := &Decision{
		ID:       "dec-123",
		Question: "Test question",
		Context:  longContext,
		Options: []DecisionOption{
			{Label: "Option 1"},
		},
		Urgency: "low",
	}

	blocks := c.buildDecisionBlocks(d)
	jsonBlocks, _ := json.Marshal(blocks)
	jsonStr := string(jsonBlocks)

	// Context should be truncated
	if strings.Contains(jsonStr, strings.Repeat("x", 2600)) {
		t.Error("expected context to be truncated")
	}

	// Should end with ...
	if !strings.Contains(jsonStr, "...") {
		t.Error("expected truncated context to end with ...")
	}
}

func TestPostDecisionSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("expected Content-Type: application/json")
		}

		// Verify the request body is valid JSON
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		if _, ok := payload["blocks"]; !ok {
			t.Error("expected 'blocks' in payload")
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient(server.URL)

	d := &Decision{
		ID:       "dec-123",
		Question: "Test question?",
		Options: []DecisionOption{
			{Label: "Yes"},
			{Label: "No"},
		},
		Urgency: "medium",
	}

	err := c.PostDecision(context.Background(), d)
	if err != nil {
		t.Errorf("PostDecision failed: %v", err)
	}
}

func TestPostDecisionRetryOn429(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("rate_limited"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient(server.URL,
		WithMaxRetries(3),
		WithBackoff(10*time.Millisecond, 100*time.Millisecond),
	)

	d := &Decision{
		ID:       "dec-123",
		Question: "Test",
		Options:  []DecisionOption{{Label: "A"}},
	}

	err := c.PostDecision(context.Background(), d)
	if err != nil {
		t.Errorf("expected success after retries, got: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestPostDecisionRetryOnServerError(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient(server.URL,
		WithMaxRetries(3),
		WithBackoff(10*time.Millisecond, 100*time.Millisecond),
	)

	d := &Decision{
		ID:       "dec-123",
		Question: "Test",
		Options:  []DecisionOption{{Label: "A"}},
	}

	err := c.PostDecision(context.Background(), d)
	if err != nil {
		t.Errorf("expected success after retry, got: %v", err)
	}

	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestPostDecisionNoRetryOn400(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("invalid_payload"))
	}))
	defer server.Close()

	c := NewClient(server.URL,
		WithMaxRetries(3),
		WithBackoff(10*time.Millisecond, 100*time.Millisecond),
	)

	d := &Decision{
		ID:       "dec-123",
		Question: "Test",
		Options:  []DecisionOption{{Label: "A"}},
	}

	err := c.PostDecision(context.Background(), d)
	if err == nil {
		t.Error("expected error on 400")
	}

	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retry on 400), got %d", attempts)
	}
}

func TestPostDecisionMaxRetriesExceeded(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	c := NewClient(server.URL,
		WithMaxRetries(2),
		WithBackoff(10*time.Millisecond, 100*time.Millisecond),
	)

	d := &Decision{
		ID:       "dec-123",
		Question: "Test",
		Options:  []DecisionOption{{Label: "A"}},
	}

	err := c.PostDecision(context.Background(), d)
	if err == nil {
		t.Error("expected error after max retries")
	}
	if !strings.Contains(err.Error(), "max retries exceeded") {
		t.Errorf("expected 'max retries exceeded' error, got: %v", err)
	}

	// Initial attempt + 2 retries = 3
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestPostDecisionContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewClient(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	d := &Decision{
		ID:       "dec-123",
		Question: "Test",
		Options:  []DecisionOption{{Label: "A"}},
	}

	err := c.PostDecision(ctx, d)
	if err == nil {
		t.Error("expected context deadline exceeded error")
	}
}

func TestRetryableError(t *testing.T) {
	err := &RetryableError{Err: fmt.Errorf("test error")}

	if err.Error() != "test error" {
		t.Errorf("Error() = %q, want %q", err.Error(), "test error")
	}

	if err.Unwrap().Error() != "test error" {
		t.Errorf("Unwrap().Error() = %q, want %q", err.Unwrap().Error(), "test error")
	}

	if !isRetryableError(err) {
		t.Error("expected isRetryableError to return true")
	}

	if isRetryableError(fmt.Errorf("regular error")) {
		t.Error("expected isRetryableError to return false for non-retryable error")
	}

	if isRetryableError(nil) {
		t.Error("expected isRetryableError to return false for nil")
	}
}

