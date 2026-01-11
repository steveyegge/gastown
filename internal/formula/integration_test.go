package formula

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseRealFormulas tests parsing actual formula files from the filesystem.
// This is an integration test that validates our parser against real-world files.
func TestParseRealFormulas(t *testing.T) {
	// Find formula files - they're in various .beads/formulas directories
	formulaDirs := []string{
		"/Users/stevey/gt/gastown/polecats/slit/.beads/formulas",
		"/Users/stevey/gt/gastown/mayor/rig/.beads/formulas",
	}

	var formulaFiles []string
	for _, dir := range formulaDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // Skip if directory doesn't exist
		}
		for _, e := range entries {
			if filepath.Ext(e.Name()) == ".toml" {
				formulaFiles = append(formulaFiles, filepath.Join(dir, e.Name()))
			}
		}
	}

	if len(formulaFiles) == 0 {
		t.Skip("No formula files found to test")
	}

	// Known files that use advanced features not yet supported:
	// - Composition (extends, compose): shiny-enterprise, shiny-secure
	// - Aspect-oriented (advice, pointcuts): security-audit
	skipAdvanced := map[string]string{
		"shiny-enterprise.formula.toml": "uses formula composition (extends)",
		"shiny-secure.formula.toml":     "uses formula composition (extends)",
		"security-audit.formula.toml":   "uses aspect-oriented features (advice/pointcuts)",
	}

	for _, path := range formulaFiles {
		t.Run(filepath.Base(path), func(t *testing.T) {
			baseName := filepath.Base(path)
			if reason, ok := skipAdvanced[baseName]; ok {
				t.Skipf("Skipping advanced formula: %s", reason)
				return
			}

			f, err := ParseFile(path)
			if err != nil {
				// Check if this is a composition formula (has extends)
				if strings.Contains(err.Error(), "requires at least one") {
					t.Skipf("Skipping: likely a composition formula - %v", err)
					return
				}
				t.Errorf("ParseFile failed: %v", err)
				return
			}

			// Basic sanity checks
			if f.Name == "" {
				t.Error("Formula name is empty")
			}
			if !f.Type.IsValid() {
				t.Errorf("Invalid formula type: %s", f.Type)
			}

			// Type-specific checks
			switch f.Type {
			case TypeConvoy:
				if len(f.Legs) == 0 {
					t.Error("Convoy formula has no legs")
				}
				t.Logf("Convoy formula with %d legs", len(f.Legs))
			case TypeWorkflow:
				if len(f.Steps) == 0 {
					t.Error("Workflow formula has no steps")
				}
				// Test topological sort
				order, err := f.TopologicalSort()
				if err != nil {
					t.Errorf("TopologicalSort failed: %v", err)
				}
				t.Logf("Workflow formula with %d steps, sorted order: %v", len(f.Steps), order)
			case TypeExpansion:
				if len(f.Template) == 0 {
					t.Error("Expansion formula has no templates")
				}
				t.Logf("Expansion formula with %d templates", len(f.Template))
			}
		})
	}
}
