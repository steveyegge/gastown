package doctor

import "testing"

func TestShouldBeWisp(t *testing.T) {
	check := NewCheckMisclassifiedWisps()

	tests := []struct {
		name      string
		id        string
		title     string
		issueType string
		labels    []string
		wantWisp  bool
		wantMsg   string // substring of reason (empty = no reason expected)
	}{
		// Types that SHOULD be wisps
		{
			name:      "merge-request type is wisp",
			issueType: "merge-request",
			wantWisp:  true,
			wantMsg:   "merge-request",
		},
		{
			name:      "event type is wisp",
			issueType: "event",
			wantWisp:  true,
			wantMsg:   "event",
		},
		{
			name:      "gate type is wisp",
			issueType: "gate",
			wantWisp:  true,
			wantMsg:   "gate",
		},
		{
			name:      "slot type is wisp",
			issueType: "slot",
			wantWisp:  true,
			wantMsg:   "slot",
		},

		// Agent type should NOT be a wisp (persistent polecats design)
		{
			name:      "agent type is NOT a wisp (persistent polecats)",
			id:        "gt-gastown-witness",
			title:     "gastown witness",
			issueType: "agent",
			labels:    []string{"gt:agent"},
			wantWisp:  false,
		},
		{
			name:      "agent type with no labels is NOT a wisp",
			issueType: "agent",
			wantWisp:  false,
		},

		// gt:agent label should NOT trigger wisp classification
		{
			name:      "gt:agent label is NOT a wisp indicator",
			id:        "bcc-witness",
			title:     "bcc witness",
			issueType: "task", // might have wrong type from legacy
			labels:    []string{"gt:agent"},
			wantWisp:  false,
		},

		// Patrol labels should still be wisps
		{
			name:     "patrol label is wisp",
			labels:   []string{"gt:patrol"},
			wantWisp: true,
			wantMsg:  "patrol",
		},

		// Mail/handoff labels should still be wisps
		{
			name:     "mail label is wisp",
			labels:   []string{"gt:mail"},
			wantWisp: true,
			wantMsg:  "mail/handoff",
		},
		{
			name:     "handoff label is wisp",
			labels:   []string{"gt:handoff"},
			wantWisp: true,
			wantMsg:  "mail/handoff",
		},

		// Patrol ID patterns
		{
			name:     "patrol molecule ID",
			id:       "mol-witness-patrol-abc123",
			wantWisp: true,
			wantMsg:  "patrol molecule",
		},

		// Patrol title patterns
		{
			name:     "patrol cycle title",
			title:    "Patrol Cycle #42",
			wantWisp: true,
			wantMsg:  "patrol title",
		},
		{
			name:     "witness patrol title",
			title:    "Witness Patrol at 14:00",
			wantWisp: true,
			wantMsg:  "patrol title",
		},

		// Regular issues should NOT be wisps
		{
			name:      "regular task",
			id:        "gt-12345",
			title:     "Fix button color",
			issueType: "task",
			labels:    []string{"ui", "bugfix"},
			wantWisp:  false,
		},
		{
			name:      "feature issue",
			id:        "bcc-9876",
			title:     "Add dark mode",
			issueType: "feature",
			wantWisp:  false,
		},
		{
			name:      "bug issue",
			issueType: "bug",
			wantWisp:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason := check.shouldBeWisp(tt.id, tt.title, tt.issueType, tt.labels)
			gotWisp := reason != ""
			if gotWisp != tt.wantWisp {
				t.Errorf("shouldBeWisp(id=%q, title=%q, type=%q, labels=%v) = %q, wantWisp=%v",
					tt.id, tt.title, tt.issueType, tt.labels, reason, tt.wantWisp)
			}
			if tt.wantMsg != "" && reason == "" {
				t.Errorf("expected reason containing %q, got empty", tt.wantMsg)
			}
		})
	}
}

func TestShouldBeWisp_AgentNotWisp_Regression(t *testing.T) {
	// Regression test: persistent polecats design (c410c10a) says agent beads
	// live in the issues table. shouldBeWisp() must NOT flag them for migration
	// back to wisps, or gt doctor --fix will undo the persistent polecats migration.
	check := NewCheckMisclassifiedWisps()

	agentIDs := []struct {
		id    string
		title string
	}{
		{"gt-gastown-witness", "gastown witness"},
		{"gt-gastown-refinery", "gastown refinery"},
		{"gt-gastown-crew-krystian", "gastown crew krystian"},
		{"bcc-witness", "bcc witness"},
		{"bcc-refinery", "bcc refinery"},
		{"bcc-crew-krystian", "bcc crew krystian"},
		{"bd-beads-witness", "beads witness"},
		{"sh-shippercrm-witness", "shippercrm witness"},
		{"ax-axon-refinery", "axon refinery"},
	}

	for _, agent := range agentIDs {
		t.Run(agent.id, func(t *testing.T) {
			// Agent with type=agent and label=gt:agent should NOT be classified as wisp
			reason := check.shouldBeWisp(agent.id, agent.title, "agent", []string{"gt:agent"})
			if reason != "" {
				t.Errorf("shouldBeWisp(%q) returned %q — would undo persistent polecats migration!", agent.id, reason)
			}
		})
	}
}
