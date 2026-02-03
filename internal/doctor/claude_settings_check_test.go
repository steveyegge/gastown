package doctor

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/claude"
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

// createValidSettings creates a valid settings.json with all required elements.
// It uses the actual embedded template to avoid template drift detection.
func createValidSettings(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}

	// Use actual template content to avoid template drift
	// For test purposes, use autonomous template (most common)
	content, err := claude.TemplateContent(claude.Autonomous)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
}

// createValidSettingsWithRole creates settings using the appropriate template for the role.
func createValidSettingsWithRole(t *testing.T, path, role string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}

	roleType := claude.RoleTypeFor(role)
	content, err := claude.TemplateContent(roleType)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, content, 0644); err != nil {
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
						map[string]any{
							"type":    "command",
							"command": "gt nudge deacon session-started",
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
							"command": "_stdin=$(cat) && echo \"$_stdin\" | gt decision turn-check",
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
		case "deacon-nudge":
			// Remove deacon nudge from SessionStart hooks
			hooks := settings["hooks"].(map[string]any)
			sessionStart := hooks["SessionStart"].([]any)
			hookObj := sessionStart[0].(map[string]any)
			innerHooks := hookObj["hooks"].([]any)
			// Filter out deacon nudge
			var filtered []any
			for _, h := range innerHooks {
				hMap := h.(map[string]any)
				if cmd, ok := hMap["command"].(string); ok && !strings.Contains(cmd, "gt nudge deacon") {
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

	// Create valid mayor settings at correct location (mayor/.claude/settings.json)
	// NOT at town root (.claude/settings.json) which is wrong location
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.json")
	createValidSettingsWithRole(t, mayorSettings, "mayor")

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for valid settings, got %v: %s", result.Status, result.Message)
	}
}

func TestClaudeSettingsCheck_ValidDeaconSettings(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid deacon settings
	deaconSettings := filepath.Join(tmpDir, "deacon", ".claude", "settings.json")
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

	// Create valid witness settings in correct location (witness/.claude/, outside git repo)
	witnessSettings := filepath.Join(tmpDir, rigName, "witness", ".claude", "settings.json")
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

	// Create valid refinery settings in correct location (refinery/.claude/, outside git repo)
	refinerySettings := filepath.Join(tmpDir, rigName, "refinery", ".claude", "settings.json")
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

	// Create valid crew settings in correct location (crew/.claude/, shared by all crew)
	crewSettings := filepath.Join(tmpDir, rigName, "crew", ".claude", "settings.json")
	createValidSettingsWithRole(t, crewSettings, "crew")

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

	// Create valid polecat settings in correct location (polecats/.claude/, shared by all polecats)
	pcSettings := filepath.Join(tmpDir, rigName, "polecats", ".claude", "settings.json")
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

	// Create stale mayor settings missing enabledPlugins (at correct location)
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.json")
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

	// Create stale settings missing hooks entirely (at correct location)
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.json")
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

	// Create stale settings missing PATH export (at correct location).
	// Note: The settings checker doesn't specifically validate PATH export,
	// but these stale settings also lack injection hooks (turn-check, etc.)
	// which ARE validated. This test verifies stale settings are detected.
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.json")
	createStaleSettings(t, mayorSettings, "PATH")

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for stale settings, got %v", result.Status)
	}
	// Stale settings are missing several required hooks
	if len(result.Details) == 0 {
		t.Error("expected non-empty details for stale settings")
	}
}

func TestClaudeSettingsCheck_MissingDeaconNudge(t *testing.T) {
	tmpDir := t.TempDir()

	// Create stale settings missing deacon nudge (at correct location)
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.json")
	createStaleSettings(t, mayorSettings, "deacon-nudge")

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing deacon nudge, got %v", result.Status)
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "deacon nudge") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention deacon nudge, got %v", result.Details)
	}
}

func TestClaudeSettingsCheck_MissingStopHook(t *testing.T) {
	tmpDir := t.TempDir()

	// Create stale settings missing Stop hook (at correct location)
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.json")
	createStaleSettings(t, mayorSettings, "Stop")

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing Stop hook, got %v", result.Status)
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "turn-check hook") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention turn-check hook, got %v", result.Details)
	}
}

