package epic

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverContributing_RootLocation(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create CONTRIBUTING.md at root
	content := "# Contributing\n\nPlease write tests."
	err := os.WriteFile(filepath.Join(tmpDir, "CONTRIBUTING.md"), []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Test discovery
	path, foundContent, err := DiscoverContributing(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverContributing failed: %v", err)
	}

	if path != "CONTRIBUTING.md" {
		t.Errorf("expected path 'CONTRIBUTING.md', got '%s'", path)
	}

	if foundContent != content {
		t.Errorf("content mismatch: expected '%s', got '%s'", content, foundContent)
	}
}

func TestDiscoverContributing_DocsLocation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create docs directory
	docsDir := filepath.Join(tmpDir, "docs")
	if err := os.Mkdir(docsDir, 0755); err != nil {
		t.Fatalf("failed to create docs dir: %v", err)
	}

	// Create CONTRIBUTING.md in docs/
	content := "# Contributing from docs\n\nDocumented guidelines."
	err := os.WriteFile(filepath.Join(docsDir, "CONTRIBUTING.md"), []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	path, foundContent, err := DiscoverContributing(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverContributing failed: %v", err)
	}

	if path != "docs/CONTRIBUTING.md" {
		t.Errorf("expected path 'docs/CONTRIBUTING.md', got '%s'", path)
	}

	if foundContent != content {
		t.Errorf("content mismatch")
	}
}

func TestDiscoverContributing_GithubLocation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .github directory
	githubDir := filepath.Join(tmpDir, ".github")
	if err := os.Mkdir(githubDir, 0755); err != nil {
		t.Fatalf("failed to create .github dir: %v", err)
	}

	// Create CONTRIBUTING.md in .github/
	content := "# GitHub Contributing\n\nGitHub-style guidelines."
	err := os.WriteFile(filepath.Join(githubDir, "CONTRIBUTING.md"), []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	path, foundContent, err := DiscoverContributing(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverContributing failed: %v", err)
	}

	if path != ".github/CONTRIBUTING.md" {
		t.Errorf("expected path '.github/CONTRIBUTING.md', got '%s'", path)
	}

	if foundContent != content {
		t.Errorf("content mismatch")
	}
}

func TestDiscoverContributing_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	// No CONTRIBUTING.md file
	path, content, err := DiscoverContributing(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverContributing should not error when file not found: %v", err)
	}

	if path != "" {
		t.Errorf("expected empty path when not found, got '%s'", path)
	}

	if content != "" {
		t.Errorf("expected empty content when not found")
	}
}

func TestDiscoverContributing_PriorityOrder(t *testing.T) {
	tmpDir := t.TempDir()

	// Create all three locations - root should have priority
	rootContent := "Root CONTRIBUTING"
	docsContent := "Docs CONTRIBUTING"
	githubContent := "GitHub CONTRIBUTING"

	// Create root version
	if err := os.WriteFile(filepath.Join(tmpDir, "CONTRIBUTING.md"), []byte(rootContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create docs version
	docsDir := filepath.Join(tmpDir, "docs")
	if err := os.Mkdir(docsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "CONTRIBUTING.md"), []byte(docsContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create .github version
	githubDir := filepath.Join(tmpDir, ".github")
	if err := os.Mkdir(githubDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(githubDir, "CONTRIBUTING.md"), []byte(githubContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Root should be found first
	path, content, err := DiscoverContributing(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	if path != "CONTRIBUTING.md" {
		t.Errorf("expected root path to have priority, got '%s'", path)
	}

	if content != rootContent {
		t.Errorf("expected root content")
	}
}

func TestContributingExists(t *testing.T) {
	tmpDir := t.TempDir()

	// Should return false when not exists
	if ContributingExists(tmpDir) {
		t.Error("expected false when file doesn't exist")
	}

	// Create file
	if err := os.WriteFile(filepath.Join(tmpDir, "CONTRIBUTING.md"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Should return true when exists
	if !ContributingExists(tmpDir) {
		t.Error("expected true when file exists")
	}
}

func TestGetContributingPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Should return empty when not exists
	if path := GetContributingPath(tmpDir); path != "" {
		t.Errorf("expected empty path, got '%s'", path)
	}

	// Create file
	if err := os.WriteFile(filepath.Join(tmpDir, "CONTRIBUTING.md"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Should return full path when exists
	expected := filepath.Join(tmpDir, "CONTRIBUTING.md")
	if path := GetContributingPath(tmpDir); path != expected {
		t.Errorf("expected '%s', got '%s'", expected, path)
	}
}
