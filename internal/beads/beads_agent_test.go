package beads

import "testing"

func TestIsAgentBeadByID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		// Full-form IDs (prefix != rig): prefix-rig-role[-name]
		{name: "full witness", id: "gt-gastown-witness", want: true},
		{name: "full refinery", id: "gt-gastown-refinery", want: true},
		{name: "full crew with name", id: "gt-gastown-crew-krystian", want: true},
		{name: "full polecat with name", id: "gt-gastown-polecat-Toast", want: true},
		{name: "full deacon", id: "sh-shippercrm-deacon", want: true},
		{name: "full mayor", id: "ax-axon-mayor", want: true},

		// Collapsed-form IDs (prefix == rig): prefix-role[-name]
		// These have only 2 parts for witness/refinery, must still be detected.
		{name: "collapsed witness", id: "bcc-witness", want: true},
		{name: "collapsed refinery", id: "bcc-refinery", want: true},
		{name: "collapsed crew with name", id: "bcc-crew-krystian", want: true},
		{name: "collapsed polecat with name", id: "bcc-polecat-obsidian", want: true},

		// Non-agent IDs
		{name: "regular issue", id: "gt-12345", want: false},
		{name: "task bead", id: "bcc-fix-button-color", want: false},
		{name: "single part", id: "witness", want: false},
		{name: "empty string", id: "", want: false},
		{name: "patrol molecule", id: "mol-patrol-abc123", want: false},
		{name: "merge request", id: "gt-mr-1234", want: false},

		// Edge cases
		{name: "role in first position", id: "witness-something", want: false},
		{name: "beads prefix collapsed", id: "bd-beads-witness", want: true},
		{name: "beads crew", id: "bd-beads-crew-krystian", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAgentBeadByID(tt.id)
			if got != tt.want {
				t.Errorf("isAgentBeadByID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}
