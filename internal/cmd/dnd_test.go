package cmd

import (
	"testing"

	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/workspace"
)

func TestAddressToAgentBeadID(t *testing.T) {
	town := workspace.TownName()
	tests := []struct {
		address  string
		expected string
	}{
		// Mayor and deacon use hq- prefix (town-level, town-namespaced)
		{"mayor", session.MayorSessionName(town)},
		{"deacon", session.DeaconSessionName(town)},
		{"gastown/witness", "gt-gastown-witness"},
		{"gastown/refinery", "gt-gastown-refinery"},
		{"gastown/alpha", "gt-gastown-polecat-alpha"},
		{"gastown/crew/max", "gt-gastown-crew-max"},
		{"beads/witness", "gt-beads-witness"},
		{"beads/beta", "gt-beads-polecat-beta"},
		// Invalid addresses should return empty string
		{"invalid", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			got := addressToAgentBeadID(tt.address)
			if got != tt.expected {
				t.Errorf("addressToAgentBeadID(%q) = %q, want %q", tt.address, got, tt.expected)
			}
		})
	}
}
