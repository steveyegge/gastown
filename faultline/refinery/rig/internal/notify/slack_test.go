package notify

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestNewSlackWebhookEmpty(t *testing.T) {
	s := NewSlackWebhook("", "", slog.Default())
	if s != nil {
		t.Error("expected nil when webhook URL is empty")
	}
}

func TestNewSlackWebhook(t *testing.T) {
	s := NewSlackWebhook("https://hooks.slack.com/test", "http://localhost:8080", slog.Default())
	if s == nil {
		t.Fatal("expected non-nil webhook")
	}
	if s.webhookURL != "https://hooks.slack.com/test" {
		t.Errorf("unexpected webhook URL: %s", s.webhookURL)
	}
}

func TestNotifyNewIssue(t *testing.T) {
	var mu sync.Mutex
	var received []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		received = body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewSlackWebhook(srv.URL, "http://faultline.local:8080", slog.Default())

	s.Notify(context.Background(), Event{
		Type:       EventNewIssue,
		ProjectID:  1,
		GroupID:    "abc123def456",
		Title:      "NullPointerException",
		Culprit:    "main.go:42",
		Level:      "error",
		Platform:   "go",
		EventCount: 5,
		BeadID:     "fl-xyz",
	})

	mu.Lock()
	defer mu.Unlock()

	if len(received) == 0 {
		t.Fatal("no payload received")
	}

	var payload slackPayload
	if err := json.Unmarshal(received, &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(payload.Blocks) < 2 {
		t.Fatalf("expected at least 2 blocks, got %d", len(payload.Blocks))
	}

	// Check header contains severity
	headerText := payload.Blocks[0].Text.Text
	if !strings.Contains(headerText, "Quake") {
		t.Errorf("header should contain Quake for error level: %s", headerText)
	}
	if !strings.Contains(headerText, "New Fault Detected") {
		t.Errorf("header should contain 'New Fault Detected': %s", headerText)
	}

	// Check fields contain exception info
	raw, _ := json.Marshal(payload.Blocks[1].Fields)
	fields := string(raw)
	if !strings.Contains(fields, "NullPointerException") {
		t.Error("fields should contain exception title")
	}
	if !strings.Contains(fields, "main.go:42") {
		t.Error("fields should contain culprit")
	}

	// Check context block has link
	if len(payload.Blocks) >= 3 {
		contextBlock := payload.Blocks[2]
		if contextBlock.Type != "context" {
			t.Errorf("third block should be context, got %s", contextBlock.Type)
		}
	}
}

func TestNotifyResolved(t *testing.T) {
	var received []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received = body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewSlackWebhook(srv.URL, "", slog.Default())
	s.Notify(context.Background(), Event{
		Type:    EventResolved,
		GroupID: "abc123",
		Title:   "NullPointerException",
		BeadID:  "fl-xyz",
	})

	if len(received) == 0 {
		t.Fatal("no payload received")
	}

	body := string(received)
	if !strings.Contains(body, "Fault Resolved") {
		t.Error("should contain 'Fault Resolved'")
	}
	if !strings.Contains(body, "NullPointerException") {
		t.Error("should contain issue title")
	}
}

func TestNotifyRegression(t *testing.T) {
	var received []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received = body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewSlackWebhook(srv.URL, "", slog.Default())
	s.Notify(context.Background(), Event{
		Type:       EventRegression,
		GroupID:    "abc123",
		Title:      "NullPointerException",
		BeadID:     "fl-new",
		PrevBeadID: "fl-old",
	})

	if len(received) == 0 {
		t.Fatal("no payload received")
	}

	body := string(received)
	if !strings.Contains(body, "Fault Regression") {
		t.Error("should contain 'Fault Regression'")
	}
	if !strings.Contains(body, "fl-old") {
		t.Error("should contain previous bead ID")
	}
}

func TestNotifyFatalSeverity(t *testing.T) {
	var received []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received = body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewSlackWebhook(srv.URL, "", slog.Default())
	s.Notify(context.Background(), Event{
		Type:  EventNewIssue,
		Level: "fatal",
		Title: "Crash",
	})

	body := string(received)
	if !strings.Contains(body, "Rupture") {
		t.Error("fatal level should show Rupture severity")
	}
}

func TestNotifyHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	// Should not panic on HTTP errors.
	s := NewSlackWebhook(srv.URL, "", slog.Default())
	s.Notify(context.Background(), Event{
		Type:  EventNewIssue,
		Title: "Test",
	})
}

func TestSeverityEmoji(t *testing.T) {
	tests := []struct {
		level string
		want  string
	}{
		{"fatal", ":rotating_light:"},
		{"error", ":warning:"},
		{"warning", ":information_source:"},
		{"info", ":information_source:"},
	}
	for _, tt := range tests {
		got := severityEmoji(tt.level)
		if got != tt.want {
			t.Errorf("severityEmoji(%q) = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestSeverityName(t *testing.T) {
	tests := []struct {
		level string
		want  string
	}{
		{"fatal", "Rupture"},
		{"error", "Quake"},
		{"warning", "Tremor"},
	}
	for _, tt := range tests {
		got := severityName(tt.level)
		if got != tt.want {
			t.Errorf("severityName(%q) = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestShortID(t *testing.T) {
	if got := shortID("abc"); got != "abc" {
		t.Errorf("shortID short string: got %q", got)
	}
	if got := shortID("abcdefghijklmnop"); got != "abcdefghijkl" {
		t.Errorf("shortID long string: got %q", got)
	}
}
