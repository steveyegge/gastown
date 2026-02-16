package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// setTestHome sets HOME (and USERPROFILE on Windows) so that
// os.UserHomeDir() returns tmpDir on all platforms.
func setTestHome(t *testing.T, tmpDir string) {
	t.Helper()
	t.Setenv("HOME", tmpDir)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", tmpDir)
	}
}

func TestLoadSaveBase(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	cfg := DefaultBase()

	if err := SaveBase(cfg); err != nil {
		t.Fatalf("SaveBase failed: %v", err)
	}

	if _, err := os.Stat(BasePath()); err != nil {
		t.Fatalf("base config file not created: %v", err)
	}

	loaded, err := LoadBase()
	if err != nil {
		t.Fatalf("LoadBase failed: %v", err)
	}

	if len(loaded.SessionStart) != 1 {
		t.Errorf("expected 1 SessionStart hook, got %d", len(loaded.SessionStart))
	}
	if len(loaded.PreCompact) != 1 {
		t.Errorf("expected 1 PreCompact hook, got %d", len(loaded.PreCompact))
	}
	if len(loaded.UserPromptSubmit) != 1 {
		t.Errorf("expected 1 UserPromptSubmit hook, got %d", len(loaded.UserPromptSubmit))
	}
	if len(loaded.Stop) != 1 {
		t.Errorf("expected 1 Stop hook, got %d", len(loaded.Stop))
	}
}

func TestLoadSaveOverride(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	cfg := &HooksConfig{
		PreToolUse: []HookEntry{
			{
				Matcher: "Bash(git push*)",
				Hooks:   []Hook{{Type: "command", Command: "echo blocked && exit 2"}},
			},
		},
	}

	if err := SaveOverride("crew", cfg); err != nil {
		t.Fatalf("SaveOverride failed: %v", err)
	}

	loaded, err := LoadOverride("crew")
	if err != nil {
		t.Fatalf("LoadOverride failed: %v", err)
	}

	if len(loaded.PreToolUse) != 1 {
		t.Fatalf("expected 1 PreToolUse hook, got %d", len(loaded.PreToolUse))
	}
	if loaded.PreToolUse[0].Matcher != "Bash(git push*)" {
		t.Errorf("expected matcher 'Bash(git push*)', got %q", loaded.PreToolUse[0].Matcher)
	}
}

func TestLoadSaveOverrideRigRole(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	cfg := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "echo gastown-crew"}}},
		},
	}

	if err := SaveOverride("gastown/crew", cfg); err != nil {
		t.Fatalf("SaveOverride failed: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, ".gt", "hooks-overrides", "gastown__crew.json")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected override file at %s: %v", expectedPath, err)
	}

	loaded, err := LoadOverride("gastown/crew")
	if err != nil {
		t.Fatalf("LoadOverride failed: %v", err)
	}

	if len(loaded.SessionStart) != 1 {
		t.Fatalf("expected 1 SessionStart hook, got %d", len(loaded.SessionStart))
	}
}

func TestLoadMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	_, err := LoadBase()
	if err == nil {
		t.Error("expected error loading missing base config")
	}

	_, err = LoadOverride("crew")
	if err == nil {
		t.Error("expected error loading missing override config")
	}
}

func TestValidTarget(t *testing.T) {
	tests := []struct {
		target string
		valid  bool
	}{
		{"crew", true},
		{"witness", true},
		{"refinery", true},
		{"polecats", true},
		{"polecat", true},
		{"mayor", true},
		{"deacon", true},
		{"rig", false},
		{"gastown/rig", false},
		{"gastown/crew", true},
		{"beads/witness", true},
		{"sky/polecats", true},
		{"wyvern/refinery", true},
		{"", false},
		{"invalid", false},
		{"gastown/invalid", false},
		{"/crew", false},
		{"gastown/", false},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			if got := ValidTarget(tt.target); got != tt.valid {
				t.Errorf("ValidTarget(%q) = %v, want %v", tt.target, got, tt.valid)
			}
		})
	}
}

