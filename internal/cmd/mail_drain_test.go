package cmd

import (
	"testing"
)

func TestIsDrainableMessage(t *testing.T) {
	tests := []struct {
		subject   string
		drainable bool
	}{
		// Drainable protocol messages
		{"POLECAT_DONE furiosa", true},
		{"POLECAT_STARTED: furiosa", true},
		{"LIFECYCLE:Shutdown furiosa", true},
		{"LIFECYCLE:Restart furiosa", true},
		{"MERGED furiosa", true},
		{"MERGE_READY furiosa", true},
		{"MERGE_FAILED furiosa", true},
		{"SWARM_START", true},

		// Non-drainable messages (need attention)
		{"HELP: stuck on implementation", false},
		{"🤝 HANDOFF", false},
		{"Status check", false},
		{"Question about deployment", false},
		{"ALERT: something", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.subject, func(t *testing.T) {
			got := isDrainableMessage(tc.subject)
			if got != tc.drainable {
				t.Errorf("isDrainableMessage(%q) = %v, want %v", tc.subject, got, tc.drainable)
			}
		})
	}
}
