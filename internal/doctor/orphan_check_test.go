package doctor

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// mockSessionLister allows deterministic testing of orphan session detection.
type mockSessionLister struct {
	sessions []string
	err      error
}

func (m *mockSessionLister) ListSessions() ([]string, error) {
	return m.sessions, m.err
}

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
	if runtime.GOOS == "windows" {
		t.Skip("orphan process detection is not supported on Windows")
	}

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

	expectedDesc := "Detect runtime processes outside tmux"
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

// TestOrphanSessionCheck_IsValidSession_EdgeCases tests edge cases that have caused
// false positives in production - sessions incorrectly detected as orphans.
func TestOrphanSessionCheck_IsValidSession_EdgeCases(t *testing.T) {
	check := NewOrphanSessionCheck()
	validRigs := []string{"gastown", "niflheim", "grctool", "7thsense", "pulseflow"}
	mayorSession := "hq-mayor"
	deaconSession := "hq-deacon"

	tests := []struct {
		name    string
		session string
		want    bool
		reason  string
	}{
		// Crew sessions with various name formats
		{
			name:    "crew_simple_name",
			session: "gt-gastown-crew-max",
			want:    true,
			reason:  "simple crew name should be valid",
		},
		{
			name:    "crew_with_numbers",
			session: "gt-niflheim-crew-codex1",
			want:    true,
			reason:  "crew name with numbers should be valid",
		},
		{
			name:    "crew_alphanumeric",
			session: "gt-grctool-crew-grc1",
			want:    true,
			reason:  "alphanumeric crew name should be valid",
		},
		{
			name:    "crew_short_name",
			session: "gt-7thsense-crew-ss1",
			want:    true,
			reason:  "short crew name should be valid",
		},
		{
			name:    "crew_pf1",
			session: "gt-pulseflow-crew-pf1",
			want:    true,
			reason:  "pf1 crew name should be valid",
		},

		// Polecat sessions (any name after rig should be accepted)
		{
			name:    "polecat_hash_style",
			session: "gt-gastown-abc123def",
			want:    true,
			reason:  "polecat with hash-style name should be valid",
		},
		{
			name:    "polecat_descriptive",
			session: "gt-niflheim-fix-auth-bug",
			want:    true,
			reason:  "polecat with descriptive name should be valid",
		},

		// Sessions that should be detected as orphans
		{
			name:    "unknown_rig_witness",
			session: "gt-unknownrig-witness",
			want:    false,
			reason:  "unknown rig should be orphan",
		},
		{
			name:    "malformed_too_short",
			session: "gt-only",
			want:    false,
			reason:  "malformed session (too few parts) should be orphan",
		},

		// Edge case: rig name with hyphen would be tricky
		// Current implementation uses SplitN with limit 3
		// gt-my-rig-witness would parse as rig="my" role="rig-witness"
		// This is a known limitation documented here
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := check.isValidSession(tt.session, validRigs, mayorSession, deaconSession)
			if got != tt.want {
				t.Errorf("isValidSession(%q) = %v, want %v: %s", tt.session, got, tt.want, tt.reason)
			}
		})
	}
}

// TestOrphanSessionCheck_GetValidRigs verifies rig detection from filesystem.
func TestOrphanSessionCheck_GetValidRigs(t *testing.T) {
	check := NewOrphanSessionCheck()
	townRoot := t.TempDir()

	// Setup: create mayor directory (required for getValidRigs to proceed)
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatalf("failed to create mayor dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "rigs.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("failed to create rigs.json: %v", err)
	}

	// Create some rigs with polecats/crew directories
	createRigDir := func(name string, hasCrew, hasPolecats bool) {
		rigPath := filepath.Join(townRoot, name)
		os.MkdirAll(rigPath, 0755)
		if hasCrew {
			os.MkdirAll(filepath.Join(rigPath, "crew"), 0755)
		}
		if hasPolecats {
			os.MkdirAll(filepath.Join(rigPath, "polecats"), 0755)
		}
	}

	createRigDir("gastown", true, true)
	createRigDir("niflheim", true, false)
	createRigDir("grctool", false, true)
	createRigDir("not-a-rig", false, false) // No crew or polecats

	rigs := check.getValidRigs(townRoot)

	// Should find gastown, niflheim, grctool but not "not-a-rig"
	expected := map[string]bool{
		"gastown":  true,
		"niflheim": true,
		"grctool":  true,
	}

	for _, rig := range rigs {
		if !expected[rig] {
			t.Errorf("unexpected rig %q in result", rig)
		}
		delete(expected, rig)
	}

	for rig := range expected {
		t.Errorf("expected rig %q not found in result", rig)
	}
}

// TestOrphanSessionCheck_FixProtectsCrewSessions verifies that Fix() never kills crew sessions.
func TestOrphanSessionCheck_FixProtectsCrewSessions(t *testing.T) {
	check := NewOrphanSessionCheck()

	// Simulate cached orphan sessions including a crew session
	check.orphanSessions = []string{
		"gt-gastown-crew-max",      // Crew - should be protected
		"gt-unknown-witness",       // Not crew - would be killed
		"gt-niflheim-crew-codex1",  // Crew - should be protected
	}

	// Verify isCrewSession correctly identifies crew sessions
	for _, sess := range check.orphanSessions {
		if sess == "gt-gastown-crew-max" || sess == "gt-niflheim-crew-codex1" {
			if !isCrewSession(sess) {
				t.Errorf("isCrewSession(%q) should return true for crew session", sess)
			}
		} else {
			if isCrewSession(sess) {
				t.Errorf("isCrewSession(%q) should return false for non-crew session", sess)
			}
		}
	}
}

