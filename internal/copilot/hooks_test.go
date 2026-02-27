package copilot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEnsureHooksAt_AutonomousRole(t *testing.T) {
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

	// Autonomous roles should have mail injection in sessionStart
	if !strings.Contains(string(content), "gt mail check --inject") {
		t.Error("Autonomous hooks should contain mail injection in sessionStart")
	}

	// Verify sessionStart has both prime and mail
	var config hooksConfig
	if err := json.Unmarshal(content, &config); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}
	sessionStart := config.Hooks["sessionStart"]
	if len(sessionStart) == 0 {
		t.Fatal("Missing sessionStart hooks")
	}
	if !strings.Contains(sessionStart[0].Bash, "gt prime --hook") {
		t.Error("sessionStart should contain gt prime --hook")
	}
	if !strings.Contains(sessionStart[0].Bash, "gt mail check --inject") {
		t.Error("Autonomous sessionStart should contain gt mail check --inject")
	}
}

func TestEnsureHooksAt_InteractiveRole(t *testing.T) {
	tmpDir := t.TempDir()

	err := EnsureHooksAt(tmpDir, "mayor", ".github/hooks", "gastown.json")
	if err != nil {
		t.Fatalf("EnsureHooksAt() error = %v", err)
	}

	hooksPath := filepath.Join(tmpDir, ".github/hooks/gastown.json")
	content, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("Failed to read hooks file: %v", err)
	}

	var config hooksConfig
	if err := json.Unmarshal(content, &config); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	// Interactive sessionStart should NOT have mail injection
	sessionStart := config.Hooks["sessionStart"]
	if len(sessionStart) == 0 {
		t.Fatal("Missing sessionStart hooks")
	}
	if strings.Contains(sessionStart[0].Bash, "gt mail check --inject") {
		t.Error("Interactive sessionStart should NOT contain gt mail check --inject")
	}
	if !strings.Contains(sessionStart[0].Bash, "gt prime --hook") {
		t.Error("Interactive sessionStart should contain gt prime --hook")
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

	var config hooksConfig
	if err := json.Unmarshal(content, &config); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	for hookName, entries := range config.Hooks {
		for i, entry := range entries {
			if !strings.Contains(entry.Bash, "export PATH=") {
				t.Errorf("Hook %s[%d] missing PATH setup", hookName, i)
			}
		}
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

	var config hooksConfig
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