func TestNormalizeTarget(t *testing.T) {
	tests := []struct {
		input      string
		normalized string
		valid      bool
	}{
		{"crew", "crew", true},
		{"polecats", "polecats", true},
		{"polecat", "polecats", true},
		{"gastown/polecats", "gastown/polecats", true},
		{"gastown/polecat", "gastown/polecats", true},
		{"mayor", "mayor", true},
		{"invalid", "", false},
		{"gastown/invalid", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := NormalizeTarget(tt.input)
			if ok != tt.valid {
				t.Errorf("NormalizeTarget(%q) valid = %v, want %v", tt.input, ok, tt.valid)
			}
			if got != tt.normalized {
				t.Errorf("NormalizeTarget(%q) = %q, want %q", tt.input, got, tt.normalized)
			}
		})
	}
}

func TestGetApplicableOverrides(t *testing.T) {
	tests := []struct {
		target   string
		expected []string
	}{
		{"mayor", []string{"mayor"}},
		{"crew", []string{"crew"}},
		{"gastown/crew", []string{"crew", "gastown/crew"}},
		{"beads/witness", []string{"witness", "beads/witness"}},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got := GetApplicableOverrides(tt.target)
			if len(got) != len(tt.expected) {
				t.Fatalf("GetApplicableOverrides(%q) returned %d items, want %d", tt.target, len(got), len(tt.expected))
			}
			for i, v := range got {
				if v != tt.expected[i] {
					t.Errorf("GetApplicableOverrides(%q)[%d] = %q, want %q", tt.target, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestDefaultBase(t *testing.T) {
	cfg := DefaultBase()

	if len(cfg.SessionStart) == 0 {
		t.Error("DefaultBase should have SessionStart hooks")
	}
	if len(cfg.PreCompact) == 0 {
		t.Error("DefaultBase should have PreCompact hooks")
	}
	if len(cfg.UserPromptSubmit) == 0 {
		t.Error("DefaultBase should have UserPromptSubmit hooks")
	}
	if len(cfg.Stop) == 0 {
		t.Error("DefaultBase should have Stop hooks")
	}

	found := false
	for _, entry := range cfg.SessionStart {
		for _, h := range entry.Hooks {
			if h.Command != "" {
				found = true
			}
		}
	}
	if !found {
		t.Error("DefaultBase SessionStart should have a command")
	}
}

func TestMerge(t *testing.T) {
	base := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "base-session"}}},
		},
		Stop: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "base-stop"}}},
		},
	}

	override := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "override-session"}}},
		},
		PreToolUse: []HookEntry{
			{Matcher: "Bash(git*)", Hooks: []Hook{{Type: "command", Command: "block-git"}}},
		},
	}

	result := Merge(base, override)

	if len(result.SessionStart) != 1 || result.SessionStart[0].Hooks[0].Command != "override-session" {
		t.Errorf("expected override SessionStart, got %v", result.SessionStart)
	}
	if len(result.Stop) != 1 || result.Stop[0].Hooks[0].Command != "base-stop" {
		t.Errorf("expected base Stop, got %v", result.Stop)
	}
	if len(result.PreToolUse) != 1 || result.PreToolUse[0].Matcher != "Bash(git*)" {
		t.Errorf("expected override PreToolUse, got %v", result.PreToolUse)
	}
	if len(base.PreToolUse) != 0 {
		t.Error("Merge mutated the original base config")
	}
}

// TestMergePerMatcherPreservation is the exact bug scenario from the spec:
// base has PreToolUse with matchers ["Bash(git*)", "Bash(rm*)"], override has
// PreToolUse with matcher ["Bash(git*)"]. The "Bash(rm*)" matcher must be preserved.
func TestMergePerMatcherPreservation(t *testing.T) {
	base := &HooksConfig{
		PreToolUse: []HookEntry{
			{Matcher: "Bash(git*)", Hooks: []Hook{{Type: "command", Command: "git-guard"}}},
			{Matcher: "Bash(rm*)", Hooks: []Hook{{Type: "command", Command: "rm-guard"}}},
		},
	}
	override := &HooksConfig{
		PreToolUse: []HookEntry{
			{Matcher: "Bash(git*)", Hooks: []Hook{{Type: "command", Command: "crew-git-guard"}}},
		},
	}

	result := Merge(base, override)

	if len(result.PreToolUse) != 2 {
		t.Fatalf("expected 2 PreToolUse entries (per-matcher merge), got %d", len(result.PreToolUse))
	}

	// Bash(git*) should be replaced by override
	if result.PreToolUse[0].Matcher != "Bash(git*)" {
		t.Errorf("expected first matcher Bash(git*), got %q", result.PreToolUse[0].Matcher)
	}
	if result.PreToolUse[0].Hooks[0].Command != "crew-git-guard" {
		t.Errorf("expected override command for Bash(git*), got %q", result.PreToolUse[0].Hooks[0].Command)
	}

	// Bash(rm*) should be preserved from base
	if result.PreToolUse[1].Matcher != "Bash(rm*)" {
		t.Errorf("expected second matcher Bash(rm*), got %q", result.PreToolUse[1].Matcher)
	}
	if result.PreToolUse[1].Hooks[0].Command != "rm-guard" {
		t.Errorf("expected base command for Bash(rm*), got %q", result.PreToolUse[1].Hooks[0].Command)
	}
}

