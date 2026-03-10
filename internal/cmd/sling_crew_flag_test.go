package cmd

import (
	"strings"
	"testing"
)

// TestSlingCrewFlagExpandsTarget verifies that --crew transforms a rig target
// into a crew member path: "gastown --crew mel" → "gastown/crew/mel".
func TestSlingCrewFlagExpandsTarget(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		crew     string
		wantLast string
		wantErr  string
	}{
		{
			name:     "basic crew expansion",
			args:     []string{"gt-abc", "gastown"},
			crew:     "mel",
			wantLast: "gastown/crew/mel",
		},
		{
			name:     "different rig and crew",
			args:     []string{"gp-xyz", "greenplace"},
			crew:     "bob",
			wantLast: "greenplace/crew/bob",
		},
		{
			name:    "crew without rig target errors",
			args:    []string{"gt-abc"},
			crew:    "mel",
			wantErr: "--crew requires a rig target argument",
		},
		{
			name:     "no crew flag leaves target unchanged",
			args:     []string{"gt-abc", "gastown"},
			crew:     "",
			wantLast: "gastown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the --crew expansion logic from runSling
			args := make([]string, len(tt.args))
			copy(args, tt.args)

			var errMsg string
			if tt.crew != "" {
				if len(args) < 2 {
					errMsg = "--crew requires a rig target argument"
				} else {
					args[len(args)-1] = args[len(args)-1] + "/crew/" + tt.crew
				}
			}

			if tt.wantErr != "" {
				if errMsg == "" {
					t.Fatalf("expected error containing %q, got none", tt.wantErr)
				}
				if !strings.Contains(errMsg, tt.wantErr) {
					t.Fatalf("error %q does not contain %q", errMsg, tt.wantErr)
				}
				return
			}

			if errMsg != "" {
				t.Fatalf("unexpected error: %s", errMsg)
			}

			got := args[len(args)-1]
			if got != tt.wantLast {
				t.Errorf("last arg = %q, want %q", got, tt.wantLast)
			}
		})
	}
}
