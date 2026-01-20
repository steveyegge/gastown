package cmd

import (
	"path/filepath"
	"testing"
)

func TestExpandOutputPath(t *testing.T) {
	tests := []struct {
		name      string
		directory string
		pattern   string
		reviewID  string
		legID     string
		want      string
	}{
		{
			name:      "basic expansion",
			directory: ".reviews/{{review_id}}",
			pattern:   "{{leg.id}}-findings.md",
			reviewID:  "abc123",
			legID:     "security",
			want:      ".reviews/abc123/security-findings.md",
		},
		{
			name:      "no templates",
			directory: ".output",
			pattern:   "results.md",
			reviewID:  "xyz",
			legID:     "test",
			want:      ".output/results.md",
		},
		{
			name:      "complex path",
			directory: "reviews/{{review_id}}/findings",
			pattern:   "leg-{{leg.id}}-analysis.md",
			reviewID:  "pr-123",
			legID:     "performance",
			want:      "reviews/pr-123/findings/leg-performance-analysis.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandOutputPath(tt.directory, tt.pattern, tt.reviewID, tt.legID)
			if filepath.ToSlash(got) != tt.want {
				t.Errorf("expandOutputPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLegOutput(t *testing.T) {
	// Test LegOutput struct
	output := LegOutput{
		LegID:    "correctness",
		Title:    "Correctness Review",
		Status:   "closed",
		FilePath: "/tmp/findings.md",
		Content:  "## Findings\n\nNo issues found.",
		HasFile:  true,
	}

	if output.LegID != "correctness" {
		t.Errorf("LegID = %q, want %q", output.LegID, "correctness")
	}

	if output.Status != "closed" {
		t.Errorf("Status = %q, want %q", output.Status, "closed")
	}

	if !output.HasFile {
		t.Error("HasFile should be true")
	}
}

func TestConvoyMeta(t *testing.T) {
	// Test ConvoyMeta struct
	meta := ConvoyMeta{
		ID:        "hq-cv-abc",
		Title:     "Code Review: PR #123",
		Status:    "open",
		Formula:   "code-review",
		ReviewID:  "pr123",
		LegIssues: []string{"gt-leg1", "gt-leg2", "gt-leg3"},
	}

	if meta.ID != "hq-cv-abc" {
		t.Errorf("ID = %q, want %q", meta.ID, "hq-cv-abc")
	}

	if len(meta.LegIssues) != 3 {
		t.Errorf("len(LegIssues) = %d, want 3", len(meta.LegIssues))
	}
}
