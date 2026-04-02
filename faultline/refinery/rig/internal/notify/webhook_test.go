package notify

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/outdoorsea/faultline/internal/db"
)

func TestNewGenericWebhookEmpty(t *testing.T) {
	g := NewGenericWebhook("", slog.Default())
	if g != nil {
		t.Error("expected nil when webhook URL is empty")
	}
}

func TestGenericWebhookNotify(t *testing.T) {
	var received []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		body, _ := io.ReadAll(r.Body)
		received = body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	g := NewGenericWebhook(srv.URL, slog.Default())
	g.Notify(context.Background(), Event{
		Type:       EventNewIssue,
		ProjectID:  42,
		GroupID:    "grp-abc",
		Title:      "TestError",
		Culprit:    "handler.go:10",
		Level:      "error",
		Platform:   "go",
		EventCount: 3,
		BeadID:     "fl-123",
	})

	if len(received) == 0 {
		t.Fatal("no payload received")
	}

	var payload GenericPayload
	if err := json.Unmarshal(received, &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if payload.EventType != "new_issue" {
		t.Errorf("expected event_type new_issue, got %s", payload.EventType)
	}
	if payload.ProjectID != 42 {
		t.Errorf("expected project_id 42, got %d", payload.ProjectID)
	}
	if payload.Title != "TestError" {
		t.Errorf("expected title TestError, got %s", payload.Title)
	}
	if payload.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestTemplatedWebhookNotify(t *testing.T) {
	var received []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received = body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	templates := []db.WebhookTemplate{
		{
			ID:        "test-tmpl",
			Name:      "Test",
			EventType: "*",
			Body:      `{"alert": "{{title}}", "severity": "{{level}}", "count": {{event_count}}}`,
		},
	}

	g := NewTemplatedWebhook(srv.URL, "http://faultline.local", templates, slog.Default())
	g.Notify(context.Background(), Event{
		Type:       EventNewIssue,
		ProjectID:  1,
		GroupID:    "grp-1",
		Title:      "TestError",
		Level:      "error",
		EventCount: 7,
	})

	if len(received) == 0 {
		t.Fatal("no payload received")
	}

	body := string(received)
	if !strings.Contains(body, `"alert": "TestError"`) {
		t.Errorf("expected title substitution, got: %s", body)
	}
	if !strings.Contains(body, `"severity": "error"`) {
		t.Errorf("expected level substitution, got: %s", body)
	}
	if !strings.Contains(body, `"count": 7`) {
		t.Errorf("expected event_count substitution, got: %s", body)
	}
}

func TestTemplatedWebhookEventTypeMatch(t *testing.T) {
	var received []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received = body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	templates := []db.WebhookTemplate{
		{ID: "new", EventType: "new_issue", Body: `{"type": "custom_new"}`},
		{ID: "all", EventType: "*", Body: `{"type": "custom_all"}`},
	}

	g := NewTemplatedWebhook(srv.URL, "", templates, slog.Default())

	// Should match "new_issue" template exactly.
	g.Notify(context.Background(), Event{Type: EventNewIssue, ProjectID: 1, GroupID: "g1"})
	if !strings.Contains(string(received), "custom_new") {
		t.Errorf("expected exact match for new_issue, got: %s", received)
	}

	// Should fall back to wildcard for regression.
	g.Notify(context.Background(), Event{Type: EventRegression, ProjectID: 1, GroupID: "g2"})
	if !strings.Contains(string(received), "custom_all") {
		t.Errorf("expected wildcard match for regression, got: %s", received)
	}
}

func TestNewTemplatedWebhookEmpty(t *testing.T) {
	g := NewTemplatedWebhook("", "", nil, slog.Default())
	if g != nil {
		t.Error("expected nil when webhook URL is empty")
	}
}

func TestGenericWebhookHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	g := NewGenericWebhook(srv.URL, slog.Default())
	// Should not panic on HTTP errors.
	g.Notify(context.Background(), Event{
		Type:  EventNewIssue,
		Title: "Test",
	})
}
