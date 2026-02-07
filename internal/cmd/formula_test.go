package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/formula"
)

// TestFindFormulaFile_EmbeddedFallback tests that embedded formulas are found when
// no local copy exists.
func TestFindFormulaFile_EmbeddedFallback(t *testing.T) {
	// Get a known embedded formula name
	names, err := formula.GetEmbeddedFormulaNames()
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaNames() error: %v", err)
	}
	if len(names) == 0 {
		t.Skip("no embedded formulas available")
	}

	// Use a temp directory as working directory to ensure no local formulas exist
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Find the embedded formula
	formulaName := names[0]
	path, err := findFormulaFile(formulaName)
	if err != nil {
		t.Fatalf("findFormulaFile(%q) error: %v", formulaName, err)
	}

	// Verify it returns embedded marker
	if !strings.HasPrefix(path, "embedded:") {
		t.Errorf("findFormulaFile(%q) = %q, want embedded:<name>", formulaName, path)
	}

	expectedPath := "embedded:" + formulaName
	if path != expectedPath {
		t.Errorf("findFormulaFile(%q) = %q, want %q", formulaName, path, expectedPath)
	}
}

// TestFindFormulaWithSource_EmbeddedFallback tests findFormulaWithSource returns
// correct source for embedded formulas.
func TestFindFormulaWithSource_EmbeddedFallback(t *testing.T) {
	names, err := formula.GetEmbeddedFormulaNames()
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaNames() error: %v", err)
	}
	if len(names) == 0 {
		t.Skip("no embedded formulas available")
	}

	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	formulaName := names[0]
	loc, err := findFormulaWithSource(formulaName)
	if err != nil {
		t.Fatalf("findFormulaWithSource(%q) error: %v", formulaName, err)
	}

	if !loc.IsEmbedded() {
		t.Errorf("findFormulaWithSource(%q).IsEmbedded() = false, want true", formulaName)
	}

	if loc.Source != FormulaSourceEmbedded {
		t.Errorf("loc.Source = %v, want FormulaSourceEmbedded", loc.Source)
	}

	if loc.Path != formulaName {
		t.Errorf("loc.Path = %q, want %q", loc.Path, formulaName)
	}
}

// TestFindFormulaFile_LocalOverridesEmbedded tests that local formulas take
// precedence over embedded ones.
func TestFindFormulaFile_LocalOverridesEmbedded(t *testing.T) {
	names, err := formula.GetEmbeddedFormulaNames()
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaNames() error: %v", err)
	}
	if len(names) == 0 {
		t.Skip("no embedded formulas available")
	}

	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create a local formula file with the same name as an embedded one
	formulaName := names[0]
	formulasDir := filepath.Join(tmpDir, ".beads", "formulas")
	if err := os.MkdirAll(formulasDir, 0755); err != nil {
		t.Fatal(err)
	}

	localContent := []byte("# Local override\nformula = \"" + formulaName + "\"\n")
	localPath := filepath.Join(formulasDir, formulaName+".formula.toml")
	if err := os.WriteFile(localPath, localContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Find the formula - should get local file, not embedded
	path, err := findFormulaFile(formulaName)
	if err != nil {
		t.Fatalf("findFormulaFile(%q) error: %v", formulaName, err)
	}

	// Should NOT be embedded
	if strings.HasPrefix(path, "embedded:") {
		t.Errorf("findFormulaFile(%q) = %q, should return local path not embedded", formulaName, path)
	}

	// Should be the local path
	if path != localPath {
		t.Errorf("findFormulaFile(%q) = %q, want %q", formulaName, path, localPath)
	}
}

// TestFindFormulaWithSource_LocalFile tests that local files are correctly identified.
func TestFindFormulaWithSource_LocalFile(t *testing.T) {
	names, err := formula.GetEmbeddedFormulaNames()
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaNames() error: %v", err)
	}
	if len(names) == 0 {
		t.Skip("no embedded formulas available")
	}

	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create local formula
	formulaName := names[0]
	formulasDir := filepath.Join(tmpDir, ".beads", "formulas")
	if err := os.MkdirAll(formulasDir, 0755); err != nil {
		t.Fatal(err)
	}

	localPath := filepath.Join(formulasDir, formulaName+".formula.toml")
	if err := os.WriteFile(localPath, []byte("# test"), 0644); err != nil {
		t.Fatal(err)
	}

	loc, err := findFormulaWithSource(formulaName)
	if err != nil {
		t.Fatalf("findFormulaWithSource(%q) error: %v", formulaName, err)
	}

	if loc.IsEmbedded() {
		t.Errorf("findFormulaWithSource(%q).IsEmbedded() = true, want false (local file exists)", formulaName)
	}

	if loc.Source != FormulaSourceFile {
		t.Errorf("loc.Source = %v, want FormulaSourceFile", loc.Source)
	}
}