func TestMergeDifferentMatchersBothIncluded(t *testing.T) {
	base := &HooksConfig{
		PreToolUse: []HookEntry{
			{Matcher: "Write", Hooks: []Hook{{Type: "command", Command: "write-check"}}},
		},
	}
	override := &HooksConfig{
		PreToolUse: []HookEntry{
			{Matcher: "Bash", Hooks: []Hook{{Type: "command", Command: "bash-check"}}},
		},
	}

	result := Merge(base, override)

	if len(result.PreToolUse) != 2 {
		t.Fatalf("expected 2 PreToolUse entries, got %d", len(result.PreToolUse))
	}
	if result.PreToolUse[0].Matcher != "Write" {
		t.Errorf("expected base Write matcher first, got %q", result.PreToolUse[0].Matcher)
	}
	if result.PreToolUse[1].Matcher != "Bash" {
		t.Errorf("expected override Bash matcher second, got %q", result.PreToolUse[1].Matcher)
	}
}

func TestMergeExplicitDisable(t *testing.T) {
	base := &HooksConfig{
		PreToolUse: []HookEntry{
			{Matcher: "Write", Hooks: []Hook{{Type: "command", Command: "write-check"}}},
			{Matcher: "Bash", Hooks: []Hook{{Type: "command", Command: "bash-check"}}},
		},
	}
	override := &HooksConfig{
		PreToolUse: []HookEntry{
			{Matcher: "Write", Hooks: []Hook{}}, // Explicit disable
		},
	}

	result := Merge(base, override)

	if len(result.PreToolUse) != 1 {
		t.Fatalf("expected 1 PreToolUse entry after disable, got %d", len(result.PreToolUse))
	}
	if result.PreToolUse[0].Matcher != "Bash" {
		t.Errorf("expected Bash matcher to remain, got %q", result.PreToolUse[0].Matcher)
	}
}

func TestMergeEmptyOverride(t *testing.T) {
	base := DefaultBase()
	override := &HooksConfig{}

	result := Merge(base, override)

	if !HooksEqual(base, result) {
		t.Error("empty override should not change base config")
	}
}

func TestComputeExpected(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	base := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "base-cmd"}}},
		},
	}
	if err := SaveBase(base); err != nil {
		t.Fatalf("SaveBase failed: %v", err)
	}

	crewOverride := &HooksConfig{
		PreToolUse: []HookEntry{
			{Matcher: "Bash(git*)", Hooks: []Hook{{Type: "command", Command: "crew-guard"}}},
		},
	}
	if err := SaveOverride("crew", crewOverride); err != nil {
		t.Fatalf("SaveOverride crew failed: %v", err)
	}

	gcOverride := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "gastown-crew-session"}}},
		},
	}
	if err := SaveOverride("gastown/crew", gcOverride); err != nil {
		t.Fatalf("SaveOverride gastown/crew failed: %v", err)
	}

	expected, err := ComputeExpected("gastown/crew")
	if err != nil {
		t.Fatalf("ComputeExpected failed: %v", err)
	}

	if len(expected.SessionStart) != 1 || expected.SessionStart[0].Hooks[0].Command != "gastown-crew-session" {
		t.Errorf("expected gastown/crew SessionStart, got %v", expected.SessionStart)
	}
	if len(expected.PreToolUse) != 1 || expected.PreToolUse[0].Hooks[0].Command != "crew-guard" {
		t.Errorf("expected crew PreToolUse, got %v", expected.PreToolUse)
	}
}

