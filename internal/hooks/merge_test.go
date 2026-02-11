package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeHooksNoOverrides(t *testing.T) {
	base := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "gt prime"}}},
		},
	}

	result := MergeHooks(base, nil, "crew")

	if len(result.SessionStart) != 1 {
		t.Fatalf("expected 1 SessionStart, got %d", len(result.SessionStart))
	}
	if result.SessionStart[0].Hooks[0].Command != "gt prime" {
		t.Errorf("expected 'gt prime', got %q", result.SessionStart[0].Hooks[0].Command)
	}
}

func TestMergeHooksNilBase(t *testing.T) {
	overrides := map[string]*HooksConfig{
		"crew": {
			PreToolUse: []HookEntry{
				{Matcher: "Bash(git push*)", Hooks: []Hook{{Type: "command", Command: "echo blocked"}}},
			},
		},
	}

	result := MergeHooks(nil, overrides, "crew")

	if len(result.PreToolUse) != 1 {
		t.Fatalf("expected 1 PreToolUse, got %d", len(result.PreToolUse))
	}
}

func TestMergeHooksRoleOverride(t *testing.T) {
	base := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "gt prime"}}},
		},
		Stop: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "gt costs record"}}},
		},
	}

	overrides := map[string]*HooksConfig{
		"crew": {
			PreToolUse: []HookEntry{
				{Matcher: "Bash(git push*)", Hooks: []Hook{{Type: "command", Command: "echo blocked && exit 2"}}},
			},
		},
	}

	result := MergeHooks(base, overrides, "crew")

	// Base hooks should be preserved
	if len(result.SessionStart) != 1 {
		t.Errorf("expected 1 SessionStart, got %d", len(result.SessionStart))
	}
	if len(result.Stop) != 1 {
		t.Errorf("expected 1 Stop, got %d", len(result.Stop))
	}
	// Override should be added
	if len(result.PreToolUse) != 1 {
		t.Fatalf("expected 1 PreToolUse, got %d", len(result.PreToolUse))
	}
	if result.PreToolUse[0].Matcher != "Bash(git push*)" {
		t.Errorf("unexpected matcher: %q", result.PreToolUse[0].Matcher)
	}
}

func TestMergeHooksSameMatcherReplaces(t *testing.T) {
	base := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "gt prime --old"}}},
		},
	}

	overrides := map[string]*HooksConfig{
		"crew": {
			SessionStart: []HookEntry{
				{Matcher: "", Hooks: []Hook{{Type: "command", Command: "gt prime --new"}}},
			},
		},
	}

	result := MergeHooks(base, overrides, "crew")

	if len(result.SessionStart) != 1 {
		t.Fatalf("expected 1 SessionStart (replaced), got %d", len(result.SessionStart))
	}
	if result.SessionStart[0].Hooks[0].Command != "gt prime --new" {
		t.Errorf("expected override command, got %q", result.SessionStart[0].Hooks[0].Command)
	}
}

func TestMergeHooksDifferentMatcherAppends(t *testing.T) {
	base := &HooksConfig{
		PreToolUse: []HookEntry{
			{Matcher: "Bash(git push*)", Hooks: []Hook{{Type: "command", Command: "block-push"}}},
		},
	}

	overrides := map[string]*HooksConfig{
		"crew": {
			PreToolUse: []HookEntry{
				{Matcher: "Bash(rm -rf*)", Hooks: []Hook{{Type: "command", Command: "block-rm"}}},
			},
		},
	}

	result := MergeHooks(base, overrides, "crew")

	if len(result.PreToolUse) != 2 {
		t.Fatalf("expected 2 PreToolUse (base + override), got %d", len(result.PreToolUse))
	}
}

func TestMergeHooksEmptyHooksDisables(t *testing.T) {
	base := &HooksConfig{
		Stop: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "gt costs record"}}},
		},
	}

	overrides := map[string]*HooksConfig{
		"mayor": {
			Stop: []HookEntry{
				{Matcher: "", Hooks: []Hook{}}, // Explicit disable
			},
		},
	}

	result := MergeHooks(base, overrides, "mayor")

	if len(result.Stop) != 0 {
		t.Errorf("expected 0 Stop hooks (disabled), got %d", len(result.Stop))
	}
}

func TestMergeHooksRigRoleLayering(t *testing.T) {
	base := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "base-prime"}}},
		},
	}

	overrides := map[string]*HooksConfig{
		"crew": {
			SessionStart: []HookEntry{
				{Matcher: "", Hooks: []Hook{{Type: "command", Command: "crew-prime"}}},
			},
		},
		"gastown/crew": {
			SessionStart: []HookEntry{
				{Matcher: "", Hooks: []Hook{{Type: "command", Command: "gastown-crew-prime"}}},
			},
		},
	}

	result := MergeHooks(base, overrides, "gastown/crew")

	// rig+role override should win (applied last)
	if len(result.SessionStart) != 1 {
		t.Fatalf("expected 1 SessionStart, got %d", len(result.SessionStart))
	}
	if result.SessionStart[0].Hooks[0].Command != "gastown-crew-prime" {
		t.Errorf("expected rig+role override, got %q", result.SessionStart[0].Hooks[0].Command)
	}
}

