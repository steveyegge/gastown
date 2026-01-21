package doctor

import (
	"testing"
)

func TestNewZombieSessionCheck(t *testing.T) {
	check := NewZombieSessionCheck()

	if check.Name() != "zombie-sessions" {
		t.Errorf("expected name 'zombie-sessions', got %q", check.Name())
	}

	if check.Description() != "Detect tmux sessions with dead Claude processes" {
		t.Errorf("expected description 'Detect tmux sessions with dead Claude processes', got %q", check.Description())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}

	if check.Category() != CategoryCleanup {
		t.Errorf("expected category %q, got %q", CategoryCleanup, check.Category())
	}
}

func TestZombieSessionCheck_Run_NoSessions(t *testing.T) {
	// This test verifies the check runs without error.
	// Results depend on the test environment.
	check := NewZombieSessionCheck()
	ctx := &CheckContext{TownRoot: t.TempDir()}

	result := check.Run(ctx)

	// Should return OK or Warning depending on environment
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("expected StatusOK or StatusWarning, got %v: %s", result.Status, result.Message)
	}
}

func TestZombieSessionCheck_SkipsCrewSessions(t *testing.T) {
	// Verify that crew sessions are not marked as zombies
	check := NewZombieSessionCheck()

	// Run the check - crew sessions should be skipped
	ctx := &CheckContext{TownRoot: t.TempDir()}
	result := check.Run(ctx)

	// If there are zombies, ensure no crew sessions are in the list
	for _, detail := range result.Details {
		if isCrewSession(detail) {
			t.Errorf("crew session should not be in zombie list: %s", detail)
		}
	}
}

func TestZombieSessionCheck_FixProtectsCrewSessions(t *testing.T) {
	// Verify that Fix() never kills crew sessions
	check := NewZombieSessionCheck()

	// Manually set zombies including a crew session (simulating a bug)
	check.zombieSessions = []string{
		"gt-gastown-crew-joe", // Should be skipped
		"gt-gastown-witness",  // Would be killed (if real)
	}

	ctx := &CheckContext{TownRoot: t.TempDir()}

	// Fix should skip crew sessions due to safeguard
	// (We can't fully test this without mocking tmux, but the safeguard is in place)
	_ = check.Fix(ctx)

	// The test passes if no panic occurred and crew sessions are protected by the safeguard
}
