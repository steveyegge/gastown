package cmd

import (
	"testing"

	"github.com/steveyegge/gastown/internal/session"
)

func setupCmdTestRegistry(t *testing.T) {
	t.Helper()
	reg := session.NewPrefixRegistry()
	reg.Register("gt", "gastown")
	reg.Register("mr", "myrig")
	old := session.DefaultRegistry()
	session.SetDefaultRegistry(reg)
	t.Cleanup(func() { session.SetDefaultRegistry(old) })
}

func TestCategorizeSessionRig(t *testing.T) {
	setupCmdTestRegistry(t)
	tests := []struct {
		session string
		wantRig string
	}{
		// Standard polecat sessions (prefix-based)
		{"gt-slit", "gastown"},
		{"gt-Toast", "gastown"},
		{"mr-worker", "myrig"},

		// Crew sessions
		{"gt-crew-max", "gastown"},
		{"mr-crew-user", "myrig"},

		// Witness sessions
		{"gt-witness", "gastown"},
		{"mr-witness", "myrig"},

		// Refinery sessions
		{"gt-refinery", "gastown"},
		{"mr-refinery", "myrig"},

		// Town-level agents (no rig, use hq- prefix)
		{"hq-mayor", ""},
		{"hq-deacon", ""},
	}

	for _, tt := range tests {
		t.Run(tt.session, func(t *testing.T) {
			agent := categorizeSession(tt.session)
			gotRig := ""
			if agent != nil {
				gotRig = agent.Rig
			}
			if gotRig != tt.wantRig {
				t.Errorf("categorizeSession(%q).Rig = %q, want %q", tt.session, gotRig, tt.wantRig)
			}
		})
	}
}

func TestCategorizeSessionType(t *testing.T) {
	setupCmdTestRegistry(t)
	tests := []struct {
		session  string
		wantType AgentType
	}{
		// Polecat sessions
		{"gt-slit", AgentPolecat},
		{"gt-Toast", AgentPolecat},
		{"mr-worker", AgentPolecat},

		// Non-polecat sessions
		{"gt-witness", AgentWitness},
		{"gt-refinery", AgentRefinery},
		{"gt-crew-max", AgentCrew},
		{"mr-crew-user", AgentCrew},

		// Town-level agents (hq- prefix)
		{"hq-mayor", AgentMayor},
		{"hq-deacon", AgentDeacon},
	}

	for _, tt := range tests {
		t.Run(tt.session, func(t *testing.T) {
			agent := categorizeSession(tt.session)
			if agent == nil {
				t.Fatalf("categorizeSession(%q) returned nil", tt.session)
			}
			if agent.Type != tt.wantType {
				t.Errorf("categorizeSession(%q).Type = %v, want %v", tt.session, agent.Type, tt.wantType)
			}
		})
	}
}
