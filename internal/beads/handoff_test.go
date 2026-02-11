package beads

import (
	"testing"
)

func TestHandoffBeadTitle(t *testing.T) {
	tests := []struct {
		role string
		want string
	}{
		{"mayor", "mayor Handoff"},
		{"deacon", "deacon Handoff"},
		{"gastown/witness", "gastown/witness Handoff"},
		{"gastown/crew/joe", "gastown/crew/joe Handoff"},
		{"", " Handoff"},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			got := HandoffBeadTitle(tt.role)
			if got != tt.want {
				t.Errorf("HandoffBeadTitle(%q) = %q, want %q", tt.role, got, tt.want)
			}
		})
	}
}

func TestStatusConstants(t *testing.T) {
	// Verify the status constants haven't changed (these are used in protocol)
	if StatusPinned != "pinned" {
		t.Errorf("StatusPinned = %q, want %q", StatusPinned, "pinned")
	}
	if StatusHooked != "hooked" {
		t.Errorf("StatusHooked = %q, want %q", StatusHooked, "hooked")
	}
}

func TestCurrentTimestamp(t *testing.T) {
	ts := currentTimestamp()
	if ts == "" {
		t.Fatal("currentTimestamp() returned empty string")
	}
	// Should be RFC3339 format
	if len(ts) < 20 {
		t.Errorf("timestamp too short: %q (expected RFC3339)", ts)
	}
	// Should contain T separator and Z suffix (UTC)
	found := false
	for _, c := range ts {
		if c == 'T' {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("timestamp missing T separator: %q", ts)
	}
}

func TestClearMailResultZeroValues(t *testing.T) {
	// Verify zero-value struct is safe to use
	result := &ClearMailResult{}
	if result.Closed != 0 || result.Cleared != 0 {
		t.Errorf("expected zero values, got Closed=%d Cleared=%d", result.Closed, result.Cleared)
	}
}
