package cmd

import (
	"path/filepath"
	"testing"
)

// Phase 4: Specification tests for the Architect role.
// These tests define the contract for architect support in the role system.
// They should FAIL until the implementation is complete.

// TestDetectRole_Architect validates cwd-based detection for architect directories.
func TestDetectRole_Architect(t *testing.T) {
	townRoot := "/tmp/gt"

	tests := []struct {
		name     string
		cwd      string
		wantRole Role
		wantRig  string
	}{
		{
			name:     "architect in rig",
			cwd:      filepath.Join(townRoot, "gastown", "architect"),
			wantRole: RoleArchitect,
			wantRig:  "gastown",
		},
		{
			name:     "architect rig subdirectory",
			cwd:      filepath.Join(townRoot, "gastown", "architect", "rig"),
			wantRole: RoleArchitect,
			wantRig:  "gastown",
		},
		{
			name:     "architect in different rig",
			cwd:      filepath.Join(townRoot, "beads", "architect"),
			wantRole: RoleArchitect,
			wantRig:  "beads",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := detectRole(tt.cwd, townRoot)
			if info.Role != tt.wantRole {
				t.Errorf("detectRole() role = %v, want %v", info.Role, tt.wantRole)
			}
			if info.Rig != tt.wantRig {
				t.Errorf("detectRole() rig = %q, want %q", info.Rig, tt.wantRig)
			}
		})
	}
}

// TestParseRoleString_Architect validates parsing of architect role strings.
func TestParseRoleString_Architect(t *testing.T) {
	tests := []struct {
		input    string
		wantRole Role
		wantRig  string
		wantName string
	}{
		{"gastown/architect", RoleArchitect, "gastown", ""},
		{"beads/architect", RoleArchitect, "beads", ""},
		// Slash normalization
		{"gastown//architect", RoleArchitect, "gastown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
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
		})
	}
}

// TestGetRoleHome_Architect validates home directory resolution for architect.
func TestGetRoleHome_Architect(t *testing.T) {
	townRoot := "/tmp/gt"

	tests := []struct {
		name     string
		rig      string
		wantHome string
	}{
		{"gastown", "gastown", filepath.Join(townRoot, "gastown", "architect")},
		{"beads", "beads", filepath.Join(townRoot, "beads", "architect")},
		{"missing rig", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getRoleHome(RoleArchitect, tt.rig, "", townRoot)
			if got != tt.wantHome {
				t.Errorf("getRoleHome(RoleArchitect, %q) = %q, want %q", tt.rig, got, tt.wantHome)
			}
		})
	}
}

// TestActorString_Architect validates actor identity string for architect.
func TestActorString_Architect(t *testing.T) {
	tests := []struct {
		name string
		info RoleInfo
		want string
	}{
		{"with rig", RoleInfo{Role: RoleArchitect, Rig: "gastown"}, "gastown/architect"},
		{"without rig", RoleInfo{Role: RoleArchitect}, "architect"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info.ActorString()
			if got != tt.want {
				t.Errorf("ActorString() = %q, want %q", got, tt.want)
			}
		})
	}
}
