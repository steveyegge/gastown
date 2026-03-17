package rig

import (
	"path/filepath"
	"testing"
)

func TestClassicLayout(t *testing.T) {
	t.Parallel()
	rigPath := "/home/user/gt/myrig"
	layout := NewClassicLayout(rigPath)

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"RigRoot", layout.RigRoot(), rigPath},
		{"InfraRoot", layout.InfraRoot(), rigPath},
		{"CanonicalClone", layout.CanonicalClone(), filepath.Join(rigPath, "mayor", "rig")},
		{"BareRepo", layout.BareRepo(), filepath.Join(rigPath, ".repo.git")},
		{"BeadsDir", layout.BeadsDir(), filepath.Join(rigPath, ".beads")},
		{"CanonicalBeadsDir", layout.CanonicalBeadsDir(), filepath.Join(rigPath, "mayor", "rig", ".beads")},
		{"RuntimeDir", layout.RuntimeDir(), filepath.Join(rigPath, ".runtime")},
		{"LocksDir", layout.LocksDir(), filepath.Join(rigPath, ".runtime", "locks")},
		{"OverlayDir", layout.OverlayDir(), filepath.Join(rigPath, ".runtime", "overlay")},
		{"SetupHooksDir", layout.SetupHooksDir(), filepath.Join(rigPath, ".runtime", "setup-hooks")},
		{"ConfigFile", layout.ConfigFile(), filepath.Join(rigPath, "config.json")},
		{"SettingsDir", layout.SettingsDir(), filepath.Join(rigPath, "settings")},
		{"RolesDir", layout.RolesDir(), filepath.Join(rigPath, "roles")},
		{"PluginsDir", layout.PluginsDir(), filepath.Join(rigPath, "plugins")},
		{"WitnessDir", layout.WitnessDir(), filepath.Join(rigPath, "witness")},
		{"RefineryWorktree", layout.RefineryWorktree(), filepath.Join(rigPath, "refinery", "rig")},
		{"ConductorDir", layout.ConductorDir(), filepath.Join(rigPath, "conductor")},
		{"ConductorFeaturesDir", layout.ConductorFeaturesDir(), filepath.Join(rigPath, "conductor", "features")},
		{"SpecialtiesFile", layout.SpecialtiesFile(), filepath.Join(rigPath, "conductor", "specialties.toml")},
		{"ArchitectDir", layout.ArchitectDir(), filepath.Join(rigPath, "architect")},
		{"ArtisansBaseDir", layout.ArtisansBaseDir(), filepath.Join(rigPath, "artisans")},
		{"ArtisanDir", layout.ArtisanDir("frontend-1"), filepath.Join(rigPath, "artisans", "frontend-1")},
		{"CrewDir", layout.CrewDir("mel"), filepath.Join(rigPath, "crew", "mel")},
		{"PolecatDir", layout.PolecatDir("nux"), filepath.Join(rigPath, "polecats", "nux")},
		{"MayorDir", layout.MayorDir(), filepath.Join(rigPath, "mayor", "rig")},
		{"GitignoreFile", layout.GitignoreFile(), filepath.Join(rigPath, ".gitignore")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}

func TestClassicLayout_ImplementsLayout(t *testing.T) {
	t.Parallel()
	var _ Layout = (*ClassicLayout)(nil)
}
