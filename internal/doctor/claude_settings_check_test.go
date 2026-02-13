package doctor

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewClaudeSettingsCheck(t *testing.T) {
	check := NewClaudeSettingsCheck()

	if check.Name() != "claude-settings" {
		t.Errorf("expected name 'claude-settings', got %q", check.Name())
	}

	if !check.CanFix() {
		t.Error("expected CanFix to return true")
	}
}

func TestClaudeSettingsCheck_NoSettingsFiles(t *testing.T) {
	tmpDir := t.TempDir()

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when no settings files, got %v", result.Status)
	}
}

// createValidSettings creates a valid settings file with all required elements.
// The filename should be settings.local.json for valid tests.
func createValidSettings(t *testing.T, path string) {
	t.Helper()

	settings := map[string]any{
		"enabledPlugins": []string{"plugin1"},
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"matcher": "**",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "export PATH=/usr/local/bin:$PATH",
						},
					},
				},
			},
			"Stop": []any{
				map[string]any{
					"matcher": "**",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "gt costs record --session $CLAUDE_SESSION_ID",
						},
					},
				},
			},
		},
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

// createStaleSettings creates a settings.json missing required elements.
func createStaleSettings(t *testing.T, path string, missingElements ...string) {
	t.Helper()

	settings := map[string]any{
		"enabledPlugins": []string{"plugin1"},
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"matcher": "**",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "export PATH=/usr/local/bin:$PATH",
						},
					},
				},
			},
			"Stop": []any{
				map[string]any{
					"matcher": "**",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "gt costs record --session $CLAUDE_SESSION_ID",
						},
					},
				},
			},
		},
	}

	for _, missing := range missingElements {
		switch missing {
		case "enabledPlugins":
			delete(settings, "enabledPlugins")
		case "hooks":
			delete(settings, "hooks")
		case "PATH":
			// Remove PATH from SessionStart hooks
			hooks := settings["hooks"].(map[string]any)
			sessionStart := hooks["SessionStart"].([]any)
			hookObj := sessionStart[0].(map[string]any)
			innerHooks := hookObj["hooks"].([]any)
			// Filter out PATH command
			var filtered []any
			for _, h := range innerHooks {
				hMap := h.(map[string]any)
				if cmd, ok := hMap["command"].(string); ok && !strings.Contains(cmd, "PATH=") {
					filtered = append(filtered, h)
				}
			}
			hookObj["hooks"] = filtered
		case "Stop":
			hooks := settings["hooks"].(map[string]any)
			delete(hooks, "Stop")
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeSettingsCheck_ValidMayorSettings(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid mayor settings at correct location (mayor/.claude/settings.local.json)
	// settings.json is now considered stale - only settings.local.json is valid.
	// See: https://github.com/anthropics/claude-code/issues/12962
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.local.json")
	createValidSettings(t, mayorSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for valid settings, got %v: %s", result.Status, result.Message)
	}
}

func TestClaudeSettingsCheck_ValidDeaconSettings(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid deacon settings (must be settings.local.json, not settings.json)
	deaconSettings := filepath.Join(tmpDir, "deacon", ".claude", "settings.local.json")
	createValidSettings(t, deaconSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for valid deacon settings, got %v: %s", result.Status, result.Message)
	}
}

func TestClaudeSettingsCheck_ValidWitnessSettings(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create valid witness settings in correct location (witness/rig/.claude/settings.local.json)
	// Claude Code does NOT traverse parent directories for settings.json.
	// Settings must be in the actual working directory (witness/rig/) not the parent.
	witnessSettings := filepath.Join(tmpDir, rigName, "witness", "rig", ".claude", "settings.local.json")
	createValidSettings(t, witnessSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for valid witness settings, got %v: %s", result.Status, result.Message)
	}
}

func TestClaudeSettingsCheck_ValidRefinerySettings(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create valid refinery settings in correct location (refinery/rig/.claude/settings.local.json)
	// Claude Code does NOT traverse parent directories for settings.json.
	refinerySettings := filepath.Join(tmpDir, rigName, "refinery", "rig", ".claude", "settings.local.json")
	createValidSettings(t, refinerySettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for valid refinery settings, got %v: %s", result.Status, result.Message)
	}
}

func TestClaudeSettingsCheck_ValidCrewSettings(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create valid crew settings in correct location (crew/<name>/.claude/settings.local.json)
	// Claude Code does NOT traverse parent directories - settings must be in working directory.
	crewSettings := filepath.Join(tmpDir, rigName, "crew", "worker1", ".claude", "settings.local.json")
	createValidSettings(t, crewSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for valid crew settings, got %v: %s", result.Status, result.Message)
	}
}

func TestClaudeSettingsCheck_ValidPolecatSettings(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create valid polecat settings in correct location (polecats/<name>/<rig>/.claude/settings.local.json)
	// Claude Code does NOT traverse parent directories - settings must be in worktree.
	pcSettings := filepath.Join(tmpDir, rigName, "polecats", "pc1", rigName, ".claude", "settings.local.json")
	createValidSettings(t, pcSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for valid polecat settings, got %v: %s", result.Status, result.Message)
	}
}

func TestClaudeSettingsCheck_MissingEnabledPlugins(t *testing.T) {
	tmpDir := t.TempDir()

	// Create stale mayor settings missing enabledPlugins (use settings.local.json for content checks)
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.local.json")
	createStaleSettings(t, mayorSettings, "enabledPlugins")

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing enabledPlugins, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "1 stale") {
		t.Errorf("expected message about stale settings, got %q", result.Message)
	}
}

func TestClaudeSettingsCheck_MissingHooks(t *testing.T) {
	tmpDir := t.TempDir()

	// Create stale settings missing hooks entirely (use settings.local.json for content checks)
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.local.json")
	createStaleSettings(t, mayorSettings, "hooks")

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing hooks, got %v", result.Status)
	}
}

