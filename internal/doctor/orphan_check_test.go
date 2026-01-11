package doctor

import (
	"testing"
)

func TestNewOrphanSessionCheck(t *testing.T) {
	check := NewOrphanSessionCheck()

	if check.Name() != "orphan-sessions" {
		t.Errorf("expected name 'orphan-sessions', got %q", check.Name())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true for session check")
	}
}

func TestNewOrphanProcessCheck(t *testing.T) {
	check := NewOrphanProcessCheck()

	if check.Name() != "orphan-processes" {
		t.Errorf("expected name 'orphan-processes', got %q", check.Name())
	}

	// OrphanProcessCheck should NOT be fixable - it's informational only
	if check.CanFix() {
		t.Error("expected CanFix to return false for process check (informational only)")
	}
}

func TestOrphanProcessCheck_Run(t *testing.T) {
	// This test verifies the check runs without error.
	// Results depend on whether Claude processes exist in the test environment.
	check := NewOrphanProcessCheck()
	ctx := &CheckContext{TownRoot: t.TempDir()}

	result := check.Run(ctx)

	// Should return OK (no processes or all inside tmux) or Warning (processes outside tmux)
	// Both are valid depending on test environment
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("expected StatusOK or StatusWarning, got %v: %s", result.Status, result.Message)
	}

	// If warning, should have informational details
	if result.Status == StatusWarning {
		if len(result.Details) < 3 {
			t.Errorf("expected at least 3 detail lines (2 info + 1 process), got %d", len(result.Details))
		}
		// Should NOT have a FixHint since this is informational only
		if result.FixHint != "" {
			t.Errorf("expected no FixHint for informational check, got %q", result.FixHint)
		}
	}
}

func TestOrphanProcessCheck_MessageContent(t *testing.T) {
	// Verify the check description is correct
	check := NewOrphanProcessCheck()

	expectedDesc := "Detect Claude processes outside tmux"
	if check.Description() != expectedDesc {
		t.Errorf("expected description %q, got %q", expectedDesc, check.Description())
	}
}

func TestIsCrewSession(t *testing.T) {
	tests := []struct {
		session string
		want    bool
	}{
		{"gt-gastown-crew-joe", true},
		{"gt-beads-crew-max", true},
		{"gt-rig-crew-a", true},
		{"gt-gastown-witness", false},
		{"gt-gastown-refinery", false},
		{"gt-gastown-polecat1", false},
		{"hq-deacon", false},
		{"hq-mayor", false},
		{"other-session", false},
		{"gt-crew", false}, // Not enough parts
	}

	for _, tt := range tests {
		t.Run(tt.session, func(t *testing.T) {
			got := isCrewSession(tt.session)
			if got != tt.want {
				t.Errorf("isCrewSession(%q) = %v, want %v", tt.session, got, tt.want)
			}
		})
	}
}

func TestOrphanSessionCheck_IsValidSession(t *testing.T) {
	check := NewOrphanSessionCheck()
	validRigs := []string{"gastown", "beads"}
	mayorSession := "hq-mayor"
	deaconSession := "hq-deacon"

	tests := []struct {
		session string
		want    bool
	}{
		// Town-level sessions
		{"hq-mayor", true},
		{"hq-deacon", true},

		// Valid rig sessions
		{"gt-gastown-witness", true},
		{"gt-gastown-refinery", true},
		{"gt-gastown-polecat1", true},
		{"gt-beads-witness", true},
		{"gt-beads-refinery", true},
		{"gt-beads-crew-max", true},

		// Invalid rig sessions (rig doesn't exist)
		{"gt-unknown-witness", false},
		{"gt-foo-refinery", false},

		// Non-gt sessions (should not be checked by this function,
		// but if called, they'd fail format validation)
		{"other-session", false},
	}

	for _, tt := range tests {
		t.Run(tt.session, func(t *testing.T) {
			got := check.isValidSession(tt.session, validRigs, mayorSession, deaconSession)
			if got != tt.want {
				t.Errorf("isValidSession(%q) = %v, want %v", tt.session, got, tt.want)
			}
		})
	}
}
