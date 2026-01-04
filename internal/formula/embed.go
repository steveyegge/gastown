package formula

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// Generate formulas directory from canonical source at .beads/formulas/
//go:generate sh -c "rm -rf formulas && mkdir -p formulas && cp ../../.beads/formulas/*.formula.toml ../../.beads/formulas/*.formula.json formulas/ 2>/dev/null || cp ../../.beads/formulas/*.formula.toml formulas/"

//go:embed formulas/*.formula.toml formulas/*.formula.json
var formulasFS embed.FS

// ProvisionFormulas creates the .beads/formulas/ directory with embedded formulas.
// This ensures new installations have the standard formula library.
// If a formula already exists, it is skipped (no overwrite).
// Returns the number of formulas provisioned.
func ProvisionFormulas(beadsPath string) (int, error) {
	entries, err := formulasFS.ReadDir("formulas")
	if err != nil {
		return 0, fmt.Errorf("reading formulas directory: %w", err)
	}

	// Create .beads/formulas/ directory
	formulasDir := filepath.Join(beadsPath, ".beads", "formulas")
	if err := os.MkdirAll(formulasDir, 0755); err != nil {
		return 0, fmt.Errorf("creating formulas directory: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		destPath := filepath.Join(formulasDir, entry.Name())

		// Skip if formula already exists (don't overwrite user customizations)
		if _, err := os.Stat(destPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			// Log unexpected errors (e.g., permission denied) but continue
			log.Printf("warning: could not check formula %s: %v", entry.Name(), err)
			continue
		}

		content, err := formulasFS.ReadFile("formulas/" + entry.Name())
		if err != nil {
			return count, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}

		if err := os.WriteFile(destPath, content, 0644); err != nil {
			return count, fmt.Errorf("writing %s: %w", entry.Name(), err)
		}
		count++
	}

	return count, nil
}
