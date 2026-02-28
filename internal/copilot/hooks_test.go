package copilot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// hooksConfigTest mirrors the hooks JSON structure for test assertions.
type hooksConfigTest struct {
	Version        int                `json:"version"`
	GastownManaged bool               `json:"x-gastown-managed"`
	Note           string             `json:"x-note"`
	Hooks          map[string][]any   `json:"hooks"`
}

func TestEnsureHooksAt_StubHooksJSON(t *testing.T) {
	tmpDir := t.TempDir()

	err := EnsureHooksAt(tmpDir, "polecat", ".github/hooks", "gastown.json")
	if err != nil {
		t.Fatalf("EnsureHooksAt() error = %v", err)
	}

	hooksPath := filepath.Join(tmpDir, ".github/hooks/gastown.json")
	content, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("Failed to read hooks file: %v", err)
	}

	var config hooksConfigTest
	if err := json.Unmarshal(content, &config); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	// Hooks JSON should be a stub — lifecycle hooks live in the plugin
	if len(config.Hooks) != 0 {
		t.Errorf("Hooks should be empty (plugin handles lifecycle), got %d entries", len(config.Hooks))
	}
	if !config.GastownManaged {
		t.Error("x-gastown-managed should be true")
	}
	if config.Version != 1 {
		t.Errorf("version = %d, want 1", config.Version)
	}
	if !strings.Contains(config.Note, "plugin") {
		t.Error("x-note should mention the plugin")
	}
}

func TestEnsureHooksAt_AutonomousAndInteractiveSameStub(t *testing.T) {
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	if err := EnsureHooksAt(tmpDir1, "polecat", ".github/hooks", "gastown.json"); err != nil {
		t.Fatalf("autonomous: %v", err)
	}
	if err := EnsureHooksAt(tmpDir2, "mayor", ".github/hooks", "gastown.json"); err != nil {
		t.Fatalf("interactive: %v", err)
	}

	c1, _ := os.ReadFile(filepath.Join(tmpDir1, ".github/hooks/gastown.json"))
	c2, _ := os.ReadFile(filepath.Join(tmpDir2, ".github/hooks/gastown.json"))

	// Both should produce empty hooks stubs (plugin handles everything)
	var cfg1, cfg2 hooksConfigTest
	json.Unmarshal(c1, &cfg1)
	json.Unmarshal(c2, &cfg2)
	if len(cfg1.Hooks) != 0 || len(cfg2.Hooks) != 0 {
		t.Error("Both autonomous and interactive should have empty hooks (plugin handles them)")
	}
}

func TestEnsureHooksAt_DoesNotOverwrite(t *testing.T) {
	tmpDir := t.TempDir()

	hooksDir := ".github/hooks"
	hooksFile := "gastown.json"
	hooksPath := filepath.Join(tmpDir, hooksDir, hooksFile)

	if err := os.MkdirAll(filepath.Dir(hooksPath), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	existingContent := []byte(`{"existing": true}`)
	if err := os.WriteFile(hooksPath, existingContent, 0644); err != nil {
		t.Fatalf("Failed to create existing file: %v", err)
	}

	err := EnsureHooksAt(tmpDir, "polecat", hooksDir, hooksFile)
	if err != nil {
		t.Fatalf("EnsureHooksAt() error = %v", err)
	}

	content, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("Failed to read hooks file: %v", err)
	}
	if string(content) != string(existingContent) {
		t.Error("EnsureHooksAt() should not overwrite existing file")
	}
}

func TestEnsureHooksAt_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	err := EnsureHooksAt(tmpDir, "polecat", ".github/hooks", "gastown.json")
	if err != nil {
		t.Fatalf("EnsureHooksAt() error = %v", err)
	}

	dirInfo, err := os.Stat(filepath.Join(tmpDir, ".github/hooks"))
	if err != nil {
		t.Fatalf("Hooks directory was not created: %v", err)
	}
	if !dirInfo.IsDir() {
		t.Error("Hooks path should be a directory")
	}
}

func TestEnsureHooksAt_GuardScriptExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode checks are not reliable on Windows")
	}

	tmpDir := t.TempDir()

	err := EnsureHooksAt(tmpDir, "polecat", ".github/hooks", "gastown.json")
	if err != nil {
		t.Fatalf("EnsureHooksAt() error = %v", err)
	}

	guardPath := filepath.Join(tmpDir, ".github/hooks/gastown-pretool-guard.sh")
	info, err := os.Stat(guardPath)
	if err != nil {
		t.Fatalf("Guard script not created: %v", err)
	}

	expectedMode := os.FileMode(0755)
	if info.Mode() != expectedMode {
		t.Errorf("Guard script mode = %v, want %v", info.Mode(), expectedMode)
	}
}

