package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/hooks"
)

func TestSyncTargetCreatesNew(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Save a base config
	base := &hooks.HooksConfig{
		SessionStart: []hooks.HookEntry{
			{Matcher: "", Hooks: []hooks.Hook{{Type: "command", Command: "echo hello"}}},
		},
	}
	if err := hooks.SaveBase(base); err != nil {
		t.Fatalf("SaveBase failed: %v", err)
	}

	// Target that doesn't exist yet
	targetPath := filepath.Join(tmpDir, "test-rig", "crew", ".claude", "settings.json")
	target := hooks.Target{
		Path: targetPath,
		Key:  "crew",
		Role: "crew",
	}

	result, err := syncTarget(target, false)
	if err != nil {
		t.Fatalf("syncTarget failed: %v", err)
	}

	if result != syncCreated {
		t.Errorf("expected syncCreated, got %d", result)
	}

	// Verify the file was written
	if _, err := os.Stat(targetPath); err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}

	// Verify contents
	settings, err := hooks.LoadSettings(targetPath)
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}

	if len(settings.Hooks.SessionStart) != 1 {
		t.Errorf("expected 1 SessionStart hook, got %d", len(settings.Hooks.SessionStart))
	}
	if settings.Hooks.SessionStart[0].Hooks[0].Command != "echo hello" {
		t.Errorf("unexpected command: %s", settings.Hooks.SessionStart[0].Hooks[0].Command)
	}
}

func TestSyncTargetUpdatesExisting(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Save a base config
	base := &hooks.HooksConfig{
		SessionStart: []hooks.HookEntry{
			{Matcher: "", Hooks: []hooks.Hook{{Type: "command", Command: "new-command"}}},
		},
	}
	if err := hooks.SaveBase(base); err != nil {
		t.Fatalf("SaveBase failed: %v", err)
	}

	// Create existing settings.json with different hooks
	targetPath := filepath.Join(tmpDir, "test", ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		t.Fatal(err)
	}

	existing := hooks.SettingsJSON{
		EditorMode: "vim",
		Hooks: hooks.HooksConfig{
			SessionStart: []hooks.HookEntry{
				{Matcher: "", Hooks: []hooks.Hook{{Type: "command", Command: "old-command"}}},
			},
		},
	}
	data, marshalErr := hooks.MarshalSettings(&existing)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	target := hooks.Target{
		Path: targetPath,
		Key:  "crew",
		Role: "crew",
	}

	result, err := syncTarget(target, false)
	if err != nil {
		t.Fatalf("syncTarget failed: %v", err)
	}

	if result != syncUpdated {
		t.Errorf("expected syncUpdated, got %d", result)
	}

	// Verify the hooks were updated but editorMode preserved
	settings, err := hooks.LoadSettings(targetPath)
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}

	if settings.EditorMode != "vim" {
		t.Errorf("editorMode not preserved: got %q", settings.EditorMode)
	}
	if settings.Hooks.SessionStart[0].Hooks[0].Command != "new-command" {
		t.Errorf("hooks not updated: got %s", settings.Hooks.SessionStart[0].Hooks[0].Command)
	}
}

func TestSyncTargetUnchanged(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Save a base config
	base := &hooks.HooksConfig{
		SessionStart: []hooks.HookEntry{
			{Matcher: "", Hooks: []hooks.Hook{{Type: "command", Command: "same-command"}}},
		},
	}
	if err := hooks.SaveBase(base); err != nil {
		t.Fatalf("SaveBase failed: %v", err)
	}

	// Create existing settings.json with matching hooks
	targetPath := filepath.Join(tmpDir, "test", ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		t.Fatal(err)
	}

	// Compute expected config for crew to ensure existing matches
	expected, err := hooks.ComputeExpected("crew")
	if err != nil {
		t.Fatalf("ComputeExpected failed: %v", err)
	}
	existing := hooks.SettingsJSON{
		Hooks: *expected,
	}
	data, marshalErr := hooks.MarshalSettings(&existing)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	target := hooks.Target{
		Path: targetPath,
		Key:  "crew",
		Role: "crew",
	}

	result, err := syncTarget(target, false)
	if err != nil {
		t.Fatalf("syncTarget failed: %v", err)
	}

	if result != syncUnchanged {
		t.Errorf("expected syncUnchanged, got %d", result)
	}
}

func TestSyncTargetDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Save a base config
	base := &hooks.HooksConfig{
		SessionStart: []hooks.HookEntry{
			{Matcher: "", Hooks: []hooks.Hook{{Type: "command", Command: "test"}}},
		},
	}
	if err := hooks.SaveBase(base); err != nil {
		t.Fatalf("SaveBase failed: %v", err)
	}

	targetPath := filepath.Join(tmpDir, "test", ".claude", "settings.json")
	target := hooks.Target{
		Path: targetPath,
		Key:  "crew",
		Role: "crew",
	}

	// Dry run should not create the file
	result, err := syncTarget(target, true)
	if err != nil {
		t.Fatalf("syncTarget dry-run failed: %v", err)
	}

	if result != syncCreated {
		t.Errorf("expected syncCreated (dry-run), got %d", result)
	}

	// File should NOT exist
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Error("dry-run should not create file")
	}
}

func TestSyncTargetSetsEnabledPlugins(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	base := &hooks.HooksConfig{
		SessionStart: []hooks.HookEntry{
			{Matcher: "", Hooks: []hooks.Hook{{Type: "command", Command: "test"}}},
		},
	}
	if err := hooks.SaveBase(base); err != nil {
		t.Fatalf("SaveBase failed: %v", err)
	}

	targetPath := filepath.Join(tmpDir, "test", ".claude", "settings.json")
	target := hooks.Target{
		Path: targetPath,
		Key:  "crew",
		Role: "crew",
	}

	if _, err := syncTarget(target, false); err != nil {
		t.Fatalf("syncTarget failed: %v", err)
	}

	settings, err := hooks.LoadSettings(targetPath)
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}

	if settings.EnabledPlugins == nil {
		t.Fatal("enabledPlugins should be set")
	}
	if settings.EnabledPlugins["beads@beads-marketplace"] != false {
		t.Error("beads@beads-marketplace should be disabled")
	}
}

func TestRunHooksSyncFailsClosedOnIntegrityViolation(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	townRoot := filepath.Join(tmpDir, "town")
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor", ".claude"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, "deacon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte(`{"type":"town","version":1,"name":"test"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", ".claude", "settings.json"), []byte(`{"hooks":{"SessionStart":"bad"}}`), 0644); err != nil {
		t.Fatal(err)
	}

	base := &hooks.HooksConfig{
		SessionStart: []hooks.HookEntry{
			{Matcher: "", Hooks: []hooks.Hook{{Type: "command", Command: "echo hello"}}},
		},
	}
	if err := hooks.SaveBase(base); err != nil {
		t.Fatalf("SaveBase failed: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(cwd)
	}()
	if err := os.Chdir(townRoot); err != nil {
		t.Fatal(err)
	}

	hooksSyncDryRun = false
	err = runHooksSync(nil, nil)
	if err == nil {
		t.Fatal("expected hooks sync to fail closed")
	}
	if !strings.Contains(err.Error(), "failed closed") {
		t.Fatalf("expected fail-closed error, got: %v", err)
	}
}
