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
		// Both running - fully active
		{"both running", true, true, "OPERATIONAL", "🟢"},

		// Parked/docked takes precedence over session state
		{"both running parked", true, true, "PARKED", "🅿️"},
		{"both running docked", true, true, "DOCKED", "🛑"},
		{"witness only parked", true, false, "PARKED", "🅿️"},
		{"refinery only docked", false, true, "DOCKED", "🛑"},

		// One running - partially active
		{"witness only", true, false, "OPERATIONAL", "🟡"},
		{"refinery only", false, true, "OPERATIONAL", "🟡"},

		// Nothing running - check config state
		{"stopped operational", false, false, "OPERATIONAL", "⚫"},
		{"stopped empty state", false, false, "", "⚫"},
		{"parked", false, false, "PARKED", "🅿️"},
		{"docked", false, false, "DOCKED", "🛑"},
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
