package formula

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestGetEmbeddedFormulas verifies embedded formulas can be read and hashed.
func TestGetEmbeddedFormulas(t *testing.T) {
	embedded, err := getEmbeddedFormulas()
	if err != nil {
		t.Fatalf("getEmbeddedFormulas() error: %v", err)
	}
	if len(embedded) == 0 {
		t.Error("should have embedded formulas")
	}

	// Verify at least one known formula exists
	if _, ok := embedded["mol-deacon-patrol.formula.toml"]; !ok {
		t.Error("should contain mol-deacon-patrol.formula.toml")
	}

	// Verify hashes are valid hex strings
	for name, hash := range embedded {
		if len(hash) != 64 {
			t.Errorf("%s hash has wrong length: %d", name, len(hash))
		}
	}
}

// TestProvisionFormulas_FreshInstall tests provisioning to an empty directory.
func TestProvisionFormulas_FreshInstall(t *testing.T) {
	tmpDir := t.TempDir()

	count, err := ProvisionFormulas(tmpDir)
	if err != nil {
		t.Fatalf("ProvisionFormulas() error: %v", err)
	}
	if count == 0 {
		t.Error("should have provisioned at least one formula")
	}

	// Verify formulas directory was created
	formulasDir := filepath.Join(tmpDir, ".beads", "formulas")
	if _, err := os.Stat(formulasDir); os.IsNotExist(err) {
		t.Error("formulas directory should exist")
	}

	// Verify .installed.json was created
	installedPath := filepath.Join(formulasDir, ".installed.json")
	if _, err := os.Stat(installedPath); os.IsNotExist(err) {
		t.Error(".installed.json should exist")
	}

	// Verify installed record contains the right checksums
	installed, err := loadInstalledRecord(formulasDir)
	if err != nil {
		t.Fatalf("loadInstalledRecord() error: %v", err)
	}
	if len(installed.Formulas) != count {
		t.Errorf("installed record has %d entries, want %d", len(installed.Formulas), count)
	}
}

