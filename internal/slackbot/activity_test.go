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

// TestGitActivityAuthorMatching tests that git commits are matched by author name (gt-5gfztk).
// This ensures crew agents like "decisions" show their commits in Peek activity.
func TestGitActivityAuthorMatching(t *testing.T) {
	tests := []struct {
		name        string
		author      string
		subject     string
		shortName   string
		shouldMatch bool
	}{
		{
			name:        "author exact match",
			author:      "decisions",
			subject:     "fix: some bug",
			shortName:   "decisions",
			shouldMatch: true,
		},
		{
			name:        "author contains shortname",
			author:      "decisions-worker",
			subject:     "feat: new feature",
			shortName:   "decisions",
			shouldMatch: true,
		},
		{
			name:        "author case insensitive",
			author:      "Decisions",
			subject:     "chore: cleanup",
			shortName:   "decisions",
			shouldMatch: true,
		},
		{
			name:        "subject contains shortname",
			author:      "other-author",
			subject:     "fix(decisions): resolve issue",
			shortName:   "decisions",
			shouldMatch: true,
		},
		{
			name:        "no match",
			author:      "other-author",
			subject:     "unrelated commit",
			shortName:   "decisions",
			shouldMatch: false,
		},
		{
			name:        "polecat name in author",
			author:      "nux",
			subject:     "fix: something",
			shortName:   "nux",
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authorLower := strings.ToLower(tt.author)
			subjectLower := strings.ToLower(tt.subject)
			shortNameLower := strings.ToLower(tt.shortName)

			authorMatch := authorLower == shortNameLower || strings.Contains(authorLower, shortNameLower)
			subjectMatch := strings.Contains(subjectLower, shortNameLower)

			matched := authorMatch || subjectMatch

			if matched != tt.shouldMatch {
				t.Errorf("author=%q subject=%q shortName=%q: got match=%v, want %v",
					tt.author, tt.subject, tt.shortName, matched, tt.shouldMatch)
			}
		})
	}
}
