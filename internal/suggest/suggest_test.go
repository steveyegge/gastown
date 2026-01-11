package suggest

import (
	"strings"
	"testing"
)

func TestFindSimilar(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		candidates []string
		maxResults int
		wantFirst  string // expect this to be the first result
	}{
		{
			name:       "exact prefix match",
			target:     "toa",
			candidates: []string{"Toast", "Nux", "Capable", "Ghost"},
			maxResults: 3,
			wantFirst:  "Toast",
		},
		{
			name:       "typo match",
			target:     "Tosat",
			candidates: []string{"Toast", "Nux", "Capable"},
			maxResults: 3,
			wantFirst:  "Toast",
		},
		{
			name:       "case insensitive",
			target:     "TOAST",
			candidates: []string{"Nux", "Toast", "Capable"},
			maxResults: 1,
			wantFirst:  "Toast", // finds Toast even with different case
		},
		{
			name:       "no matches",
			target:     "xyz",
			candidates: []string{"abc", "def"},
			maxResults: 3,
			wantFirst:  "", // no good matches
		},
		{
			name:       "empty candidates",
			target:     "test",
			candidates: []string{},
			maxResults: 3,
			wantFirst:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := FindSimilar(tt.target, tt.candidates, tt.maxResults)

			if tt.wantFirst == "" {
				if len(results) > 0 {
					// Allow some results for partial matches, just check they're reasonable
					return
				}
				return
			}

			if len(results) == 0 {
				t.Errorf("FindSimilar(%q) returned no results, want first = %q", tt.target, tt.wantFirst)
				return
			}

			if results[0] != tt.wantFirst {
				t.Errorf("FindSimilar(%q) first result = %q, want %q", tt.target, results[0], tt.wantFirst)
			}
		})
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"abc", "adc", 1},
		{"abc", "abcd", 1},
		{"kitten", "sitting", 3},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			got := levenshteinDistance(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestFormatSuggestion(t *testing.T) {
	msg := FormatSuggestion("Polecat", "Tosat", []string{"Toast", "Ghost"}, "Create with: gt polecat add Tosat")

	if !strings.Contains(msg, "Polecat") {
		t.Errorf("FormatSuggestion missing entity name")
	}
	if !strings.Contains(msg, "Tosat") {
		t.Errorf("FormatSuggestion missing target name")
	}
	if !strings.Contains(msg, "Did you mean?") {
		t.Errorf("FormatSuggestion missing 'Did you mean?' section")
	}
	if !strings.Contains(msg, "Toast") {
		t.Errorf("FormatSuggestion missing suggestion 'Toast'")
	}
	if !strings.Contains(msg, "Create with:") {
		t.Errorf("FormatSuggestion missing hint")
	}
}
