package archive

import (
	"testing"
)

func TestCharDiff_NoChange(t *testing.T) {
	result := CharDiff("hello world", "hello world")

	if result.HasChanges() {
		t.Error("expected no changes for identical strings")
	}
	if result.PrefixLen != 11 {
		t.Errorf("expected prefix length 11, got %d", result.PrefixLen)
	}
	if result.OldMiddle != "" || result.NewMiddle != "" {
		t.Errorf("expected empty middles, got old=%q new=%q", result.OldMiddle, result.NewMiddle)
	}
}

func TestCharDiff_MiddleChange(t *testing.T) {
	// Typical progress bar update
	old := "Progress: 45% complete"
	new := "Progress: 67% complete"

	result := CharDiff(old, new)

	if !result.HasChanges() {
		t.Error("expected changes")
	}
	if result.PrefixLen != 10 { // "Progress: "
		t.Errorf("expected prefix length 10, got %d", result.PrefixLen)
	}
	if result.OldMiddle != "45" {
		t.Errorf("expected old middle '45', got %q", result.OldMiddle)
	}
	if result.NewMiddle != "67" {
		t.Errorf("expected new middle '67', got %q", result.NewMiddle)
	}
	if result.SuffixLen != 10 { // "% complete"
		t.Errorf("expected suffix length 10, got %d", result.SuffixLen)
	}
}

func TestCharDiff_PrefixOnlyChange(t *testing.T) {
	old := "ERROR: something failed"
	new := "WARN: something failed"

	result := CharDiff(old, new)

	if result.PrefixLen != 0 {
		t.Errorf("expected prefix length 0, got %d", result.PrefixLen)
	}
	if result.OldMiddle != "ERROR" {
		t.Errorf("expected old middle 'ERROR', got %q", result.OldMiddle)
	}
	if result.NewMiddle != "WARN" {
		t.Errorf("expected new middle 'WARN', got %q", result.NewMiddle)
	}
	if result.SuffixLen != 18 { // ": something failed"
		t.Errorf("expected suffix length 18, got %d", result.SuffixLen)
	}
}

func TestCharDiff_SuffixOnlyChange(t *testing.T) {
	old := "Status: running"
	new := "Status: stopped"

	result := CharDiff(old, new)

	if result.PrefixLen != 8 { // "Status: "
		t.Errorf("expected prefix length 8, got %d", result.PrefixLen)
	}
	if result.OldMiddle != "running" {
		t.Errorf("expected old middle 'running', got %q", result.OldMiddle)
	}
	if result.NewMiddle != "stopped" {
		t.Errorf("expected new middle 'stopped', got %q", result.NewMiddle)
	}
	if result.SuffixLen != 0 {
		t.Errorf("expected suffix length 0, got %d", result.SuffixLen)
	}
}

func TestCharDiff_CompleteChange(t *testing.T) {
	old := "abc"
	new := "xyz"

	result := CharDiff(old, new)

	if result.PrefixLen != 0 {
		t.Errorf("expected prefix length 0, got %d", result.PrefixLen)
	}
	if result.OldMiddle != "abc" {
		t.Errorf("expected old middle 'abc', got %q", result.OldMiddle)
	}
	if result.NewMiddle != "xyz" {
		t.Errorf("expected new middle 'xyz', got %q", result.NewMiddle)
	}
	if result.SuffixLen != 0 {
		t.Errorf("expected suffix length 0, got %d", result.SuffixLen)
	}
}

func TestCharDiff_EmptyStrings(t *testing.T) {
	tests := []struct {
		name      string
		old, new  string
		prefixLen int
		oldMiddle string
		newMiddle string
		suffixLen int
	}{
		{"both empty", "", "", 0, "", "", 0},
		{"old empty", "", "hello", 0, "", "hello", 0},
		{"new empty", "hello", "", 0, "hello", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CharDiff(tt.old, tt.new)
			if result.PrefixLen != tt.prefixLen {
				t.Errorf("prefix: got %d, want %d", result.PrefixLen, tt.prefixLen)
			}
			if result.OldMiddle != tt.oldMiddle {
				t.Errorf("old middle: got %q, want %q", result.OldMiddle, tt.oldMiddle)
			}
			if result.NewMiddle != tt.newMiddle {
				t.Errorf("new middle: got %q, want %q", result.NewMiddle, tt.newMiddle)
			}
			if result.SuffixLen != tt.suffixLen {
				t.Errorf("suffix: got %d, want %d", result.SuffixLen, tt.suffixLen)
			}
		})
	}
}

