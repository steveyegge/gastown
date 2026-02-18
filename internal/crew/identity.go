package crew

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// IdentityInfo describes an available identity file.
type IdentityInfo struct {
	Name      string // identity name (filename without .md)
	Source    string // "rig" or "town"
	Overrides bool   // true if rig-level overrides a town-level file
}

// ResolveIdentityFile loads an identity file by name using layered
// resolution. Checks rig-level first, then town-level. Returns
// content, source ("rig" or "town"), and error. Returns empty
// content and source when no file is found (not an error).
func ResolveIdentityFile(
	townRoot, rigPath, name string,
) (string, string, error) {
	// Rig-level (highest priority)
	rigFile := filepath.Join(rigPath, "identities", name+".md")
	if data, err := os.ReadFile(rigFile); err == nil {
		return string(data), "rig", nil
	}

	// Town-level (fallback)
	townFile := filepath.Join(townRoot, "identities", name+".md")
	if data, err := os.ReadFile(townFile); err == nil {
		return string(data), "town", nil
	}

	return "", "", nil
}

// ListIdentities returns all available identities from rig and town
// levels. Rig-level identities override town-level identities with
// the same name.
func ListIdentities(
	townRoot, rigPath string,
) ([]IdentityInfo, error) {
	townNames := readIdentityDir(
		filepath.Join(townRoot, "identities"),
	)
	rigNames := readIdentityDir(
		filepath.Join(rigPath, "identities"),
	)

	// Build merged map: rig overrides town
	merged := make(map[string]IdentityInfo)
	for _, name := range townNames {
		merged[name] = IdentityInfo{Name: name, Source: "town"}
	}
	for _, name := range rigNames {
		_, townExists := merged[name]
		merged[name] = IdentityInfo{
			Name:      name,
			Source:    "rig",
			Overrides: townExists,
		}
	}

	result := make([]IdentityInfo, 0, len(merged))
	for _, info := range merged {
		result = append(result, info)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

// ValidateIdentityName checks that an identity name is valid.
// Uses the same rules as crew names.
func ValidateIdentityName(name string) error {
	return validateCrewName(name)
}

// readIdentityDir returns identity names (without .md extension)
// from a directory. Returns nil if the directory doesn't exist.
func readIdentityDir(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		names = append(
			names,
			strings.TrimSuffix(entry.Name(), ".md"),
		)
	}
	return names
}