func TestClaudeSettingsCheck_MissingPATH(t *testing.T) {
	tmpDir := t.TempDir()

	// Create stale settings missing PATH export (use settings.local.json for content checks)
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.local.json")
	createStaleSettings(t, mayorSettings, "PATH")

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing PATH, got %v", result.Status)
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "PATH export") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention PATH export, got %v", result.Details)
	}
}

func TestClaudeSettingsCheck_MissingStopHook(t *testing.T) {
	tmpDir := t.TempDir()

	// Create stale settings missing Stop hook (use settings.local.json for content checks)
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.local.json")
	createStaleSettings(t, mayorSettings, "Stop")

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing Stop hook, got %v", result.Status)
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "Stop hook") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention Stop hook, got %v", result.Details)
	}
}

func TestClaudeSettingsCheck_WrongLocationWitness(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create settings in wrong location (witness/rig/.claude/ instead of witness/.claude/)
	// Settings inside git repos should be flagged as wrong location
	wrongSettings := filepath.Join(tmpDir, rigName, "witness", "rig", ".claude", "settings.json")
	createValidSettings(t, wrongSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for wrong location, got %v", result.Status)
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "wrong location") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention wrong location, got %v", result.Details)
	}
}

func TestClaudeSettingsCheck_WrongLocationRefinery(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create settings in wrong location (refinery/rig/.claude/ instead of refinery/.claude/)
	// Settings inside git repos should be flagged as wrong location
	wrongSettings := filepath.Join(tmpDir, rigName, "refinery", "rig", ".claude", "settings.json")
	createValidSettings(t, wrongSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for wrong location, got %v", result.Status)
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "wrong location") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention wrong location, got %v", result.Details)
	}
}

func TestClaudeSettingsCheck_MultipleStaleFiles(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create multiple stale settings files (all using old settings.json which is now stale)
	// settings.json is stale even in correct locations - should be settings.local.json
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.json")
	createValidSettings(t, mayorSettings) // Valid content but stale filename

	deaconSettings := filepath.Join(tmpDir, "deacon", ".claude", "settings.json")
	createValidSettings(t, deaconSettings) // Valid content but stale filename

	// Settings in wrong location (witness/rig/.claude/settings.json)
	// This creates BOTH a stale file AND a missing settings.local.json issue
	witnessWrong := filepath.Join(tmpDir, rigName, "witness", "rig", ".claude", "settings.json")
	createValidSettings(t, witnessWrong) // Valid content but stale filename

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for multiple stale files, got %v", result.Status)
	}
	// 3 stale files + 3 missing settings.local.json = 6 issues
	// Each directory with stale settings.json also reports missing settings.local.json
	// because the stale file might have local modifications needing manual review
	if !strings.Contains(result.Message, "6") {
		t.Errorf("expected message about 6 issues (3 stale + 3 missing), got %q", result.Message)
	}
}

