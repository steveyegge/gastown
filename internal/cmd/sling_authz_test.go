package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAddMCPPermissionsToSettings_NewFile(t *testing.T) {
	dir := t.TempDir()
	worktreeRoot := filepath.Join(dir, "worktree")
	if err := os.MkdirAll(worktreeRoot, 0755); err != nil {
		t.Fatal(err)
	}

	// No existing settings.json — should create one
	err := addMCPPermissionsToSettings(worktreeRoot, []string{"github", "linear"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read and verify
	data, err := os.ReadFile(filepath.Join(worktreeRoot, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings: %v", err)
	}

	// Check enableAllProjectMcpServers
	if v, ok := settings["enableAllProjectMcpServers"].(bool); !ok || !v {
		t.Error("enableAllProjectMcpServers should be true")
	}

	// Check permissions
	perms, ok := settings["permissions"].(map[string]interface{})
	if !ok {
		t.Fatal("missing permissions")
	}
	allow, ok := perms["allow"].([]interface{})
	if !ok {
		t.Fatal("missing permissions.allow")
	}

	expected := map[string]bool{
		"mcp__github__*": false,
		"mcp__linear__*": false,
	}
	for _, a := range allow {
		s, ok := a.(string)
		if !ok {
			continue
		}
		if _, exists := expected[s]; exists {
			expected[s] = true
		}
	}
	for k, found := range expected {
		if !found {
			t.Errorf("missing permission %q", k)
		}
	}
}

func TestAddMCPPermissionsToSettings_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	worktreeRoot := filepath.Join(dir, "worktree")
	claudeDir := filepath.Join(worktreeRoot, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create existing settings with some permissions
	existing := map[string]interface{}{
		"permissions": map[string]interface{}{
			"allow": []interface{}{"Read", "Write", "mcp__github__*"},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	err := addMCPPermissionsToSettings(worktreeRoot, []string{"github", "linear"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read and verify
	data, err = os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings: %v", err)
	}

	perms := settings["permissions"].(map[string]interface{})
	allow := perms["allow"].([]interface{})

	// github should not be duplicated (already existed)
	githubCount := 0
	linearFound := false
	for _, a := range allow {
		s := a.(string)
		if s == "mcp__github__*" {
			githubCount++
		}
		if s == "mcp__linear__*" {
			linearFound = true
		}
	}
	if githubCount != 1 {
		t.Errorf("mcp__github__* should appear exactly once, got %d", githubCount)
	}
	if !linearFound {
		t.Error("mcp__linear__* should have been added")
	}

	// Original permissions should be preserved
	readFound := false
	for _, a := range allow {
		if a.(string) == "Read" {
			readFound = true
		}
	}
	if !readFound {
		t.Error("existing 'Read' permission should be preserved")
	}
}

func TestAddMCPPermissionsToSettings_ParentDir(t *testing.T) {
	dir := t.TempDir()
	// Simulate polecats/<name>/worktree layout
	polecatsDir := filepath.Join(dir, "polecats")
	worktreeRoot := filepath.Join(polecatsDir, "Toast")
	if err := os.MkdirAll(worktreeRoot, 0755); err != nil {
		t.Fatal(err)
	}

	// Create shared settings at polecats/.claude/settings.json
	sharedClaudeDir := filepath.Join(polecatsDir, ".claude")
	if err := os.MkdirAll(sharedClaudeDir, 0755); err != nil {
		t.Fatal(err)
	}
	existing := map[string]interface{}{
		"permissions": map[string]interface{}{
			"allow": []interface{}{"Read", "Write"},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(filepath.Join(sharedClaudeDir, "settings.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	err := addMCPPermissionsToSettings(worktreeRoot, []string{"github"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have written to the parent (shared) settings
	settingsData, err := os.ReadFile(filepath.Join(sharedClaudeDir, "settings.json"))
	if err != nil {
		t.Fatalf("reading shared settings: %v", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		t.Fatalf("parsing settings: %v", err)
	}

	perms := settings["permissions"].(map[string]interface{})
	allow := perms["allow"].([]interface{})

	githubFound := false
	for _, a := range allow {
		if a.(string) == "mcp__github__*" {
			githubFound = true
		}
	}
	if !githubFound {
		t.Error("mcp__github__* should have been added to shared settings")
	}
}
