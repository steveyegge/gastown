package rig

// Layout encapsulates all rig directory conventions.
// Two implementations: ClassicLayout (current) and DotfileLayout (future .gastown/).
type Layout interface {
	// RigRoot returns the top-level rig directory.
	// Classic: ~/gt/<rigName>
	// Dotfile: ~/projects/myapp/.gastown
	RigRoot() string

	// InfraRoot returns where gastown infrastructure lives.
	// Classic: same as RigRoot
	// Dotfile: <projectRoot>/.gastown
	InfraRoot() string

	// CanonicalClone returns the path to the main code checkout.
	// Classic: <rigRoot>/mayor/rig
	// Dotfile: <projectRoot> (the project root itself)
	CanonicalClone() string

	// BareRepo returns the path to the shared bare git repository.
	BareRepo() string

	// BeadsDir returns the path to the rig-level beads directory.
	BeadsDir() string

	// CanonicalBeadsDir returns the path to beads in the canonical clone.
	CanonicalBeadsDir() string

	// RuntimeDir returns the path to the runtime state directory.
	RuntimeDir() string

	// LocksDir returns the path to the runtime locks directory.
	LocksDir() string

	// OverlayDir returns the path to the runtime overlay directory.
	OverlayDir() string

	// SetupHooksDir returns the path to the setup hooks directory.
	SetupHooksDir() string

	// ConfigFile returns the path to the rig config.json.
	ConfigFile() string

	// SettingsDir returns the path to the rig settings directory.
	SettingsDir() string

	// RolesDir returns the path to the rig roles directory.
	RolesDir() string

	// PluginsDir returns the path to the rig plugins directory.
	PluginsDir() string

	// WitnessDir returns the witness agent home directory.
	WitnessDir() string

	// RefineryWorktree returns the refinery merge worktree path.
	RefineryWorktree() string

	// ConductorDir returns the conductor agent directory.
	ConductorDir() string

	// ConductorFeaturesDir returns where conductor feature state is stored.
	ConductorFeaturesDir() string

	// SpecialtiesFile returns the path to conductor/specialties.toml.
	SpecialtiesFile() string

	// ArchitectDir returns the architect agent directory.
	ArchitectDir() string

	// ArtisanDir returns the directory for a named artisan.
	ArtisanDir(name string) string

	// ArtisansBaseDir returns the base directory containing all artisans.
	ArtisansBaseDir() string

	// CrewDir returns the directory for a named crew member.
	CrewDir(name string) string

	// PolecatDir returns the directory for a named polecat.
	PolecatDir(name string) string

	// MayorDir returns the mayor clone directory (same as CanonicalClone in classic).
	MayorDir() string

	// GitignoreFile returns the path to the rig .gitignore.
	GitignoreFile() string
}