// TestClaudeSettingsCheck_MissingStdinPiping tests detection of turn-clear without stdin piping (gt-te4okj).
func TestClaudeSettingsCheck_MissingStdinPiping(t *testing.T) {
	tmpDir := t.TempDir()

	// Create settings with turn-clear but WITHOUT stdin piping
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.json")
	settings := map[string]any{
		"enabledPlugins": []string{"plugin1"},
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"matcher": "**",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "gt prime --hook && gt mail check --inject && gt nudge deacon session-started",
						},
					},
				},
			},
			"UserPromptSubmit": []any{
				map[string]any{
					"matcher": "**",
					"hooks": []any{
						map[string]any{
							"type": "command",
							// Missing stdin piping - this is the bug gt-te4okj
							"command": "bd decision check --inject && gt decision turn-clear",
						},
					},
				},
			},
			"PostToolUse": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "gt inject drain --quiet",
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
							"command": "_stdin=$(cat) && echo \"$_stdin\" | gt decision turn-check",
						},
					},
				},
			},
		},
	}

	if err := os.MkdirAll(filepath.Dir(mayorSettings), 0755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.MarshalIndent(settings, "", "  ")
	if err := os.WriteFile(mayorSettings, data, 0644); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for missing stdin piping, got %v", result.Status)
	}
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "turn-clear stdin piping") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention turn-clear stdin piping, got %v", result.Details)
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

	// Create multiple stale settings files (all at correct locations)
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.json")
	createStaleSettings(t, mayorSettings, "PATH")

	deaconSettings := filepath.Join(tmpDir, "deacon", ".claude", "settings.json")
	createStaleSettings(t, deaconSettings, "Stop")

	// Settings inside git repo (witness/rig/.claude/) are wrong location
	witnessWrong := filepath.Join(tmpDir, rigName, "witness", "rig", ".claude", "settings.json")
	createValidSettings(t, witnessWrong) // Valid content but wrong location

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for multiple stale files, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "3 stale") {
		t.Errorf("expected message about 3 stale files, got %q", result.Message)
	}
}

func TestClaudeSettingsCheck_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()

	// Create invalid JSON file (at correct location)
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.json")
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

	// Run to detect
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify file was deleted
	if _, err := os.Stat(wrongSettings); !os.IsNotExist(err) {
		t.Error("expected wrong location settings to be deleted")
	}

	// Verify check passes (no settings files means OK)
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after fix, got %v", result.Status)
	}
}

func TestClaudeSettingsCheck_SkipsNonRigDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create directories that should be skipped
	for _, skipDir := range []string{"mayor", "deacon", "daemon", ".git", "docs", ".hidden"} {
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

	// Should only find mayor and deacon settings in their specific locations
	// The witness settings in these dirs should be ignored
	// Since we didn't create valid mayor/deacon settings, those will be stale
	// But the ones in "mayor/witness/rig/.claude" should be ignored

	// Count how many stale files were found - should be 0 since none of the
	// skipped directories have their settings detected
	if len(check.staleSettings) != 0 {
		t.Errorf("expected 0 stale files (skipped dirs), got %d", len(check.staleSettings))
	}
}

