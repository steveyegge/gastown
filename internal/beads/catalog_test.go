package beads

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewMoleculeCatalog(t *testing.T) {
	cat := NewMoleculeCatalog()
	if cat == nil {
		t.Fatal("expected non-nil catalog")
	}
	if cat.Count() != 0 {
		t.Errorf("new catalog should be empty, got count %d", cat.Count())
	}
	if list := cat.List(); len(list) != 0 {
		t.Errorf("new catalog list should be empty, got %d items", len(list))
	}
}

func TestCatalogAddAndGet(t *testing.T) {
	cat := NewMoleculeCatalog()

	mol := &CatalogMolecule{
		ID:          "code-review",
		Title:       "Code Review",
		Description: "Perform a code review",
		Source:      "town",
	}
	cat.Add(mol)

	if cat.Count() != 1 {
		t.Errorf("count = %d, want 1", cat.Count())
	}

	got := cat.Get("code-review")
	if got == nil {
		t.Fatal("Get returned nil for existing molecule")
	}
	if got.ID != mol.ID {
		t.Errorf("ID = %q, want %q", got.ID, mol.ID)
	}
	if got.Title != mol.Title {
		t.Errorf("Title = %q, want %q", got.Title, mol.Title)
	}
	if got.Description != mol.Description {
		t.Errorf("Description = %q, want %q", got.Description, mol.Description)
	}
	if got.Source != mol.Source {
		t.Errorf("Source = %q, want %q", got.Source, mol.Source)
	}
}

func TestCatalogGetNotFound(t *testing.T) {
	cat := NewMoleculeCatalog()
	if got := cat.Get("nonexistent"); got != nil {
		t.Errorf("expected nil for missing molecule, got %+v", got)
	}
}

func TestCatalogAddOverride(t *testing.T) {
	cat := NewMoleculeCatalog()

	mol1 := &CatalogMolecule{ID: "review", Title: "Town Review", Source: "town"}
	mol2 := &CatalogMolecule{ID: "review", Title: "Rig Review", Source: "rig"}

	cat.Add(mol1)
	cat.Add(mol2)

	// Count should still be 1 (override, not duplicate)
	if cat.Count() != 1 {
		t.Errorf("count = %d, want 1 after override", cat.Count())
	}

	// Should have the overridden version
	got := cat.Get("review")
	if got.Title != "Rig Review" {
		t.Errorf("Title = %q, want %q (should be overridden)", got.Title, "Rig Review")
	}
	if got.Source != "rig" {
		t.Errorf("Source = %q, want %q", got.Source, "rig")
	}
}

func TestCatalogListOrder(t *testing.T) {
	cat := NewMoleculeCatalog()

	cat.Add(&CatalogMolecule{ID: "alpha", Title: "Alpha"})
	cat.Add(&CatalogMolecule{ID: "beta", Title: "Beta"})
	cat.Add(&CatalogMolecule{ID: "gamma", Title: "Gamma"})

	list := cat.List()
	if len(list) != 3 {
		t.Fatalf("list length = %d, want 3", len(list))
	}

	// Should be in insertion order
	expected := []string{"alpha", "beta", "gamma"}
	for i, want := range expected {
		if list[i].ID != want {
			t.Errorf("list[%d].ID = %q, want %q", i, list[i].ID, want)
		}
	}
}

func TestCatalogListOrderAfterOverride(t *testing.T) {
	cat := NewMoleculeCatalog()

	cat.Add(&CatalogMolecule{ID: "first", Title: "First v1"})
	cat.Add(&CatalogMolecule{ID: "second", Title: "Second"})
	cat.Add(&CatalogMolecule{ID: "first", Title: "First v2"}) // Override

	list := cat.List()
	if len(list) != 2 {
		t.Fatalf("list length = %d, want 2", len(list))
	}

	// "first" should keep its original position
	if list[0].ID != "first" || list[0].Title != "First v2" {
		t.Errorf("list[0] = {%q, %q}, want {first, First v2}", list[0].ID, list[0].Title)
	}
	if list[1].ID != "second" {
		t.Errorf("list[1].ID = %q, want %q", list[1].ID, "second")
	}
}

func TestCatalogLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	molsPath := filepath.Join(dir, "molecules.jsonl")

	content := `{"id": "review", "title": "Code Review", "description": "Review code changes"}
{"id": "deploy", "title": "Deploy", "description": "Deploy to production"}
`
	if err := os.WriteFile(molsPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cat := NewMoleculeCatalog()
	if err := cat.LoadFromFile(molsPath, "test"); err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}

	if cat.Count() != 2 {
		t.Errorf("count = %d, want 2", cat.Count())
	}

	review := cat.Get("review")
	if review == nil {
		t.Fatal("review molecule not found")
	}
	if review.Title != "Code Review" {
		t.Errorf("review.Title = %q, want %q", review.Title, "Code Review")
	}
	if review.Source != "test" {
		t.Errorf("review.Source = %q, want %q", review.Source, "test")
	}

	deploy := cat.Get("deploy")
	if deploy == nil {
		t.Fatal("deploy molecule not found")
	}
	if deploy.Title != "Deploy" {
		t.Errorf("deploy.Title = %q, want %q", deploy.Title, "Deploy")
	}
}

func TestCatalogLoadFromFileSkipsCommentsAndBlanks(t *testing.T) {
	dir := t.TempDir()
	molsPath := filepath.Join(dir, "molecules.jsonl")

	content := `# This is a comment
{"id": "first", "title": "First", "description": ""}

// Another comment
{"id": "second", "title": "Second", "description": ""}
`
	if err := os.WriteFile(molsPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cat := NewMoleculeCatalog()
	if err := cat.LoadFromFile(molsPath, "test"); err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}

	if cat.Count() != 2 {
		t.Errorf("count = %d, want 2 (should skip comments and blanks)", cat.Count())
	}
}

func TestCatalogLoadFromFileMissingID(t *testing.T) {
	dir := t.TempDir()
	molsPath := filepath.Join(dir, "molecules.jsonl")

	content := `{"title": "No ID", "description": "Missing ID field"}`
	if err := os.WriteFile(molsPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cat := NewMoleculeCatalog()
	err := cat.LoadFromFile(molsPath, "test")
	if err == nil {
		t.Fatal("expected error for missing ID")
	}
}

func TestCatalogLoadFromFileInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	molsPath := filepath.Join(dir, "molecules.jsonl")

	content := `not valid json`
	if err := os.WriteFile(molsPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cat := NewMoleculeCatalog()
	err := cat.LoadFromFile(molsPath, "test")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestCatalogLoadFromFileNotFound(t *testing.T) {
	cat := NewMoleculeCatalog()
	err := cat.LoadFromFile("/nonexistent/path/molecules.jsonl", "test")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected not-exist error, got: %v", err)
	}
}

func TestCatalogSaveToFile(t *testing.T) {
	cat := NewMoleculeCatalog()
	cat.Add(&CatalogMolecule{ID: "alpha", Title: "Alpha", Description: "First", Source: "town"})
	cat.Add(&CatalogMolecule{ID: "beta", Title: "Beta", Description: "Second", Source: "rig"})

	dir := t.TempDir()
	outPath := filepath.Join(dir, "exported.jsonl")

	if err := cat.SaveToFile(outPath); err != nil {
		t.Fatalf("SaveToFile error: %v", err)
	}

	// Load back and verify
	loaded := NewMoleculeCatalog()
	if err := loaded.LoadFromFile(outPath, "reimported"); err != nil {
		t.Fatalf("LoadFromFile error: %v", err)
	}

	if loaded.Count() != 2 {
		t.Errorf("reloaded count = %d, want 2", loaded.Count())
	}

	alpha := loaded.Get("alpha")
	if alpha == nil {
		t.Fatal("alpha not found after save/load")
	}
	if alpha.Title != "Alpha" {
		t.Errorf("alpha.Title = %q, want %q", alpha.Title, "Alpha")
	}
	// Source should be overwritten by LoadFromFile
	if alpha.Source != "reimported" {
		t.Errorf("alpha.Source = %q, want %q (source from LoadFromFile)", alpha.Source, "reimported")
	}
}

func TestCatalogMoleculeToIssue(t *testing.T) {
	mol := &CatalogMolecule{
		ID:          "test-mol",
		Title:       "Test Molecule",
		Description: "A test molecule template",
		Source:      "town",
	}

	issue := mol.ToIssue()
	if issue.ID != mol.ID {
		t.Errorf("ID = %q, want %q", issue.ID, mol.ID)
	}
	if issue.Title != mol.Title {
		t.Errorf("Title = %q, want %q", issue.Title, mol.Title)
	}
	if issue.Description != mol.Description {
		t.Errorf("Description = %q, want %q", issue.Description, mol.Description)
	}
	if issue.Type != "molecule" {
		t.Errorf("Type = %q, want %q", issue.Type, "molecule")
	}
	if issue.Status != "open" {
		t.Errorf("Status = %q, want %q", issue.Status, "open")
	}
	if issue.Priority != 2 {
		t.Errorf("Priority = %d, want %d", issue.Priority, 2)
	}
}
