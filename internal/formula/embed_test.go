package formula

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGetEmbeddedFormula tests reading embedded formulas by name.
func TestGetEmbeddedFormula(t *testing.T) {
	// Test reading a known formula without suffix
	content, err := GetEmbeddedFormula("mol-deacon-patrol")
	if err != nil {
		t.Fatalf("GetEmbeddedFormula(mol-deacon-patrol) error: %v", err)
	}
	if len(content) == 0 {
		t.Error("content should not be empty")
	}

	// Test reading with suffix
	contentWithSuffix, err := GetEmbeddedFormula("mol-deacon-patrol.formula.toml")
	if err != nil {
		t.Fatalf("GetEmbeddedFormula(mol-deacon-patrol.formula.toml) error: %v", err)
	}
	if string(content) != string(contentWithSuffix) {
		t.Error("content should be the same with or without suffix")
	}

	// Test reading non-existent formula
	_, err = GetEmbeddedFormula("non-existent-formula")
	if err == nil {
		t.Error("should error for non-existent formula")
	}
}

// TestGetEmbeddedFormulaNames tests listing all embedded formula names.
func TestGetEmbeddedFormulaNames(t *testing.T) {
	names, err := GetEmbeddedFormulaNames()
	if err != nil {
		t.Fatalf("GetEmbeddedFormulaNames() error: %v", err)
	}
	if len(names) == 0 {
		t.Error("should have embedded formula names")
	}

	// Verify names don't have suffix
	for _, name := range names {
		if len(name) > 13 && name[len(name)-13:] == ".formula.toml" {
			t.Errorf("name %q should not have .formula.toml suffix", name)
		}
	}

	// Verify a known formula is in the list
	found := false
	for _, name := range names {
		if name == "mol-deacon-patrol" {
			found = true
			break
		}
	}
	if !found {
		t.Error("should contain mol-deacon-patrol")
	}
}

// TestEmbeddedFormulaExists tests the existence check helper.
func TestEmbeddedFormulaExists(t *testing.T) {
	// Test existing formula without suffix
	if !EmbeddedFormulaExists("mol-deacon-patrol") {
		t.Error("mol-deacon-patrol should exist")
	}

	// Test existing formula with suffix
	if !EmbeddedFormulaExists("mol-deacon-patrol.formula.toml") {
		t.Error("mol-deacon-patrol.formula.toml should exist")
	}

	// Test non-existent formula
	if EmbeddedFormulaExists("non-existent-formula") {
		t.Error("non-existent-formula should not exist")
	}

	// Test non-existent formula with suffix
	if EmbeddedFormulaExists("non-existent-formula.formula.toml") {
		t.Error("non-existent-formula.formula.toml should not exist")
	}
}

// TestFormulaNameConversions tests the name/filename conversion helpers.
func TestFormulaNameConversions(t *testing.T) {
	tests := []struct {
		input    string
		toFile   string
		fromFile string
	}{
		{"test", "test.formula.toml", "test"},
		{"test.formula.toml", "test.formula.toml", "test"},
		{"my-formula", "my-formula.formula.toml", "my-formula"},
		{"my-formula.formula.toml", "my-formula.formula.toml", "my-formula"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := formulaNameToFilename(tt.input); got != tt.toFile {
				t.Errorf("formulaNameToFilename(%q) = %q, want %q", tt.input, got, tt.toFile)
			}
			if got := filenameToFormulaName(tt.toFile); got != tt.fromFile {
				t.Errorf("filenameToFormulaName(%q) = %q, want %q", tt.toFile, got, tt.fromFile)
			}
		})
	}
}

// TestCopyFormulaTo tests copying an embedded formula to a destination.
func TestCopyFormulaTo(t *testing.T) {
	tmpDir := t.TempDir()
	destDir := filepath.Join(tmpDir, ".beads", "formulas")

	// Test copying a formula
	destPath, err := CopyFormulaTo("mol-deacon-patrol", destDir)
	if err != nil {
		t.Fatalf("CopyFormulaTo() error: %v", err)
	}

	expectedPath := filepath.Join(destDir, "mol-deacon-patrol.formula.toml")
	if destPath != expectedPath {
		t.Errorf("destPath = %q, want %q", destPath, expectedPath)
	}

	// Verify file was created
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		t.Error("formula file should exist")
	}

	// Verify copied content has the hash header and the embedded content after it
	embeddedContent, _ := GetEmbeddedFormula("mol-deacon-patrol")
	copiedContent, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatal(err)
	}

	// Copied content should start with the header comment
	if !strings.HasPrefix(string(copiedContent), "# Formula override created by gt formula modify") {
		t.Error("copied content should start with hash header comment")
	}

	// Copied content should contain the embedded content after the header
	if !bytes.Contains(copiedContent, embeddedContent) {
		t.Error("copied content should contain the embedded formula content")
	}

	// Test that copying again fails (already exists)
	_, err = CopyFormulaTo("mol-deacon-patrol", destDir)
	if err == nil {
		t.Error("should error when formula already exists")
	}
}

// TestCopyFormulaTo_NonExistent tests copying a non-existent formula.
func TestCopyFormulaTo_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	destDir := filepath.Join(tmpDir, ".beads", "formulas")

	_, err := CopyFormulaTo("non-existent-formula", destDir)
	if err == nil {
		t.Error("should error for non-existent formula")
	}
}
