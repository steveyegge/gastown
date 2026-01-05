package cmd

import "testing"

func TestParsePolecatSessionName(t *testing.T) {
	tests := []struct {
		name        string
		sessionName string
		wantRig     string
		wantPolecat string
		wantOk      bool
	}{
		// Valid polecat sessions
		{
			name:        "simple polecat",
			sessionName: "gt-greenplace-Toast",
			wantRig:     "greenplace",
			wantPolecat: "Toast",
			wantOk:      true,
		},
		{
			name:        "another polecat",
			sessionName: "gt-greenplace-Nux",
			wantRig:     "greenplace",
			wantPolecat: "Nux",
			wantOk:      true,
		},
		{
			name:        "polecat in different rig",
			sessionName: "gt-beads-Worker",
			wantRig:     "beads",
			wantPolecat: "Worker",
			wantOk:      true,
		},
		{
			name:        "polecat with hyphen in name",
			sessionName: "gt-greenplace-Max-01",
			wantRig:     "greenplace",
			wantPolecat: "Max-01",
			wantOk:      true,
		},

		// Not polecat sessions (should return false)
		{
			name:        "crew session",
			sessionName: "gt-greenplace-crew-jack",
			wantRig:     "",
			wantPolecat: "",
			wantOk:      false,
		},
		{
			name:        "witness session",
			sessionName: "gt-greenplace-witness",
			wantRig:     "",
			wantPolecat: "",
			wantOk:      false,
		},
		{
			name:        "refinery session",
			sessionName: "gt-greenplace-refinery",
			wantRig:     "",
			wantPolecat: "",
			wantOk:      false,
		},
		{
			name:        "mayor session",
			sessionName: "gt-ai-mayor",
			wantRig:     "",
			wantPolecat: "",
			wantOk:      false,
		},
		{
			name:        "deacon session",
			sessionName: "gt-ai-deacon",
			wantRig:     "",
			wantPolecat: "",
			wantOk:      false,
		},
		{
			name:        "no gt prefix",
			sessionName: "gastown-Toast",
			wantRig:     "",
			wantPolecat: "",
			wantOk:      false,
		},
		{
			name:        "empty string",
			sessionName: "",
			wantRig:     "",
			wantPolecat: "",
			wantOk:      false,
		},
		{
			name:        "just gt prefix",
			sessionName: "gt-",
			wantRig:     "",
			wantPolecat: "",
			wantOk:      false,
		},
		{
			name:        "no name after rig",
			sessionName: "gt-greenplace-",
			wantRig:     "",
			wantPolecat: "",
			wantOk:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRig, gotPolecat, gotOk := parsePolecatSessionName(tt.sessionName)
			if gotRig != tt.wantRig || gotPolecat != tt.wantPolecat || gotOk != tt.wantOk {
				t.Errorf("parsePolecatSessionName(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.sessionName, gotRig, gotPolecat, gotOk, tt.wantRig, tt.wantPolecat, tt.wantOk)
			}
		})
	}
}