func TestCharDiff_OneCharChange(t *testing.T) {
	old := "test1"
	new := "test2"

	result := CharDiff(old, new)

	if result.PrefixLen != 4 { // "test"
		t.Errorf("expected prefix length 4, got %d", result.PrefixLen)
	}
	if result.OldMiddle != "1" {
		t.Errorf("expected old middle '1', got %q", result.OldMiddle)
	}
	if result.NewMiddle != "2" {
		t.Errorf("expected new middle '2', got %q", result.NewMiddle)
	}
	if result.SuffixLen != 0 {
		t.Errorf("expected suffix length 0, got %d", result.SuffixLen)
	}
}

func TestCharDiff_DifferentLengths(t *testing.T) {
	old := "short"
	new := "shorter"

	result := CharDiff(old, new)

	// "short" is a prefix of "shorter"
	if result.PrefixLen != 5 { // "short"
		t.Errorf("expected prefix length 5, got %d", result.PrefixLen)
	}
	if result.OldMiddle != "" {
		t.Errorf("expected empty old middle, got %q", result.OldMiddle)
	}
	if result.NewMiddle != "er" {
		t.Errorf("expected new middle 'er', got %q", result.NewMiddle)
	}
}

func TestCharDiff_ChangedRange(t *testing.T) {
	old := "Hello World!"
	new := "Hello Earth!"

	result := CharDiff(old, new)
	start, end := result.ChangedRange()

	// "Hello " is prefix (6), "World" is old middle, "!" is suffix
	if start != 6 {
		t.Errorf("expected start 6, got %d", start)
	}
	if end != 11 { // 6 + len("World")
		t.Errorf("expected end 11, got %d", end)
	}

	// Verify the range is correct
	if old[start:end] != "World" {
		t.Errorf("expected range to be 'World', got %q", old[start:end])
	}
}

func TestCommonPrefixLen(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"hello", "hello", 5},
		{"hello", "help", 3},
		{"abc", "xyz", 0},
		{"", "hello", 0},
		{"hello", "", 0},
		{"prefix123", "prefix456", 6},
	}

	for _, tt := range tests {
		result := commonPrefixLen(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("commonPrefixLen(%q, %q) = %d, want %d", tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestCommonSuffixLen(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"hello", "hello", 5},
		{"world", "bold", 2}, // "ld" (w-o-r-l-d vs b-o-l-d)
		{"abc", "xyz", 0},
		{"", "hello", 0},
		{"hello", "", 0},
		{"123suffix", "456suffix", 6},
	}

	for _, tt := range tests {
		result := commonSuffixLen(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("commonSuffixLen(%q, %q) = %d, want %d", tt.a, tt.b, result, tt.expected)
		}
	}
}

// Test existing diff functions

func TestFindOverlap_ExactMatch(t *testing.T) {
	prev := []string{"a", "b", "c", "d"}
	next := []string{"c", "d", "e", "f"}

	k, score := FindOverlap(prev, next)

	if k != 2 {
		t.Errorf("expected overlap of 2, got %d", k)
	}
	if score != 1.0 {
		t.Errorf("expected score 1.0, got %f", score)
	}
}

func TestFindOverlap_NoOverlap(t *testing.T) {
	prev := []string{"a", "b", "c"}
	next := []string{"x", "y", "z"}

	k, score := FindOverlap(prev, next)

	if k != 0 {
		t.Errorf("expected no overlap, got %d", k)
	}
	if score != 0.0 {
		t.Errorf("expected score 0.0, got %f", score)
	}
}

func TestFindChangedLines(t *testing.T) {
	prev := []string{"a", "b", "c", "d"}
	next := []string{"a", "X", "c", "Y"}

	changed := FindChangedLines(prev, next)

	if len(changed) != 2 {
		t.Errorf("expected 2 changed lines, got %d", len(changed))
	}
	if changed[0] != 1 || changed[1] != 3 {
		t.Errorf("expected changes at indices [1, 3], got %v", changed)
	}
}

func TestNormalize(t *testing.T) {
	lines := []string{"hello   ", "world\t\t", "test\r"}
	normalized := Normalize(lines)

	expected := []string{"hello", "world", "test"}
	for i, want := range expected {
		if normalized[i] != want {
			t.Errorf("normalized[%d] = %q, want %q", i, normalized[i], want)
		}
	}
}
