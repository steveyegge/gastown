// Package beads provides molecule catalog support for hierarchical template loading.
package beads

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CatalogMolecule represents a molecule template in the catalog.
// Unlike regular issues, catalog molecules are read-only templates.
type CatalogMolecule struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Source      string `json:"source,omitempty"` // "town", "rig", "project"
}

// MoleculeCatalog provides hierarchical molecule template loading.
// It loads molecules from multiple sources in priority order:
// 1. Town-level: <town>/.beads/molecules.jsonl
// 2. Rig-level: <town>/<rig>/.beads/molecules.jsonl
// 3. Project-level: .beads/molecules.jsonl in current directory
//
// Later sources can override earlier ones by ID.
type MoleculeCatalog struct {
	molecules map[string]*CatalogMolecule // ID -> molecule
	order     []string                    // Insertion order for listing
}

// NewMoleculeCatalog creates an empty catalog.
func NewMoleculeCatalog() *MoleculeCatalog {
	return &MoleculeCatalog{
		molecules: make(map[string]*CatalogMolecule),
		order:     make([]string, 0),
	}
}

// LoadCatalog creates a catalog with all molecule sources loaded.
// Parameters:
//   - townRoot: Path to the Gas Town root (e.g., ~/gt). Empty to skip town-level.
//   - rigPath: Path to the rig directory (e.g., ~/gt/gastown). Empty to skip rig-level.
//   - projectPath: Path to the project directory. Empty to skip project-level.
//
// Molecules are loaded from town, rig, and project levels (no builtin molecules).
// Each level follows .beads/redirect if present (for shared beads support).
func LoadCatalog(townRoot, rigPath, projectPath string) (*MoleculeCatalog, error) {
	catalog := NewMoleculeCatalog()

	// 1. Load town-level molecules (follows redirect if present)
	if townRoot != "" {
		townBeadsDir := ResolveBeadsDir(townRoot)
		townMolsPath := filepath.Join(townBeadsDir, "molecules.jsonl")
		if err := catalog.LoadFromFile(townMolsPath, "town"); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("loading town molecules: %w", err)
		}
	}

	// 2. Load rig-level molecules (follows redirect if present)
	if rigPath != "" {
		rigBeadsDir := ResolveBeadsDir(rigPath)
		rigMolsPath := filepath.Join(rigBeadsDir, "molecules.jsonl")
		if err := catalog.LoadFromFile(rigMolsPath, "rig"); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("loading rig molecules: %w", err)
		}
	}

	// 3. Load project-level molecules (follows redirect if present)
	if projectPath != "" {
		projectBeadsDir := ResolveBeadsDir(projectPath)
		projectMolsPath := filepath.Join(projectBeadsDir, "molecules.jsonl")
		if err := catalog.LoadFromFile(projectMolsPath, "project"); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("loading project molecules: %w", err)
		}
	}

	return catalog, nil
}

// Add adds or replaces a molecule in the catalog.
func (c *MoleculeCatalog) Add(mol *CatalogMolecule) {
	if _, exists := c.molecules[mol.ID]; !exists {
		c.order = append(c.order, mol.ID)
	}
	c.molecules[mol.ID] = mol
}

// Get returns a molecule by ID, or nil if not found.
func (c *MoleculeCatalog) Get(id string) *CatalogMolecule {
	return c.molecules[id]
}

// List returns all molecules in insertion order.
func (c *MoleculeCatalog) List() []*CatalogMolecule {
	result := make([]*CatalogMolecule, 0, len(c.order))
	for _, id := range c.order {
		if mol, ok := c.molecules[id]; ok {
			result = append(result, mol)
		}
	}
	return result
}

// Count returns the number of molecules in the catalog.
func (c *MoleculeCatalog) Count() int {
	return len(c.molecules)
}

// LoadFromFile loads molecules from a JSONL file.
// Each line should be a JSON object with id, title, and description fields.
// The source parameter is added to each loaded molecule.
func (c *MoleculeCatalog) LoadFromFile(path, source string) error {
	file, err := os.Open(path) //nolint:gosec // G304: path is from trusted molecule catalog locations
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		var mol CatalogMolecule
		if err := json.Unmarshal([]byte(line), &mol); err != nil {
			return fmt.Errorf("line %d: %w", lineNum, err)
		}

		if mol.ID == "" {
			return fmt.Errorf("line %d: molecule missing id", lineNum)
		}

		mol.Source = source
		c.Add(&mol)
	}

	return scanner.Err()
}

// SaveToFile writes all molecules to a JSONL file.
// This is useful for exporting the catalog or creating template files.
func (c *MoleculeCatalog) SaveToFile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	for _, mol := range c.List() {
		// Don't include source in exported file
		exportMol := struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			Description string `json:"description"`
		}{
			ID:          mol.ID,
			Title:       mol.Title,
			Description: mol.Description,
		}
		if err := encoder.Encode(exportMol); err != nil {
			return err
		}
	}

	return nil
}

// ToIssue converts a catalog molecule to an Issue struct for compatibility.
// The issue has Type="molecule" and is marked as a template.
func (mol *CatalogMolecule) ToIssue() *Issue {
	return &Issue{
		ID:          mol.ID,
		Title:       mol.Title,
		Description: mol.Description,
		Type:        "molecule",
		Status:      "open",
		Priority:    2,
	}
}

