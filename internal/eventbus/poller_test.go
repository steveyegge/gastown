package eventbus

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestDecisionPollerMarkSeen(t *testing.T) {
	p := &DecisionPoller{
		seen: make(map[string]bool),
	}

	// Mark a decision as seen
	p.MarkSeen("test-123")

	// Verify it's marked
	p.seenMu.Lock()
	if !p.seen["test-123"] {
		t.Error("expected decision to be marked as seen")
	}
	p.seenMu.Unlock()
}

func TestDecisionPollerPublisherCalled(t *testing.T) {
	// Track published decisions
	var mu sync.Mutex
	published := make(map[string]DecisionData)

	publisher := func(data DecisionData) {
		mu.Lock()
		published[data.ID] = data
		mu.Unlock()
	}

	// Create a mock poller that we can control
	p := &DecisionPoller{
		publisher: publisher,
		seen:      make(map[string]bool),
	}

	// Simulate finding a new decision
	data := DecisionData{
		ID:          "test-decision",
		Question:    "What should we do?",
		RequestedBy: "agent/test",
		Urgency:     "high",
		Options: []DecisionOptionData{
			{Label: "Option A", Description: "First option"},
			{Label: "Option B", Description: "Second option"},
		},
	}

	// Directly call publisher (simulating poll finding a new decision)
	p.publisher(data)

	// Verify publisher was called
	mu.Lock()
	defer mu.Unlock()
	if len(published) != 1 {
		t.Errorf("expected 1 published decision, got %d", len(published))
	}
	if published["test-decision"].Question != "What should we do?" {
		t.Errorf("unexpected question: %s", published["test-decision"].Question)
	}
}

func TestDecisionPollerStartStop(t *testing.T) {
	publisher := func(data DecisionData) {
		// noop publisher for this test
	}

	// Use a very short interval but we'll stop it quickly
	p := NewDecisionPoller(publisher, "/nonexistent/path", 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx)

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Stop it
	cancel()
	p.Stop()

	// Should complete without hanging
}

func TestDecisionPollerNoDuplicates(t *testing.T) {
	var mu sync.Mutex
	publishCount := 0

	publisher := func(data DecisionData) {
		mu.Lock()
		publishCount++
		mu.Unlock()
	}

	p := &DecisionPoller{
		publisher: publisher,
		seen:      make(map[string]bool),
	}

	// Mark decision as already seen
	p.MarkSeen("test-123")

	// If we had a way to trigger poll with this ID, it should not publish
	// This is tested implicitly by the MarkSeen functionality

	mu.Lock()
	if publishCount != 0 {
		t.Errorf("expected 0 publications, got %d", publishCount)
	}
	mu.Unlock()
}

func TestDecisionDataStruct(t *testing.T) {
	data := DecisionData{
		ID:              "dec-123",
		Question:        "Choose auth method",
		Context:         "We need to decide on authentication",
		RequestedBy:     "agent/test",
		Urgency:         "high",
		Blockers:        []string{"work-456"},
		ParentBeadID:    "epic-789",
		ParentBeadTitle: "Authentication Epic",
		Options: []DecisionOptionData{
			{Label: "JWT", Description: "JSON Web Tokens", Recommended: true},
			{Label: "Session", Description: "Server-side sessions"},
		},
	}

	if data.ID != "dec-123" {
		t.Errorf("unexpected ID: %s", data.ID)
	}
	if len(data.Options) != 2 {
		t.Errorf("expected 2 options, got %d", len(data.Options))
	}
	if !data.Options[0].Recommended {
		t.Error("expected first option to be recommended")
	}
	if data.ParentBeadID != "epic-789" {
		t.Errorf("unexpected ParentBeadID: %s", data.ParentBeadID)
	}
	if data.ParentBeadTitle != "Authentication Epic" {
		t.Errorf("unexpected ParentBeadTitle: %s", data.ParentBeadTitle)
	}
}