func TestMergeHooksDoesNotMutateBase(t *testing.T) {
	base := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "original"}}},
		},
	}

	overrides := map[string]*HooksConfig{
		"crew": {
			SessionStart: []HookEntry{
				{Matcher: "", Hooks: []Hook{{Type: "command", Command: "modified"}}},
			},
		},
	}

	MergeHooks(base, overrides, "crew")

	// Base should be unchanged
	if base.SessionStart[0].Hooks[0].Command != "original" {
		t.Errorf("base was mutated: got %q", base.SessionStart[0].Hooks[0].Command)
	}
}

func TestMergeHooksOverrideAddsNewType(t *testing.T) {
	base := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "gt prime"}}},
		},
	}

	overrides := map[string]*HooksConfig{
		"crew": {
			PreToolUse: []HookEntry{
				{Matcher: "Bash(git push*)", Hooks: []Hook{{Type: "command", Command: "block"}}},
			},
		},
	}

	result := MergeHooks(base, overrides, "crew")

	if len(result.SessionStart) != 1 {
		t.Errorf("expected base SessionStart preserved")
	}
	if len(result.PreToolUse) != 1 {
		t.Errorf("expected override PreToolUse added")
	}
}

func TestLoadAllOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	// Create some override files
	crew := &HooksConfig{
		PreToolUse: []HookEntry{
			{Matcher: "Bash(git push*)", Hooks: []Hook{{Type: "command", Command: "block"}}},
		},
	}
	if err := SaveOverride("crew", crew); err != nil {
		t.Fatalf("SaveOverride crew: %v", err)
	}

	gasCrewOverride := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "gastown-prime"}}},
		},
	}
	if err := SaveOverride("gastown/crew", gasCrewOverride); err != nil {
		t.Fatalf("SaveOverride gastown/crew: %v", err)
	}

	overrides, err := LoadAllOverrides()
	if err != nil {
		t.Fatalf("LoadAllOverrides: %v", err)
	}

	if len(overrides) != 2 {
		t.Fatalf("expected 2 overrides, got %d", len(overrides))
	}

	if _, ok := overrides["crew"]; !ok {
		t.Error("missing 'crew' override")
	}
	if _, ok := overrides["gastown/crew"]; !ok {
		t.Error("missing 'gastown/crew' override")
	}
}

func TestLoadAllOverridesEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	overrides, err := LoadAllOverrides()
	if err != nil {
		t.Fatalf("LoadAllOverrides on empty dir: %v", err)
	}

	if len(overrides) != 0 {
		t.Errorf("expected 0 overrides, got %d", len(overrides))
	}
}

func TestLoadAllOverridesSkipsInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	// Create a valid override first
	crew := &HooksConfig{
		PreToolUse: []HookEntry{
			{Matcher: "Bash(git push*)", Hooks: []Hook{{Type: "command", Command: "block"}}},
		},
	}
	if err := SaveOverride("crew", crew); err != nil {
		t.Fatalf("SaveOverride crew: %v", err)
	}

	// Write an invalid JSON file directly into overrides dir
	invalidPath := filepath.Join(OverridesDir(), "polecats.json")
	if err := os.WriteFile(invalidPath, []byte("{invalid json!!}"), 0644); err != nil {
		t.Fatalf("writing invalid file: %v", err)
	}

	overrides, err := LoadAllOverrides()
	if err != nil {
		t.Fatalf("LoadAllOverrides should not return error for invalid JSON: %v", err)
	}

	// Valid override should still load
	if _, ok := overrides["crew"]; !ok {
		t.Error("missing 'crew' override — valid overrides should still load")
	}

	// Invalid file should be skipped (not present in map)
	if _, ok := overrides["polecats"]; ok {
		t.Error("invalid 'polecats' override should have been skipped")
	}
}

func TestLoadAllOverridesReturnsReadDirError(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	// Create the overrides dir as a file (not a directory) to force a ReadDir error
	overridesDir := OverridesDir()
	if err := os.MkdirAll(filepath.Dir(overridesDir), 0755); err != nil {
		t.Fatalf("creating parent dir: %v", err)
	}
	if err := os.WriteFile(overridesDir, []byte("not a directory"), 0644); err != nil {
		t.Fatalf("writing file at overrides path: %v", err)
	}

	_, err := LoadAllOverrides()
	if err == nil {
		t.Fatal("expected error when overrides dir is not a directory")
	}
}
