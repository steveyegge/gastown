package cmd

import (
	"strings"
	"testing"
)

func TestValidateTarget(t *testing.T) {
	tests := []struct {
		name        string
		target      string
		wantValid   bool
		wantReason  string // substring to match in reason
		wantSuggest string // substring to find in suggestions
	}{
		// Valid targets
		{
			name:      "self dot",
			target:    ".",
			wantValid: true,
		},
		{
			name:      "mayor",
			target:    "mayor",
			wantValid: true,
		},
		{
			name:      "mayor shortcut",
			target:    "may",
			wantValid: true,
		},
		{
			name:      "deacon",
			target:    "deacon",
			wantValid: true,
		},
		{
			name:      "deacon shortcut",
			target:    "dea",
			wantValid: true,
		},
		{
			name:      "crew shortcut",
			target:    "crew",
			wantValid: true,
		},
		{
			name:      "witness shortcut",
			target:    "wit",
			wantValid: true,
		},
		{
			name:      "refinery shortcut",
			target:    "ref",
			wantValid: true,
		},
		{
			name:      "dog pool",
			target:    "deacon/dogs",
			wantValid: true,
		},
		{
			name:      "specific dog",
			target:    "deacon/dogs/alpha",
			wantValid: true,
		},

		// Path-style valid targets (format validation only - rig existence checked separately)
		{
			name:      "polecat path",
			target:    "gastown/polecats/Toast",
			wantValid: true,
		},
		{
			name:      "crew path",
			target:    "gastown/crew/max",
			wantValid: true,
		},
		{
			name:      "witness path",
			target:    "gastown/witness",
			wantValid: true,
		},
		{
			name:      "refinery path",
			target:    "gastown/refinery",
			wantValid: true,
		},

		// Invalid targets
		{
			name:        "empty target",
			target:      "",
			wantValid:   false,
			wantReason:  "cannot be empty",
			wantSuggest: "Use '.'",
		},
		{
			name:        "mayor with subpath",
			target:      "mayor/something",
			wantValid:   false,
			wantReason:  "does not have sub-agents",
			wantSuggest: "Use 'mayor'",
		},
		{
			name:        "deacon invalid subpath",
			target:      "deacon/polecats",
			wantValid:   false,
			wantReason:  "deacon has no 'polecats' sub-path",
			wantSuggest: "deacon/dogs",
		},
		{
			name:        "dog path too deep",
			target:      "deacon/dogs/alpha/extra",
			wantValid:   false,
			wantReason:  "invalid dog target format",
			wantSuggest: "deacon/dogs/<name>",
		},
		{
			name:        "polecat missing name",
			target:      "gastown/polecats/",
			wantValid:   false,
			wantReason:  "polecat name cannot be empty",
			wantSuggest: "polecats/<name>",
		},
		{
			name:        "polecat only role",
			target:      "gastown/polecats",
			wantValid:   false,
			wantReason:  "polecat name is required",
			wantSuggest: "auto-spawn",
		},
		{
			name:        "polecat path too deep",
			target:      "gastown/polecats/Toast/extra",
			wantValid:   false,
			wantReason:  "too many path segments",
			wantSuggest: "gastown/polecats/Toast",
		},
		{
			name:        "crew missing name",
			target:      "gastown/crew/",
			wantValid:   false,
			wantReason:  "crew member name cannot be empty",
		},
		{
			name:        "crew only role",
			target:      "gastown/crew",
			wantValid:   false,
			wantReason:  "crew member name is required",
		},
		{
			name:        "crew path too deep",
			target:      "gastown/crew/max/extra",
			wantValid:   false,
			wantReason:  "too many path segments",
		},
		{
			name:        "witness with subpath",
			target:      "gastown/witness/extra",
			wantValid:   false,
			wantReason:  "does not have sub-agents",
		},
		{
			name:        "refinery with subpath",
			target:      "gastown/refinery/extra",
			wantValid:   false,
			wantReason:  "does not have sub-agents",
		},
		{
			name:        "invalid role typo",
			target:      "gastown/polecat/Toast",
			wantValid:   false,
			wantReason:  "not a valid role",
			wantSuggest: "polecats",
		},
		{
			name:        "invalid role workers",
			target:      "gastown/workers/Toast",
			wantValid:   false,
			wantReason:  "not a valid role",
			wantSuggest: "polecats",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateTarget(tt.target)

			if result.Valid != tt.wantValid {
				t.Errorf("ValidateTarget(%q).Valid = %v, want %v", tt.target, result.Valid, tt.wantValid)
			}

			if !tt.wantValid {
				if tt.wantReason != "" && !strings.Contains(result.Reason, tt.wantReason) {
					t.Errorf("ValidateTarget(%q).Reason = %q, want to contain %q", tt.target, result.Reason, tt.wantReason)
				}

				if tt.wantSuggest != "" {
					found := false
					for _, s := range result.Suggestions {
						if strings.Contains(s, tt.wantSuggest) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("ValidateTarget(%q).Suggestions does not contain %q\nGot: %v", tt.target, tt.wantSuggest, result.Suggestions)
					}
				}
			}
		})
	}
}

func TestFormatValidationError(t *testing.T) {
	tests := []struct {
		name   string
		result *TargetValidationResult
		want   string
	}{
		{
			name:   "valid result returns empty",
			result: &TargetValidationResult{Valid: true},
			want:   "",
		},
		{
			name: "invalid with reason only",
			result: &TargetValidationResult{
				Valid:  false,
				Reason: "target is invalid",
			},
			want: "target is invalid",
		},
		{
			name: "invalid with suggestions",
			result: &TargetValidationResult{
				Valid:       false,
				Reason:      "unknown target",
				Suggestions: []string{"Try this", "Or that"},
			},
			want: "Suggestions:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatValidationError(tt.result)
			if tt.want == "" && got != "" {
				t.Errorf("FormatValidationError() = %q, want empty", got)
			}
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Errorf("FormatValidationError() = %q, want to contain %q", got, tt.want)
			}
		})
	}
}

func TestValidateTargetTypoSuggestions(t *testing.T) {
	// Test that common typos get helpful suggestions
	typoTests := []struct {
		typo       string
		suggestion string
	}{
		{"mayors", "mayor"},
		{"deacons", "deacon"},
		{"dogs", "deacon/dogs"},
	}

	for _, tt := range typoTests {
		t.Run(tt.typo, func(t *testing.T) {
			result := ValidateTarget(tt.typo)
			if result.Valid {
				// Might be a valid rig name in some setups, skip
				t.Skip("target is valid (might be a rig name)")
			}

			found := false
			for _, s := range result.Suggestions {
				if strings.Contains(s, tt.suggestion) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ValidateTarget(%q) should suggest %q\nGot: %v", tt.typo, tt.suggestion, result.Suggestions)
			}
		})
	}
}
