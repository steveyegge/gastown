package refinery

import (
	"strings"
	"testing"

	gh "github.com/steveyegge/gastown/internal/github"
)

func TestExtractFieldKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "simple key", line: "convoy_status: awaiting_review", want: "convoy_status"},
		{name: "mixed case", line: "Convoy_Status: foo", want: "convoy_status"},
		{name: "with leading spaces", line: "  pr_number: 42", want: "pr_number"},
		{name: "no colon", line: "just a line", want: ""},
		{name: "empty string", line: "", want: ""},
		{name: "colon at start", line: ": value", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractFieldKey(tt.line)
			if got != tt.want {
				t.Errorf("extractFieldKey(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestReplaceMetadataFields_ReplacesExisting(t *testing.T) {
	t.Parallel()
	desc := "convoy_status: awaiting_review\npr_number: 42\nsome text"
	result := replaceMetadataFields(desc, map[string]string{
		"convoy_status": "approved",
	})
	if !strings.Contains(result, "convoy_status: approved") {
		t.Errorf("expected convoy_status: approved, got:\n%s", result)
	}
	if !strings.Contains(result, "pr_number: 42") {
		t.Errorf("expected pr_number: 42 preserved, got:\n%s", result)
	}
}

func TestReplaceMetadataFields_AppendsNew(t *testing.T) {
	t.Parallel()
	desc := "convoy_status: awaiting_review"
	result := replaceMetadataFields(desc, map[string]string{
		"review_approved_at": "2026-01-01T00:00:00Z",
	})
	if !strings.Contains(result, "convoy_status: awaiting_review") {
		t.Errorf("expected original field preserved, got:\n%s", result)
	}
	if !strings.Contains(result, "review_approved_at: 2026-01-01T00:00:00Z") {
		t.Errorf("expected new field appended, got:\n%s", result)
	}
}

func TestReplaceMetadataFields_MultipleFields(t *testing.T) {
	t.Parallel()
	desc := "convoy_status: awaiting_review\npr_number: 42"
	result := replaceMetadataFields(desc, map[string]string{
		"convoy_status":     "changes_requested",
		"review_changes_at": "2026-01-01T00:00:00Z",
	})
	if !strings.Contains(result, "convoy_status: changes_requested") {
		t.Errorf("expected convoy_status replaced, got:\n%s", result)
	}
	if !strings.Contains(result, "pr_number: 42") {
		t.Errorf("expected pr_number preserved, got:\n%s", result)
	}
	if !strings.Contains(result, "review_changes_at: 2026-01-01T00:00:00Z") {
		t.Errorf("expected new field appended, got:\n%s", result)
	}
}

func TestReplaceMetadataFields_EmptyDescription(t *testing.T) {
	t.Parallel()
	result := replaceMetadataFields("", map[string]string{
		"convoy_status": "approved",
	})
	if !strings.Contains(result, "convoy_status: approved") {
		t.Errorf("expected field in result, got: %q", result)
	}
}

func TestFormatReviewComments_Empty(t *testing.T) {
	t.Parallel()
	got := formatReviewComments(nil)
	if got != "(no comments)" {
		t.Errorf("formatReviewComments(nil) = %q, want %q", got, "(no comments)")
	}

	got = formatReviewComments([]gh.ReviewComment{})
	if got != "(no comments)" {
		t.Errorf("formatReviewComments([]) = %q, want %q", got, "(no comments)")
	}
}

func TestFormatReviewComments_SingleComment(t *testing.T) {
	t.Parallel()
	comments := []gh.ReviewComment{
		{User: "alice", Path: "main.go", Line: 10, Body: "Fix this typo"},
	}
	got := formatReviewComments(comments)
	if !strings.Contains(got, "alice") {
		t.Errorf("missing user in output: %s", got)
	}
	if !strings.Contains(got, "main.go") {
		t.Errorf("missing path in output: %s", got)
	}
	if !strings.Contains(got, "Fix this typo") {
		t.Errorf("missing body in output: %s", got)
	}
}

func TestFormatReviewComments_MultipleComments(t *testing.T) {
	t.Parallel()
	comments := []gh.ReviewComment{
		{User: "alice", Path: "main.go", Line: 10, Body: "Fix typo"},
		{User: "bob", Path: "util.go", Line: 20, Body: "Add error handling"},
	}
	got := formatReviewComments(comments)
	if !strings.Contains(got, "alice") || !strings.Contains(got, "bob") {
		t.Errorf("missing users in output: %s", got)
	}
	if !strings.Contains(got, "main.go") || !strings.Contains(got, "util.go") {
		t.Errorf("missing paths in output: %s", got)
	}
}

func TestFindRigNameWithRefinery_ReturnsBaseName(t *testing.T) {
	t.Parallel()
	// findRigNameWithRefinery wraps findRigWithRefinery and returns filepath.Base.
	// When no rig exists, returns empty string.
	townRoot := t.TempDir()
	got := findRigNameWithRefinery(townRoot)
	if got != "" {
		t.Errorf("findRigNameWithRefinery(empty) = %q, want empty", got)
	}
}