func TestClaudeSettingsCheck_MixedValidAndStale(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create valid mayor settings (at correct location) - using actual template
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.json")
	createValidSettingsWithRole(t, mayorSettings, "mayor")

	// Create stale witness settings in correct location (missing PATH)
	witnessSettings := filepath.Join(tmpDir, rigName, "witness", ".claude", "settings.json")
	createStaleSettings(t, witnessSettings, "PATH")

	// Create valid refinery settings in correct location - using actual template
	refinerySettings := filepath.Join(tmpDir, rigName, "refinery", ".claude", "settings.json")
	createValidSettingsWithRole(t, refinerySettings, "refinery")

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

// TestClaudeSettingsCheck_CrewSymlinkNotFlagged verifies that when crew workers
// have .claude symlinks pointing to the shared crew/.claude directory, these
// are NOT flagged as wrong location. This tests the fix for hq-s2rip1.
func TestClaudeSettingsCheck_CrewSymlinkNotFlagged(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create the correct shared settings at crew/.claude/settings.json
	sharedClaudeDir := filepath.Join(tmpDir, rigName, "crew", ".claude")
	sharedSettings := filepath.Join(sharedClaudeDir, "settings.json")
	createValidSettings(t, sharedSettings)

	// Create crew worker directory with symlink to shared .claude
	workerDir := filepath.Join(tmpDir, rigName, "crew", "worker1")
	if err := os.MkdirAll(workerDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create symlink: crew/worker1/.claude -> ../.claude
	workerClaudeDir := filepath.Join(workerDir, ".claude")
	if err := os.Symlink("../.claude", workerClaudeDir); err != nil {
		t.Skipf("cannot create symlinks: %v", err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// Should NOT find any wrong location errors (only the shared settings exist)
	if result.Status == StatusError {
		for _, d := range result.Details {
			if strings.Contains(d, "wrong location") && strings.Contains(d, "worker1") {
				t.Errorf("crew worker symlink should NOT be flagged as wrong location: %s", d)
			}
		}
	}
}

// TestClaudeSettingsCheck_PolecatSymlinkNotFlagged verifies that when polecats
// have .claude symlinks pointing to the shared polecats/.claude directory, these
// are NOT flagged as wrong location.
func TestClaudeSettingsCheck_PolecatSymlinkNotFlagged(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create the correct shared settings at polecats/.claude/settings.json
	sharedClaudeDir := filepath.Join(tmpDir, rigName, "polecats", ".claude")
	sharedSettings := filepath.Join(sharedClaudeDir, "settings.json")
	createValidSettings(t, sharedSettings)

	// Create polecat directory with symlink to shared .claude
	polecatDir := filepath.Join(tmpDir, rigName, "polecats", "pc1")
	if err := os.MkdirAll(polecatDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create symlink: polecats/pc1/.claude -> ../.claude
	polecatClaudeDir := filepath.Join(polecatDir, ".claude")
	if err := os.Symlink("../.claude", polecatClaudeDir); err != nil {
		t.Skipf("cannot create symlinks: %v", err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// Should NOT find any wrong location errors (only the shared settings exist)
	if result.Status == StatusError {
		for _, d := range result.Details {
			if strings.Contains(d, "wrong location") && strings.Contains(d, "pc1") {
				t.Errorf("polecat symlink should NOT be flagged as wrong location: %s", d)
			}
		}
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
	wrongSettings := filepath.Join(rigDir, ".claude", "settings.json")
	createValidSettings(t, wrongSettings)
	gitAddAndCommit(t, rigDir, wrongSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for wrong location, got %v", result.Status)
	}
	// Should mention "tracked but unmodified"
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "tracked but unmodified") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention tracked but unmodified, got %v", result.Details)
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
	wrongSettings := filepath.Join(rigDir, ".claude", "settings.json")
	createValidSettings(t, wrongSettings)
	gitAddAndCommit(t, rigDir, wrongSettings)

	// Modify the file after commit
	if err := os.WriteFile(wrongSettings, []byte(`{"modified": true}`), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for wrong location, got %v", result.Status)
	}
	// Should mention "local modifications"
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "local modifications") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention local modifications, got %v", result.Details)
	}
	// Should mention .bak files for backup
	if !strings.Contains(result.FixHint, ".bak files") {
		t.Errorf("expected fix hint to mention .bak files, got %q", result.FixHint)
	}
}

func TestClaudeSettingsCheck_FixRenamesModifiedFiles(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create a git repo to simulate a source repo
	rigDir := filepath.Join(tmpDir, rigName, "witness", "rig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	initTestGitRepo(t, rigDir)

	// Create settings and commit it
	wrongSettings := filepath.Join(rigDir, ".claude", "settings.json")
	createValidSettings(t, wrongSettings)
	gitAddAndCommit(t, rigDir, wrongSettings)

	// Modify the file after commit
	if err := os.WriteFile(wrongSettings, []byte(`{"modified": true}`), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	// Run to detect
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix - should rename the modified file to a backup
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify original file was renamed (no longer exists)
	if _, err := os.Stat(wrongSettings); !os.IsNotExist(err) {
		t.Error("expected modified file to be renamed, but it still exists")
	}

	// Verify backup file exists (with .bak. prefix in name)
	claudeDir := filepath.Dir(wrongSettings)
	entries, err := os.ReadDir(claudeDir)
	if err != nil {
		t.Fatalf("failed to read .claude dir: %v", err)
	}

	foundBackup := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "settings.json.bak.") {
			foundBackup = true
			break
		}
	}
	if !foundBackup {
		t.Error("expected backup file to exist with .bak. extension")
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

func TestClaudeSettingsCheck_FixDeletesTrackedCleanFiles(t *testing.T) {
	tmpDir := t.TempDir()
	rigName := "testrig"

	// Create a git repo to simulate a source repo
	rigDir := filepath.Join(tmpDir, rigName, "witness", "rig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	initTestGitRepo(t, rigDir)

	// Create settings and commit it (tracked, clean)
	wrongSettings := filepath.Join(rigDir, ".claude", "settings.json")
	createValidSettings(t, wrongSettings)
	gitAddAndCommit(t, rigDir, wrongSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	// Run to detect
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix - should delete the tracked clean file
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify file was deleted
	if _, err := os.Stat(wrongSettings); !os.IsNotExist(err) {
		t.Error("expected tracked clean file to be deleted")
	}
}

func TestClaudeSettingsCheck_DetectsStaleCLAUDEmdAtTownRoot(t *testing.T) {
	tmpDir := t.TempDir()

	// Create CLAUDE.md at town root (wrong location)
	staleCLAUDEmd := filepath.Join(tmpDir, "CLAUDE.md")
	if err := os.WriteFile(staleCLAUDEmd, []byte("# Mayor Context\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("expected StatusError for stale CLAUDE.md at town root, got %v", result.Status)
	}

	// Should mention wrong location
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "CLAUDE.md") && strings.Contains(d, "wrong location") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention CLAUDE.md wrong location, got %v", result.Details)
	}
}

func TestClaudeSettingsCheck_FixMovesCLAUDEmdToMayor(t *testing.T) {
	tmpDir := t.TempDir()

	// Create mayor directory (needed for fix to create CLAUDE.md there)
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create CLAUDE.md at town root (wrong location)
	staleCLAUDEmd := filepath.Join(tmpDir, "CLAUDE.md")
	if err := os.WriteFile(staleCLAUDEmd, []byte("# Mayor Context\n"), 0644); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	// Run to detect
	result := check.Run(ctx)
	if result.Status != StatusError {
		t.Fatalf("expected StatusError before fix, got %v", result.Status)
	}

	// Apply fix
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify old file was deleted
	if _, err := os.Stat(staleCLAUDEmd); !os.IsNotExist(err) {
		t.Error("expected CLAUDE.md at town root to be deleted")
	}

	// Verify new file was created at mayor/
	correctCLAUDEmd := filepath.Join(mayorDir, "CLAUDE.md")
	if _, err := os.Stat(correctCLAUDEmd); os.IsNotExist(err) {
		t.Error("expected CLAUDE.md to be created at mayor/")
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

	// Verify a symlink was created from town root to mayor settings
	// (the fix now creates a symlink so mayor session can find its hooks)
	info, err := os.Lstat(staleTownRootSettings)
	if err != nil {
		t.Fatalf("expected symlink at town root, but got error: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected settings.json at town root to be a symlink, not a regular file")
	}

	// Verify symlink points to mayor settings
	target, err := os.Readlink(staleTownRootSettings)
	if err != nil {
		t.Fatalf("failed to read symlink target: %v", err)
	}
	expectedTarget := filepath.Join("..", "mayor", ".claude", "settings.json")
	if target != expectedTarget {
		t.Errorf("expected symlink target %q, got %q", expectedTarget, target)
	}
}

// TestClaudeSettingsCheck_TemplateDrift tests detection of settings that have valid
// patterns but differ from the current template (template drift).
func TestClaudeSettingsCheck_TemplateDrift(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid settings with all required patterns but with extra content
	// that makes it different from the template
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.json")
	settings := map[string]any{
		"enabledPlugins": []string{"plugin1"},
		"extraField":     "this makes the file different from template",
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"matcher": "**",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "export PATH=/usr/local/bin:$PATH",
						},
						map[string]any{
							"type":    "command",
							"command": "gt prime --hook && gt mail check --inject && gt nudge deacon session-started",
						},
					},
				},
			},
			"UserPromptSubmit": []any{
				map[string]any{
					"matcher": "**",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "_stdin=$(cat) && (echo \"$_stdin\" | bd decision check --inject) && (echo \"$_stdin\" | gt decision turn-clear)",
						},
					},
				},
			},
			"PostToolUse": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "gt inject drain --quiet && gt nudge drain --quiet",
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
							"command": "_stdin=$(cat) && echo \"$_stdin\" | gt decision turn-check",
						},
					},
				},
			},
		},
	}

	if err := os.MkdirAll(filepath.Dir(mayorSettings), 0755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.MarshalIndent(settings, "", "  ")
	if err := os.WriteFile(mayorSettings, data, 0644); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// Should detect template drift (patterns valid but file differs from template)
	if result.Status != StatusWarning {
		t.Errorf("expected StatusWarning for template drift, got %v: %s", result.Status, result.Message)
	}

	// Should mention template drift in details
	found := false
	for _, d := range result.Details {
		if strings.Contains(d, "template drift") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected details to mention template drift, got %v", result.Details)
	}
}

