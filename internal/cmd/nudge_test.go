package cmd

import (
	"testing"
)

func TestResolveNudgePattern(t *testing.T) {
	// Create test agent sessions (mayor/deacon use hq- prefix)
	agents := []*AgentSession{
		{Name: "hq-mayor", Type: AgentMayor},
		{Name: "hq-deacon", Type: AgentDeacon},
		{Name: "gt-gastown-witness", Type: AgentWitness, Rig: "gastown"},
		{Name: "gt-gastown-refinery", Type: AgentRefinery, Rig: "gastown"},
		{Name: "gt-gastown-crew-max", Type: AgentCrew, Rig: "gastown", AgentName: "max"},
		{Name: "gt-gastown-crew-jack", Type: AgentCrew, Rig: "gastown", AgentName: "jack"},
		{Name: "gt-gastown-alpha", Type: AgentPolecat, Rig: "gastown", AgentName: "alpha"},
		{Name: "gt-gastown-beta", Type: AgentPolecat, Rig: "gastown", AgentName: "beta"},
		{Name: "gt-beads-witness", Type: AgentWitness, Rig: "beads"},
		{Name: "gt-beads-gamma", Type: AgentPolecat, Rig: "beads", AgentName: "gamma"},
	}

	tests := []struct {
		name     string
		pattern  string
		expected []string
	}{
		{
			name:     "mayor special case",
			pattern:  "mayor",
			expected: []string{"hq-mayor"},
		},
		{
			name:     "deacon special case",
			pattern:  "deacon",
			expected: []string{"hq-deacon"},
		},
		{
			name:     "specific witness",
			pattern:  "gastown/witness",
			expected: []string{"gt-gastown-witness"},
		},
		{
			name:     "all witnesses",
			pattern:  "*/witness",
			expected: []string{"gt-gastown-witness", "gt-beads-witness"},
		},
		{
			name:     "specific refinery",
			pattern:  "gastown/refinery",
			expected: []string{"gt-gastown-refinery"},
		},
		{
			name:     "all polecats in rig",
			pattern:  "gastown/polecats/*",
			expected: []string{"gt-gastown-alpha", "gt-gastown-beta"},
		},
		{
			name:     "specific polecat",
			pattern:  "gastown/polecats/alpha",
			expected: []string{"gt-gastown-alpha"},
		},
		{
			name:     "all crew in rig",
			pattern:  "gastown/crew/*",
			expected: []string{"gt-gastown-crew-max", "gt-gastown-crew-jack"},
		},
		{
			name:     "specific crew member",
			pattern:  "gastown/crew/max",
			expected: []string{"gt-gastown-crew-max"},
		},
		{
			name:     "legacy polecat format",
			pattern:  "gastown/alpha",
			expected: []string{"gt-gastown-alpha"},
		},
		{
			name:     "no matches",
			pattern:  "nonexistent/polecats/*",
			expected: nil,
		},
		{
			name:     "invalid pattern",
			pattern:  "invalid",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveNudgePattern(tt.pattern, agents)

			if len(got) != len(tt.expected) {
				t.Errorf("resolveNudgePattern(%q) returned %d results, want %d: got %v, want %v",
					tt.pattern, len(got), len(tt.expected), got, tt.expected)
				return
			}

			// Check each expected value is present
			gotMap := make(map[string]bool)
			for _, g := range got {
				gotMap[g] = true
			}
			for _, e := range tt.expected {
				if !gotMap[e] {
					t.Errorf("resolveNudgePattern(%q) missing expected %q, got %v",
						tt.pattern, e, got)
				}
			}
		})
	}
}
