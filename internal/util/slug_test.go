package util

import "testing"

func TestDeriveChannelSlug(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "epic title from issue",
			input:    "Ephemeral Polecat Merge Workflow: Rebase-as-Work Architecture",
			expected: "ephemeral-polecat-merge",
		},
		{
			name:     "simple title",
			input:    "Fix the parser bug",
			expected: "fix-the-parser-bug",
		},
		{
			name:     "title with numbers",
			input:    "Bug #123 in parser",
			expected: "bug-123-in-parser",
		},
		{
			name:     "title with special chars",
			input:    "Fix: bug! @special #chars",
			expected: "fix-bug-special-chars",
		},
		{
			name:     "title with multiple spaces",
			input:    "Too   many    spaces",
			expected: "too-many-spaces",
		},
		{
			name:     "title already lowercase with hyphens",
			input:    "already-hyphenated-slug",
			expected: "already-hyphenated-slug",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only special chars",
			input:    "!@#$%^&*()",
			expected: "",
		},
		{
			name:     "long title truncated at word boundary",
			input:    "This is a very long title that should be truncated properly",
			expected: "this-is-a-very-long-title",
		},
		{
			name:     "exactly 30 chars",
			input:    "exactly-thirty-chars-here-now",
			expected: "exactly-thirty-chars-here-now",
		},
		{
			name:     "over 30 chars truncated at word boundary",
			input:    "this-is-more-than-thirty-characters-long",
			expected: "this-is-more-than-thirty",
		},
		{
			name:     "leading and trailing punctuation",
			input:    "---leading and trailing---",
			expected: "leading-and-trailing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeriveChannelSlug(tt.input)
			if result != tt.expected {
				t.Errorf("DeriveChannelSlug(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDeriveChannelSlugWithMaxLen(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "custom max length 20",
			input:    "This is a long title that needs truncation",
			maxLen:   20,
			expected: "this-is-a-long",
		},
		{
			name:     "custom max length 80 (Slack limit)",
			input:    "This is a title",
			maxLen:   80,
			expected: "this-is-a-title",
		},
		{
			name:     "max length 10",
			input:    "Very long title here",
			maxLen:   10,
			expected: "very-long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeriveChannelSlugWithMaxLen(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("DeriveChannelSlugWithMaxLen(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic title",
			input:    "Cache Strategy Decision",
			expected: "cache_strategy_decision",
		},
		{
			name:     "title with stop words",
			input:    "What is the best approach for caching",
			expected: "best_approach_caching",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "untitled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateSlug(tt.input)
			if result != tt.expected {
				t.Errorf("GenerateSlug(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