// TestFindFormulaFile_NotFound tests that missing formulas return an error.
func TestFindFormulaFile_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	_, err = findFormulaFile("non-existent-formula-xyz")
	if err == nil {
		t.Error("findFormulaFile(non-existent) should error")
	}
}

// TestParseFormulaFile_Embedded tests parsing embedded formulas via the
// "embedded:<name>" marker.
func TestParseFormulaFile_Embedded(t *testing.T) {
	names, err := formula.GetEmbeddedFormulaNames()
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaNames() error: %v", err)
	}
	if len(names) == 0 {
		t.Skip("no embedded formulas available")
	}

	// Parse an embedded formula
	formulaName := names[0]
	f, err := parseFormulaFile("embedded:" + formulaName)
	if err != nil {
		t.Fatalf("parseFormulaFile(embedded:%s) error: %v", formulaName, err)
	}

	// Basic sanity checks
	if f == nil {
		t.Fatal("parseFormulaFile returned nil")
	}

	// Formula should have a name (from the TOML content)
	if f.Name == "" {
		t.Log("Warning: embedded formula has empty name field")
	}
}

// TestParseFormulaFile_EmbeddedNotFound tests that parsing a non-existent
// embedded formula returns an error.
func TestParseFormulaFile_EmbeddedNotFound(t *testing.T) {
	_, err := parseFormulaFile("embedded:non-existent-formula-xyz")
	if err == nil {
		t.Error("parseFormulaFile(embedded:non-existent) should error")
	}
}

// TestFormulaLocation_IsEmbedded tests the IsEmbedded helper method.
func TestFormulaLocation_IsEmbedded(t *testing.T) {
	tests := []struct {
		loc      FormulaLocation
		expected bool
	}{
		{FormulaLocation{Source: FormulaSourceEmbedded}, true},
		{FormulaLocation{Source: FormulaSourceFile}, false},
	}

	for _, tt := range tests {
		if got := tt.loc.IsEmbedded(); got != tt.expected {
			t.Errorf("FormulaLocation{Source: %v}.IsEmbedded() = %v, want %v",
				tt.loc.Source, got, tt.expected)
		}
	}
}

// TestFreshInstall_CanRunFormulas simulates a fresh install scenario where
// no .beads/formulas/ directory exists, and verifies formulas can still be
// found via embedded fallback.
func TestFreshInstall_CanRunFormulas(t *testing.T) {
	// Get embedded formula names
	names, err := formula.GetEmbeddedFormulaNames()
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaNames() error: %v", err)
	}
	if len(names) == 0 {
		t.Skip("no embedded formulas available")
	}

	// Create a completely empty temp directory (simulates fresh install)
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Verify no .beads directory exists
	if _, err := os.Stat(filepath.Join(tmpDir, ".beads")); !os.IsNotExist(err) {
		t.Fatal(".beads directory should not exist in fresh temp directory")
	}

	// Test each embedded formula can be found
	for _, name := range names {
		path, err := findFormulaFile(name)
		if err != nil {
			t.Errorf("findFormulaFile(%q) error: %v", name, err)
			continue
		}

		// Should return embedded marker
		expectedPrefix := "embedded:"
		if !strings.HasPrefix(path, expectedPrefix) {
			t.Errorf("findFormulaFile(%q) = %q, want prefix %q", name, path, expectedPrefix)
		}

		// Should be able to parse it
		f, err := parseFormulaFile(path)
		if err != nil {
			t.Errorf("parseFormulaFile(%q) error: %v", path, err)
			continue
		}

		if f == nil {
			t.Errorf("parseFormulaFile(%q) returned nil", path)
		}
	}
}

// TestResolutionOrder verifies the resolution order:
// rig .beads/formulas/ → town $GT_ROOT/.beads/formulas/ → embedded
func TestResolutionOrder(t *testing.T) {
	names, err := formula.GetEmbeddedFormulaNames()
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaNames() error: %v", err)
	}
	if len(names) == 0 {
		t.Skip("no embedded formulas available")
	}

	formulaName := names[0]
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(originalDir)

	// Test 1: No local files → should use embedded
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	loc, err := findFormulaWithSource(formulaName)
	if err != nil {
		t.Fatalf("findFormulaWithSource(%q) error: %v", formulaName, err)
	}
	if !loc.IsEmbedded() {
		t.Errorf("Expected embedded source when no local files exist")
	}

	// Test 2: Create rig-level formula → should use rig file
	rigFormulasDir := filepath.Join(tmpDir, ".beads", "formulas")
	if err := os.MkdirAll(rigFormulasDir, 0755); err != nil {
		t.Fatal(err)
	}

	rigFormulaPath := filepath.Join(rigFormulasDir, formulaName+".formula.toml")
	rigContent := []byte("# Rig-level override\nformula = \"" + formulaName + "\"\n")
	if err := os.WriteFile(rigFormulaPath, rigContent, 0644); err != nil {
		t.Fatal(err)
	}

	loc, err = findFormulaWithSource(formulaName)
	if err != nil {
		t.Fatalf("findFormulaWithSource(%q) error: %v", formulaName, err)
	}
	if loc.IsEmbedded() {
		t.Errorf("Expected file source when rig-level formula exists, got embedded")
	}
	if loc.Path != rigFormulaPath {
		t.Errorf("Expected path %q, got %q", rigFormulaPath, loc.Path)
	}
}

