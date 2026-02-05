package tmux

import (
	"bytes"
	"testing"
)

func TestTailLines(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		n        int
		expected []string
	}{
		{
			name:     "empty data",
			data:     "",
			n:        5,
			expected: nil,
		},
		{
			name:     "zero lines requested",
			data:     "line1\nline2\n",
			n:        0,
			expected: nil,
		},
		{
			name:     "single line no newline",
			data:     "only line",
			n:        5,
			expected: []string{"only line"},
		},
		{
			name:     "single line with newline",
			data:     "only line\n",
			n:        5,
			expected: []string{"only line"},
		},
		{
			name:     "multiple lines request more",
			data:     "line1\nline2\nline3\n",
			n:        10,
			expected: []string{"line1", "line2", "line3"},
		},
		{
			name:     "multiple lines request fewer",
			data:     "line1\nline2\nline3\nline4\nline5\n",
			n:        2,
			expected: []string{"line4", "line5"},
		},
		{
			name:     "exact match",
			data:     "line1\nline2\nline3\n",
			n:        3,
			expected: []string{"line1", "line2", "line3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tailLines([]byte(tt.data), tt.n)

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d lines, got %d", len(tt.expected), len(result))
				return
			}

			for i, line := range result {
				if string(line) != tt.expected[i] {
					t.Errorf("line %d: expected %q, got %q", i, tt.expected[i], string(line))
				}
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		expected []string
	}{
		{
			name:     "empty data",
			data:     "",
			expected: nil,
		},
		{
			name:     "single line",
			data:     "only line",
			expected: []string{"only line"},
		},
		{
			name:     "multiple lines",
			data:     "line1\nline2\nline3",
			expected: []string{"line1", "line2", "line3"},
		},
		{
			name:     "trailing newline",
			data:     "line1\nline2\n",
			expected: []string{"line1", "line2", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitLines([]byte(tt.data))

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d lines, got %d", len(tt.expected), len(result))
				return
			}

			for i, line := range result {
				if string(line) != tt.expected[i] {
					t.Errorf("line %d: expected %q, got %q", i, tt.expected[i], string(line))
				}
			}
		})
	}
}

