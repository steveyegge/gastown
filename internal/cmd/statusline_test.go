package cmd

import "testing"

func TestAbbreviateRigName(t *testing.T) {
	tests := []struct {
		name     string
		rigName  string
		wantAbbr string
	}{
		// Known rigs should abbreviate
		{"design_forge", "design_forge", "df"},
		{"gastown", "gastown", "gt"},
		{"mediaforge", "mediaforge", "mf"},
		{"mt5optimizer", "mt5optimizer", "mt"},
		{"opencode", "opencode", "oc"},
		{"pod_automation", "pod_automation", "pa"},
		{"promptforge", "promptforge", "pf"},
		{"autoaffiliate", "autoaffiliate", "aa"},

		// Unknown rigs should return as-is
		{"unknown rig", "unknown_rig", "unknown_rig"},
		{"myrig", "myrig", "myrig"},
		{"", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := abbreviateRigName(tt.rigName)
			if got != tt.wantAbbr {
				t.Errorf("abbreviateRigName(%q) = %q, want %q", tt.rigName, got, tt.wantAbbr)
			}
		})
	}
}

func TestCategorizeSessionRig(t *testing.T) {
	tests := []struct {
		session string
		wantRig string
	}{
		// Standard polecat sessions
		{"gt-gastown-slit", "gastown"},
		{"gt-gastown-Toast", "gastown"},
		{"gt-myrig-worker", "myrig"},

		// Crew sessions
		{"gt-gastown-crew-max", "gastown"},
		{"gt-myrig-crew-user", "myrig"},

		// Witness sessions (canonical format: gt-<rig>-witness)
		{"gt-gastown-witness", "gastown"},
		{"gt-myrig-witness", "myrig"},
		// Legacy format still works as fallback
		{"gt-witness-gastown", "gastown"},
		{"gt-witness-myrig", "myrig"},

		// Refinery sessions
		{"gt-gastown-refinery", "gastown"},
		{"gt-myrig-refinery", "myrig"},

		// Edge cases
		{"gt-a-b", "a"}, // minimum valid

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
	tests := []struct {
		session  string
		wantType AgentType
	}{
		// Polecat sessions
		{"gt-gastown-slit", AgentPolecat},
		{"gt-gastown-Toast", AgentPolecat},
		{"gt-myrig-worker", AgentPolecat},
		{"gt-a-b", AgentPolecat},

		// Non-polecat sessions
		{"gt-gastown-witness", AgentWitness}, // canonical format
		{"gt-witness-gastown", AgentWitness}, // legacy fallback
		{"gt-gastown-refinery", AgentRefinery},
		{"gt-gastown-crew-max", AgentCrew},
		{"gt-myrig-crew-user", AgentCrew},

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