func TestComputeExpectedNoBase(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	// Mayor should get DefaultBase + built-in mayor override (task-dispatch guard)
	expected, err := ComputeExpected("mayor")
	if err != nil {
		t.Fatalf("ComputeExpected failed: %v", err)
	}

	defaultBase := DefaultBase()
	mayorDefaults := DefaultOverrides()["mayor"]
	merged := Merge(defaultBase, mayorDefaults)
	if !HooksEqual(expected, merged) {
		t.Error("expected DefaultBase + mayor default override when no configs exist")
	}

	// Non-mayor target should still just get DefaultBase
	crew, err := ComputeExpected("crew")
	if err != nil {
		t.Fatalf("ComputeExpected(crew) failed: %v", err)
	}
	if !HooksEqual(crew, defaultBase) {
		t.Error("expected DefaultBase for crew when no configs exist")
	}
}

// TestComputeExpectedBuiltinPlusOnDisk verifies that on-disk overrides layer
// on top of built-in defaults rather than replacing them.
func TestComputeExpectedBuiltinPlusOnDisk(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	// Save an on-disk mayor override that adds a custom SessionStart hook
	customOverride := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "custom-mayor-session"}}},
		},
	}
	if err := SaveOverride("mayor", customOverride); err != nil {
		t.Fatalf("SaveOverride failed: %v", err)
	}

	expected, err := ComputeExpected("mayor")
	if err != nil {
		t.Fatalf("ComputeExpected failed: %v", err)
	}

	// Should have the built-in Task guard from DefaultOverrides
	foundTaskGuard := false
	for _, entry := range expected.PreToolUse {
		if entry.Matcher == "Task" {
			foundTaskGuard = true
			break
		}
	}
	if !foundTaskGuard {
		t.Error("built-in Task guard should be present even with on-disk mayor override")
	}

	// Should also have the custom SessionStart from on-disk override
	if len(expected.SessionStart) == 0 {
		t.Error("on-disk SessionStart override should be present")
	} else if expected.SessionStart[0].Hooks[0].Command != "custom-mayor-session" {
		t.Errorf("expected custom-mayor-session, got %q", expected.SessionStart[0].Hooks[0].Command)
	}
}

// TestComputeExpectedOnDiskReplacesBuiltin verifies that an on-disk override
// with a conflicting "Task" matcher replaces the built-in guard command.
func TestComputeExpectedOnDiskReplacesBuiltin(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	// On-disk override replaces the Task matcher with a custom command
	customOverride := &HooksConfig{
		PreToolUse: []HookEntry{
			{Matcher: "Task", Hooks: []Hook{{Type: "command", Command: "custom-task-guard"}}},
		},
	}
	if err := SaveOverride("mayor", customOverride); err != nil {
		t.Fatalf("SaveOverride failed: %v", err)
	}

	expected, err := ComputeExpected("mayor")
	if err != nil {
		t.Fatalf("ComputeExpected failed: %v", err)
	}

	// The on-disk Task entry should replace the built-in one
	foundCustom := false
	for _, entry := range expected.PreToolUse {
		if entry.Matcher == "Task" {
			if len(entry.Hooks) == 1 && entry.Hooks[0].Command == "custom-task-guard" {
				foundCustom = true
			} else {
				t.Errorf("Task matcher should have custom command, got %v", entry.Hooks)
			}
			break
		}
	}
	if !foundCustom {
		t.Error("on-disk Task override should replace built-in guard")
	}
}

