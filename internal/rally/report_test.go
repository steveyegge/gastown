package rally

import (
	"strings"
	"testing"
)

func TestReportValidate_Valid(t *testing.T) {
	cases := []struct {
		name string
		r    Report
	}{
		{"stale with reason", Report{EntryID: "gas-town-upgrade-sequence", Kind: ReportKindStale, Reason: "brew no longer used for beads"}},
		{"wrong with reason", Report{EntryID: "gas-town-dolt-query-direct", Kind: ReportKindWrong, Reason: "port changed to 3308"}},
		{"improve with improvement", Report{EntryTag: "dolt", Kind: ReportKindImprove, Improvement: "add example for --no-tls flag"}},
		{"verify", Report{EntryID: "tmux-mouse-support", Kind: ReportKindVerify}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.r.Validate(); err != nil {
				t.Errorf("expected valid, got: %v", err)
			}
		})
	}
}

func TestReportValidate_MissingTarget(t *testing.T) {
	r := Report{Kind: ReportKindVerify}
	if err := r.Validate(); err == nil {
		t.Error("expected error for missing entry_id and entry_tag")
	}
}

func TestReportValidate_BadKind(t *testing.T) {
	r := Report{EntryID: "foo", Kind: "opinion"}
	if err := r.Validate(); err == nil {
		t.Error("expected error for invalid kind")
	}
}

func TestReportValidate_StaleRequiresReason(t *testing.T) {
	r := Report{EntryID: "foo", Kind: ReportKindStale}
	if err := r.Validate(); err == nil {
		t.Error("expected error for stale without reason")
	}
}

func TestReportValidate_ImproveRequiresImprovement(t *testing.T) {
	r := Report{EntryID: "foo", Kind: ReportKindImprove}
	if err := r.Validate(); err == nil {
		t.Error("expected error for improve without improvement text")
	}
}

func TestReportWireFormat_RoundTrip(t *testing.T) {
	r := &Report{
		EntryID:    "gas-town-upgrade-sequence",
		Kind:       ReportKindStale,
		Reason:     "beads is now installed via go install, not brew",
		ReportedBy: "gastown/polecat-capable",
		ReportedAt: "2026-03-12T18:00:00Z",
		ReportID:   "rpt-c4f2a1",
	}

	body, err := r.ToMailBody()
	if err != nil {
		t.Fatalf("ToMailBody: %v", err)
	}
	if !strings.HasPrefix(body, "RALLY_REPORT_V1\n---\n") {
		t.Error("missing sentinel")
	}

	parsed, err := ParseReportFromMailBody(body)
	if err != nil {
		t.Fatalf("ParseReportFromMailBody: %v", err)
	}
	if parsed.EntryID != r.EntryID {
		t.Errorf("entry_id mismatch: got %q", parsed.EntryID)
	}
	if parsed.Kind != r.Kind {
		t.Errorf("kind mismatch: got %q", parsed.Kind)
	}
	if parsed.Reason != r.Reason {
		t.Errorf("reason mismatch: got %q", parsed.Reason)
	}
}

func TestParseReportFromMailBody_BadSentinel(t *testing.T) {
	_, err := ParseReportFromMailBody("not a report")
	if err == nil {
		t.Error("expected error for missing sentinel")
	}
}

func TestReportSubjectLine(t *testing.T) {
	r := &Report{EntryID: "tmux-mouse-support", Kind: ReportKindVerify}
	want := "RALLY_REPORT: tmux-mouse-support [verify]"
	if got := r.SubjectLine(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSearchFiltersDeprecated(t *testing.T) {
	idx := &KnowledgeIndex{
		entries: []KnowledgeEntry{
			{ID: "old", Title: "Old Entry", Summary: "outdated", Tags: []string{"tmux"}, Deprecated: true},
			{ID: "new", Title: "New Entry", Summary: "current", Tags: []string{"tmux"}},
		},
	}
	results := idx.Search(SearchQuery{Tags: []string{"tmux"}})
	if len(results) != 1 {
		t.Fatalf("expected 1 result (deprecated filtered), got %d", len(results))
	}
	if results[0].ID != "new" {
		t.Errorf("expected 'new', got %q", results[0].ID)
	}
}
