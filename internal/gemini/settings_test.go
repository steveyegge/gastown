package gemini

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRoleTypeFor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		role string
		want RoleType
	}{
		{"polecat", Autonomous},
		{"witness", Autonomous},
		{"refinery", Autonomous},
		{"deacon", Autonomous},
		{"boot", Autonomous},
		{"mayor", Interactive},
		{"crew", Interactive},
		{"unknown", Interactive},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			if got := RoleTypeFor(tt.role); got != tt.want {
				t.Errorf("RoleTypeFor(%q) = %q, want %q", tt.role, got, tt.want)
			}
		})
	}
}

func TestEnsureSettingsAt_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()

	settingsDir := ".gemini"
	settingsFile := "settings.json"
	settingsPath := filepath.Join(tmpDir, settingsDir, settingsFile)

	// Ensure settings don't exist yet
	if _, err := os.Stat(settingsPath); err == nil {
		t.Fatal("Settings file should not exist yet")
	}

	// Create settings
	err := EnsureSettingsAt(tmpDir, Interactive, settingsDir, settingsFile)
	if err != nil {
		t.Fatalf("EnsureSettingsAt() error = %v", err)
	}

	// Verify file was created
	info, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatalf("Settings file was not created: %v", err)
	}
	if info.IsDir() {
		t.Error("Settings path should be a file, not a directory")
	}

	// Verify file has content
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("Failed to read settings file: %v", err)
	}
	if len(content) == 0 {
		t.Error("Settings file should have content")
	}
}

func TestEnsureSettingsAt_FileExists(t *testing.T) {
	tmpDir := t.TempDir()

	settingsDir := ".gemini"
	settingsFile := "settings.json"
	settingsPath := filepath.Join(tmpDir, settingsDir, settingsFile)

	// Create the settings file first
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	existingContent := []byte(`{"existing": true}`)
	if err := os.WriteFile(settingsPath, existingContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// EnsureSettingsAt should not overwrite existing file
	err := EnsureSettingsAt(tmpDir, Interactive, settingsDir, settingsFile)
	if err != nil {
		t.Fatalf("EnsureSettingsAt() error = %v", err)
	}

	// Verify file content is unchanged
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("Failed to read settings file: %v", err)
	}
	if string(content) != string(existingContent) {
		t.Error("EnsureSettingsAt() should not overwrite existing file")
	}
}

func TestEnsureSettingsAt_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	settingsDir := "nested/.gemini"
	settingsFile := "settings.json"
	settingsPath := filepath.Join(tmpDir, settingsDir, settingsFile)

	err := EnsureSettingsAt(tmpDir, Autonomous, settingsDir, settingsFile)
	if err != nil {
		t.Fatalf("EnsureSettingsAt() error = %v", err)
	}

	// Verify directory was created
	dirInfo, err := os.Stat(filepath.Dir(settingsPath))
	if err != nil {
		t.Fatalf("Settings directory was not created: %v", err)
	}
	if !dirInfo.IsDir() {
		t.Error("Settings parent path should be a directory")
	}
}

func TestEnsureSettingsAt_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode checks are not reliable on Windows")
	}

	tmpDir := t.TempDir()

	settingsDir := ".gemini"
	settingsFile := "settings.json"
	settingsPath := filepath.Join(tmpDir, settingsDir, settingsFile)

	err := EnsureSettingsAt(tmpDir, Interactive, settingsDir, settingsFile)
	if err != nil {
		t.Fatalf("EnsureSettingsAt() error = %v", err)
	}

	info, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatalf("Failed to stat settings file: %v", err)
	}

	// Check file mode is 0600 (rw-------)
	expectedMode := os.FileMode(0600)
	if info.Mode() != expectedMode {
		t.Errorf("Settings file mode = %v, want %v", info.Mode(), expectedMode)
	}
}