// TestComputeExpectedOnDiskDisablesBuiltin verifies that an on-disk override
// with an empty Hooks list for "Task" disables the built-in guard entirely.
func TestComputeExpectedOnDiskDisablesBuiltin(t *testing.T) {
	tmpDir := t.TempDir()
	setTestHome(t, tmpDir)

	// On-disk override disables the Task guard with empty hooks
	disableOverride := &HooksConfig{
		PreToolUse: []HookEntry{
			{Matcher: "Task", Hooks: []Hook{}},
		},
	}
	if err := SaveOverride("mayor", disableOverride); err != nil {
		t.Fatalf("SaveOverride failed: %v", err)
	}

	expected, err := ComputeExpected("mayor")
	if err != nil {
		t.Fatalf("ComputeExpected failed: %v", err)
	}

	// The Task entry should be removed (empty Hooks = explicit disable)
	for _, entry := range expected.PreToolUse {
		if entry.Matcher == "Task" {
			t.Error("Task matcher should be removed when on-disk override has empty Hooks")
		}
	}
}

func TestHooksEqual(t *testing.T) {
	a := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "test"}}},
		},
	}
	b := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "test"}}},
		},
	}
	c := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "different"}}},
		},
	}

	if !HooksEqual(a, b) {
		t.Error("identical configs should be equal")
	}
	if HooksEqual(a, c) {
		t.Error("different configs should not be equal")
	}
	if !HooksEqual(&HooksConfig{}, &HooksConfig{}) {
		t.Error("empty configs should be equal")
	}
}

func TestLoadSettings(t *testing.T) {
	tmpDir := t.TempDir()

	// Write raw JSON to test LoadSettings (SettingsJSON uses json:"-" tags)
	settingsJSON := `{
  "editorMode": "vim",
  "hooks": {
    "SessionStart": [
      {"matcher": "", "hooks": [{"type": "command", "command": "test"}]}
    ]
  }
}`
	path := filepath.Join(tmpDir, "settings.json")
	if err := os.WriteFile(path, []byte(settingsJSON), 0644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	loaded, err := LoadSettings(path)
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if loaded.EditorMode != "vim" {
		t.Errorf("expected editorMode vim, got %q", loaded.EditorMode)
	}
	if len(loaded.Hooks.SessionStart) != 1 {
		t.Errorf("expected 1 SessionStart hook, got %d", len(loaded.Hooks.SessionStart))
	}

	// Test loading non-existent file (should return zero-value)
	missing, err := LoadSettings(filepath.Join(tmpDir, "missing.json"))
	if err != nil {
		t.Fatalf("LoadSettings missing file failed: %v", err)
	}
	if missing.EditorMode != "" || len(missing.Hooks.SessionStart) != 0 {
		t.Error("missing file should return zero-value SettingsJSON")
	}
}

func TestDiscoverTargets(t *testing.T) {
	tmpDir := t.TempDir()

	os.MkdirAll(filepath.Join(tmpDir, "mayor"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "deacon"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "testrig", "crew", "alice"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "testrig", "crew", "bob"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "testrig", "witness"), 0755)

	targets, err := DiscoverTargets(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverTargets failed: %v", err)
	}

	if len(targets) < 4 {
		t.Errorf("expected at least 4 targets, got %d", len(targets))
		for _, tgt := range targets {
			t.Logf("  target: %s (key=%s)", tgt.DisplayKey(), tgt.Key)
		}
	}

	found := make(map[string]bool)
	for _, tgt := range targets {
		found[tgt.DisplayKey()] = true
	}

	for _, expected := range []string{"mayor", "deacon", "testrig/crew", "testrig/witness"} {
		if !found[expected] {
			t.Errorf("expected target %q not found", expected)
		}
	}
}

func TestDiscoverTargets_RoleNames(t *testing.T) {
	tmpDir := t.TempDir()

	os.MkdirAll(filepath.Join(tmpDir, "mayor"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "deacon"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "rig1", "crew", "alice"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "rig1", "polecats", "toast"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "rig1", "witness"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "rig1", "refinery"), 0755)

	targets, err := DiscoverTargets(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverTargets failed: %v", err)
	}

	// Verify Role field uses singular form (matching RoleSettingsDir conventions)
	roleByKey := make(map[string]string)
	for _, tgt := range targets {
		roleByKey[tgt.Key] = tgt.Role
	}

	expected := map[string]string{
		"mayor":         "mayor",
		"deacon":        "deacon",
		"rig1/crew":     "crew",
		"rig1/polecats": "polecat",
		"rig1/witness":  "witness",
		"rig1/refinery": "refinery",
	}

	for key, wantRole := range expected {
		gotRole, ok := roleByKey[key]
		if !ok {
			t.Errorf("target %q not found", key)
			continue
		}
		if gotRole != wantRole {
			t.Errorf("target %q: Role = %q, want %q", key, gotRole, wantRole)
		}
	}
}

func TestTargetDisplayKey(t *testing.T) {
	tests := []struct {
		target   Target
		expected string
	}{
		{Target{Key: "mayor", Role: "mayor"}, "mayor"},
		{Target{Key: "gastown/crew", Rig: "gastown", Role: "crew"}, "gastown/crew"},
		{Target{Key: "beads/witness", Rig: "beads", Role: "witness"}, "beads/witness"},
	}

	for _, tt := range tests {
		if got := tt.target.DisplayKey(); got != tt.expected {
			t.Errorf("DisplayKey() = %q, want %q", got, tt.expected)
		}
	}
}

func TestGetSetEntries(t *testing.T) {
	cfg := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "test"}}},
		},
	}

	entries := cfg.GetEntries("SessionStart")
	if len(entries) != 1 {
		t.Errorf("expected 1 SessionStart entry, got %d", len(entries))
	}

	entries = cfg.GetEntries("PreToolUse")
	if len(entries) != 0 {
		t.Errorf("expected 0 PreToolUse entries, got %d", len(entries))
	}

	entries = cfg.GetEntries("Unknown")
	if entries != nil {
		t.Errorf("expected nil for unknown event type, got %v", entries)
	}

	cfg.SetEntries("PreToolUse", []HookEntry{
		{Matcher: "Bash(*)", Hooks: []Hook{{Type: "command", Command: "guard"}}},
	})
	if len(cfg.PreToolUse) != 1 {
		t.Errorf("expected 1 PreToolUse entry after SetEntries, got %d", len(cfg.PreToolUse))
	}
}