func TestEnsureHooksAt_GuardScriptContent(t *testing.T) {
	tmpDir := t.TempDir()

	err := EnsureHooksAt(tmpDir, "polecat", ".github/hooks", "gastown.json")
	if err != nil {
		t.Fatalf("EnsureHooksAt() error = %v", err)
	}

	guardPath := filepath.Join(tmpDir, ".github/hooks/gastown-pretool-guard.sh")
	content, err := os.ReadFile(guardPath)
	if err != nil {
		t.Fatalf("Failed to read guard script: %v", err)
	}

	checks := []string{
		"#!/bin/bash",
		"jq -r '.toolName'",
		"gh pr create",
		"git checkout -b",
		"git switch -c",
		"gt tap guard pr-workflow",
		"permissionDecision",
	}
	for _, check := range checks {
		if !strings.Contains(string(content), check) {
			t.Errorf("Guard script missing expected pattern: %q", check)
		}
	}
}

func TestEnsureHooksAt_PathSetup(t *testing.T) {
	// Hooks JSON is now a stub — no hook entries with PATH setup needed.
	// PATH setup is in the plugin's hooks.json instead.
	tmpDir := t.TempDir()

	err := EnsureHooksAt(tmpDir, "polecat", ".github/hooks", "gastown.json")
	if err != nil {
		t.Fatalf("EnsureHooksAt() error = %v", err)
	}

	hooksPath := filepath.Join(tmpDir, ".github/hooks/gastown.json")
	content, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("Failed to read hooks file: %v", err)
	}

	var config hooksConfigTest
	if err := json.Unmarshal(content, &config); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if len(config.Hooks) != 0 {
		t.Error("Hooks should be empty (lifecycle hooks are in plugin)")
	}
}

func TestEnsureHooksAt_ValidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	err := EnsureHooksAt(tmpDir, "polecat", ".github/hooks", "gastown.json")
	if err != nil {
		t.Fatalf("EnsureHooksAt() error = %v", err)
	}

	hooksPath := filepath.Join(tmpDir, ".github/hooks/gastown.json")
	content, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("Failed to read hooks file: %v", err)
	}

	var raw json.RawMessage
	if err := json.Unmarshal(content, &raw); err != nil {
		t.Errorf("Hooks file is not valid JSON: %v", err)
	}
}

func TestEnsureHooksAt_SchemaVersion(t *testing.T) {
	tmpDir := t.TempDir()

	err := EnsureHooksAt(tmpDir, "polecat", ".github/hooks", "gastown.json")
	if err != nil {
		t.Fatalf("EnsureHooksAt() error = %v", err)
	}

	hooksPath := filepath.Join(tmpDir, ".github/hooks/gastown.json")
	content, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("Failed to read hooks file: %v", err)
	}

	var config hooksConfigTest
	if err := json.Unmarshal(content, &config); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if config.Version != 1 {
		t.Errorf("Hooks version = %d, want 1", config.Version)
	}
	if !config.GastownManaged {
		t.Error("Hooks x-gastown-managed should be true")
	}
}

func TestEnsureHooksAt_CustomHooksDir(t *testing.T) {
	tmpDir := t.TempDir()

	customDir := ".copilot/hooks"
	err := EnsureHooksAt(tmpDir, "polecat", customDir, "gastown.json")
	if err != nil {
		t.Fatalf("EnsureHooksAt() error = %v", err)
	}

	// Hooks JSON should be a stub
	hooksPath := filepath.Join(tmpDir, customDir, "gastown.json")
	content, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("Failed to read hooks file: %v", err)
	}

	var config hooksConfigTest
	if err := json.Unmarshal(content, &config); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}
	if len(config.Hooks) != 0 {
		t.Error("Hooks should be empty (plugin handles lifecycle)")
	}

	// Guard script should still be in custom dir
	guardPath := filepath.Join(tmpDir, customDir, "gastown-pretool-guard.sh")
	if _, err := os.Stat(guardPath); err != nil {
		t.Fatalf("Guard script not created in custom dir: %v", err)
	}
}

func TestEnsureHooksAt_EmptyParameters(t *testing.T) {
	t.Run("empty hooksDir", func(t *testing.T) {
		err := EnsureHooksAt("/tmp/work", "polecat", "", "gastown.json")
		if err != nil {
			t.Errorf("EnsureHooksAt() with empty hooksDir should return nil, got %v", err)
		}
	})

	t.Run("empty hooksFile", func(t *testing.T) {
		err := EnsureHooksAt("/tmp/work", "polecat", ".github/hooks", "")
		if err != nil {
			t.Errorf("EnsureHooksAt() with empty hooksFile should return nil, got %v", err)
		}
	})

	t.Run("both empty", func(t *testing.T) {
		err := EnsureHooksAt("/tmp/work", "polecat", "", "")
		if err != nil {
			t.Errorf("EnsureHooksAt() with both empty should return nil, got %v", err)
		}
	})
}
