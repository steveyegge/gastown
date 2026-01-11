package feed

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/events"
)

func TestCurator_FiltersByVisibility(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "feed-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create events file with test events
	eventsPath := filepath.Join(tmpDir, events.EventsFile)
	feedPath := filepath.Join(tmpDir, FeedFile)

	// Write a feed-visible event
	feedEvent := events.Event{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Source:     "gt",
		Type:       events.TypeSling,
		Actor:      "mayor",
		Payload:    map[string]interface{}{"bead": "gt-123", "target": "gastown/slit"},
		Visibility: events.VisibilityFeed,
	}
	feedData, _ := json.Marshal(feedEvent)

	// Write an audit-only event (should be filtered out)
	auditEvent := events.Event{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Source:     "gt",
		Type:       "internal_check",
		Actor:      "daemon",
		Visibility: events.VisibilityAudit,
	}
	auditData, _ := json.Marshal(auditEvent)

	// Create events file
	if err := os.WriteFile(eventsPath, []byte{}, 0644); err != nil {
		t.Fatalf("creating events file: %v", err)
	}

	// Start curator
	curator := NewCurator(tmpDir)
	if err := curator.Start(); err != nil {
		t.Fatalf("starting curator: %v", err)
	}
	defer curator.Stop()

	// Give curator time to start
	time.Sleep(50 * time.Millisecond)

	// Append events
	f, err := os.OpenFile(eventsPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("opening events file: %v", err)
	}
	f.Write(append(feedData, '\n'))
	f.Write(append(auditData, '\n'))
	f.Close()

	// Wait for processing
	time.Sleep(300 * time.Millisecond)

	// Check feed file
	feedContent, err := os.ReadFile(feedPath)
	if err != nil {
		t.Fatalf("reading feed file: %v", err)
	}

	// Should contain feed event but not audit event
	if len(feedContent) == 0 {
		t.Error("feed file is empty, expected at least one event")
	}

	var writtenEvent FeedEvent
	if err := json.Unmarshal(feedContent[:len(feedContent)-1], &writtenEvent); err != nil {
		t.Fatalf("parsing feed event: %v", err)
	}

	if writtenEvent.Type != events.TypeSling {
		t.Errorf("expected type %s, got %s", events.TypeSling, writtenEvent.Type)
	}
	if writtenEvent.Actor != "mayor" {
		t.Errorf("expected actor 'mayor', got %s", writtenEvent.Actor)
	}
}

func TestCurator_DedupesDoneEvents(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "feed-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	eventsPath := filepath.Join(tmpDir, events.EventsFile)
	feedPath := filepath.Join(tmpDir, FeedFile)

	// Create events file
	if err := os.WriteFile(eventsPath, []byte{}, 0644); err != nil {
		t.Fatalf("creating events file: %v", err)
	}

	// Start curator
	curator := NewCurator(tmpDir)
	if err := curator.Start(); err != nil {
		t.Fatalf("starting curator: %v", err)
	}
	defer curator.Stop()

	time.Sleep(50 * time.Millisecond)

	// Write 3 identical done events from same actor
	f, _ := os.OpenFile(eventsPath, os.O_APPEND|os.O_WRONLY, 0644)
	for i := 0; i < 3; i++ {
		doneEvent := events.Event{
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			Source:     "gt",
			Type:       events.TypeDone,
			Actor:      "gastown/slit",
			Payload:    map[string]interface{}{"bead": "slit-12345"},
			Visibility: events.VisibilityFeed,
		}
		data, _ := json.Marshal(doneEvent)
		f.Write(append(data, '\n'))
	}
	f.Close()

	// Wait for processing
	time.Sleep(300 * time.Millisecond)

	// Count feed events
	feedContent, _ := os.ReadFile(feedPath)
	lines := 0
	for _, b := range feedContent {
		if b == '\n' {
			lines++
		}
	}

	// Should only have 1 event due to deduplication
	if lines != 1 {
		t.Errorf("expected 1 feed event after deduplication, got %d", lines)
	}
}

func TestCurator_GeneratesSummary(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "feed-test-*")
	defer os.RemoveAll(tmpDir)

	curator := NewCurator(tmpDir)

	tests := []struct {
		event    *events.Event
		expected string
	}{
		{
			event: &events.Event{
				Type:    events.TypeSling,
				Actor:   "mayor",
				Payload: map[string]interface{}{"bead": "gt-123", "target": "gastown/slit"},
			},
			expected: "mayor assigned gt-123 to gastown/slit",
		},
		{
			event: &events.Event{
				Type:    events.TypeDone,
				Actor:   "gastown/slit",
				Payload: map[string]interface{}{"bead": "slit-12345"},
			},
			expected: "gastown/slit completed work on slit-12345",
		},
		{
			event: &events.Event{
				Type:  events.TypeHandoff,
				Actor: "gastown/witness",
			},
			expected: "gastown/witness handed off to fresh session",
		},
	}

	for _, tc := range tests {
		summary := curator.generateSummary(tc.event)
		if summary != tc.expected {
			t.Errorf("generateSummary(%s): expected %q, got %q", tc.event.Type, tc.expected, summary)
		}
	}
}
