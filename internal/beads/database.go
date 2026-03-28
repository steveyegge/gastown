package beads

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// DatabaseNameFromMetadata reads the dolt_database field from .beads/metadata.json.
// Returns empty string if metadata doesn't exist or has no database configured.
func DatabaseNameFromMetadata(beadsDir string) string {
	data, err := os.ReadFile(filepath.Join(beadsDir, "metadata.json"))
	if err != nil {
		return ""
	}
	var meta struct {
		DoltDatabase string `json:"dolt_database"`
	}
	if json.Unmarshal(data, &meta) != nil {
		return ""
	}
	return meta.DoltDatabase
}

// DatabaseEnv returns the BEADS_DOLT_SERVER_DATABASE=<name> env var string
// for the given beadsDir, or empty string if no database is configured.
func DatabaseEnv(beadsDir string) string {
	db := DatabaseNameFromMetadata(beadsDir)
	if db == "" {
		return ""
	}
	return "BEADS_DOLT_SERVER_DATABASE=" + db
}
