package rig

import (
	"os"
	"path/filepath"
)

// bareRepoCandidates lists directory names to check for the shared bare repo,
// in priority order. ".repo.git" is the canonical name created by `gt rig add`.
// "repo.git" supports bridge rig layouts where the bare clone lives at
// rigs/<rig>/repo.git (without the dot prefix).
var bareRepoCandidates = []string{".repo.git", "repo.git"}

// FindBareRepo returns the absolute path of the bare repository directory
// inside rigPath, or "" if none exists. It checks candidates in priority order:
// .repo.git (standard), then repo.git (bridge rig layout).
func FindBareRepo(rigPath string) string {
	for _, name := range bareRepoCandidates {
		p := filepath.Join(rigPath, name)
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	return ""
}
