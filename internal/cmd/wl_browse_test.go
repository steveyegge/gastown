package cmd

import (
	"strings"
	"testing"
)

func TestWlParseCSV_Empty(t *testing.T) {
	t.Parallel()
	got := wlParseCSV("")
	if len(got) != 0 {
		t.Errorf("wlParseCSV(\"\") = %v, want empty", got)
	}
}

func TestWlParseCSV_SingleRow(t *testing.T) {
	t.Parallel()
	got := wlParseCSV("id,title\nw-abc,Fix bug")
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2 (header + data)", len(got))
	}
	if got[0][0] != "id" || got[0][1] != "title" {
		t.Errorf("header = %v, want [id title]", got[0])
	}
	if got[1][0] != "w-abc" || got[1][1] != "Fix bug" {
		t.Errorf("data = %v, want [w-abc Fix bug]", got[1])
	}
}

func TestWlParseCSV_QuotedFields(t *testing.T) {
	t.Parallel()
	got := wlParseCSV("id,title\nw-1,\"Hello, World\"")
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}
	if got[1][1] != "Hello, World" {
		t.Errorf("quoted field = %q, want %q", got[1][1], "Hello, World")
	}
}

func TestWlParseCSV_SkipsBlankLines(t *testing.T) {
	t.Parallel()
	got := wlParseCSV("id\nw-1\n\nw-2\n")
	if len(got) != 3 {
		t.Fatalf("got %d rows, want 3", len(got))
	}
}

func TestWlParseCSVLine_Simple(t *testing.T) {
	t.Parallel()
	got := wlParseCSVLine("a,b,c")
	if len(got) != 3 {
		t.Fatalf("got %d fields, want 3", len(got))
	}
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("fields = %v, want [a b c]", got)
	}
}

func TestWlParseCSVLine_QuotedComma(t *testing.T) {
	t.Parallel()
	got := wlParseCSVLine(`"hello, world",b`)
	if len(got) != 2 {
		t.Fatalf("got %d fields, want 2", len(got))
	}
	if got[0] != "hello, world" {
		t.Errorf("field[0] = %q, want %q", got[0], "hello, world")
	}
}

func TestWlParseCSVLine_EscapedQuotes(t *testing.T) {
	t.Parallel()
	got := wlParseCSVLine(`"say ""hello""",b`)
	if len(got) != 2 {
		t.Fatalf("got %d fields, want 2", len(got))
	}
	if got[0] != `say "hello"` {
		t.Errorf("field[0] = %q, want %q", got[0], `say "hello"`)
	}
}

func TestWlParseCSVLine_TrailingComma(t *testing.T) {
	t.Parallel()
	got := wlParseCSVLine("a,b,")
	if len(got) != 3 {
		t.Fatalf("got %d fields, want 3", len(got))
	}
	if got[2] != "" {
		t.Errorf("field[2] = %q, want empty", got[2])
	}
}

func TestWlParseCSVLine_EmptyFields(t *testing.T) {
	t.Parallel()
	got := wlParseCSVLine(",,")
	if len(got) != 3 {
		t.Fatalf("got %d fields, want 3", len(got))
	}
	for i, f := range got {
		if f != "" {
			t.Errorf("field[%d] = %q, want empty", i, f)
		}
	}
}

func TestWlFormatPriority(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"0", "P0"},
		{"1", "P1"},
		{"2", "P2"},
		{"3", "P3"},
		{"4", "P4"},
		{"5", "5"},
		{"", ""},
		{"high", "high"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := wlFormatPriority(tt.input)
			if got != tt.want {
				t.Errorf("wlFormatPriority(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildBrowseQuery_DefaultFilters(t *testing.T) {
	t.Parallel()
	f := BrowseFilter{
		Status:   "open",
		Priority: -1,
		Limit:    50,
	}
	got := buildBrowseQuery(f)
	want := "SELECT id, title, project, type, priority, posted_by, status, effort_level FROM wanted WHERE status = 'open' ORDER BY priority ASC, created_at DESC LIMIT 50"
	if got != want {
		t.Errorf("buildBrowseQuery(default) =\n  %q\nwant\n  %q", got, want)
	}
}

func TestBuildBrowseQuery_AllFilters(t *testing.T) {
	t.Parallel()
	f := BrowseFilter{
		Status:   "open",
		Project:  "gastown",
		Type:     "bug",
		Priority: 0,
		Limit:    5,
	}
	got := buildBrowseQuery(f)
	// All four conditions should be present
	for _, substr := range []string{
		"status = 'open'",
		"project = 'gastown'",
		"type = 'bug'",
		"priority = 0",
		"LIMIT 5",
	} {
		if !strings.Contains(got, substr) {
			t.Errorf("buildBrowseQuery(all) missing %q in %q", substr, got)
		}
	}
}

func TestBuildBrowseQuery_NoFilters(t *testing.T) {
	t.Parallel()
	f := BrowseFilter{
		Priority: -1,
		Limit:    50,
	}
	got := buildBrowseQuery(f)
	if strings.Contains(got, "WHERE") {
		t.Errorf("buildBrowseQuery(none) should not have WHERE clause: %q", got)
	}
}

func TestBuildBrowseQuery_EscapesSQL(t *testing.T) {
	t.Parallel()
	f := BrowseFilter{
		Status:   "it's",
		Priority: -1,
		Limit:    50,
	}
	got := buildBrowseQuery(f)
	if !strings.Contains(got, "it''s") {
		t.Errorf("buildBrowseQuery should escape single quotes: %q", got)
	}
}

