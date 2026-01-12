package rig

import (
	"testing"
)

func TestBeadsPath_AlwaysReturnsRigRoot(t *testing.T) {
	t.Parallel()

	// BeadsPath should always return the rig root path, regardless of HasMayor.
	// The redirect system at <rig>/.beads/redirect handles finding the actual
	// beads location (either local at <rig>/.beads/ or tracked at mayor/rig/.beads/).
	//
	// This ensures:
	// 1. We don't write files to the user's repo clone (mayor/rig/)
	// 2. The redirect architecture is respected
	// 3. All code paths use the same beads resolution logic

	tests := []struct {
		name     string
		rig      Rig
		wantPath string
	}{
		{
			name: "rig with mayor only",
			rig: Rig{
				Name:     "testrig",
				Path:     "/home/user/gt/testrig",
				HasMayor: true,
			},
			wantPath: "/home/user/gt/testrig",
		},
		{
			name: "rig with witness only",
			rig: Rig{
				Name:       "testrig",
				Path:       "/home/user/gt/testrig",
				HasWitness: true,
			},
			wantPath: "/home/user/gt/testrig",
		},
		{
			name: "rig with refinery only",
			rig: Rig{
				Name:        "testrig",
				Path:        "/home/user/gt/testrig",
				HasRefinery: true,
			},
			wantPath: "/home/user/gt/testrig",
		},
		{
			name: "rig with no agents",
			rig: Rig{
				Name: "testrig",
				Path: "/home/user/gt/testrig",
			},
			wantPath: "/home/user/gt/testrig",
		},
		{
			name: "rig with mayor and witness",
			rig: Rig{
				Name:       "testrig",
				Path:       "/home/user/gt/testrig",
				HasMayor:   true,
				HasWitness: true,
			},
			wantPath: "/home/user/gt/testrig",
		},
		{
			name: "rig with mayor and refinery",
			rig: Rig{
				Name:        "testrig",
				Path:        "/home/user/gt/testrig",
				HasMayor:    true,
				HasRefinery: true,
			},
			wantPath: "/home/user/gt/testrig",
		},
		{
			name: "rig with witness and refinery",
			rig: Rig{
				Name:        "testrig",
				Path:        "/home/user/gt/testrig",
				HasWitness:  true,
				HasRefinery: true,
			},
			wantPath: "/home/user/gt/testrig",
		},
		{
			name: "rig with all agents",
			rig: Rig{
				Name:        "fullrig",
				Path:        "/tmp/gt/fullrig",
				HasMayor:    true,
				HasWitness:  true,
				HasRefinery: true,
			},
			wantPath: "/tmp/gt/fullrig",
		},
		{
			name: "rig with polecats",
			rig: Rig{
				Name:     "testrig",
				Path:     "/home/user/gt/testrig",
				HasMayor: true,
				Polecats: []string{"polecat1", "polecat2"},
			},
			wantPath: "/home/user/gt/testrig",
		},
		{
			name: "rig with crew",
			rig: Rig{
				Name:     "testrig",
				Path:     "/home/user/gt/testrig",
				HasMayor: true,
				Crew:     []string{"crew1", "crew2"},
			},
			wantPath: "/home/user/gt/testrig",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.rig.BeadsPath()
			if got != tt.wantPath {
				t.Errorf("BeadsPath() = %q, want %q", got, tt.wantPath)
			}
		})
	}
}

func TestDefaultBranch_FallsBackToMain(t *testing.T) {
	t.Parallel()

	// DefaultBranch should return "main" when config cannot be loaded
	rig := Rig{
		Name: "testrig",
		Path: "/nonexistent/path",
	}

	got := rig.DefaultBranch()
	if got != "main" {
		t.Errorf("DefaultBranch() = %q, want %q", got, "main")
	}
}
