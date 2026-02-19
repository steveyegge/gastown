package crew

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PersonaInfo describes an available persona file.
type PersonaInfo struct {
	Name      string // persona name (filename without .md)
	Source    string // "rig" or "town"
	Overrides bool   // true if rig-level overrides a town-level file
}

// ResolvePersonaFile loads a persona file by name using layered resolution.
// Checks rig-level first, then town-level. Returns content, source ("rig" or
// "town"), and error. Returns empty content and source when no file is found
// (not an error). Non-ENOENT filesystem errors are propagated.
func ResolvePersonaFile(
	townRoot, rigPath, name string,
) (string, string, error) {
	// Rig-level (highest priority)
	rigDir := filepath.Join(rigPath, ".personas")
	rigFile := filepath.Join(rigDir, name+".md")
	if !isUnderDir(rigDir, rigFile) {
		return "", "", fmt.Errorf("invalid persona name %q: path escapes personas directory", name)
	}
	if data, err := os.ReadFile(rigFile); err == nil {
		return string(data), "rig", nil
	} else if !os.IsNotExist(err) {
		return "", "", fmt.Errorf("reading persona file %s: %w", rigFile, err)
	}

	// Town-level (fallback)
	townDir := filepath.Join(townRoot, ".personas")
	townFile := filepath.Join(townDir, name+".md")
	if !isUnderDir(townDir, townFile) {
		return "", "", fmt.Errorf("invalid persona name %q: path escapes personas directory", name)
	}
	if data, err := os.ReadFile(townFile); err == nil {
		return string(data), "town", nil
	} else if !os.IsNotExist(err) {
		return "", "", fmt.Errorf("reading persona file %s: %w", townFile, err)
	}

	return "", "", nil
}

// ListPersonas returns all available personas from rig and town levels.
// Rig-level personas override town-level personas with the same name.
func ListPersonas(
	townRoot, rigPath string,
) ([]PersonaInfo, error) {
	townNames, err := readPersonaDir(filepath.Join(townRoot, ".personas"))
	if err != nil {
		return nil, err
	}
	rigNames, err := readPersonaDir(filepath.Join(rigPath, ".personas"))
	if err != nil {
		return nil, err
	}

	// Build merged map: rig overrides town
	merged := make(map[string]PersonaInfo)
	for _, name := range townNames {
		merged[name] = PersonaInfo{Name: name, Source: "town"}
	}
	for _, name := range rigNames {
		_, townExists := merged[name]
		merged[name] = PersonaInfo{
			Name:      name,
			Source:    "rig",
			Overrides: townExists,
		}
	}

	result := make([]PersonaInfo, 0, len(merged))
	for _, info := range merged {
		result = append(result, info)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

// ValidatePersonaName checks that a persona name is valid as a file stem.
// Allows hyphens, dots, and underscores; rejects path traversal characters.
func ValidatePersonaName(name string) error {
	return validatePersonaName(name)
}

// validatePersonaName is the internal implementation of name validation.
func validatePersonaName(name string) error {
	if name == "" {
		return fmt.Errorf("persona name cannot be empty")
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid persona name %q: path separators not allowed", name)
	}
	return nil
}

// isUnderDir reports whether child is under the parent directory.
func isUnderDir(parent, child string) bool {
	rel, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(child))
	return err == nil && !strings.HasPrefix(rel, "..")
}

// readPersonaDir returns persona names (without .md extension) from a directory.
// Returns nil slice (not an error) if the directory doesn't exist.
// Returns an error for non-ENOENT failures.
func readPersonaDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading personas directory %s: %w", dir, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		names = append(names, strings.TrimSuffix(entry.Name(), ".md"))
	}
	return names, nil
}
