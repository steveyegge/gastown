// Package epic provides upstream contribution workflow support for epics.
package epic

import (
	"os"
	"path/filepath"
)

// ContributingLocations lists standard locations for CONTRIBUTING.md files.
var ContributingLocations = []string{
	"CONTRIBUTING.md",
	"docs/CONTRIBUTING.md",
	".github/CONTRIBUTING.md",
}

// DiscoverContributing finds CONTRIBUTING.md in a rig's repo.
// It searches the standard locations and returns the path and content if found.
// Returns empty strings if no CONTRIBUTING.md exists (not an error).
func DiscoverContributing(rigPath string) (path, content string, err error) {
	for _, loc := range ContributingLocations {
		fullPath := filepath.Join(rigPath, loc)
		data, err := os.ReadFile(fullPath)
		if err == nil {
			return loc, string(data), nil
		}
		// Ignore errors (file not found is expected)
		if !os.IsNotExist(err) {
			// Real error reading file
			continue
		}
	}
	// No CONTRIBUTING.md found - this is not an error
	return "", "", nil
}

// ContributingExists checks if a CONTRIBUTING.md file exists in the rig.
func ContributingExists(rigPath string) bool {
	path, _, _ := DiscoverContributing(rigPath)
	return path != ""
}

// GetContributingPath returns the path to CONTRIBUTING.md if it exists.
func GetContributingPath(rigPath string) string {
	path, _, _ := DiscoverContributing(rigPath)
	if path == "" {
		return ""
	}
	return filepath.Join(rigPath, path)
}