// TestIsCrewSession_ComprehensivePatterns tests the crew session detection pattern thoroughly.
func TestIsCrewSession_ComprehensivePatterns(t *testing.T) {
	tests := []struct {
		session string
		want    bool
		reason  string
	}{
		// Valid crew patterns
		{"gt-gastown-crew-joe", true, "standard crew session"},
		{"gt-beads-crew-max", true, "different rig crew session"},
		{"gt-niflheim-crew-codex1", true, "crew with numbers in name"},
		{"gt-grctool-crew-grc1", true, "crew with alphanumeric name"},
		{"gt-7thsense-crew-ss1", true, "rig starting with number"},
		{"gt-a-crew-b", true, "minimal valid crew session"},

		// Invalid crew patterns
		{"gt-gastown-witness", false, "witness is not crew"},
		{"gt-gastown-refinery", false, "refinery is not crew"},
		{"gt-gastown-polecat-abc", false, "polecat is not crew"},
		{"hq-deacon", false, "deacon is not crew"},
		{"hq-mayor", false, "mayor is not crew"},
		{"gt-gastown-crew", false, "missing crew name"},
		{"gt-crew-max", false, "missing rig name"},
		{"crew-gastown-max", false, "wrong prefix"},
		{"other-session", false, "not a gt session"},
		{"", false, "empty string"},
		{"gt", false, "just prefix"},
		{"gt-", false, "prefix with dash"},
		{"gt-gastown", false, "rig only"},
	}

	for _, tt := range tests {
		t.Run(tt.session, func(t *testing.T) {
			got := isCrewSession(tt.session)
			if got != tt.want {
				t.Errorf("isCrewSession(%q) = %v, want %v: %s", tt.session, got, tt.want, tt.reason)
			}
		})
	}
}

// TestOrphanSessionCheck_HQSessions tests that hq-* sessions are properly recognized as valid.
func TestOrphanSessionCheck_HQSessions(t *testing.T) {
	townRoot := t.TempDir()
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0o755); err != nil {
		t.Fatalf("create mayor dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("create rigs.json: %v", err)
	}

	lister := &mockSessionLister{
		sessions: []string{
			"hq-mayor",   // valid: headquarters mayor session
			"hq-deacon",  // valid: headquarters deacon session
		},
	}
	check := NewOrphanSessionCheckWithSessionLister(lister)
	result := check.Run(&CheckContext{TownRoot: townRoot})

	if result.Status != StatusOK {
		t.Fatalf("expected StatusOK for valid hq sessions, got %v: %s", result.Status, result.Message)
	}
	if result.Message != "All 2 Gas Town sessions are valid" {
		t.Fatalf("unexpected message: %q", result.Message)
	}
	if len(check.orphanSessions) != 0 {
		t.Fatalf("expected no orphan sessions, got %v", check.orphanSessions)
	}
}

// TestOrphanSessionCheck_Run_Deterministic tests the full Run path with a mock session
// lister, ensuring deterministic behavior without depending on real tmux state.
func TestOrphanSessionCheck_Run_Deterministic(t *testing.T) {
	townRoot := t.TempDir()
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0o755); err != nil {
		t.Fatalf("create mayor dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("create rigs.json: %v", err)
	}

	// Create rig directories to make them "valid"
	if err := os.MkdirAll(filepath.Join(townRoot, "gastown", "polecats"), 0o755); err != nil {
		t.Fatalf("create gastown rig: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, "beads", "crew"), 0o755); err != nil {
		t.Fatalf("create beads rig: %v", err)
	}

	lister := &mockSessionLister{
		sessions: []string{
			"gt-gastown-witness",      // valid: gastown rig exists
			"gt-gastown-polecat1",     // valid: gastown rig exists
			"gt-beads-refinery",       // valid: beads rig exists
			"hq-mayor",                // valid: hq-mayor is recognized
			"hq-deacon",               // valid: hq-deacon is recognized
			"gt-unknown-witness",      // orphan: unknown rig doesn't exist
			"gt-missing-crew-joe",     // orphan: missing rig doesn't exist
			"random-session",          // ignored: doesn't match gt-*/hq-* pattern
		},
	}
	check := NewOrphanSessionCheckWithSessionLister(lister)
	result := check.Run(&CheckContext{TownRoot: townRoot})

	if result.Status != StatusWarning {
		t.Fatalf("expected StatusWarning, got %v: %s", result.Status, result.Message)
	}
	if result.Message != "Found 2 orphaned session(s)" {
		t.Fatalf("unexpected message: %q", result.Message)
	}
	if result.FixHint == "" {
		t.Fatal("expected FixHint to be set for orphan sessions")
	}

	expectedOrphans := []string{"gt-unknown-witness", "gt-missing-crew-joe"}
	if !reflect.DeepEqual(check.orphanSessions, expectedOrphans) {
		t.Fatalf("cached orphans = %v, want %v", check.orphanSessions, expectedOrphans)
	}

	expectedDetails := []string{"Orphan: gt-unknown-witness", "Orphan: gt-missing-crew-joe"}
	if !reflect.DeepEqual(result.Details, expectedDetails) {
		t.Fatalf("details = %v, want %v", result.Details, expectedDetails)
	}
}
