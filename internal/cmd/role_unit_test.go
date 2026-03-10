package cmd

import (
	"path/filepath"
	"testing"
)

// TestDetectRole_AllRoles validates cwd-based role detection for every known role.
func TestDetectRole_AllRoles(t *testing.T) {
	townRoot := "/tmp/gt"

	tests := []struct {
		name        string
		cwd         string
		wantRole    Role
		wantRig     string
		wantPolecat string
	}{
		// Town root → unknown
		{
			name:     "town root",
			cwd:      townRoot,
			wantRole: RoleUnknown,
		},
		// Mayor
		{
			name:     "mayor at town level",
			cwd:      filepath.Join(townRoot, "mayor"),
			wantRole: RoleMayor,
		},
		{
			name:     "mayor rig subdirectory",
			cwd:      filepath.Join(townRoot, "mayor", "rig"),
			wantRole: RoleMayor,
		},
		{
			name:     "mayor at rig level",
			cwd:      filepath.Join(townRoot, "gastown", "mayor"),
			wantRole: RoleMayor,
			wantRig:  "gastown",
		},
		{
			name:     "mayor at rig level with rig subdir",
			cwd:      filepath.Join(townRoot, "gastown", "mayor", "rig"),
			wantRole: RoleMayor,
			wantRig:  "gastown",
		},
		// Deacon
		{
			name:     "deacon",
			cwd:      filepath.Join(townRoot, "deacon"),
			wantRole: RoleDeacon,
		},
		{
			name:     "deacon subdirectory",
			cwd:      filepath.Join(townRoot, "deacon", "state"),
			wantRole: RoleDeacon,
		},
		// Boot (under deacon/dogs/boot)
		{
			name:     "boot",
			cwd:      filepath.Join(townRoot, "deacon", "dogs", "boot"),
			wantRole: RoleBoot,
		},
		{
			name:     "boot subdirectory",
			cwd:      filepath.Join(townRoot, "deacon", "dogs", "boot", "rig"),
			wantRole: RoleBoot,
		},
		// Dog (under deacon/dogs/<name>)
		{
			name:        "dog alpha",
			cwd:         filepath.Join(townRoot, "deacon", "dogs", "alpha"),
			wantRole:    RoleDog,
			wantPolecat: "alpha",
		},
		{
			name:        "dog beta subdirectory",
			cwd:         filepath.Join(townRoot, "deacon", "dogs", "beta", "rig"),
			wantRole:    RoleDog,
			wantPolecat: "beta",
		},
		// Witness
		{
			name:     "witness",
			cwd:      filepath.Join(townRoot, "gastown", "witness"),
			wantRole: RoleWitness,
			wantRig:  "gastown",
		},
		{
			name:     "witness rig subdirectory",
			cwd:      filepath.Join(townRoot, "gastown", "witness", "rig"),
			wantRole: RoleWitness,
			wantRig:  "gastown",
		},
		// Refinery
		{
			name:     "refinery",
			cwd:      filepath.Join(townRoot, "gastown", "refinery"),
			wantRole: RoleRefinery,
			wantRig:  "gastown",
		},
		{
			name:     "refinery rig subdirectory",
			cwd:      filepath.Join(townRoot, "gastown", "refinery", "rig"),
			wantRole: RoleRefinery,
			wantRig:  "gastown",
		},
		// Polecat
		{
			name:        "polecat Toast",
			cwd:         filepath.Join(townRoot, "gastown", "polecats", "Toast"),
			wantRole:    RolePolecat,
			wantRig:     "gastown",
			wantPolecat: "Toast",
		},
		{
			name:        "polecat Toast rig subdirectory",
			cwd:         filepath.Join(townRoot, "gastown", "polecats", "Toast", "rig"),
			wantRole:    RolePolecat,
			wantRig:     "gastown",
			wantPolecat: "Toast",
		},
		{
			name:        "polecat Imperator",
			cwd:         filepath.Join(townRoot, "gastown", "polecats", "Imperator"),
			wantRole:    RolePolecat,
			wantRig:     "gastown",
			wantPolecat: "Imperator",
		},
		// Crew
		{
			name:        "crew dave",
			cwd:         filepath.Join(townRoot, "gastown", "crew", "dave"),
			wantRole:    RoleCrew,
			wantRig:     "gastown",
			wantPolecat: "dave",
		},
		{
			name:        "crew dave rig subdirectory",
			cwd:         filepath.Join(townRoot, "gastown", "crew", "dave", "rig"),
			wantRole:    RoleCrew,
			wantRig:     "gastown",
			wantPolecat: "dave",
		},
		// Edge cases: incomplete paths
		{
			name:     "rig root is unknown",
			cwd:      filepath.Join(townRoot, "gastown"),
			wantRole: RoleUnknown,
			wantRig:  "gastown",
		},
		{
			name:     "polecats without name is unknown",
			cwd:      filepath.Join(townRoot, "gastown", "polecats"),
			wantRole: RoleUnknown,
			wantRig:  "gastown",
		},
		{
			name:     "crew without name is unknown",
			cwd:      filepath.Join(townRoot, "gastown", "crew"),
			wantRole: RoleUnknown,
			wantRig:  "gastown",
		},
		// Different rig names
		{
			name:     "witness in different rig",
			cwd:      filepath.Join(townRoot, "beads", "witness"),
			wantRole: RoleWitness,
			wantRig:  "beads",
		},
		{
			name:        "polecat in different rig",
			cwd:         filepath.Join(townRoot, "myproject", "polecats", "Worker1"),
			wantRole:    RolePolecat,
			wantRig:     "myproject",
			wantPolecat: "Worker1",
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
			if info.Source != "cwd" {
				t.Errorf("detectRole() source = %q, want %q", info.Source, "cwd")
			}
		})
	}
}