func TestLinesMatch(t *testing.T) {
	tests := []struct {
		name     string
		a, b     string
		expected bool
	}{
		{
			name:     "identical",
			a:        "hello world",
			b:        "hello world",
			expected: true,
		},
		{
			name:     "trailing space in a",
			a:        "hello world   ",
			b:        "hello world",
			expected: true,
		},
		{
			name:     "trailing space in b",
			a:        "hello world",
			b:        "hello world\t",
			expected: true,
		},
		{
			name:     "trailing space in both",
			a:        "hello world  ",
			b:        "hello world\t\t",
			expected: true,
		},
		{
			name:     "different content",
			a:        "hello",
			b:        "world",
			expected: false,
		},
		{
			name:     "leading space matters",
			a:        "  hello",
			b:        "hello",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := linesMatch([]byte(tt.a), []byte(tt.b))
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestHasPastedTextPlaceholder(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		maxLines int
		expected bool
	}{
		{
			name:     "no placeholder",
			data:     "line1\nline2\nline3\n",
			maxLines: 50,
			expected: false,
		},
		{
			name:     "placeholder present",
			data:     "some text\n[Pasted text #3 +47 lines]\nmore text\n",
			maxLines: 50,
			expected: true,
		},
		{
			name:     "placeholder with different numbers",
			data:     "prefix\n[Pasted text #123 +999 lines]\nsuffix\n",
			maxLines: 50,
			expected: true,
		},
		{
			name:     "similar but not matching",
			data:     "This is [Pasted text] but no numbers\n",
			maxLines: 50,
			expected: false,
		},
		{
			name:     "placeholder outside scan range",
			data:     "[Pasted text #1 +10 lines]\nline2\nline3\nline4\nline5\n",
			maxLines: 2,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasPastedTextPlaceholder([]byte(tt.data), tt.maxLines)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestPreservedInputCombined(t *testing.T) {
	tests := []struct {
		name     string
		input    preservedInput
		expected string
	}{
		{
			name:     "all empty",
			input:    preservedInput{},
			expected: "",
		},
		{
			name:     "only original",
			input:    preservedInput{original: []byte("original text")},
			expected: "original text",
		},
		{
			name:     "all parts",
			input: preservedInput{
				original:    []byte("original "),
				extraBefore: []byte("before "),
				extraAfter:  []byte("after"),
			},
			expected: "original before after",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input.combined()
			if string(result) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, string(result))
			}
		})
	}
}

func TestVerifyNudgeIntegrity(t *testing.T) {
	tests := []struct {
		name          string
		afterTail     string
		nudgeMessage  string
		expectFound   bool
		expectBefore  string
		expectAfter   string
		expectCorrupt bool
	}{
		{
			name:          "clean delivery",
			afterTail:     "some output\n> abc123-[from sender] test message\n",
			nudgeMessage:  "abc123-[from sender] test message",
			expectFound:   true,
			expectBefore:  ">",
			expectAfter:   "",
			expectCorrupt: false,
		},
		{
			name:          "nudge not found",
			afterTail:     "some output\nno nudge here\n",
			nudgeMessage:  "abc123-[from sender] test message",
			expectFound:   false,
			expectBefore:  "",
			expectAfter:   "",
			expectCorrupt: false,
		},
		{
			name:          "text after nudge",
			afterTail:     "output\nabc123-[from sender] test messageXXX\n",
			nudgeMessage:  "abc123-[from sender] test message",
			expectFound:   true,
			expectBefore:  "",
			expectAfter:   "XXX",
			expectCorrupt: false,
		},
		{
			name:          "text before and after",
			afterTail:     "output\nprefix abc123-[from sender] test message suffix\n",
			nudgeMessage:  "abc123-[from sender] test message",
			expectFound:   true,
			expectBefore:  "prefix",
			expectAfter:   "suffix",
			expectCorrupt: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, before, after, corrupted := verifyNudgeIntegrity([]byte(tt.afterTail), tt.nudgeMessage)

			if found != tt.expectFound {
				t.Errorf("found: expected %v, got %v", tt.expectFound, found)
			}
			if string(bytes.TrimSpace(before)) != tt.expectBefore {
				t.Errorf("before: expected %q, got %q", tt.expectBefore, string(before))
			}
			if string(bytes.TrimSpace(after)) != tt.expectAfter {
				t.Errorf("after: expected %q, got %q", tt.expectAfter, string(after))
			}
			if corrupted != tt.expectCorrupt {
				t.Errorf("corrupted: expected %v, got %v", tt.expectCorrupt, corrupted)
			}
		})
	}
}

func TestFindOriginalInput(t *testing.T) {
	tests := []struct {
		name          string
		beforeFull    string
		afterTail     string
		nudgeMessage  string
		expectedInput string
	}{
		{
			name: "find input with context",
			beforeFull: `Agent output line 1
Agent output line 2
Agent output line 3
Agent output line 4
Agent output line 5
user input here`,
			afterTail: `Agent output line 3
Agent output line 4
Agent output line 5
abc123-[from sender] nudge message`,
			nudgeMessage:  "abc123-[from sender] nudge message",
			expectedInput: "user input here",
		},
		{
			name: "no context match",
			beforeFull: `Different content
totally unrelated`,
			afterTail: `Agent output
abc123-[from sender] nudge message`,
			nudgeMessage:  "abc123-[from sender] nudge message",
			expectedInput: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findOriginalInput([]byte(tt.beforeFull), []byte(tt.afterTail), tt.nudgeMessage)
			if string(result) != tt.expectedInput {
				t.Errorf("expected %q, got %q", tt.expectedInput, string(result))
			}
		})
	}
}