func TestClaudeSettingsCheck_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	// Create invalid JSON file (use settings.local.json for content validation)
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(mayorSettings), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mayorSettings, []byte("not valid json {"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for invalid JSON, got %v", result.Status)
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "invalid JSON") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention invalid JSON, got %v", result.Details)
	}
}

func TestClaudeSettingsCheck_FixDeletesStaleFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create stale settings in wrong location (inside git repo - easy to test - just delete, no recreate)
	rigName := "testrig"
	wrongSettings := filepath.Join(tmpDir, rigName, "witness", "rig", ".claude", "settings.json")
	createValidSettings(t, wrongSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	// Run to detect - should find both stale file AND missing settings.local.json
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify stale file was deleted
	if _, err := os.Stat(wrongSettings); !os.IsNotExist(err) {
		t.Error("expected wrong location settings to be deleted")
	}

	// After fix, settings.local.json is recreated at the correct location by EnsureSettingsForRole.
	// The check should now pass since the correct file exists.
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after fix (settings recreated at correct location), got %v: %v", result.Status, result.Details)
	}
}

func TestClaudeSettingsCheck_SkipsNonRigDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directories that should be skipped as rigs
	// Note: don't use mayor/deacon here because those are legitimate town-level agent
	// directories - creating subdirs there triggers missing settings detection
	for _, skipDir := range []string{"daemon", ".git", "docs", ".hidden"} {
		dir := filepath.Join(tmpDir, skipDir, "witness", "rig", ".claude")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		// These should NOT be detected as rig witness settings
		settingsPath := filepath.Join(dir, "settings.json")
		createStaleSettings(t, settingsPath, "PATH")
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	_ = check.Run(ctx)

	// Count how many stale files were found - should be 0 since none of the
	// skipped directories (daemon, .git, docs, .hidden) are detected as rigs
	if len(check.staleSettings) != 0 {
		t.Errorf("expected 0 stale files (skipped dirs), got %d", len(check.staleSettings))
	}
}

func TestClaudeSettingsCheck_MixedValidAndStale(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create valid mayor settings (settings.local.json in correct location)
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.local.json")
	createValidSettings(t, mayorSettings)

	// Create stale witness settings (settings.local.json missing PATH, in correct location)
	witnessSettings := filepath.Join(tmpDir, rigName, "witness", "rig", ".claude", "settings.local.json")
	createStaleSettings(t, witnessSettings, "PATH")

	// Create valid refinery settings (settings.local.json in correct location)
	refinerySettings := filepath.Join(tmpDir, rigName, "refinery", "rig", ".claude", "settings.local.json")
	createValidSettings(t, refinerySettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for mixed valid/stale, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "1 stale") {
		t.Errorf("expected message about 1 stale file, got %q", result.Message)
	}
	// Should only report the witness settings as stale
	if len(result.Details) != 1 {
		t.Errorf("expected 1 detail, got %d: %v", len(result.Details), result.Details)
	}
}

func TestClaudeSettingsCheck_WrongLocationCrew(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create settings in wrong location (crew/<name>/.claude/ instead of crew/.claude/)
	// Settings inside git repos should be flagged as wrong location
	wrongSettings := filepath.Join(tmpDir, rigName, "crew", "agent1", ".claude", "settings.json")
	createValidSettings(t, wrongSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for wrong location, got %v", result.Status)
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "wrong location") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention wrong location, got %v", result.Details)
	}
}

