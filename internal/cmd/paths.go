package cmd

import (
	"os"
	"path/filepath"
)

// gtDataDir returns the directory used for GT's runtime data files
// (logs, telemetry, cost records, etc.).
//
// Resolution order:
//  1. $GT_HOME/.gt  — when GT_HOME is set, data is kept alongside the GT
//     workspace rather than in the user's home directory.
//  2. ~/.gt         — default location when GT_HOME is not set.
func gtDataDir() string {
	if h := os.Getenv("GT_HOME"); h != "" {
		return filepath.Join(h, ".gt")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".gt")
	}
	return filepath.Join(home, ".gt")
}
