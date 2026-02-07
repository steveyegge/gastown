package formula

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Embedded formulas - this directory (internal/formula/formulas/) is the source of truth.
//
// To add or modify formulas, edit files directly in internal/formula/formulas/
// Do NOT edit .beads/formulas/ - that directory is for user overrides only.
//
// Formula resolution order (most specific wins):
//   1. Rig:      <rig>/.beads/formulas/     (project-specific)
//   2. Town:     $GT_ROOT/.beads/formulas/  (user customizations)
//   3. Embedded: (compiled in binary)        (defaults, this directory)

//go:embed formulas/*.formula.toml
var formulasFS embed.FS

// formulaNameToFilename converts a formula name to its filename.
// If the name already has the .formula.toml suffix, returns it as-is.
// Otherwise, appends .formula.toml.
func formulaNameToFilename(name string) string {
	const suffix = ".formula.toml"
	if len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix {
		return name
	}
	return name + suffix
}

// filenameToFormulaName converts a filename to a formula name by removing the .formula.toml suffix.
func filenameToFormulaName(filename string) string {
	const suffix = ".formula.toml"
	if len(filename) > len(suffix) && filename[len(filename)-len(suffix):] == suffix {
		return filename[:len(filename)-len(suffix)]
	}
	return filename
}

// GetEmbeddedFormula returns the content of an embedded formula by name.
// The name can be provided with or without the .formula.toml suffix.
// Returns an error if the formula does not exist.
func GetEmbeddedFormula(name string) ([]byte, error) {
	filename := formulaNameToFilename(name)
	content, err := formulasFS.ReadFile("formulas/" + filename)
	if err != nil {
		return nil, fmt.Errorf("embedded formula %q not found: %w", name, err)
	}
	return content, nil
}

// GetEmbeddedFormulaNames returns a list of all embedded formula names.
// The names are returned without the .formula.toml suffix.
func GetEmbeddedFormulaNames() ([]string, error) {
	entries, err := formulasFS.ReadDir("formulas")
	if err != nil {
		return nil, fmt.Errorf("reading formulas directory: %w", err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, filenameToFormulaName(entry.Name()))
	}
	return names, nil
}

// EmbeddedFormulaExists returns true if an embedded formula with the given name exists.
// The name can be provided with or without the .formula.toml suffix.
func EmbeddedFormulaExists(name string) bool {
	filename := formulaNameToFilename(name)
	_, err := formulasFS.ReadFile("formulas/" + filename)
	return err == nil
}

// GetEmbeddedFormulaHash computes and returns the SHA-256 hash of an embedded formula.
// The name can be provided with or without the .formula.toml suffix.
func GetEmbeddedFormulaHash(name string) (string, error) {
	content, err := GetEmbeddedFormula(name)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:]), nil
}

// ExtractBaseHash extracts the hash from the "# Based on embedded version: sha256:XXXX"
// line in a formula file's content. Returns empty string if not found.
func ExtractBaseHash(content []byte) string {
	const prefix = "# Based on embedded version: sha256:"
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(line[len(prefix):])
		}
	}
	return ""
}

// CopyFormulaTo copies an embedded formula to the specified destination path.
// This is used by `gt formula modify` to create a local override.
// The destination path should be a directory (e.g., ~/.beads/formulas/).
// A hash comment header is prepended to track which embedded version the override is based on.
// Returns the full path to the copied file.
func CopyFormulaTo(name, destDir string) (string, error) {
	content, err := GetEmbeddedFormula(name)
	if err != nil {
		return "", err
	}

	// Compute hash of the embedded content
	hash := sha256.Sum256(content)
	hashStr := hex.EncodeToString(hash[:])

	// Prepend hash header
	header := fmt.Sprintf("# Formula override created by gt formula modify\n# Based on embedded version: sha256:%s\n# To update: gt formula update %s\n\n", hashStr, name)
	contentWithHeader := append([]byte(header), content...)

	// Ensure destination directory exists
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("creating destination directory: %w", err)
	}

	filename := formulaNameToFilename(name)
	destPath := filepath.Join(destDir, filename)

	// Check if file already exists
	if _, err := os.Stat(destPath); err == nil {
		return "", fmt.Errorf("formula override already exists at %s", destPath)
	}

	if err := os.WriteFile(destPath, contentWithHeader, 0644); err != nil {
		return "", fmt.Errorf("writing formula: %w", err)
	}

	return destPath, nil
}