func TestEnsureSettingsAt_AutonomousTemplate(t *testing.T) {
	tmpDir := t.TempDir()

	settingsDir := ".gemini"
	settingsFile := "settings.json"
	settingsPath := filepath.Join(tmpDir, settingsDir, settingsFile)

	err := EnsureSettingsAt(tmpDir, Autonomous, settingsDir, settingsFile)
	if err != nil {
		t.Fatalf("EnsureSettingsAt() error = %v", err)
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("Failed to read settings file: %v", err)
	}

	// Autonomous template should include mail check --inject in SessionStart
	contentStr := string(content)
	if !containsStr(contentStr, "gt mail check --inject") {
		t.Error("Autonomous template should contain 'gt mail check --inject' in SessionStart")
	}
}

func TestEnsureSettingsAt_InteractiveTemplate(t *testing.T) {
	tmpDir := t.TempDir()

	settingsDir := ".gemini"
	settingsFile := "settings.json"
	settingsPath := filepath.Join(tmpDir, settingsDir, settingsFile)

	err := EnsureSettingsAt(tmpDir, Interactive, settingsDir, settingsFile)
	if err != nil {
		t.Fatalf("EnsureSettingsAt() error = %v", err)
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("Failed to read settings file: %v", err)
	}

	// Interactive SessionStart should NOT include mail check --inject
	contentStr := string(content)
	if containsStr(contentStr, "gt prime --hook && gt mail check --inject") {
		t.Error("Interactive template should NOT contain mail injection in SessionStart")
	}

	// But BeforeAgent should still have mail check
	if !containsStr(contentStr, "BeforeAgent") {
		t.Error("Interactive template should contain BeforeAgent hook")
	}
}

func TestEnsureSettingsForRoleAt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		role     string
		wantType RoleType
	}{
		{"polecat", Autonomous},
		{"crew", Interactive},
		{"mayor", Interactive},
		{"witness", Autonomous},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			tmpDir := t.TempDir()
			err := EnsureSettingsForRoleAt(tmpDir, tt.role, ".gemini", "settings.json")
			if err != nil {
				t.Fatalf("EnsureSettingsForRoleAt() error = %v", err)
			}

			// Verify file was created
			settingsPath := filepath.Join(tmpDir, ".gemini", "settings.json")
			if _, err := os.Stat(settingsPath); err != nil {
				t.Fatalf("Settings file not created for role %s: %v", tt.role, err)
			}
		})
	}
}