func TestToMap(t *testing.T) {
	cfg := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "start"}}},
		},
		Stop: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "stop"}}},
		},
	}

	m := cfg.ToMap()
	if len(m) != 2 {
		t.Errorf("expected 2 entries in map, got %d", len(m))
	}
	if _, ok := m["SessionStart"]; !ok {
		t.Error("expected SessionStart in map")
	}
	if _, ok := m["Stop"]; !ok {
		t.Error("expected Stop in map")
	}
	if _, ok := m["PreToolUse"]; ok {
		t.Error("empty PreToolUse should not be in map")
	}
}

func TestAddEntry(t *testing.T) {
	cfg := &HooksConfig{}

	added := cfg.AddEntry("PreToolUse", HookEntry{
		Matcher: "Bash(git*)",
		Hooks:   []Hook{{Type: "command", Command: "guard"}},
	})
	if !added {
		t.Error("expected first entry to be added")
	}
	if len(cfg.PreToolUse) != 1 {
		t.Errorf("expected 1 PreToolUse entry, got %d", len(cfg.PreToolUse))
	}

	added = cfg.AddEntry("PreToolUse", HookEntry{
		Matcher: "Bash(git*)",
		Hooks:   []Hook{{Type: "command", Command: "different"}},
	})
	if added {
		t.Error("expected duplicate matcher to not be added")
	}
	if len(cfg.PreToolUse) != 1 {
		t.Errorf("expected still 1 PreToolUse entry, got %d", len(cfg.PreToolUse))
	}

	added = cfg.AddEntry("PreToolUse", HookEntry{
		Matcher: "Bash(rm*)",
		Hooks:   []Hook{{Type: "command", Command: "block"}},
	})
	if !added {
		t.Error("expected new matcher to be added")
	}
	if len(cfg.PreToolUse) != 2 {
		t.Errorf("expected 2 PreToolUse entries, got %d", len(cfg.PreToolUse))
	}
}

func TestMarshalConfig(t *testing.T) {
	cfg := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "test"}}},
		},
	}

	data, err := MarshalConfig(cfg)
	if err != nil {
		t.Fatalf("MarshalConfig failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("MarshalConfig returned empty data")
	}

	loaded := &HooksConfig{}
	if err := json.Unmarshal(data, loaded); err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}

	if len(loaded.SessionStart) != 1 {
		t.Errorf("round-trip lost SessionStart hooks")
	}
}
