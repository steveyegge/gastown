package cmd

import (
	"path/filepath"
	"testing"
)

// Phase 4: Specification tests for the Artisan role.
// These tests define the contract for artisan support in the role system.
// They should FAIL until the implementation is complete.

// TestDetectRole_Artisan validates cwd-based detection for artisan directories.
func TestDetectRole_Artisan(t *testing.T) {
	townRoot := "/tmp/gt"

	tests := []struct {
		name        string
		cwd         string
		wantRole    Role
		wantRig     string
		wantPolecat string
	}{
		{
			name:        "artisan in rig",
			cwd:         filepath.Join(townRoot, "gastown", "artisans", "frontend-1"),
			wantRole:    RoleArtisan,
			wantRig:     "gastown",
			wantPolecat: "frontend-1",
		},
		{
			name:        "artisan rig subdirectory",
			cwd:         filepath.Join(townRoot, "gastown", "artisans", "frontend-1", "rig"),
			wantRole:    RoleArtisan,
			wantRig:     "gastown",
			wantPolecat: "frontend-1",
		},
		{
			name:        "artisan backend",
			cwd:         filepath.Join(townRoot, "gastown", "artisans", "backend-1"),
			wantRole:    RoleArtisan,
			wantRig:     "gastown",
			wantPolecat: "backend-1",
		},
		{
			name:        "artisan tests",
			cwd:         filepath.Join(townRoot, "gastown", "artisans", "tests-1"),
			wantRole:    RoleArtisan,
			wantRig:     "gastown",
			wantPolecat: "tests-1",
		},
		{
			name:        "artisan docs",
			cwd:         filepath.Join(townRoot, "gastown", "artisans", "docs-1"),
			wantRole:    RoleArtisan,
			wantRig:     "gastown",
			wantPolecat: "docs-1",
		},
		{
			name:        "artisan security",
			cwd:         filepath.Join(townRoot, "gastown", "artisans", "security-1"),
			wantRole:    RoleArtisan,
			wantRig:     "gastown",
			wantPolecat: "security-1",
		},
		{
			name:        "artisan in different rig",
			cwd:         filepath.Join(townRoot, "beads", "artisans", "frontend-1"),
			wantRole:    RoleArtisan,
			wantRig:     "beads",
			wantPolecat: "frontend-1",
		},
		{
			name:     "artisans dir without name is unknown",
			cwd:      filepath.Join(townRoot, "gastown", "artisans"),
			wantRole: RoleUnknown,
			wantRig:  "gastown",
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
			if info.Polecat != tt.wantPolecat {
				t.Errorf("detectRole() polecat = %q, want %q", info.Polecat, tt.wantPolecat)
			}
		})
	}
}

// TestParseRoleString_Artisan validates parsing of artisan role strings.
func TestParseRoleString_Artisan(t *testing.T) {
	tests := []struct {
		input    string
		wantRole Role
		wantRig  string
		wantName string
	}{
		{"gastown/artisans/frontend-1", RoleArtisan, "gastown", "frontend-1"},
		{"gastown/artisans/backend-1", RoleArtisan, "gastown", "backend-1"},
		{"gastown/artisans/tests-1", RoleArtisan, "gastown", "tests-1"},
		{"beads/artisans/docs-1", RoleArtisan, "beads", "docs-1"},
		{"gastown/artisans", RoleArtisan, "gastown", ""},
		// Slash normalization
		{"gastown//artisans//frontend-1", RoleArtisan, "gastown", "frontend-1"},
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

// TestGetRoleHome_Artisan validates home directory resolution for artisans.
func TestGetRoleHome_Artisan(t *testing.T) {
	townRoot := "/tmp/gt"

	tests := []struct {
		name     string
		rig      string
		polecat  string
		wantHome string
	}{
		{"frontend-1", "gastown", "frontend-1", filepath.Join(townRoot, "gastown", "artisans", "frontend-1")},
		{"backend-1", "gastown", "backend-1", filepath.Join(townRoot, "gastown", "artisans", "backend-1")},
		{"different rig", "beads", "tests-1", filepath.Join(townRoot, "beads", "artisans", "tests-1")},
		{"missing rig", "", "frontend-1", ""},
		{"missing name", "gastown", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getRoleHome(RoleArtisan, tt.rig, tt.polecat, townRoot)
			if got != tt.wantHome {
				t.Errorf("getRoleHome(RoleArtisan, %q, %q) = %q, want %q", tt.rig, tt.polecat, got, tt.wantHome)
			}
		})
	}
}

// TestActorString_Artisan validates actor identity string for artisans.
func TestActorString_Artisan(t *testing.T) {
	tests := []struct {
		name string
		info RoleInfo
		want string
	}{
		{"with rig and name", RoleInfo{Role: RoleArtisan, Rig: "gastown", Polecat: "frontend-1"}, "gastown/artisans/frontend-1"},
		{"without rig", RoleInfo{Role: RoleArtisan, Polecat: "frontend-1"}, "artisan"},
		{"without name", RoleInfo{Role: RoleArtisan, Rig: "gastown"}, "artisan"},
		{"bare", RoleInfo{Role: RoleArtisan}, "artisan"},
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
