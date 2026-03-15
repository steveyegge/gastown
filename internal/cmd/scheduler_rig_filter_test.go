package cmd

import (
	"testing"

	"github.com/steveyegge/gastown/internal/session"
)

// TestCountActivePolecatsForRig_Filtering verifies that the rig-filtering logic
// in countActivePolecatsForRig correctly filters polecat sessions by rig name.
// Since countActivePolecatsForRig calls tmux directly, we test the filtering
// logic by parsing session names and applying the same filter.
func TestCountActivePolecatsForRig_Filtering(t *testing.T) {
	// Set up a registry matching the session name format
	reg := session.NewPrefixRegistry()
	reg.Register("be", "beacon")
	reg.Register("gt", "gastown")

	// Session names use the format: <prefix>-<name> for polecats
	// <prefix>-witness for witness, <prefix>-crew-<name> for crew
	sessionNames := []string{
		"be-Toast",       // polecat in beacon
		"be-Ember",       // polecat in beacon
		"gt-Alpha",       // polecat in gastown
		"be-witness",     // witness in beacon (not a polecat)
		"gt-Beta",        // polecat in gastown
		"be-crew-emmett", // crew in beacon (not a polecat)
	}

	// Count polecats per rig using the same logic as countActivePolecatsForRig
	countForRig := func(rigName string) int {
		count := 0
		for _, name := range sessionNames {
			identity, err := session.ParseSessionNameWithRegistry(name, reg)
			if err != nil {
				continue
			}
			if identity.Role == session.RolePolecat {
				if rigName == "" || identity.Rig == rigName {
					count++
				}
			}
		}
		return count
	}

	tests := []struct {
		rig  string
		want int
	}{
		{"", 4},            // all polecats
		{"beacon", 2},      // beacon only
		{"gastown", 2},     // gastown only
		{"nonexistent", 0}, // no match
	}

	for _, tt := range tests {
		name := "all"
		if tt.rig != "" {
			name = "rig=" + tt.rig
		}
		t.Run(name, func(t *testing.T) {
			if got := countForRig(tt.rig); got != tt.want {
				t.Errorf("countForRig(%q) = %d, want %d", tt.rig, got, tt.want)
			}
		})
	}
}
