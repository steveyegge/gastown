package upgrade

// Binary and archive naming constants.
const (
	// BinaryName is the name of the gt binary on Unix systems.
	BinaryName = "gt"

	// BinaryNameWindows is the name of the gt binary on Windows.
	BinaryNameWindows = "gt.exe"

	// ArchiveExtTarGz is the extension for tar.gz archives (Unix).
	ArchiveExtTarGz = ".tar.gz"

	// ArchiveExtZip is the extension for zip archives (Windows).
	ArchiveExtZip = ".zip"

	// ChecksumFileSuffix is the suffix for checksum files in releases.
	ChecksumFileSuffix = "_checksums.txt"

	// ChecksumFileName is the alternative name for checksum files.
	ChecksumFileName = "checksums.txt"

	// CompatibilityFileName is the name of the compatibility info file in releases.
	CompatibilityFileName = "compatibility.json"
)

// Workspace file paths.
const (
	// MayorDirName is the name of the mayor directory.
	MayorDirName = "mayor"

	// TownConfigFileName is the name of the town configuration file.
	TownConfigFileName = "town.json"

	// RigsConfigFileName is the name of the rigs configuration file.
	RigsConfigFileName = "rigs.json"
)
