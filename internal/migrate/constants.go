package migrate

// File and directory names used in migrations.
const (
	// MayorDir is the name of the mayor directory.
	MayorDir = "mayor"

	// BeadsDir is the name of the beads directory.
	BeadsDir = ".beads"

	// RuntimeDir is the name of the runtime directory.
	RuntimeDir = ".runtime"

	// SettingsDir is the name of the settings directory.
	SettingsDir = "settings"

	// LegacyGastownDir is the name of the legacy .gastown directory.
	LegacyGastownDir = ".gastown"
)

// Configuration file names.
const (
	// TownConfigFile is the name of the town configuration file.
	TownConfigFile = "town.json"

	// RigsConfigFile is the name of the rigs configuration file.
	RigsConfigFile = "rigs.json"

	// AccountsConfigFile is the name of the accounts configuration file.
	AccountsConfigFile = "accounts.json"

	// BeadsConfigFile is the name of the beads configuration file.
	BeadsConfigFile = "config.json"

	// BeadsRoutesFile is the name of the beads routes file.
	BeadsRoutesFile = "routes.jsonl"

	// BeadsSequenceFile is the name of the beads sequence file.
	BeadsSequenceFile = "next_sequence.json"

	// GitignoreFile is the name of the gitignore file.
	GitignoreFile = ".gitignore"
)

// RigMarkers are directory names that indicate a directory is a rig.
var RigMarkers = []string{"crew", "polecats", "witness", "refinery"}

// ExcludedDirs are directory names that should not be considered as rigs.
var ExcludedDirs = []string{MayorDir, BeadsDir}

// BackupFiles returns the list of files/directories to backup during migration.
func BackupFiles() []string {
	return []string{
		TownConfigFile,                                    // Legacy root-level config
		RigsConfigFile,                                    // Legacy root-level rigs
		MayorDir + "/" + TownConfigFile,                   // New location config
		MayorDir + "/" + RigsConfigFile,                   // New location rigs
		MayorDir + "/" + AccountsConfigFile,               // Accounts config
		BeadsDir + "/" + BeadsConfigFile,                  // Beads config
		BeadsDir + "/" + BeadsRoutesFile,                  // Beads routing
		BeadsDir + "/" + BeadsSequenceFile,                // Beads sequence
		GitignoreFile,                                     // Git ignore file
	}
}

// ConfigFilesToMove returns the list of config files to move during v0.1 to v0.2 migration.
func ConfigFilesToMove() []string {
	return []string{TownConfigFile, RigsConfigFile, AccountsConfigFile}
}