// TestProvisionFormulas_SkipsExisting tests that existing files are not overwritten.
func TestProvisionFormulas_SkipsExisting(t *testing.T) {
	tmpDir := t.TempDir()

	// Create formulas directory with a custom formula
	formulasDir := filepath.Join(tmpDir, ".beads", "formulas")
	if err := os.MkdirAll(formulasDir, 0755); err != nil {
		t.Fatal(err)
	}

	customContent := []byte("# Custom user formula\nformula = \"mol-deacon-patrol\"\n")
	customPath := filepath.Join(formulasDir, "mol-deacon-patrol.formula.toml")
	if err := os.WriteFile(customPath, customContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Provision formulas
	_, err := ProvisionFormulas(tmpDir)
	if err != nil {
		t.Fatalf("ProvisionFormulas() error: %v", err)
	}

	// Verify custom content was NOT overwritten
	content, err := os.ReadFile(customPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != string(customContent) {
		t.Error("existing formula should not have been overwritten")
	}
}

// TestCheckFormulaHealth_AllOK tests when all formulas are up to date.
func TestCheckFormulaHealth_AllOK(t *testing.T) {
	tmpDir := t.TempDir()

	// Provision fresh
	_, err := ProvisionFormulas(tmpDir)
	if err != nil {
		t.Fatalf("ProvisionFormulas() error: %v", err)
	}

	// Check health
	report, err := CheckFormulaHealth(tmpDir)
	if err != nil {
		t.Fatalf("CheckFormulaHealth() error: %v", err)
	}

	if report.Outdated != 0 {
		t.Errorf("Outdated = %d, want 0", report.Outdated)
	}
	if report.Missing != 0 {
		t.Errorf("Missing = %d, want 0", report.Missing)
	}
	if report.Modified != 0 {
		t.Errorf("Modified = %d, want 0", report.Modified)
	}
	if report.OK == 0 {
		t.Error("OK should be > 0")
	}
}

// TestCheckFormulaHealth_UserModified tests detection of user-modified formulas.
func TestCheckFormulaHealth_UserModified(t *testing.T) {
	tmpDir := t.TempDir()

	// Provision fresh
	_, err := ProvisionFormulas(tmpDir)
	if err != nil {
		t.Fatalf("ProvisionFormulas() error: %v", err)
	}

	// Modify a formula
	formulasDir := filepath.Join(tmpDir, ".beads", "formulas")
	formulaPath := filepath.Join(formulasDir, "mol-deacon-patrol.formula.toml")
	modifiedContent := []byte("# User modified this\nformula = \"mol-deacon-patrol\"\nversion = 999\n")
	if err := os.WriteFile(formulaPath, modifiedContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Check health
	report, err := CheckFormulaHealth(tmpDir)
	if err != nil {
		t.Fatalf("CheckFormulaHealth() error: %v", err)
	}

	if report.Modified != 1 {
		t.Errorf("Modified = %d, want 1", report.Modified)
	}

	// Verify the specific formula is marked as modified
	found := false
	for _, f := range report.Formulas {
		if f.Name == "mol-deacon-patrol.formula.toml" {
			if f.Status != "modified" {
				t.Errorf("mol-deacon-patrol status = %q, want %q", f.Status, "modified")
			}
			found = true
			break
		}
	}
	if !found {
		t.Error("mol-deacon-patrol.formula.toml not found in report")
	}
}

// TestCheckFormulaHealth_Missing tests detection of deleted formulas.
func TestCheckFormulaHealth_Missing(t *testing.T) {
	tmpDir := t.TempDir()

	// Provision fresh
	_, err := ProvisionFormulas(tmpDir)
	if err != nil {
		t.Fatalf("ProvisionFormulas() error: %v", err)
	}

	// Delete a formula
	formulasDir := filepath.Join(tmpDir, ".beads", "formulas")
	formulaPath := filepath.Join(formulasDir, "mol-deacon-patrol.formula.toml")
	if err := os.Remove(formulaPath); err != nil {
		t.Fatal(err)
	}

	// Check health
	report, err := CheckFormulaHealth(tmpDir)
	if err != nil {
		t.Fatalf("CheckFormulaHealth() error: %v", err)
	}

	if report.Missing != 1 {
		t.Errorf("Missing = %d, want 1", report.Missing)
	}
}

// TestCheckFormulaHealth_Outdated simulates an outdated formula.
func TestCheckFormulaHealth_Outdated(t *testing.T) {
	tmpDir := t.TempDir()

	// Provision fresh
	_, err := ProvisionFormulas(tmpDir)
	if err != nil {
		t.Fatalf("ProvisionFormulas() error: %v", err)
	}

	// Simulate "old" installed record by changing the installed hash for a formula
	// This mimics what happens when a new binary has updated formula content
	formulasDir := filepath.Join(tmpDir, ".beads", "formulas")
	installed, err := loadInstalledRecord(formulasDir)
	if err != nil {
		t.Fatal(err)
	}

	embedded, err := getEmbeddedFormulas()
	if err != nil {
		t.Fatal(err)
	}

	// Pick a formula that exists
	var targetFormula string
	for name := range installed.Formulas {
		targetFormula = name
		break
	}
	if targetFormula == "" {
		t.Skip("no formulas installed")
	}

	// Write a file that simulates "old version" - content differs from embedded
	formulaPath := filepath.Join(formulasDir, targetFormula)
	oldContent := []byte("# Old version of formula\n")
	if err := os.WriteFile(formulaPath, oldContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Update installed record to match the old content's hash
	hash := sha256.Sum256(oldContent)
	installed.Formulas[targetFormula] = hex.EncodeToString(hash[:])

	if err := saveInstalledRecord(formulasDir, installed); err != nil {
		t.Fatal(err)
	}

	// Now: file matches what we "installed" but differs from embedded = outdated
	report, err := CheckFormulaHealth(tmpDir)
	if err != nil {
		t.Fatalf("CheckFormulaHealth() error: %v", err)
	}

	if report.Outdated != 1 {
		t.Errorf("Outdated = %d, want 1", report.Outdated)
	}

	// Verify the embedded hash is different from installed
	embeddedHash := embedded[targetFormula]
	if embeddedHash == installed.Formulas[targetFormula] {
		t.Error("embedded hash should differ from installed hash for this test")
	}
}

// TestUpdateFormulas_UpdatesOutdated tests that outdated formulas are updated.
func TestUpdateFormulas_UpdatesOutdated(t *testing.T) {
	tmpDir := t.TempDir()

	// Provision fresh
	_, err := ProvisionFormulas(tmpDir)
	if err != nil {
		t.Fatalf("ProvisionFormulas() error: %v", err)
	}

	// Simulate outdated formula
	formulasDir := filepath.Join(tmpDir, ".beads", "formulas")
	installed, err := loadInstalledRecord(formulasDir)
	if err != nil {
		t.Fatal(err)
	}

	var targetFormula string
	for name := range installed.Formulas {
		targetFormula = name
		break
	}
	if targetFormula == "" {
		t.Skip("no formulas installed")
	}

	// Write old content
	formulaPath := filepath.Join(formulasDir, targetFormula)
	oldContent := []byte("# Old version\n")
	if err := os.WriteFile(formulaPath, oldContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Update installed record with old content's hash
	hash := sha256.Sum256(oldContent)
	installed.Formulas[targetFormula] = hex.EncodeToString(hash[:])
	if err := saveInstalledRecord(formulasDir, installed); err != nil {
		t.Fatal(err)
	}

	// Run update
	updated, skipped, reinstalled, err := UpdateFormulas(tmpDir)
	if err != nil {
		t.Fatalf("UpdateFormulas() error: %v", err)
	}

	if updated != 1 {
		t.Errorf("updated = %d, want 1", updated)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}
	if reinstalled != 0 {
		t.Errorf("reinstalled = %d, want 0", reinstalled)
	}

	// Verify file was updated
	report, err := CheckFormulaHealth(tmpDir)
	if err != nil {
		t.Fatalf("CheckFormulaHealth() error: %v", err)
	}
	if report.Outdated != 0 {
		t.Errorf("after update, Outdated = %d, want 0", report.Outdated)
	}
}

// TestUpdateFormulas_SkipsModified tests that user-modified formulas are skipped.
func TestUpdateFormulas_SkipsModified(t *testing.T) {
	tmpDir := t.TempDir()

	// Provision fresh
	_, err := ProvisionFormulas(tmpDir)
	if err != nil {
		t.Fatalf("ProvisionFormulas() error: %v", err)
	}

	// Modify a formula (user customization)
	formulasDir := filepath.Join(tmpDir, ".beads", "formulas")
	installed, err := loadInstalledRecord(formulasDir)
	if err != nil {
		t.Fatal(err)
	}

	var targetFormula string
	for name := range installed.Formulas {
		targetFormula = name
		break
	}
	if targetFormula == "" {
		t.Skip("no formulas installed")
	}

	// Write different content that doesn't match installed hash
	formulaPath := filepath.Join(formulasDir, targetFormula)
	modifiedContent := []byte("# User customized this formula\nformula = \"custom\"\n")
	if err := os.WriteFile(formulaPath, modifiedContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Run update - should skip the modified formula
	_, skipped, _, err := UpdateFormulas(tmpDir)
	if err != nil {
		t.Fatalf("UpdateFormulas() error: %v", err)
	}

	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}

	// Verify file was NOT changed
	content, err := os.ReadFile(formulaPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != string(modifiedContent) {
		t.Error("modified formula should not have been changed")
	}
}

// TestUpdateFormulas_ReinstallsMissing tests that deleted formulas are reinstalled.
func TestUpdateFormulas_ReinstallsMissing(t *testing.T) {
	tmpDir := t.TempDir()

	// Provision fresh
	_, err := ProvisionFormulas(tmpDir)
	if err != nil {
		t.Fatalf("ProvisionFormulas() error: %v", err)
	}

	// Delete a formula
	formulasDir := filepath.Join(tmpDir, ".beads", "formulas")
	installed, err := loadInstalledRecord(formulasDir)
	if err != nil {
		t.Fatal(err)
	}

	var targetFormula string
	for name := range installed.Formulas {
		targetFormula = name
		break
	}
	if targetFormula == "" {
		t.Skip("no formulas installed")
	}

	formulaPath := filepath.Join(formulasDir, targetFormula)
	if err := os.Remove(formulaPath); err != nil {
		t.Fatal(err)
	}

	// Run update
	_, _, reinstalled, err := UpdateFormulas(tmpDir)
	if err != nil {
		t.Fatalf("UpdateFormulas() error: %v", err)
	}

	if reinstalled != 1 {
		t.Errorf("reinstalled = %d, want 1", reinstalled)
	}

	// Verify file was restored
	if _, err := os.Stat(formulaPath); os.IsNotExist(err) {
		t.Error("missing formula should have been reinstalled")
	}
}

// TestUpdateFormulas_InstallsNew tests that new formulas are installed.
func TestUpdateFormulas_InstallsNew(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directory structure but with empty installed record
	formulasDir := filepath.Join(tmpDir, ".beads", "formulas")
	if err := os.MkdirAll(formulasDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write empty installed record
	emptyInstalled := &InstalledRecord{Formulas: make(map[string]string)}
	if err := saveInstalledRecord(formulasDir, emptyInstalled); err != nil {
		t.Fatal(err)
	}

	// Run update - should install all formulas as "new"
	updated, skipped, reinstalled, err := UpdateFormulas(tmpDir)
	if err != nil {
		t.Fatalf("UpdateFormulas() error: %v", err)
	}

	// All formulas should be installed
	embedded, err := getEmbeddedFormulas()
	if err != nil {
		t.Fatal(err)
	}

	total := updated + reinstalled
	if total != len(embedded) {
		t.Errorf("total installed = %d, want %d", total, len(embedded))
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}
}

// TestInstalledRecordPersistence tests that the installed record survives across operations.
func TestInstalledRecordPersistence(t *testing.T) {
	tmpDir := t.TempDir()

	// Provision
	count, err := ProvisionFormulas(tmpDir)
	if err != nil {
		t.Fatalf("ProvisionFormulas() error: %v", err)
	}

	// Load and verify
	formulasDir := filepath.Join(tmpDir, ".beads", "formulas")
	installed, err := loadInstalledRecord(formulasDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(installed.Formulas) != count {
		t.Errorf("installed has %d formulas, want %d", len(installed.Formulas), count)
	}

	// Verify file is valid JSON
	installedPath := filepath.Join(formulasDir, ".installed.json")
	data, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatal(err)
	}

	var decoded InstalledRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Errorf("installed.json is not valid JSON: %v", err)
	}
}

// TestCheckFormulaHealth_NewFormula tests detection of new formulas that were never installed.
func TestCheckFormulaHealth_NewFormula(t *testing.T) {
	tmpDir := t.TempDir()

	// Create formulas directory with empty installed record
	formulasDir := filepath.Join(tmpDir, ".beads", "formulas")
	if err := os.MkdirAll(formulasDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write empty installed record - simulates pre-existing install without this formula
	emptyInstalled := &InstalledRecord{Formulas: make(map[string]string)}
	if err := saveInstalledRecord(formulasDir, emptyInstalled); err != nil {
		t.Fatal(err)
	}

	// Check health - all embedded formulas should be "new"
	report, err := CheckFormulaHealth(tmpDir)
	if err != nil {
		t.Fatalf("CheckFormulaHealth() error: %v", err)
	}

	embedded, _ := getEmbeddedFormulas()
	if report.New != len(embedded) {
		t.Errorf("New = %d, want %d", report.New, len(embedded))
	}
	if report.OK != 0 {
		t.Errorf("OK = %d, want 0", report.OK)
	}
}
