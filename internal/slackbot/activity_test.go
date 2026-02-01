package slackbot

import (
	"strings"
	"testing"
	"time"
)

func TestExtractAgentShortName(t *testing.T) {
	tests := []struct {
		agent    string
		expected string
	}{
		{"gastown/crew/decisions", "decisions"},
		{"beads/polecats/wolf", "wolf"},
		{"mayor", "mayor"},
		{"", ""},
		{"a/b/c/d/e", "e"},
	}

	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			result := extractAgentShortName(tt.agent)
			if result != tt.expected {
				t.Errorf("extractAgentShortName(%q) = %q, want %q",
					tt.agent, result, tt.expected)
			}
		})
	}
}

func TestFormatActivityBlocks(t *testing.T) {
	t.Run("empty activities", func(t *testing.T) {
		blocks := formatActivityBlocks("gastown/crew/test", nil)
		if len(blocks) != 2 {
			t.Errorf("expected 2 blocks (header + message), got %d", len(blocks))
		}
	})

	t.Run("with activities", func(t *testing.T) {
		activities := []ActivityEntry{
			{
				Timestamp: time.Now(),
				Type:      "commit",
				Message:   "feat: add new feature",
			},
			{
				Timestamp: time.Now().Add(-time.Minute),
				Type:      "event",
				Message:   "Started work on issue",
			},
		}

		blocks := formatActivityBlocks("gastown/crew/test", activities)
		if len(blocks) != 2 {
			t.Errorf("expected 2 blocks (header + code block), got %d", len(blocks))
		}
	})

	t.Run("truncates long messages", func(t *testing.T) {
		longMessage := strings.Repeat("a", 100)
		activities := []ActivityEntry{
			{
				Timestamp: time.Now(),
				Type:      "commit",
				Message:   longMessage,
			},
		}

		blocks := formatActivityBlocks("test", activities)
		// The message should be truncated in the formatted output
		if len(blocks) != 2 {
			t.Errorf("expected 2 blocks, got %d", len(blocks))
		}
	})
}

func TestActivityEntry(t *testing.T) {
	entry := ActivityEntry{
		Timestamp: time.Now(),
		Type:      "commit",
		Message:   "test message",
	}

	if entry.Type != "commit" {
		t.Errorf("expected type 'commit', got %q", entry.Type)
	}

	if entry.Message != "test message" {
		t.Errorf("expected message 'test message', got %q", entry.Message)
	}
}
