package doltserver

import (
	"strings"
	"testing"
)

func TestParseSimpleCSV_Empty(t *testing.T) {
	t.Parallel()
	got := parseSimpleCSV("")
	if got != nil {
		t.Errorf("parseSimpleCSV(\"\") = %v, want nil", got)
	}
}

func TestParseSimpleCSV_HeaderOnly(t *testing.T) {
	t.Parallel()
	got := parseSimpleCSV("id,title,status\n")
	if got != nil {
		t.Errorf("parseSimpleCSV(header-only) = %v, want nil", got)
	}
}

func TestParseSimpleCSV_SingleRow(t *testing.T) {
	t.Parallel()
	data := "id,title,status\nw-abc,Fix bug,open"
	got := parseSimpleCSV(data)
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	if got[0]["id"] != "w-abc" {
		t.Errorf("id = %q, want %q", got[0]["id"], "w-abc")
	}
	if got[0]["title"] != "Fix bug" {
		t.Errorf("title = %q, want %q", got[0]["title"], "Fix bug")
	}
	if got[0]["status"] != "open" {
		t.Errorf("status = %q, want %q", got[0]["status"], "open")
	}
}

func TestParseSimpleCSV_MultiRow(t *testing.T) {
	t.Parallel()
	data := "id,title\nw-1,First\nw-2,Second\nw-3,Third"
	got := parseSimpleCSV(data)
	if len(got) != 3 {
		t.Fatalf("got %d rows, want 3", len(got))
	}
	for i, wantID := range []string{"w-1", "w-2", "w-3"} {
		if got[i]["id"] != wantID {
			t.Errorf("row %d id = %q, want %q", i, got[i]["id"], wantID)
		}
	}
}

func TestParseSimpleCSV_MissingFields(t *testing.T) {
	t.Parallel()
	data := "id,title,status\nw-abc,Fix bug"
	got := parseSimpleCSV(data)
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	// Missing 'status' field should not be present in map
	if _, ok := got[0]["status"]; ok {
		t.Error("expected missing 'status' field to not be present")
	}
}

func TestParseSimpleCSV_SkipsBlankLines(t *testing.T) {
	t.Parallel()
	data := "id,title\nw-1,First\n\nw-2,Second\n"
	got := parseSimpleCSV(data)
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}
}

func TestParseSimpleCSV_TrimsWhitespace(t *testing.T) {
	t.Parallel()
	data := " id , title \n w-abc , Fix bug "
	got := parseSimpleCSV(data)
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	if got[0]["id"] != "w-abc" {
		t.Errorf("id = %q, want %q", got[0]["id"], "w-abc")
	}
	if got[0]["title"] != "Fix bug" {
		t.Errorf("title = %q, want %q", got[0]["title"], "Fix bug")
	}
}

func TestEscapeSQL_SingleQuotes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"it's", "it''s"},
		{"", ""},
		{"'; DROP TABLE wanted;--", "''; DROP TABLE wanted;--"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := EscapeSQL(tt.input)
			if got != tt.want {
				t.Errorf("EscapeSQL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEscapeSQL_Backslashes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{`path\to\file`, `path\\to\\file`},
		{`trailing\`, `trailing\\`},
		{`it\'s`, `it\\''s`},
		{`no special`, `no special`},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := EscapeSQL(tt.input)
			if got != tt.want {
				t.Errorf("EscapeSQL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGenerateWantedID_Format(t *testing.T) {
	t.Parallel()
	id := GenerateWantedID("Test Title")
	if !strings.HasPrefix(id, "w-") {
		t.Errorf("GenerateWantedID() = %q, want prefix 'w-'", id)
	}
	// "w-" + 10 hex chars = 12 chars total
	if len(id) != 12 {
		t.Errorf("GenerateWantedID() length = %d, want 12", len(id))
	}
	// Verify hex chars after prefix
	hexPart := id[2:]
	for _, c := range hexPart {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("GenerateWantedID() contains non-hex char %q in %q", string(c), id)
		}
	}
}

func TestGenerateWantedID_Uniqueness(t *testing.T) {
	t.Parallel()
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := GenerateWantedID("Same Title")
		if seen[id] {
			t.Fatalf("duplicate ID generated: %s", id)
		}
		seen[id] = true
	}
}