// TestClaudeSettingsCheck_TemplateDriftFixUpdatesFromTemplate verifies that --fix
// updates settings with template drift from the current template.
func TestClaudeSettingsCheck_TemplateDriftFixUpdatesFromTemplate(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid settings with drift (all patterns valid but extra content)
	mayorSettings := filepath.Join(tmpDir, "mayor", ".claude", "settings.json")
	settings := map[string]any{
		"enabledPlugins": []string{"plugin1"},
		"extraField":     "this makes the file different from template",
		"hooks": map[string]any{
			"SessionStart": []any{
				map[string]any{
					"matcher": "**",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "gt prime --hook && gt mail check --inject && gt nudge deacon session-started",
						},
					},
				},
			},
			"UserPromptSubmit": []any{
				map[string]any{
					"matcher": "**",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "_stdin=$(cat) && (echo \"$_stdin\" | bd decision check --inject) && (echo \"$_stdin\" | gt decision turn-clear)",
						},
					},
				},
			},
			"PostToolUse": []any{
				map[string]any{
					"matcher": "",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "gt inject drain --quiet",
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
							"command": "_stdin=$(cat) && echo \"$_stdin\" | gt decision turn-check",
						},
					},
				},
			},
		},
	}

	if err := os.MkdirAll(filepath.Dir(mayorSettings), 0755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.MarshalIndent(settings, "", "  ")
	if err := os.WriteFile(mayorSettings, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Get original file content
	originalContent, _ := os.ReadFile(mayorSettings)

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	// Run to detect drift
	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Fatalf("expected StatusWarning before fix, got %v: %s", result.Status, result.Message)
	}

	// Apply fix
	if err := check.Fix(ctx); err != nil {
		t.Fatalf("Fix failed: %v", err)
	}

	// Verify file was updated (content changed)
	newContent, err := os.ReadFile(mayorSettings)
	if err != nil {
		t.Fatalf("failed to read settings after fix: %v", err)
	}

	if string(newContent) == string(originalContent) {
		t.Error("expected settings content to change after fix")
	}

	// Run check again - should pass now
	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK after fix, got %v: %s", result.Status, result.Message)
	}
}

// TestClaudeSettingsCheck_NoTemplateDriftWhenFresh verifies that fresh settings
// (matching template exactly) don't trigger template drift warning.
func TestClaudeSettingsCheck_NoTemplateDriftWhenFresh(t *testing.T) {
	tmpDir := t.TempDir()

	// Create settings directory and let EnsureSettingsForRole create the file
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}

	// This creates settings from the template - should be fresh
	if err := claude.EnsureSettingsForRole(mayorDir, "mayor"); err != nil {
		t.Fatal(err)
	}

	check := NewClaudeSettingsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// Should NOT detect template drift for fresh template-created settings
	if result.Status != StatusOK {
		t.Errorf("expected StatusOK for fresh settings, got %v: %s", result.Status, result.Message)
	}
}
