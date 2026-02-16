package cmd

import (
	"strings"
	"testing"
)

func TestValidateTarget(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		wantErr bool
		errMsg  string // substring that must appear in error
	}{
		// Valid targets
		{name: "empty target", target: "", wantErr: false},
		{name: "self target", target: ".", wantErr: false},
		{name: "bare rig name", target: "gastown", wantErr: false},
		{name: "role shortcut mayor", target: "mayor", wantErr: false},
		{name: "role shortcut deacon", target: "deacon", wantErr: false},
		{name: "rig/polecats/name", target: "gastown/polecats/nux", wantErr: false},
		{name: "rig/crew/name", target: "gastown/crew/burke", wantErr: false},
		{name: "rig/witness", target: "gastown/witness", wantErr: false},
		{name: "rig/refinery", target: "gastown/refinery", wantErr: false},
		{name: "deacon/dogs", target: "deacon/dogs", wantErr: false},
		{name: "deacon/dogs/name", target: "deacon/dogs/rex", wantErr: false},
		{name: "polecat shorthand", target: "gastown/nux", wantErr: false},
		{name: "crew shorthand", target: "gastown/max", wantErr: false},

		// Invalid targets — empty segments
		{name: "trailing slash", target: "gastown/", wantErr: true, errMsg: "empty path segment"},
		{name: "double slash", target: "gastown//polecats", wantErr: true, errMsg: "empty path segment"},
		{name: "leading slash", target: "/polecats", wantErr: true, errMsg: "empty path segment"},

		// Invalid targets — unknown role (only rejected with 3+ segments)
		{name: "unknown role 3-seg", target: "gastown/badrole/name", wantErr: true, errMsg: "unknown role"},
		{name: "typo in role 3-seg", target: "gastown/polecat/name", wantErr: true, errMsg: "unknown role"},

		// Invalid targets — missing name
		{name: "crew no name", target: "gastown/crew", wantErr: true, errMsg: "requires a worker name"},
		{name: "polecats no name", target: "gastown/polecats", wantErr: true, errMsg: "requires a polecat name"},

		// Invalid targets — witness/refinery with sub-agents
		{name: "witness with name", target: "gastown/witness/extra", wantErr: true, errMsg: "does not have named sub-agents"},
		{name: "refinery with name", target: "gastown/refinery/extra", wantErr: true, errMsg: "does not have named sub-agents"},

		// Invalid targets — too many segments
		{name: "too many segments", target: "gastown/crew/burke/extra", wantErr: true, errMsg: "too many path segments"},

		// Invalid targets — mayor sub-paths
		{name: "mayor sub-agent", target: "mayor/something", wantErr: true, errMsg: "does not have sub-agents"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTarget(tc.target)
			if tc.wantErr && err == nil {
				t.Fatalf("ValidateTarget(%q) = nil, want error containing %q", tc.target, tc.errMsg)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("ValidateTarget(%q) = %v, want nil", tc.target, err)
			}
			if tc.wantErr && err != nil && tc.errMsg != "" {
				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Fatalf("ValidateTarget(%q) error = %q, want it to contain %q", tc.target, err.Error(), tc.errMsg)
				}
			}
		})
	}
}
