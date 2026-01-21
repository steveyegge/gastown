package cmd

import "testing"

func TestIsValidGroupName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"ops-team", true},
		{"all_witnesses", true},
		{"team123", true},
		{"A", true},
		{"abc", true},
		{"my-cool-group", true},

		// Invalid
		{"", false},
		{"with spaces", false},
		{"with.dots", false},
		{"@team", false},
		{"group/name", false},
		{"team!", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidGroupName(tt.name); got != tt.want {
				t.Errorf("isValidGroupName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestIsValidMemberPattern(t *testing.T) {
	tests := []struct {
		pattern string
		want    bool
	}{
		// Direct addresses
		{"gastown/crew/max", true},
		{"mayor/", true},
		{"deacon/", true},
		{"gastown/witness", true},

		// Wildcard patterns
		{"*/witness", true},
		{"gastown/*", true},
		{"gastown/crew/*", true},

		// Special patterns
		{"@town", true},
		{"@crew", true},
		{"@witnesses", true},
		{"@rig/gastown", true},

		// Group names
		{"ops-team", true},
		{"all_witnesses", true},

		// Invalid
		{"", false},
		{"@", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			if got := isValidMemberPattern(tt.pattern); got != tt.want {
				t.Errorf("isValidMemberPattern(%q) = %v, want %v", tt.pattern, got, tt.want)
			}
		})
	}
}