// TestFormulaModify_CopiesEmbeddedFormula tests that gt formula modify
// copies an embedded formula to the specified destination.
func TestFormulaModify_CopiesEmbeddedFormula(t *testing.T) {
	names, err := formula.GetEmbeddedFormulaNames()
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaNames() error: %v", err)
	}
	if len(names) == 0 {
		t.Skip("no embedded formulas available")
	}

	formulaName := names[0]

	// Create a temp directory to act as town root
	tmpDir := t.TempDir()
	destDir := filepath.Join(tmpDir, ".beads", "formulas")

	// Use the formula package's CopyFormulaTo directly (simulates what runFormulaModify does)
	copiedPath, err := formula.CopyFormulaTo(formulaName, destDir)
	if err != nil {
		t.Fatalf("CopyFormulaTo(%q, %q) error: %v", formulaName, destDir, err)
	}

	// Verify the file was created
	expectedPath := filepath.Join(destDir, formulaName+".formula.toml")
	if copiedPath != expectedPath {
		t.Errorf("CopyFormulaTo returned %q, want %q", copiedPath, expectedPath)
	}

	// Verify the file exists
	if _, err := os.Stat(copiedPath); os.IsNotExist(err) {
		t.Errorf("Copied formula file does not exist at %q", copiedPath)
	}

	// Verify the copied content has the hash header and contains the embedded content
	embeddedContent, err := formula.GetEmbeddedFormula(formulaName)
	if err != nil {
		t.Fatalf("GetEmbeddedFormula(%q) error: %v", formulaName, err)
	}

	localContent, err := os.ReadFile(copiedPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", copiedPath, err)
	}

	// Copied content should start with the header comment
	if !strings.HasPrefix(string(localContent), "# Formula override created by gt formula modify") {
		t.Errorf("Copied formula content should start with hash header comment")
	}

	// Copied content should contain the embedded content after the header
	if !bytes.Contains(localContent, embeddedContent) {
		t.Errorf("Copied formula content should contain the embedded formula content")
	}
}

// TestFormulaModify_ErrorsOnExistingOverride tests that gt formula modify
// returns an error when an override already exists.
func TestFormulaModify_ErrorsOnExistingOverride(t *testing.T) {
	names, err := formula.GetEmbeddedFormulaNames()
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaNames() error: %v", err)
	}
	if len(names) == 0 {
		t.Skip("no embedded formulas available")
	}

	formulaName := names[0]

	// Create a temp directory and pre-create the formula file
	tmpDir := t.TempDir()
	destDir := filepath.Join(tmpDir, ".beads", "formulas")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatal(err)
	}

	existingPath := filepath.Join(destDir, formulaName+".formula.toml")
	if err := os.WriteFile(existingPath, []byte("# existing"), 0644); err != nil {
		t.Fatal(err)
	}

	// Try to copy - should fail
	_, err = formula.CopyFormulaTo(formulaName, destDir)
	if err == nil {
		t.Error("CopyFormulaTo should error when file already exists")
	}

	// Verify error message mentions "already exists"
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("Error message should mention 'already exists', got: %v", err)
	}
}

// TestFormulaModify_NonExistentFormula tests that gt formula modify
// returns an error for formulas that don't exist in embedded.
func TestFormulaModify_NonExistentFormula(t *testing.T) {
	tmpDir := t.TempDir()
	destDir := filepath.Join(tmpDir, ".beads", "formulas")

	_, err := formula.CopyFormulaTo("non-existent-formula-xyz", destDir)
	if err == nil {
		t.Error("CopyFormulaTo should error for non-existent formula")
	}
}

// TestFormulaModify_ToRigLevel tests copying a formula to a rig's .beads/formulas/
func TestFormulaModify_ToRigLevel(t *testing.T) {
	names, err := formula.GetEmbeddedFormulaNames()
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaNames() error: %v", err)
	}
	if len(names) == 0 {
		t.Skip("no embedded formulas available")
	}

	formulaName := names[0]

	// Create a temp directory structure simulating town with rig
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "my-rig", ".beads", "formulas")

	// Copy to rig level
	copiedPath, err := formula.CopyFormulaTo(formulaName, rigDir)
	if err != nil {
		t.Fatalf("CopyFormulaTo to rig level error: %v", err)
	}

	expectedPath := filepath.Join(rigDir, formulaName+".formula.toml")
	if copiedPath != expectedPath {
		t.Errorf("CopyFormulaTo returned %q, want %q", copiedPath, expectedPath)
	}

	// Verify file exists
	if _, err := os.Stat(copiedPath); os.IsNotExist(err) {
		t.Errorf("Copied formula file does not exist at %q", copiedPath)
	}
}
