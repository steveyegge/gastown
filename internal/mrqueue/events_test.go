package mrqueue

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEventLogger(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "mrqueue-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("Failed to create beads dir: %v", err)
	}

	logger := NewEventLogger(beadsDir)

	// Test MR
	mr := &MR{
		ID:          "mr-test-123",
		Branch:      "polecat/test",
		Target:      "main",
		SourceIssue: "gt-abc",
		Worker:      "test-worker",
		Rig:         "test-rig",
	}

	// Log merge_started
	if err := logger.LogMergeStarted(mr); err != nil {
		t.Errorf("LogMergeStarted failed: %v", err)
	}

	// Log merged
	if err := logger.LogMerged(mr, "abc123def456"); err != nil {
		t.Errorf("LogMerged failed: %v", err)
	}

	// Log merge_failed
	if err := logger.LogMergeFailed(mr, "conflict in file.go"); err != nil {
		t.Errorf("LogMergeFailed failed: %v", err)
	}

	// Log merge_skipped
	if err := logger.LogMergeSkipped(mr, "already merged"); err != nil {
		t.Errorf("LogMergeSkipped failed: %v", err)
	}

	// Read and verify events
	logPath := logger.LogPath()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	lines := splitLines(string(data))
	if len(lines) != 4 {
		t.Errorf("Expected 4 events, got %d", len(lines))
	}

	// Verify each event type
	expectedTypes := []EventType{EventMergeStarted, EventMerged, EventMergeFailed, EventMergeSkipped}
	for i, line := range lines {
		if line == "" {
			continue
		}
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Errorf("Failed to parse event %d: %v", i, err)
			continue
		}

		if event.Type != expectedTypes[i] {
			t.Errorf("Event %d: expected type %s, got %s", i, expectedTypes[i], event.Type)
		}

		if event.MRID != mr.ID {
			t.Errorf("Event %d: expected MR ID %s, got %s", i, mr.ID, event.MRID)
		}

		if event.Branch != mr.Branch {
			t.Errorf("Event %d: expected branch %s, got %s", i, mr.Branch, event.Branch)
		}

		// Check timestamp is recent
		if time.Since(event.Timestamp) > time.Minute {
			t.Errorf("Event %d: timestamp too old: %v", i, event.Timestamp)
		}
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if start < i {
				lines = append(lines, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
