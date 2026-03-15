package cmd

import "testing"

func TestGetRigLED(t *testing.T) {
	tests := []struct {
		name        string
		hasWitness  bool
		hasRefinery bool
		opState     string
		want        string
	}{
		// Operational state overrides session state (GH#2555)
		{"parked no sessions", false, false, "PARKED", "🅿️"},
		{"parked with sessions", true, true, "PARKED", "🅿️"},
		{"parked partial", true, false, "PARKED", "🅿️"},
		{"docked no sessions", false, false, "DOCKED", "🛑"},
		{"docked with sessions", true, true, "DOCKED", "🛑"},

		// Both running - fully active
		{"both running", true, true, "OPERATIONAL", "🟢"},

		// One running - partially active
		{"witness only", true, false, "OPERATIONAL", "🟡"},
		{"refinery only", false, true, "OPERATIONAL", "🟡"},

		// Nothing running
		{"stopped operational", false, false, "OPERATIONAL", "⚫"},
		{"stopped empty state", false, false, "", "⚫"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetRigLED(tt.hasWitness, tt.hasRefinery, tt.opState)
			if got != tt.want {
				t.Errorf("GetRigLED(%v, %v, %q) = %q, want %q",
					tt.hasWitness, tt.hasRefinery, tt.opState, got, tt.want)
			}
		})
	}
}