func TestEnsureSettingsAt_GeminiHookEvents(t *testing.T) {
	tmpDir := t.TempDir()

	err := EnsureSettingsAt(tmpDir, Interactive, ".gemini", "settings.json")
	if err != nil {
		t.Fatalf("EnsureSettingsAt() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(tmpDir, ".gemini", "settings.json"))
	if err != nil {
		t.Fatalf("Failed to read settings file: %v", err)
	}

	contentStr := string(content)

	// Verify Gemini-specific hook event names (not Claude names)
	geminiEvents := []string{"BeforeTool", "SessionStart", "PreCompress", "BeforeAgent", "SessionEnd"}
	for _, event := range geminiEvents {
		if !containsStr(contentStr, event) {
			t.Errorf("Settings should contain Gemini hook event %q", event)
		}
	}

	// Verify Claude-specific event names are NOT present
	claudeEvents := []string{"PreToolUse", "PreCompact", "UserPromptSubmit"}
	for _, event := range claudeEvents {
		if containsStr(contentStr, event) {
			t.Errorf("Settings should NOT contain Claude hook event %q", event)
		}
	}
}

func TestEmbeddedTemplates_ValidJSON(t *testing.T) {
	t.Parallel()

	templates := []string{
		"config/settings-autonomous.json",
		"config/settings-interactive.json",
	}

	for _, tmpl := range templates {
		t.Run(tmpl, func(t *testing.T) {
			content, err := configFS.ReadFile(tmpl)
			if err != nil {
				t.Fatalf("Failed to read embedded template %s: %v", tmpl, err)
			}

			if len(content) == 0 {
				t.Fatalf("Embedded template %s is empty", tmpl)
			}

			// Verify valid JSON
			var parsed map[string]interface{}
			if err := json.Unmarshal(content, &parsed); err != nil {
				t.Fatalf("Embedded template %s is not valid JSON: %v", tmpl, err)
			}

			// Verify it has a hooks section
			if _, ok := parsed["hooks"]; !ok {
				t.Errorf("Template %s missing 'hooks' key", tmpl)
			}
		})
	}
}

func TestAutonomousTemplate_MailInjectInSessionStart(t *testing.T) {
	content, err := configFS.ReadFile("config/settings-autonomous.json")
	if err != nil {
		t.Fatalf("Failed to read autonomous template: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	hooks, ok := parsed["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("hooks is not a map")
	}

	sessionStart, ok := hooks["SessionStart"]
	if !ok {
		t.Fatal("Missing SessionStart in hooks")
	}

	// Verify at least one SessionStart hook command contains mail inject
	entries, ok := sessionStart.([]interface{})
	if !ok {
		t.Fatal("SessionStart is not an array")
	}

	found := false
	for _, entry := range entries {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		hookActions, ok := entryMap["hooks"].([]interface{})
		if !ok {
			continue
		}
		for _, action := range hookActions {
			actionMap, ok := action.(map[string]interface{})
			if !ok {
				continue
			}
			cmd, _ := actionMap["command"].(string)
			if containsStr(cmd, "gt mail check --inject") {
				found = true
			}
		}
	}

	if !found {
		t.Error("Autonomous template SessionStart must include 'gt mail check --inject'")
	}
}

func TestInteractiveTemplate_NoMailInjectInSessionStart(t *testing.T) {
	content, err := configFS.ReadFile("config/settings-interactive.json")
	if err != nil {
		t.Fatalf("Failed to read interactive template: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	hooks, ok := parsed["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("hooks is not a map")
	}

	sessionStart, ok := hooks["SessionStart"]
	if !ok {
		t.Fatal("Missing SessionStart in hooks")
	}

	entries, ok := sessionStart.([]interface{})
	if !ok {
		t.Fatal("SessionStart is not an array")
	}

	for _, entry := range entries {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		hookActions, ok := entryMap["hooks"].([]interface{})
		if !ok {
			continue
		}
		for _, action := range hookActions {
			actionMap, ok := action.(map[string]interface{})
			if !ok {
				continue
			}
			cmd, _ := actionMap["command"].(string)
			if containsStr(cmd, "gt mail check --inject") {
				t.Error("Interactive template SessionStart must NOT include 'gt mail check --inject'")
			}
		}
	}
}

func TestAllTemplates_SessionStartHasHookFlag(t *testing.T) {
	t.Parallel()

	templates := []string{
		"config/settings-autonomous.json",
		"config/settings-interactive.json",
	}

	for _, tmpl := range templates {
		t.Run(tmpl, func(t *testing.T) {
			content, err := configFS.ReadFile(tmpl)
			if err != nil {
				t.Fatalf("Failed to read template: %v", err)
			}

			var parsed map[string]interface{}
			if err := json.Unmarshal(content, &parsed); err != nil {
				t.Fatalf("Invalid JSON: %v", err)
			}

			hooks, ok := parsed["hooks"].(map[string]interface{})
			if !ok {
				t.Fatal("hooks is not a map")
			}

			sessionStart, ok := hooks["SessionStart"]
			if !ok {
				t.Fatal("Missing SessionStart in hooks")
			}

			entries, ok := sessionStart.([]interface{})
			if !ok {
				t.Fatal("SessionStart is not an array")
			}

			for _, entry := range entries {
				entryMap, ok := entry.(map[string]interface{})
				if !ok {
					continue
				}
				hookActions, ok := entryMap["hooks"].([]interface{})
				if !ok {
					continue
				}
				for _, action := range hookActions {
					actionMap, ok := action.(map[string]interface{})
					if !ok {
						continue
					}
					cmd, _ := actionMap["command"].(string)
					if containsStr(cmd, "gt prime") && !containsStr(cmd, "--hook") {
						t.Errorf("SessionStart hook with 'gt prime' must include '--hook' flag: %s", cmd)
					}
				}
			}
		})
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
