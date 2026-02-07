package cmd

import (
	"testing"
)

func TestFormatOverrides(t *testing.T) {
	// Empty overrides
	got := formatOverridesPlain(nil)
	if got != "(none)" {
		t.Errorf("empty overrides: got %q, want %q", got, "(none)")
	}

	// Single override
	got = formatOverridesPlain([]string{"crew"})
	if got != "[crew]" {
		t.Errorf("single override: got %q, want %q", got, "[crew]")
	}

	// Multiple overrides
	got = formatOverridesPlain([]string{"crew", "gastown/crew"})
	if got != "[crew, gastown/crew]" {
		t.Errorf("multiple overrides: got %q, want %q", got, "[crew, gastown/crew]")
	}
}

func TestFormatOverridesStyled(t *testing.T) {
	// Styled version of empty should contain "(none)" text
	got := formatOverrides(nil)
	if len(got) == 0 {
		t.Error("styled empty overrides should not be empty string")
	}

	// Non-empty should just be brackets with no ANSI
	got = formatOverrides([]string{"crew"})
	if got != "[crew]" {
		t.Errorf("non-empty overrides should have no ANSI: got %q", got)
	}
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		s     string
		width int
		want  string
	}{
		{"abc", 5, "abc  "},
		{"abc", 3, "abc"},
		{"abc", 2, "abc"},
		{"", 3, "   "},
	}

	for _, tt := range tests {
		got := padRight(tt.s, tt.width)
		if got != tt.want {
			t.Errorf("padRight(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.want)
		}
	}
}

func TestRenderSyncStatus(t *testing.T) {
	tests := []struct {
		status string
		empty  bool
	}{
		{"in sync", false},
		{"out of sync", false},
		{"missing", false},
		{"error", false},
		{"unknown", false},
	}

	for _, tt := range tests {
		got := renderSyncStatus(tt.status)
		if (len(got) == 0) != tt.empty {
			t.Errorf("renderSyncStatus(%q) empty=%v, want empty=%v", tt.status, len(got) == 0, tt.empty)
		}
	}
}

func TestBuildTargetInfoMissingFile(t *testing.T) {
	// Use a non-existent path
	target := struct {
		status string
	}{
		status: "missing",
	}

	// Can't easily test buildTargetInfo directly without a real workspace,
	// but we can verify the status rendering
	got := renderSyncStatus(target.status)
	if len(got) == 0 {
		t.Error("missing status should render non-empty")
	}
}
