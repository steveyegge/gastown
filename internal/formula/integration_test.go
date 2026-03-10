package formula

import (
	"io/fs"
	"strings"
	"testing"
)

// TestParseRealFormulas tests parsing all embedded formula files.
// Composition formulas (extends/compose) are now also resolved and validated.
func TestParseRealFormulas(t *testing.T) {
	// Formulas that use aspect-oriented features not yet implemented.
	skipFormulas := map[string]string{
		"security-audit.formula.toml": "uses aspect-oriented features (advice/pointcuts)",
	}

	entries, err := fs.ReadDir(formulasFS, "formulas")
	if err != nil {
		t.Fatalf("reading embedded formulas: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".formula.toml") {
			continue
		}
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			if reason, ok := skipFormulas[name]; ok {
				t.Skipf("skipping: %s", reason)
				return
			}

			data, err := formulasFS.ReadFile("formulas/" + name)
			if err != nil {
				t.Fatalf("reading formula: %v", err)
			}

			f, err := Parse(data)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			if f.Name == "" {
				t.Error("Formula name is empty")
			}
			if !f.Type.IsValid() {
				t.Errorf("Invalid formula type: %s", f.Type)
			}

			// Resolve composition formulas (extends/compose).
			if len(f.Extends) > 0 || f.Compose != nil {
				resolved, err := Resolve(f, nil)
				if err != nil {
					t.Fatalf("Resolve: %v", err)
				}
				f = resolved
				t.Logf("Resolved composition formula with %d steps", len(f.Steps))
			}

			// Type-specific checks on the (possibly resolved) formula.
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
