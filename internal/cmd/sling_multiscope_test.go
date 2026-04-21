package cmd

import "testing"

func TestIsMultiScopeBead(t *testing.T) {
	tests := []struct {
		name        string
		description string
		wantMatch   bool
	}{
		{
			name:        "empty description",
			description: "",
			wantMatch:   false,
		},
		{
			name:        "single-scope bead",
			description: "Install pre-commit in polecat spawn template.",
			wantMatch:   false,
		},
		{
			name:        "narrative mention of part A (no colon)",
			description: "Part A landed in PR #157. This bead covers the rest.",
			wantMatch:   false,
		},
		{
			name:        "declarative Part A: and Part B:",
			description: "Part A: update CLAUDE.md conventions.\nPart B: install pre-commit in spawn template.",
			wantMatch:   true,
		},
		{
			name:        "parenthetical scope markers",
			description: "Part A (app repo): add conventions.\nPart B (Gas Town infra): add spawn hook.",
			wantMatch:   true,
		},
		{
			name:        "em-dash separator",
			description: "Part A — docs.\nPart B — code.",
			wantMatch:   true,
		},
		{
			name:        "case-insensitive match",
			description: "PART A: first.\nPART B: second.",
			wantMatch:   true,
		},
		{
			name:        "only part B with scope marker",
			description: "Part B: the leftover scope from the previous bead.",
			wantMatch:   false,
		},
		{
			name:        "three-part split",
			description: "Part B: second task.\nPart C: third task.",
			wantMatch:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := &beadInfo{Description: tc.description}
			got, reason := isMultiScopeBead(info)
			if got != tc.wantMatch {
				t.Errorf("isMultiScopeBead() = %v, want %v (reason=%q)", got, tc.wantMatch, reason)
			}
			if got && reason == "" {
				t.Errorf("expected non-empty reason when match is true")
			}
		})
	}
}
