package rally

import (
	"strings"
	"testing"
)

func TestNominationValidate_Valid(t *testing.T) {
	n := &Nomination{
		Category: "practice",
		Title:    "Enable tmux mouse support",
		Summary:  "Add 'setw -g mouse on' to ~/.tmux.conf for mouse wheel scrolling",
		Tags:     []string{"tmux", "developer-tools", "terminal"},
	}
	if err := n.Validate(); err != nil {
		t.Errorf("expected valid nomination, got error: %v", err)
	}
}

func TestNominationValidate_MissingCategory(t *testing.T) {
	n := &Nomination{Title: "foo", Summary: "bar"}
	if err := n.Validate(); err == nil {
		t.Error("expected error for missing category")
	}
}

func TestNominationValidate_BadCategory(t *testing.T) {
	n := &Nomination{Category: "opinion", Title: "foo", Summary: "bar"}
	if err := n.Validate(); err == nil {
		t.Error("expected error for invalid category")
	}
}

func TestNominationValidate_MissingTitle(t *testing.T) {
	n := &Nomination{Category: "practice", Summary: "bar"}
	if err := n.Validate(); err == nil {
		t.Error("expected error for missing title")
	}
}

func TestNominationValidate_MissingSummary(t *testing.T) {
	n := &Nomination{Category: "practice", Title: "foo"}
	if err := n.Validate(); err == nil {
		t.Error("expected error for missing summary")
	}
}

func TestNominationValidate_TitleTooLong(t *testing.T) {
	n := &Nomination{
		Category: "learned",
		Title:    strings.Repeat("x", 121),
		Summary:  "bar",
	}
	if err := n.Validate(); err == nil {
		t.Error("expected error for title > 120 chars")
	}
}

func TestNominationWireFormat_RoundTrip(t *testing.T) {
	n := &Nomination{
		Category:     "practice",
		Title:        "Enable tmux mouse support",
		Summary:      "Add 'setw -g mouse on' to ~/.tmux.conf for mouse wheel scrolling",
		Tags:         []string{"tmux", "developer-tools", "terminal"},
		CodebaseType: "general",
		NominatedBy:  "gastown/polecat-sherpa",
		NominatedAt:  "2026-03-12T14:22:05Z",
		NominationID: "nom-a3f9c2",
	}

	body, err := n.ToMailBody()
	if err != nil {
		t.Fatalf("ToMailBody: %v", err)
	}

	if !strings.HasPrefix(body, "RALLY_NOMINATION_V1\n---\n") {
		t.Errorf("missing sentinel in mail body")
	}

	parsed, err := ParseNominationFromMailBody(body)
	if err != nil {
		t.Fatalf("ParseNominationFromMailBody: %v", err)
	}

	if parsed.Title != n.Title {
		t.Errorf("title mismatch: got %q, want %q", parsed.Title, n.Title)
	}
	if parsed.Category != n.Category {
		t.Errorf("category mismatch: got %q, want %q", parsed.Category, n.Category)
	}
	if parsed.NominationID != n.NominationID {
		t.Errorf("nomination_id mismatch: got %q, want %q", parsed.NominationID, n.NominationID)
	}
	if len(parsed.Tags) != len(n.Tags) {
		t.Errorf("tags mismatch: got %v, want %v", parsed.Tags, n.Tags)
	}
}

func TestParseNominationFromMailBody_BadSentinel(t *testing.T) {
	_, err := ParseNominationFromMailBody("not a nomination")
	if err == nil {
		t.Error("expected error for missing sentinel")
	}
}

func TestNominationToKnowledgeEntry(t *testing.T) {
	n := &Nomination{
		Category:     "practice",
		Title:        "Enable tmux mouse support",
		Summary:      "Add 'setw -g mouse on' to ~/.tmux.conf for mouse wheel scrolling",
		Tags:         []string{"tmux", "developer-tools"},
		NominationID: "nom-a3f9c2",
		NominatedBy:  "gastown/polecat-sherpa",
	}

	e := n.ToKnowledgeEntry()

	if e.ID != "enable-tmux-mouse-support-a3f9c2" {
		t.Errorf("unexpected ID: %q", e.ID)
	}
	if e.Title != n.Title {
		t.Errorf("title mismatch: got %q", e.Title)
	}
	if e.Kind != "practice" {
		t.Errorf("kind mismatch: got %q", e.Kind)
	}
}

func TestTitleToSlug(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Enable tmux mouse support", "enable-tmux-mouse-support"},
		{"Swift 6: NSMutableArray box", "swift-6-nsmutablearray-box"},
		{"  leading spaces  ", "leading-spaces"},
		{"foo--bar", "foo-bar"},
	}
	for _, c := range cases {
		got := titleToSlug(c.in)
		if got != c.want {
			t.Errorf("titleToSlug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGenerateNominationID(t *testing.T) {
	id := GenerateNominationID()
	if !strings.HasPrefix(id, "nom-") {
		t.Errorf("expected nom- prefix, got %q", id)
	}
	if len(id) != 10 { // "nom-" + 6 hex chars
		t.Errorf("expected length 10, got %d: %q", len(id), id)
	}
}
