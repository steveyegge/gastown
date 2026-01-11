package cmd

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"1h", time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"24h", 24 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"2s", 2 * time.Second, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseDuration(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("parseDuration(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got != tt.expected {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestExtractAuthorName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"gastown/crew/joe", "joe"},
		{"gastown/polecats/toast", "toast"},
		{"mayor", "mayor"},
		{"gastown/witness", "witness"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractAuthorName(tt.input)
			if got != tt.expected {
				t.Errorf("extractAuthorName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestMatchesActor(t *testing.T) {
	tests := []struct {
		name     string
		actor    string
		expected bool
	}{
		// Exact matches
		{"joe", "joe", true},
		{"Joe", "joe", true}, // Case insensitive
		{"JOE", "joe", true},

		// Actor as path, name as simple name
		{"joe", "gastown/crew/joe", true},
		{"Joe", "gastown/crew/joe", true},

		// Partial matches
		{"joe-session1", "joe", true},
		{"gastown-joe", "joe", true},

		// Non-matches
		{"bob", "joe", false},
		{"", "joe", false},
		{"witness", "gastown/crew/joe", false},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_"+tt.actor, func(t *testing.T) {
			got := matchesActor(tt.name, tt.actor)
			if got != tt.expected {
				t.Errorf("matchesActor(%q, %q) = %v, want %v", tt.name, tt.actor, got, tt.expected)
			}
		})
	}
}

func TestParseBeadsTimestamp(t *testing.T) {
	tests := []struct {
		input    string
		expected string // Format: "2006-01-02 15:04"
		isZero   bool
	}{
		{"2025-12-30T16:19:00Z", "2025-12-30 16:19", false},
		{"2025-12-30 16:19", "2025-12-30 16:19", false},
		{"2025-12-30", "2025-12-30 00:00", false},
		{"invalid", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseBeadsTimestamp(tt.input)
			if tt.isZero {
				if !got.IsZero() {
					t.Errorf("parseBeadsTimestamp(%q) expected zero time, got %v", tt.input, got)
				}
				return
			}
			gotStr := got.Format("2006-01-02 15:04")
			if gotStr != tt.expected {
				t.Errorf("parseBeadsTimestamp(%q) = %q, want %q", tt.input, gotStr, tt.expected)
			}
		})
	}
}

func TestFormatSource(t *testing.T) {
	// Just verify it doesn't panic and returns non-empty strings
	sources := []string{"git", "beads", "townlog", "events", "unknown"}
	for _, s := range sources {
		result := formatSource(s)
		if result == "" {
			t.Errorf("formatSource(%q) returned empty string", s)
		}
	}
}

func TestFormatType(t *testing.T) {
	// Just verify it doesn't panic and returns non-empty strings
	types := []string{"commit", "bead_created", "bead_closed", "spawn", "done", "handoff", "crash", "kill", "merged", "merge_failed", "unknown"}
	for _, typ := range types {
		result := formatType(typ)
		if result == "" {
			t.Errorf("formatType(%q) returned empty string", typ)
		}
	}
}
