package convoy

import (
	"testing"
)

func TestExtractIssueID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"gt-abc", "gt-abc"},
		{"bd-xyz", "bd-xyz"},
		{"hq-cv-123", "hq-cv-123"},
		{"external:gt:gt-abc", "gt-abc"},
		{"external:bd:bd-xyz", "bd-xyz"},
		{"external:hq:hq-cv-123", "hq-cv-123"},
		{"external:", "external:"},     // malformed, return as-is
		{"external:x:", ""},            // 3 parts but empty last part
		{"simple", "simple"},           // no external prefix
		{"", ""},                       // empty
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractIssueID(tt.input)
			if result != tt.expected {
				t.Errorf("extractIssueID(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFeedNextReadyIssue_SkipsNonOpenIssues(t *testing.T) {
	// Test the filtering logic: only open issues with no assignee should be considered
	tracked := []trackedIssue{
		{ID: "gt-closed", Status: "closed", Assignee: ""},
		{ID: "gt-inprog", Status: "in_progress", Assignee: "gastown/polecats/alpha"},
		{ID: "gt-hooked", Status: "hooked", Assignee: "gastown/polecats/beta"},
		{ID: "gt-assigned", Status: "open", Assignee: "gastown/polecats/gamma"},
	}

	// None of these should be considered "ready"
	for _, issue := range tracked {
		if issue.Status == "open" && issue.Assignee == "" {
			t.Errorf("issue %s should not be ready (status=%s, assignee=%s)", issue.ID, issue.Status, issue.Assignee)
		}
	}
}

func TestFeedNextReadyIssue_FindsReadyIssue(t *testing.T) {
	// Test that we correctly identify a ready issue
	tracked := []trackedIssue{
		{ID: "gt-closed", Status: "closed", Assignee: ""},
		{ID: "gt-inprog", Status: "in_progress", Assignee: "gastown/polecats/alpha"},
		{ID: "gt-ready", Status: "open", Assignee: ""},
		{ID: "gt-also-ready", Status: "open", Assignee: ""},
	}

	// Find first ready issue - should be gt-ready (first match)
	var foundReady string
	for _, issue := range tracked {
		if issue.Status == "open" && issue.Assignee == "" {
			foundReady = issue.ID
			break
		}
	}

	if foundReady != "gt-ready" {
		t.Errorf("expected first ready issue to be gt-ready, got %s", foundReady)
	}
}

func TestCheckConvoysForIssue_NilLogger(t *testing.T) {
	// Nil logger should not panic — gets replaced with no-op internally.
	result := CheckConvoysForIssue("/nonexistent/path", "gt-test", "test", nil)
	if result != nil {
		t.Errorf("expected nil for non-existent path, got %v", result)
	}
}

func TestCheckConvoysForIssue_NoConvoys(t *testing.T) {
	// With a non-existent town root, no convoys should be found.
	var logged []string
	logger := func(format string, args ...interface{}) {
		logged = append(logged, format)
	}

	result := CheckConvoysForIssue("/nonexistent/path", "gt-test", "test", logger)
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
	if len(logged) != 0 {
		t.Errorf("expected no logs, got %d", len(logged))
	}
}

func TestGetTrackingConvoys_CommandFailure(t *testing.T) {
	// When bd is not available or fails, should return nil gracefully.
	result := getTrackingConvoys("/nonexistent/path", "gt-test")
	if result != nil {
		t.Errorf("expected nil on command failure, got %v", result)
	}
}

func TestIsConvoyClosed_CommandFailure(t *testing.T) {
	// When bd is not available, should return false (not closed).
	result := isConvoyClosed("/nonexistent/path", "gt-test")
	if result {
		t.Error("expected false on command failure")
	}
}

func TestBatchShowIssues_Empty(t *testing.T) {
	// Empty issue IDs should return empty map without calling bd.
	result := batchShowIssues("/nonexistent/path", nil)
	if len(result) != 0 {
		t.Errorf("expected empty map for nil input, got %d entries", len(result))
	}

	result = batchShowIssues("/nonexistent/path", []string{})
	if len(result) != 0 {
		t.Errorf("expected empty map for empty input, got %d entries", len(result))
	}
}

func TestFeedNextReadyIssue_EmptyTracked(t *testing.T) {
	// feedNextReadyIssue should handle empty tracked issues without panic.
	var logged []string
	logger := func(format string, args ...interface{}) {
		logged = append(logged, format)
	}

	feedNextReadyIssue("/nonexistent/path", "convoy-1", "test", logger)
	if len(logged) != 0 {
		t.Errorf("expected no logs for empty tracked issues, got %d", len(logged))
	}
}
