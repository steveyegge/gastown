package cmd

import "testing"

func TestExtractRigFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantRig  string
		wantArgs []string
	}{
		{
			name:     "no rig flag",
			args:     []string{"--status=open", "--label", "backend"},
			wantRig:  "",
			wantArgs: []string{"--status=open", "--label", "backend"},
		},
		{
			name:     "rig flag with separate value",
			args:     []string{"--rig", "nw", "--status=open"},
			wantRig:  "nw",
			wantArgs: []string{"--status=open"},
		},
		{
			name:     "rig flag with equals form",
			args:     []string{"--rig=hq", "--status=open"},
			wantRig:  "hq",
			wantArgs: []string{"--status=open"},
		},
		{
			name:     "rig flag at end",
			args:     []string{"--status=open", "--rig", "gt"},
			wantRig:  "gt",
			wantArgs: []string{"--status=open"},
		},
		{
			name:     "rig flag among other flags",
			args:     []string{"--label", "bug", "--rig", "nw", "--all"},
			wantRig:  "nw",
			wantArgs: []string{"--label", "bug", "--all"},
		},
		{
			name:     "empty args",
			args:     []string{},
			wantRig:  "",
			wantArgs: nil,
		},
		{
			name:     "rig flag with no following value (edge case)",
			args:     []string{"--rig"},
			wantRig:  "",
			wantArgs: []string{"--rig"},
		},
		{
			name:     "rig equals empty value",
			args:     []string{"--rig="},
			wantRig:  "",
			wantArgs: nil,
		},
		{
			name:     "positional args preserved",
			args:     []string{"auth bug", "--rig", "nw", "--status", "open"},
			wantRig:  "nw",
			wantArgs: []string{"auth bug", "--status", "open"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRig, gotArgs := extractRigFlag(tt.args)
			if gotRig != tt.wantRig {
				t.Errorf("extractRigFlag(%v) rig = %q, want %q", tt.args, gotRig, tt.wantRig)
			}
			if len(gotArgs) != len(tt.wantArgs) {
				t.Fatalf("extractRigFlag(%v) args = %v (len %d), want %v (len %d)",
					tt.args, gotArgs, len(gotArgs), tt.wantArgs, len(tt.wantArgs))
			}
			for i := range gotArgs {
				if gotArgs[i] != tt.wantArgs[i] {
					t.Errorf("extractRigFlag(%v) args[%d] = %q, want %q",
						tt.args, i, gotArgs[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func TestRouteForPrefix(t *testing.T) {
	// RouteForPrefix with empty prefix should be a no-op
	bdc := &bdCmd{
		args:   []string{"list"},
		env:    []string{"PATH=/usr/bin", "BEADS_DIR=/town/.beads"},
		stderr: nil,
	}

	result := bdc.RouteForPrefix("")
	if result != bdc {
		t.Error("RouteForPrefix('') should return receiver")
	}
	// Dir should remain unchanged (no routing for empty prefix)
	if bdc.dir != "" {
		t.Errorf("dir = %q, want empty (no routing for empty prefix)", bdc.dir)
	}
}

func TestRouteForPrefix_Chaining(t *testing.T) {
	bdc := BdCmd("list")
	if bdc.RouteForPrefix("nw-") != bdc {
		t.Error("RouteForPrefix() should return receiver for chaining")
	}
}

func TestRouteForBead_Chaining(t *testing.T) {
	bdc := BdCmd("show", "id")
	if bdc.RouteForBead("nw-abc") != bdc {
		t.Error("RouteForBead() should return receiver for chaining")
	}
}
