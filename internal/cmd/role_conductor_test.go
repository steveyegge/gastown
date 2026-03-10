package cmd

import (
	"path/filepath"
	"testing"
)

// Phase 4: Specification tests for the Conductor role.
// These tests define the contract for conductor support in the role system.
// They should FAIL until the implementation is complete.

// TestDetectRole_Conductor validates cwd-based detection for conductor directories.
func TestDetectRole_Conductor(t *testing.T) {
	townRoot := "/tmp/gt"

	tests := []struct {
		name     string
		cwd      string
		wantRole Role
		wantRig  string
	}{
		{
			name:     "conductor in rig",
			cwd:      filepath.Join(townRoot, "gastown", "conductor"),
			wantRole: RoleConductor,
			wantRig:  "gastown",
		},
		{
			name:     "conductor rig subdirectory",
			cwd:      filepath.Join(townRoot, "gastown", "conductor", "rig"),
			wantRole: RoleConductor,
			wantRig:  "gastown",
		},
		{
			name:     "conductor in different rig",
			cwd:      filepath.Join(townRoot, "beads", "conductor"),
			wantRole: RoleConductor,
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

// TestParseRoleString_Conductor validates parsing of conductor role strings.
func TestParseRoleString_Conductor(t *testing.T) {
	tests := []struct {
		input    string
		wantRole Role
		wantRig  string
		wantName string
	}{
		{"gastown/conductor", RoleConductor, "gastown", ""},
		{"beads/conductor", RoleConductor, "beads", ""},
		// Slash normalization
		{"gastown//conductor", RoleConductor, "gastown", ""},
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

// TestGetRoleHome_Conductor validates home directory resolution for conductor.
func TestGetRoleHome_Conductor(t *testing.T) {
	townRoot := "/tmp/gt"

	tests := []struct {
		name     string
		rig      string
		wantHome string
	}{
		{"gastown", "gastown", filepath.Join(townRoot, "gastown", "conductor")},
		{"beads", "beads", filepath.Join(townRoot, "beads", "conductor")},
		{"missing rig", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getRoleHome(RoleConductor, tt.rig, "", townRoot)
			if got != tt.wantHome {
				t.Errorf("getRoleHome(RoleConductor, %q) = %q, want %q", tt.rig, got, tt.wantHome)
			}
		})
	}
}

// TestActorString_Conductor validates actor identity string for conductor.
func TestActorString_Conductor(t *testing.T) {
	tests := []struct {
		name string
		info RoleInfo
		want string
	}{
		{"with rig", RoleInfo{Role: RoleConductor, Rig: "gastown"}, "gastown/conductor"},
		{"without rig", RoleInfo{Role: RoleConductor}, "conductor"},
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
