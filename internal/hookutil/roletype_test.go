package hookutil

import "testing"

func TestIsAutonomousRole(t *testing.T) {
	autonomous := []string{"polecat", "witness", "refinery", "deacon", "boot"}
	for _, role := range autonomous {
		if !IsAutonomousRole(role) {
			t.Errorf("IsAutonomousRole(%q) = false, want true", role)
		}
	}

	interactive := []string{"mayor", "crew", "unknown", ""}
	for _, role := range interactive {
		if IsAutonomousRole(role) {
			t.Errorf("IsAutonomousRole(%q) = true, want false", role)
		}
	}
}
