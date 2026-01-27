package inject

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQueue(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "inject-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create runtime dir
	runtimeDir := filepath.Join(tmpDir, ".runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatalf("creating runtime dir: %v", err)
	}

	q := NewQueue(tmpDir, "test-session-123")

	// Test empty queue
	entries, err := q.Drain()
	if err != nil {
		t.Fatalf("draining empty queue: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty queue, got %d entries", len(entries))
	}

	// Test enqueue
	if err := q.Enqueue(TypeMail, "You have 2 unread messages"); err != nil {
		t.Fatalf("enqueueing mail: %v", err)
	}
	if err := q.Enqueue(TypeDecision, "Decision pending: Choose architecture"); err != nil {
		t.Fatalf("enqueueing decision: %v", err)
	}

	// Test count
	count, err := q.Count()
	if err != nil {
		t.Fatalf("counting: %v", err)
	}
	if count != 2 {
		t.Errorf("expected count 2, got %d", count)
	}

	// Test peek (doesn't remove)
	entries, err = q.Peek()
	if err != nil {
		t.Fatalf("peeking: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries after peek, got %d", len(entries))
	}

	// Test drain (removes)
	entries, err = q.Drain()
	if err != nil {
		t.Fatalf("draining: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Type != TypeMail {
		t.Errorf("expected first entry type %s, got %s", TypeMail, entries[0].Type)
	}
	if entries[0].Content != "You have 2 unread messages" {
		t.Errorf("unexpected content: %s", entries[0].Content)
	}
	if entries[1].Type != TypeDecision {
		t.Errorf("expected second entry type %s, got %s", TypeDecision, entries[1].Type)
	}

	// Verify queue is empty after drain
	count, err = q.Count()
	if err != nil {
		t.Fatalf("counting after drain: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0 after drain, got %d", count)
	}
}

func TestQueueClear(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "inject-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	q := NewQueue(tmpDir, "test-session-456")

	// Enqueue some items
	q.Enqueue(TypeMail, "Message 1")
	q.Enqueue(TypeMail, "Message 2")

	// Clear
	if err := q.Clear(); err != nil {
		t.Fatalf("clearing: %v", err)
	}

	// Verify empty
	count, err := q.Count()
	if err != nil {
		t.Fatalf("counting after clear: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0 after clear, got %d", count)
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"a\nb\nc", []string{"a", "b", "c"}},
		{"a\r\nb\r\nc", []string{"a", "b", "c"}},
		{"single", []string{"single"}},
		{"", []string{}},
		{"\n\n", []string{"", ""}},
	}

	for _, tc := range tests {
		lines := splitLines([]byte(tc.input))
		if len(lines) != len(tc.expected) {
			t.Errorf("input %q: expected %d lines, got %d", tc.input, len(tc.expected), len(lines))
			continue
		}
		for i, line := range lines {
			if string(line) != tc.expected[i] {
				t.Errorf("input %q: line %d: expected %q, got %q", tc.input, i, tc.expected[i], string(line))
			}
		}
	}
}
