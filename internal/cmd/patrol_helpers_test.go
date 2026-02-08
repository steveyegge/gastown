package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

func TestBuildRefineryPatrolVars_NilContext(t *testing.T) {
	ctx := RoleContext{}
	vars := buildRefineryPatrolVars(ctx)
	if len(vars) != 0 {
		t.Errorf("expected empty vars for nil context, got %v", vars)
	}
}

func TestBuildRefineryPatrolVars_MissingSettings(t *testing.T) {
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "testrig")
	if err := os.MkdirAll(filepath.Join(rigDir, "settings"), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx := RoleContext{
		TownRoot: tmpDir,
		Rig:      "testrig",
	}
	vars := buildRefineryPatrolVars(ctx)
	if len(vars) != 0 {
		t.Errorf("expected empty vars when settings file missing, got %v", vars)
	}
}

func TestBuildRefineryPatrolVars_NilMergeQueue(t *testing.T) {
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "testrig")
	settingsDir := filepath.Join(rigDir, "settings")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write settings with no merge_queue
	settings := config.RigSettings{
		Type:    "rig-settings",
		Version: 1,
	}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := RoleContext{
		TownRoot: tmpDir,
		Rig:      "testrig",
	}
	vars := buildRefineryPatrolVars(ctx)
	if len(vars) != 0 {
		t.Errorf("expected empty vars when merge_queue is nil, got %v", vars)
	}
}

func TestBuildRefineryPatrolVars_FullConfig(t *testing.T) {
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "testrig")
	settingsDir := filepath.Join(rigDir, "settings")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mq := config.DefaultMergeQueueConfig()
	settings := config.RigSettings{
		Type:       "rig-settings",
		Version:    1,
		MergeQueue: mq,
	}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := RoleContext{
		TownRoot: tmpDir,
		Rig:      "testrig",
	}
	vars := buildRefineryPatrolVars(ctx)

	// DefaultMergeQueueConfig: refinery_enabled=true, auto_land=false, run_tests=true,
	// test_command="go test ./...", target_branch="main", delete_merged_branches=true
	expected := map[string]string{
		"integration_branch_refinery_enabled": "true",
		"integration_branch_auto_land":        "false",
		"run_tests":                           "true",
		"test_command":                        "go test ./...",
		"target_branch":                       "main",
		"delete_merged_branches":              "true",
	}

	varMap := make(map[string]string)
	for _, v := range vars {
		parts := splitFirstEquals(v)
		if len(parts) == 2 {
			varMap[parts[0]] = parts[1]
		}
	}

	for key, want := range expected {
		got, ok := varMap[key]
		if !ok {
			t.Errorf("missing var %q", key)
			continue
		}
		if got != want {
			t.Errorf("var %q = %q, want %q", key, got, want)
		}
	}

	if len(vars) != len(expected) {
		t.Errorf("expected %d vars, got %d: %v", len(expected), len(vars), vars)
	}
}

func TestBuildRefineryPatrolVars_EmptyTestCommand(t *testing.T) {
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "testrig")
	settingsDir := filepath.Join(rigDir, "settings")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mq := &config.MergeQueueConfig{
		Enabled:              true,
		TargetBranch:         "main",
		RunTests:             false,
		TestCommand:          "", // empty - should be omitted
		DeleteMergedBranches: true,
	}
	settings := config.RigSettings{
		Type:       "rig-settings",
		Version:    1,
		MergeQueue: mq,
	}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := RoleContext{
		TownRoot: tmpDir,
		Rig:      "testrig",
	}
	vars := buildRefineryPatrolVars(ctx)

	varMap := make(map[string]string)
	for _, v := range vars {
		parts := splitFirstEquals(v)
		if len(parts) == 2 {
			varMap[parts[0]] = parts[1]
		}
	}

	// test_command should not be present when empty
	if _, ok := varMap["test_command"]; ok {
		t.Error("test_command should be omitted when empty")
	}

	// run_tests should be "false"
	if got := varMap["run_tests"]; got != "false" {
		t.Errorf("run_tests = %q, want %q", got, "false")
	}
}

func TestBuildRefineryPatrolVars_BoolFormat(t *testing.T) {
	tmpDir := t.TempDir()
	rigDir := filepath.Join(tmpDir, "testrig")
	settingsDir := filepath.Join(rigDir, "settings")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	trueVal := true
	mq := &config.MergeQueueConfig{
		Enabled:                         true,
		IntegrationBranchAutoLand:       &trueVal,
		IntegrationBranchRefineryEnabled: &trueVal,
		RunTests:                         true,
		TargetBranch:                     "develop",
		TestCommand:                      "make test",
		DeleteMergedBranches:             false,
	}
	settings := config.RigSettings{
		Type:       "rig-settings",
		Version:    1,
		MergeQueue: mq,
	}
	data, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(settingsDir, "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := RoleContext{
		TownRoot: tmpDir,
		Rig:      "testrig",
	}
	vars := buildRefineryPatrolVars(ctx)

	varMap := make(map[string]string)
	for _, v := range vars {
		parts := splitFirstEquals(v)
		if len(parts) == 2 {
			varMap[parts[0]] = parts[1]
		}
	}

	// Check bool format is "true"/"false" strings
	if got := varMap["integration_branch_auto_land"]; got != "true" {
		t.Errorf("integration_branch_auto_land = %q, want %q", got, "true")
	}
	if got := varMap["delete_merged_branches"]; got != "false" {
		t.Errorf("delete_merged_branches = %q, want %q", got, "false")
	}
	if got := varMap["target_branch"]; got != "develop" {
		t.Errorf("target_branch = %q, want %q", got, "develop")
	}
	if got := varMap["test_command"]; got != "make test" {
		t.Errorf("test_command = %q, want %q", got, "make test")
	}
}

// splitFirstEquals splits a string on the first '=' only.
func splitFirstEquals(s string) []string {
	idx := -1
	for i, c := range s {
		if c == '=' {
			idx = i
			break
		}
	}
	if idx < 0 {
		return []string{s}
	}
	return []string{s[:idx], s[idx+1:]}
}