func TestClaudeSettingsCheck_WrongLocationPolecat(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create settings in wrong location (polecats/<name>/.claude/ instead of polecats/.claude/)
	// Settings inside git repos should be flagged as wrong location
	wrongSettings := filepath.Join(tmpDir, rigName, "polecats", "pc1", ".claude", "settings.json")
	createValidSettings(t, wrongSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for wrong location, got %v", result.Status)
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "wrong location") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention wrong location, got %v", result.Details)
	}
}

// initTestGitRepo initializes a git repo in the given directory for settings tests.
func initTestGitRepo(t *testing.T, dir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test User"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git command %v failed: %v\n%s", args, err, out)
		}
	}
}

// gitAddAndCommit adds and commits a file.
func gitAddAndCommit(t *testing.T, repoDir, filePath string) {
	t.Helper()
	// Get relative path from repo root
	relPath, err := filepath.Rel(repoDir, filePath)
	if err != nil {
		t.Fatal(err)
	}

	cmds := [][]string{
		{"git", "add", relPath},
		{"git", "commit", "-m", "Add file"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git command %v failed: %v\n%s", args, err, out)
		}
	}
}

func TestClaudeSettingsCheck_GitStatusUntracked(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create a git repo to simulate a source repo
	rigDir := filepath.Join(tmpDir, rigName, "witness", "rig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	initTestGitRepo(t, rigDir)

	// Create an untracked settings file (not git added)
	wrongSettings := filepath.Join(rigDir, ".claude", "settings.json")
	createValidSettings(t, wrongSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for wrong location, got %v", result.Status)
	}
	// Should mention "untracked"
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "untracked") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention untracked, got %v", result.Details)
	}
}