// TestParseRoleString_AllRoles validates parsing of all role string formats.
func TestParseRoleString_AllRoles(t *testing.T) {
	tests := []struct {
		input    string
		wantRole Role
		wantRig  string
		wantName string
	}{
		// Simple roles
		{"mayor", RoleMayor, "", ""},
		{"deacon", RoleDeacon, "", ""},
		{"boot", RoleBoot, "", ""},
		{"dog", RoleDog, "", ""},

		// Compound: rig/role
		{"gastown/witness", RoleWitness, "gastown", ""},
		{"gastown/refinery", RoleRefinery, "gastown", ""},
		{"beads/witness", RoleWitness, "beads", ""},
		{"myproject/refinery", RoleRefinery, "myproject", ""},

		// Compound: rig/polecats/name
		{"gastown/polecats/Toast", RolePolecat, "gastown", "Toast"},
		{"gastown/polecats/Imperator", RolePolecat, "gastown", "Imperator"},
		{"beads/polecats/Worker1", RolePolecat, "beads", "Worker1"},

		// Compound: rig/crew/name
		{"gastown/crew/dave", RoleCrew, "gastown", "dave"},
		{"gastown/crew/emma", RoleCrew, "gastown", "emma"},
		{"beads/crew/mel", RoleCrew, "beads", "mel"},

		// Compound: deacon/boot
		{"deacon/boot", RoleBoot, "", ""},

		// Compound: polecats without name
		{"gastown/polecats", RolePolecat, "gastown", ""},

		// Compound: crew without name
		{"gastown/crew", RoleCrew, "gastown", ""},

		// Slash normalization
		{"gastown//witness", RoleWitness, "gastown", ""},
		{"gastown///refinery", RoleRefinery, "gastown", ""},
		{"gastown/polecats/Toast/", RolePolecat, "gastown", "Toast"},
		{"gastown//polecats//alpha", RolePolecat, "gastown", "alpha"},

		// Whitespace trimming
		{"  mayor  ", RoleMayor, "", ""},
		{" gastown/witness ", RoleWitness, "gastown", ""},

		// Default case: rig/unknownName → treated as polecat
		{"gastown/someWorker", RolePolecat, "gastown", "someWorker"},

		// Non-deacon compound boot → unrecognized
		{"west/boot", Role("west/boot"), "", ""},
		{"deacon/boot/extra", Role("deacon/boot/extra"), "", ""},

		// Unknown simple role
		{"randomrole", Role("randomrole"), "", ""},
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

// TestGetRoleHome_AllRoles validates home directory resolution for all roles.
func TestGetRoleHome_AllRoles(t *testing.T) {
	townRoot := "/tmp/gt"

	tests := []struct {
		name     string
		role     Role
		rig      string
		polecat  string
		wantHome string
	}{
		// Town-level roles
		{"mayor", RoleMayor, "", "", filepath.Join(townRoot, "mayor")},
		{"deacon", RoleDeacon, "", "", filepath.Join(townRoot, "deacon")},
		{"boot", RoleBoot, "", "", filepath.Join(townRoot, "deacon", "dogs", "boot")},
		{"dog alpha", RoleDog, "", "alpha", filepath.Join(townRoot, "deacon", "dogs", "alpha")},

		// Rig-level roles
		{"witness", RoleWitness, "gastown", "", filepath.Join(townRoot, "gastown", "witness")},
		{"refinery", RoleRefinery, "gastown", "", filepath.Join(townRoot, "gastown", "refinery", "rig")},
		{"polecat", RolePolecat, "gastown", "Toast", filepath.Join(townRoot, "gastown", "polecats", "Toast")},
		{"crew", RoleCrew, "gastown", "dave", filepath.Join(townRoot, "gastown", "crew", "dave")},

		// Different rig
		{"witness in beads rig", RoleWitness, "beads", "", filepath.Join(townRoot, "beads", "witness")},
		{"polecat in beads rig", RolePolecat, "beads", "Worker1", filepath.Join(townRoot, "beads", "polecats", "Worker1")},

		// Missing required fields → empty string
		{"witness without rig", RoleWitness, "", "", ""},
		{"refinery without rig", RoleRefinery, "", "", ""},
		{"polecat without rig", RolePolecat, "", "Toast", ""},
		{"polecat without name", RolePolecat, "gastown", "", ""},
		{"crew without rig", RoleCrew, "", "dave", ""},
		{"crew without name", RoleCrew, "gastown", "", ""},
		{"dog without name", RoleDog, "", "", ""},

		// Unknown role
		{"unknown", RoleUnknown, "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getRoleHome(tt.role, tt.rig, tt.polecat, townRoot)
			if got != tt.wantHome {
				t.Errorf("getRoleHome(%v, %q, %q) = %q, want %q", tt.role, tt.rig, tt.polecat, got, tt.wantHome)
			}
		})
	}
}

// TestActorString_AllRoles validates actor identity string for all roles.
func TestActorString_AllRoles(t *testing.T) {
	tests := []struct {
		name string
		info RoleInfo
		want string
	}{
		// Town-level roles
		{"mayor", RoleInfo{Role: RoleMayor}, "mayor"},
		{"deacon", RoleInfo{Role: RoleDeacon}, "deacon"},
		{"boot", RoleInfo{Role: RoleBoot}, "deacon-boot"},

		// Rig-level roles with full info
		{"witness with rig", RoleInfo{Role: RoleWitness, Rig: "gastown"}, "gastown/witness"},
		{"refinery with rig", RoleInfo{Role: RoleRefinery, Rig: "gastown"}, "gastown/refinery"},
		{"polecat with rig and name", RoleInfo{Role: RolePolecat, Rig: "gastown", Polecat: "Toast"}, "gastown/polecats/Toast"},
		{"crew with rig and name", RoleInfo{Role: RoleCrew, Rig: "gastown", Polecat: "dave"}, "gastown/crew/dave"},

		// Rig-level roles without full info → fallback
		{"witness without rig", RoleInfo{Role: RoleWitness}, "witness"},
		{"refinery without rig", RoleInfo{Role: RoleRefinery}, "refinery"},
		{"polecat without rig", RoleInfo{Role: RolePolecat, Polecat: "Toast"}, "polecat"},
		{"polecat without name", RoleInfo{Role: RolePolecat, Rig: "gastown"}, "polecat"},
		{"crew without rig", RoleInfo{Role: RoleCrew, Polecat: "dave"}, "crew"},
		{"crew without name", RoleInfo{Role: RoleCrew, Rig: "gastown"}, "crew"},

		// Different rigs
		{"witness in beads rig", RoleInfo{Role: RoleWitness, Rig: "beads"}, "beads/witness"},
		{"polecat in beads rig", RoleInfo{Role: RolePolecat, Rig: "beads", Polecat: "Worker1"}, "beads/polecats/Worker1"},

		// Dog role
		{"dog", RoleInfo{Role: RoleDog}, "dog"},

		// Unknown role
		{"unknown", RoleInfo{Role: RoleUnknown}, "unknown"},
		{"custom role", RoleInfo{Role: Role("custom")}, "custom"},
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

// TestDetectRole_EmptyAndDotPaths validates edge cases for path detection.
func TestDetectRole_EmptyAndDotPaths(t *testing.T) {
	townRoot := "/tmp/gt"

	tests := []struct {
		name     string
		cwd      string
		wantRole Role
	}{
		{"town root exact", townRoot, RoleUnknown},
		{"empty relative path", townRoot + "/", RoleUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := detectRole(tt.cwd, townRoot)
			if info.Role != tt.wantRole {
				t.Errorf("detectRole(%q, %q) role = %v, want %v", tt.cwd, townRoot, info.Role, tt.wantRole)
			}
		})
	}
}
