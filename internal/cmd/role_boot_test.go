package cmd

import (
	"path/filepath"
	"testing"
)

func TestParseRoleStringBoot(t *testing.T) {
	tests := []struct {
		input    string
		wantRole Role
		wantRig  string
		wantName string
	}{
		// Simple "boot" → RoleBoot
		{"boot", RoleBoot, "", ""},
		// Compound "deacon/boot" → RoleBoot
		{"deacon/boot", RoleBoot, "", ""},
		// Non-deacon compound should NOT match RoleBoot
		{"west/boot", Role("west/boot"), "", ""},
		// Extra path segments should NOT match RoleBoot
		{"deacon/boot/extra", Role("deacon/boot/extra"), "", ""},
	}

	for _, tt := range tests {
		role, rig, name := parseRoleString(tt.input)
		if role != tt.wantRole {
			t.Errorf("parseRoleString(%q) role = %v, want %v", tt.input, role, tt.wantRole)
		}
		if rig != tt.wantRig {
			t.Errorf("parseRoleString(%q) rig = %q, want %q", tt.input, rig, tt.wantRig)
		}
		if name != tt.wantName {
			t.Errorf("parseRoleString(%q) name = %q, want %q", tt.input, name, tt.wantName)
		}
	}
}

func TestGetRoleHomeBoot(t *testing.T) {
	townRoot := "/tmp/gt"
	got := getRoleHome(RoleBoot, "", "", townRoot)
	want := filepath.Join(townRoot, "deacon", "dogs", "boot")
	if got != want {
		t.Errorf("getRoleHome(RoleBoot) = %q, want %q", got, want)
	}
}

func TestIsTownLevelRoleBoot(t *testing.T) {
	tests := []struct {
		agentID string
		want    bool
	}{
		{"boot", true},
		{"deacon/boot", true},
		{"mayor", true},
		{"mayor/", true},
		{"deacon", true},
		{"deacon/", true},
		{"gastown/witness", false},
		{"west/boot", false},
	}

	for _, tt := range tests {
		got := isTownLevelRole(tt.agentID)
		if got != tt.want {
			t.Errorf("isTownLevelRole(%q) = %v, want %v", tt.agentID, got, tt.want)
		}
	}
}
