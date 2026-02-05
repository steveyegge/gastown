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

func TestResolveSemanticSlug(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "6-char random",
			input:    "gt-dec-cache_strategyabc123",
			expected: "gt-abc123",
		},
		{
			name:     "7-char random with underscore boundary (gt-3vqgi4 bug fix)",
			input:    "lo-dec-refinery_patrol_complete_merged_1syec3r",
			expected: "lo-1syec3r",
		},
		{
			name:     "already canonical",
			input:    "gt-abc123",
			expected: "gt-abc123",
		},
		{
			name:     "6-char numeric random",
			input:    "gt-dec-test_patrol774053",
			expected: "gt-774053",
		},
		{
			name:     "6-char mixed random starting with digit",
			input:    "gt-dec-some_topic30p5ls",
			expected: "gt-30p5ls",
		},
		{
			name:     "with child suffix",
			input:    "gt-dec-cache_strategyabc123.child_name",
			expected: "gt-abc123.child_name",
		},
		{
			name:     "not a semantic slug",
			input:    "not-a-valid-slug",
			expected: "not-a-valid-slug",
		},
		{
			name:     "simple id with hyphen",
			input:    "hq-946577",
			expected: "hq-946577",
		},
		{
			name:     "8-char random with underscore boundary",
			input:    "gt-dec-topic_12345678",
			expected: "gt-12345678",
		},
		{
			name:     "wisp format (wsp abbreviation)",
			input:    "gt-wsp-mol_polecat_work4uihs6",
			expected: "gt-4uihs6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveSemanticSlug(tt.input)
			if result != tt.expected {
				t.Errorf("ResolveSemanticSlug(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