func TestClaudeSettingsCheck_GitStatusTrackedClean(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create a git repo to simulate a source repo
	rigDir := filepath.Join(tmpDir, rigName, "witness", "rig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	initTestGitRepo(t, rigDir)

	// Create settings and commit it (tracked, clean)
	trackedSettings := filepath.Join(rigDir, ".claude", "settings.json")
	createValidSettings(t, trackedSettings)
	gitAddAndCommit(t, rigDir, trackedSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// Tracked settings.json in a worktree is the customer's legitimate project config.
	// It should NOT be flagged as stale or wrong-location.
	// The only issue should be the missing settings.local.json (informational).
	for _, d := range result.Details {
		if strings.Contains(d, "wrong location") && strings.Contains(d, "settings.json") {
			t.Errorf("tracked settings.json should NOT be flagged as wrong location, got: %s", d)
		}
	}
}

func TestClaudeSettingsCheck_GitStatusTrackedModified(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create a git repo to simulate a source repo
	rigDir := filepath.Join(tmpDir, rigName, "witness", "rig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	initTestGitRepo(t, rigDir)

	// Create settings and commit it
	trackedSettings := filepath.Join(rigDir, ".claude", "settings.json")
	createValidSettings(t, trackedSettings)
	gitAddAndCommit(t, rigDir, trackedSettings)

	// Modify the file after commit
	if err := os.WriteFile(trackedSettings, []byte(`{"modified": true}`), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// Tracked settings.json (even modified) in a worktree is the customer's project config.
	// It should NOT be flagged as stale or wrong-location.
	for _, d := range result.Details {
		if strings.Contains(d, "wrong location") && strings.Contains(d, "settings.json") {
			t.Errorf("tracked-modified settings.json should NOT be flagged as wrong location, got: %s", d)
		}
	}
}

func TestClaudeSettingsCheck_FixPreservesModifiedFiles(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create a git repo to simulate a source repo
	rigDir := filepath.Join(tmpDir, rigName, "witness", "rig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	initTestGitRepo(t, rigDir)

	// Create settings and commit it
	trackedSettings := filepath.Join(rigDir, ".claude", "settings.json")
	createValidSettings(t, trackedSettings)
	gitAddAndCommit(t, rigDir, trackedSettings)

	// Modify the file after commit
	if err := os.WriteFile(trackedSettings, []byte(`{"modified": true}`), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	// Run to detect and fix
	_ = check.Run(ctx)
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Tracked-modified file should be preserved (customer's project config)
	if _, err := os.Stat(trackedSettings); os.IsNotExist(err) {
		t.Error("expected tracked-modified file to be preserved, but it was deleted")
	}
}

func TestClaudeSettingsCheck_FixDeletesUntrackedFiles(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create a git repo to simulate a source repo
	rigDir := filepath.Join(tmpDir, rigName, "witness", "rig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	initTestGitRepo(t, rigDir)

	// Create an untracked settings file (not git added)
	wrongSettings := filepath.Join(rigDir, ".claude", "settings.json")
	createValidSettings(t, wrongSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	// Run to detect
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix - should delete the untracked file
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify file was deleted
	if _, err := os.Stat(wrongSettings); !os.IsNotExist(err) {
		t.Error("expected untracked file to be deleted")
	}
}

func TestClaudeSettingsCheck_FixPreservesTrackedCleanFiles(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create a git repo to simulate a source repo
	rigDir := filepath.Join(tmpDir, rigName, "witness", "rig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	initTestGitRepo(t, rigDir)

	// Create settings and commit it (tracked, clean) — customer's project config
	trackedSettings := filepath.Join(rigDir, ".claude", "settings.json")
	createValidSettings(t, trackedSettings)
	gitAddAndCommit(t, rigDir, trackedSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	// Run to detect
	_ = check.Run(ctx)

	// Apply fix
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Tracked settings.json should be preserved (customer's project config)
	if _, err := os.Stat(trackedSettings); os.IsNotExist(err) {
		t.Error("expected tracked settings.json to be preserved, but it was deleted")
	}
}

// NOTE: TestClaudeSettingsCheck_DetectsStaleCLAUDEmdAtTownRoot and
// TestClaudeSettingsCheck_FixMovesCLAUDEmdToMayor were removed because
// CLAUDE.md at town root is now intentionally created by gt install.
// It serves as an identity anchor for Mayor/Deacon who run from the town root.
// See install.go createTownRootCLAUDEmd() for details.

func TestClaudeSettingsCheck_GitIgnoredFilesNotFlagged(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize git repo at town root
	initTestGitRepo(t, tmpDir)

	// Create .gitignore with CLAUDE.md
	gitignorePath := filepath.Join(tmpDir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte("CLAUDE.md\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitAddAndCommit(t, tmpDir, gitignorePath)

	// Create CLAUDE.md at town root (wrong location but gitignored)
	claudeMdPath := filepath.Join(tmpDir, "CLAUDE.md")
	if err := os.WriteFile(claudeMdPath, []byte("# Mayor Context\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// Should pass because the file is properly gitignored
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for gitignored CLAUDE.md, got %v: %s\nDetails: %v",
			result.Status, result.Message, result.Details)
	}
}

func TestClaudeSettingsCheck_TownRootSettingsWarnsInsteadOfKilling(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mayor directory (needed for fix to recreate settings there)
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create settings.json at town root (wrong location - pollutes all agents)
	staleTownRootDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(staleTownRootDir, 0755); err != nil {
		t.Fatal(err)
	}
	staleTownRootSettings := filepath.Join(staleTownRootDir, "settings.json")
	// Create valid settings content
	settingsContent := `{
		"env": {"PATH": "/usr/bin"},
		"enabledPlugins": ["claude-code-expert"],
		"hooks": {
			"SessionStart": [{"matcher": "", "hooks": [{"type": "command", "command": "gt prime"}]}],
			"Stop": [{"matcher": "", "hooks": [{"type": "command", "command": "gt handoff"}]}]
		}
	}`
	if err := os.WriteFile(staleTownRootSettings, []byte(settingsContent), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	// Run to detect
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError for town root settings, got %v", result.Status)
	}

	// Verify it's flagged as wrong location
	foundWrongLocation := false
	for _, d := range result.Details {
		if strings.Contains(d, "wrong location") {
			foundWrongLocation = true
			break
		}
	}
	if !foundWrongLocation {
		t.Errorf("expected details to mention wrong location, got %v", result.Details)
	}

	// Apply fix - should NOT return error and should NOT kill sessions
	// (session killing would require tmux which isn't available in tests)
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify stale file was deleted
	if _, err := os.Stat(staleTownRootSettings); !os.IsNotExist(err) {
		t.Error("expected settings.json at town root to be deleted")
	}

	// Verify .claude directory was cleaned up (best-effort)
	if _, err := os.Stat(staleTownRootDir); !os.IsNotExist(err) {
		t.Error("expected .claude directory at town root to be deleted")
	}
}

// Tests for missing file detection
// When working directory exists but settings.local.json is missing, the check should
// report it as a missing file that needs agent restart to create.

func TestClaudeSettingsCheck_MissingWitnessSettings(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create witness working directory but NOT the settings.local.json
	witnessWorkDir := filepath.Join(tmpDir, rigName, "witness", "rig")
	if err := os.MkdirAll(witnessWorkDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing witness settings, got %v", result.Status)
	}

	// Should mention "missing" and "restart"
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "missing") && strings.Contains(d, "restart") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention missing and restart, got %v", result.Details)
	}

	// Should mention witness agent type
	foundWitness := false
	for _, d := range result.Details {
		if strings.Contains(d, "witness") {
			foundWitness = true
			break
		}
	}
	if !foundWitness {
		t.Errorf("expected details to mention witness, got %v", result.Details)
	}

	// Verify the staleSettings entry has missingFile set to true
	if len(check.staleSettings) != 1 {
		t.Fatalf("expected 1 stale setting, got %d", len(check.staleSettings))
	}
	if !check.staleSettings[0].missingFile {
		t.Error("expected missingFile to be true for missing witness settings")
	}
	if check.staleSettings[0].agentType != "witness" {
		t.Errorf("expected agentType 'witness', got %q", check.staleSettings[0].agentType)
	}
}

func TestClaudeSettingsCheck_MissingRefinerySettings(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create refinery working directory but NOT the settings.local.json
	refineryWorkDir := filepath.Join(tmpDir, rigName, "refinery", "rig")
	if err := os.MkdirAll(refineryWorkDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing refinery settings, got %v", result.Status)
	}

	// Verify the staleSettings entry has missingFile set to true
	if len(check.staleSettings) != 1 {
		t.Fatalf("expected 1 stale setting, got %d", len(check.staleSettings))
	}
	if !check.staleSettings[0].missingFile {
		t.Error("expected missingFile to be true for missing refinery settings")
	}
	if check.staleSettings[0].agentType != "refinery" {
		t.Errorf("expected agentType 'refinery', got %q", check.staleSettings[0].agentType)
	}

	// Should include hint about restarting agents
	if !strings.Contains(result.FixHint, "restart") {
		t.Errorf("expected fix hint to mention restart, got %q", result.FixHint)
	}
}

func TestClaudeSettingsCheck_MissingCrewSettings(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create crew worker directory but NOT the settings.local.json
	crewWorkDir := filepath.Join(tmpDir, rigName, "crew", "worker1")
	if err := os.MkdirAll(crewWorkDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing crew settings, got %v", result.Status)
	}

	// Verify the staleSettings entry has missingFile set to true
	if len(check.staleSettings) != 1 {
		t.Fatalf("expected 1 stale setting, got %d", len(check.staleSettings))
	}
	if !check.staleSettings[0].missingFile {
		t.Error("expected missingFile to be true for missing crew settings")
	}
	if check.staleSettings[0].agentType != "crew" {
		t.Errorf("expected agentType 'crew', got %q", check.staleSettings[0].agentType)
	}
}

func TestClaudeSettingsCheck_MissingPolecatSettings(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create polecat worktree directory but NOT the settings.local.json
	polecatWorkDir := filepath.Join(tmpDir, rigName, "polecats", "pc1", rigName)
	if err := os.MkdirAll(polecatWorkDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing polecat settings, got %v", result.Status)
	}

	// Verify the staleSettings entry has missingFile set to true
	if len(check.staleSettings) != 1 {
		t.Fatalf("expected 1 stale setting, got %d", len(check.staleSettings))
	}
	if !check.staleSettings[0].missingFile {
		t.Error("expected missingFile to be true for missing polecat settings")
	}
	if check.staleSettings[0].agentType != "polecat" {
		t.Errorf("expected agentType 'polecat', got %q", check.staleSettings[0].agentType)
	}
}

func TestClaudeSettingsCheck_MissingMultipleAgentSettings(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create multiple working directories without settings.local.json
	dirs := []string{
		filepath.Join(tmpDir, rigName, "witness", "rig"),
		filepath.Join(tmpDir, rigName, "refinery", "rig"),
		filepath.Join(tmpDir, rigName, "crew", "worker1"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing settings, got %v", result.Status)
	}

	// Should report 3 missing files
	if len(check.staleSettings) != 3 {
		t.Errorf("expected 3 stale settings, got %d", len(check.staleSettings))
	}

	// All should have missingFile set to true
	for _, sf := range check.staleSettings {
		if !sf.missingFile {
			t.Errorf("expected missingFile to be true for %s", sf.path)
		}
	}

	// Message should mention multiple agents
	if !strings.Contains(result.Message, "3") {
		t.Errorf("expected message to mention 3 agents, got %q", result.Message)
	}
}

func TestClaudeSettingsCheck_MixedMissingAndStale(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create witness with valid settings
	witnessSettings := filepath.Join(tmpDir, rigName, "witness", "rig", ".claude", "settings.local.json")
	createValidSettings(t, witnessSettings)

	// Create refinery working directory without settings (missing)
	refineryWorkDir := filepath.Join(tmpDir, rigName, "refinery", "rig")
	if err := os.MkdirAll(refineryWorkDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create stale settings.json (wrong filename) for mayor
	mayorStaleSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.json")
	createValidSettings(t, mayorStaleSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for mixed issues, got %v", result.Status)
	}

	// Should have 3 issues:
	// 1. mayor stale settings.json
	// 2. mayor missing settings.local.json (reported separately from stale)
	// 3. refinery missing settings.local.json
	if len(check.staleSettings) != 3 {
		t.Errorf("expected 3 stale settings, got %d: %+v", len(check.staleSettings), check.staleSettings)
	}

	// Verify we have both types
	var hasMissing, hasStale bool
	for _, sf := range check.staleSettings {
		if sf.missingFile {
			hasMissing = true
		}
		if sf.wrongLocation {
			hasStale = true
		}
	}
	if !hasMissing {
		t.Error("expected at least one missing file")
	}
	if !hasStale {
		t.Error("expected at least one stale file")
	}
}

func TestClaudeSettingsCheck_MissingFileOnlyMessage(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create only missing files (no stale files)
	witnessWorkDir := filepath.Join(tmpDir, rigName, "witness", "rig")
	if err := os.MkdirAll(witnessWorkDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing settings, got %v", result.Status)
	}

	// When only missing files, message should mention "missing settings"
	if !strings.Contains(result.Message, "missing") {
		t.Errorf("expected message to mention 'missing', got %q", result.Message)
	}

	// Fix hint should mention restart for missing files
	if !strings.Contains(result.FixHint, "gt up --restart") {
		t.Errorf("expected fix hint to mention 'gt up --restart', got %q", result.FixHint)
	}
}

func TestClaudeSettingsCheck_NoMissingFileWhenDirNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create rig directory structure but NOT the witness/rig working directory
	// This simulates a rig that doesn't have witness set up yet
	rigDir := filepath.Join(tmpDir, rigName)
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// Should be OK - no settings issues if working directory doesn't exist
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK when witness working dir doesn't exist, got %v: %s", result.Status, result.Message)
	}
}

func TestClaudeSettingsCheck_FixDoesNotDeleteMissingFiles(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create witness working directory but NOT the settings.local.json
	witnessWorkDir := filepath.Join(tmpDir, rigName, "witness", "rig")
	if err := os.MkdirAll(witnessWorkDir, 0755); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	// Run to detect
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix - should NOT try to delete a file that doesn't exist
	// and should NOT error
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed unexpectedly: %v", err)
	}

	// Working directory should still exist
	if _, err := os.Stat(witnessWorkDir); os.IsNotExist(err) {
		t.Error("expected working directory to still exist after fix")
	}
}
