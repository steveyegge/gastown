package rig

import "path/filepath"

// ClassicLayout implements Layout for the current rig directory structure
// where infrastructure and code are mixed at the rig root level.
//
// Structure:
//
//	~/gt/<rigName>/
//	├── config.json
//	├── .repo.git/
//	├── .beads/
//	├── .runtime/
//	├── settings/
//	├── roles/
//	├── mayor/rig/          (canonical clone)
//	├── refinery/rig/
//	├── witness/
//	├── crew/<name>/
//	├── polecats/<name>/
//	├── artisans/<name>/
//	├── conductor/
//	└── architect/
type ClassicLayout struct {
	rigPath string
}

// NewClassicLayout creates a ClassicLayout for the given rig path.
func NewClassicLayout(rigPath string) *ClassicLayout {
	return &ClassicLayout{rigPath: rigPath}
}

func (l *ClassicLayout) RigRoot() string           { return l.rigPath }
func (l *ClassicLayout) InfraRoot() string          { return l.rigPath }
func (l *ClassicLayout) CanonicalClone() string     { return filepath.Join(l.rigPath, "mayor", "rig") }
func (l *ClassicLayout) BareRepo() string           { return filepath.Join(l.rigPath, ".repo.git") }
func (l *ClassicLayout) BeadsDir() string           { return filepath.Join(l.rigPath, ".beads") }
func (l *ClassicLayout) CanonicalBeadsDir() string  { return filepath.Join(l.rigPath, "mayor", "rig", ".beads") }
func (l *ClassicLayout) RuntimeDir() string         { return filepath.Join(l.rigPath, ".runtime") }
func (l *ClassicLayout) LocksDir() string           { return filepath.Join(l.rigPath, ".runtime", "locks") }
func (l *ClassicLayout) OverlayDir() string         { return filepath.Join(l.rigPath, ".runtime", "overlay") }
func (l *ClassicLayout) SetupHooksDir() string      { return filepath.Join(l.rigPath, ".runtime", "setup-hooks") }
func (l *ClassicLayout) ConfigFile() string         { return filepath.Join(l.rigPath, "config.json") }
func (l *ClassicLayout) SettingsDir() string        { return filepath.Join(l.rigPath, "settings") }
func (l *ClassicLayout) RolesDir() string           { return filepath.Join(l.rigPath, "roles") }
func (l *ClassicLayout) PluginsDir() string         { return filepath.Join(l.rigPath, "plugins") }
func (l *ClassicLayout) WitnessDir() string         { return filepath.Join(l.rigPath, "witness") }
func (l *ClassicLayout) RefineryWorktree() string   { return filepath.Join(l.rigPath, "refinery", "rig") }
func (l *ClassicLayout) ConductorDir() string       { return filepath.Join(l.rigPath, "conductor") }
func (l *ClassicLayout) ConductorFeaturesDir() string { return filepath.Join(l.rigPath, "conductor", "features") }
func (l *ClassicLayout) SpecialtiesFile() string    { return filepath.Join(l.rigPath, "conductor", "specialties.toml") }
func (l *ClassicLayout) ArchitectDir() string       { return filepath.Join(l.rigPath, "architect") }
func (l *ClassicLayout) ArtisansBaseDir() string    { return filepath.Join(l.rigPath, "artisans") }
func (l *ClassicLayout) GitignoreFile() string      { return filepath.Join(l.rigPath, ".gitignore") }
func (l *ClassicLayout) MayorDir() string           { return filepath.Join(l.rigPath, "mayor", "rig") }

func (l *ClassicLayout) ArtisanDir(name string) string {
	return filepath.Join(l.rigPath, "artisans", name)
}

func (l *ClassicLayout) CrewDir(name string) string {
	return filepath.Join(l.rigPath, "crew", name)
}

func (l *ClassicLayout) PolecatDir(name string) string {
	return filepath.Join(l.rigPath, "polecats", name)
}
