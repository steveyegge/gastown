package feed

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/mrqueue"
)

func TestParseMQEventLine(t *testing.T) {
	tests := []struct {
		name         string
		event        mrqueue.Event
		wantType     string
		wantTarget   string
		wantContains string // Substring in message
	}{
		{
			name: "merge_started",
			event: mrqueue.Event{
				Timestamp: time.Now(),
				Type:      mrqueue.EventMergeStarted,
				MRID:      "mr-123",
				Branch:    "polecat/nux",
				Target:    "main",
				Worker:    "nux",
				Rig:       "gastown",
			},
			wantType:     "merge_started",
			wantTarget:   "mr-123",
			wantContains: "Merge started",
		},
		{
			name: "merged",
			event: mrqueue.Event{
				Timestamp:   time.Now(),
				Type:        mrqueue.EventMerged,
				MRID:        "mr-456",
				Branch:      "polecat/toast",
				Target:      "main",
				Worker:      "toast",
				Rig:         "gastown",
				MergeCommit: "abc123def456789",
			},
			wantType:     "merged",
			wantTarget:   "mr-456",
			wantContains: "abc123de", // Short SHA
		},
		{
			name: "merge_failed",
			event: mrqueue.Event{
				Timestamp: time.Now(),
				Type:      mrqueue.EventMergeFailed,
				MRID:      "mr-789",
				Branch:    "polecat/capable",
				Target:    "main",
				Worker:    "capable",
				Rig:       "gastown",
				Reason:    "conflict in main.go",
			},
			wantType:     "merge_failed",
			wantTarget:   "mr-789",
			wantContains: "conflict in main.go",
		},
		{
			name: "merge_skipped",
			event: mrqueue.Event{
				Timestamp: time.Now(),
				Type:      mrqueue.EventMergeSkipped,
				MRID:      "mr-999",
				Branch:    "polecat/skip",
				Target:    "main",
				Reason:    "already merged",
			},
			wantType:     "merge_skipped",
			wantTarget:   "mr-999",
			wantContains: "already merged",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON line
			data, err := json.Marshal(tt.event)
			if err != nil {
				t.Fatalf("Failed to marshal event: %v", err)
			}

			// Parse the line
			result := parseMQEventLine(string(data))
			if result == nil {
				t.Fatal("parseMQEventLine returned nil")
			}

			if result.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", result.Type, tt.wantType)
			}

			if result.Target != tt.wantTarget {
				t.Errorf("Target = %q, want %q", result.Target, tt.wantTarget)
			}

			if tt.wantContains != "" && !contains(result.Message, tt.wantContains) {
				t.Errorf("Message = %q, want to contain %q", result.Message, tt.wantContains)
			}

			// Actor should be refinery
			if result.Actor != "refinery" {
				t.Errorf("Actor = %q, want %q", result.Actor, "refinery")
			}

			if result.Role != "refinery" {
				t.Errorf("Role = %q, want %q", result.Role, "refinery")
			}
		})
	}
}

func TestParseMQEventLineEmpty(t *testing.T) {
	result := parseMQEventLine("")
	if result != nil {
		t.Error("Expected nil for empty line")
	}

	result = parseMQEventLine("   ")
	if result != nil {
		t.Error("Expected nil for whitespace-only line")
	}

	result = parseMQEventLine("not valid json")
	if result != nil {
		t.Error("Expected nil for invalid JSON")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
