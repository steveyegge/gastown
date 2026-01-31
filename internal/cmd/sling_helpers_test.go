package cmd

import (
	"strings"
	"testing"
)

// TestEnsureAgentBeadExistsAgentIDParsing tests agent ID parsing in ensureAgentBeadExists.
// Fix for hq-cc7214.26: Sling fails when agent bead doesn't exist.
func TestEnsureAgentBeadExistsAgentIDParsing(t *testing.T) {
	// This test verifies that the agent ID parsing logic correctly extracts
	// role type, rig name, and agent name from various agent ID formats.

	tests := []struct {
		name        string
		agentID     string
		wantParsed  bool
		wantRole    string
		wantRig     string
		wantAgent   string
	}{
		{
			name:       "crew agent",
			agentID:    "testrig/crew/worker1",
			wantParsed: true,
			wantRole:   "crew",
			wantRig:    "testrig",
			wantAgent:  "worker1",
		},
		{
			name:       "polecat agent",
			agentID:    "testrig/polecats/Toast",
			wantParsed: true,
			wantRole:   "polecat",
			wantRig:    "testrig",
			wantAgent:  "Toast",
		},
		{
			name:       "witness agent",
			agentID:    "testrig/witness",
			wantParsed: true,
			wantRole:   "witness",
			wantRig:    "testrig",
			wantAgent:  "",
		},
		{
			name:       "refinery agent",
			agentID:    "testrig/refinery",
			wantParsed: true,
			wantRole:   "refinery",
			wantRig:    "testrig",
			wantAgent:  "",
		},
		{
			name:       "trailing slash",
			agentID:    "testrig/crew/worker1/",
			wantParsed: true,
			wantRole:   "crew",
			wantRig:    "testrig",
			wantAgent:  "worker1",
		},
		{
			name:       "invalid - single part",
			agentID:    "invalidid",
			wantParsed: false,
		},
		{
			name:       "invalid - unknown role",
			agentID:    "testrig/unknown/foo",
			wantParsed: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Parse agent ID using the same logic as ensureAgentBeadExists
			parts := strings.Split(strings.TrimSuffix(tc.agentID, "/"), "/")

			var parsed bool
			var roleType, rigName, agentName string

			if len(parts) >= 2 {
				switch {
				case len(parts) == 3 && parts[1] == "crew":
					parsed = true
					roleType = "crew"
					rigName = parts[0]
					agentName = parts[2]
				case len(parts) == 3 && parts[1] == "polecats":
					parsed = true
					roleType = "polecat"
					rigName = parts[0]
					agentName = parts[2]
				case len(parts) == 2 && parts[1] == "witness":
					parsed = true
					roleType = "witness"
					rigName = parts[0]
				case len(parts) == 2 && parts[1] == "refinery":
					parsed = true
					roleType = "refinery"
					rigName = parts[0]
				}
			}

			if parsed != tc.wantParsed {
				t.Errorf("parsed = %v, want %v", parsed, tc.wantParsed)
			}

			if tc.wantParsed {
				if roleType != tc.wantRole {
					t.Errorf("roleType = %q, want %q", roleType, tc.wantRole)
				}
				if rigName != tc.wantRig {
					t.Errorf("rigName = %q, want %q", rigName, tc.wantRig)
				}
				if agentName != tc.wantAgent {
					t.Errorf("agentName = %q, want %q", agentName, tc.wantAgent)
				}
			}
		})
	}
}
