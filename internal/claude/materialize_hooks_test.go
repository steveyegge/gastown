package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// --- MergeHooksConfig tests ---

func TestMergeHooksConfig_SingleLayer(t *testing.T) {
	payloads := []string{
		`{"permissions":["read","write"],"hooks":{"PreToolUse":[{"matcher":"Bash","command":"echo pre"}]}}`,
	}

	result, err := MergeHooksConfig(payloads)
	if err != nil {
		t.Fatalf("MergeHooksConfig() error: %v", err)
	}

	// Check top-level key
	perms, ok := result["permissions"].([]interface{})
	if !ok {
		t.Fatal("expected permissions array")
	}
	if len(perms) != 2 {
		t.Errorf("expected 2 permissions, got %d", len(perms))
	}

	// Check hooks
	hooks, ok := result["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("expected hooks map")
	}
	preToolUse, ok := hooks["PreToolUse"].([]interface{})
	if !ok {
		t.Fatal("expected PreToolUse array")
	}
	if len(preToolUse) != 1 {
		t.Errorf("expected 1 PreToolUse hook, got %d", len(preToolUse))
	}
}

func TestMergeHooksConfig_TopLevelOverride(t *testing.T) {
	payloads := []string{
		`{"permissions":["read"],"model":"sonnet"}`,
		`{"permissions":["read","write","admin"],"model":"opus"}`,
	}

	result, err := MergeHooksConfig(payloads)
	if err != nil {
		t.Fatalf("MergeHooksConfig() error: %v", err)
	}

	// More specific layer should override
	perms, ok := result["permissions"].([]interface{})
	if !ok {
		t.Fatal("expected permissions array")
	}
	if len(perms) != 3 {
		t.Errorf("expected 3 permissions (override), got %d", len(perms))
	}

	if result["model"] != "opus" {
		t.Errorf("expected model override to 'opus', got %v", result["model"])
	}
}

func TestMergeHooksConfig_HooksAppend(t *testing.T) {
	payloads := []string{
		`{"hooks":{"PreToolUse":[{"matcher":"Bash","command":"echo global"}]}}`,
		`{"hooks":{"PreToolUse":[{"matcher":"Write","command":"echo rig"}]}}`,
	}

	result, err := MergeHooksConfig(payloads)
	if err != nil {
		t.Fatalf("MergeHooksConfig() error: %v", err)
	}

	hooks := result["hooks"].(map[string]interface{})
	preToolUse := hooks["PreToolUse"].([]interface{})

	if len(preToolUse) != 2 {
		t.Fatalf("expected 2 PreToolUse hooks (appended), got %d", len(preToolUse))
	}

	// First hook should be from global layer
	h0 := preToolUse[0].(map[string]interface{})
	if h0["matcher"] != "Bash" {
		t.Errorf("expected first hook matcher 'Bash', got %v", h0["matcher"])
	}

	// Second hook should be from rig layer
	h1 := preToolUse[1].(map[string]interface{})
	if h1["matcher"] != "Write" {
		t.Errorf("expected second hook matcher 'Write', got %v", h1["matcher"])
	}
}

func TestMergeHooksConfig_NullSuppressTopLevel(t *testing.T) {
	payloads := []string{
		`{"permissions":["read","write"],"model":"sonnet"}`,
		`{"permissions":null}`,
	}

	result, err := MergeHooksConfig(payloads)
	if err != nil {
		t.Fatalf("MergeHooksConfig() error: %v", err)
	}

	if _, ok := result["permissions"]; ok {
		t.Error("expected permissions to be suppressed by null")
	}
	if result["model"] != "sonnet" {
		t.Errorf("expected model to be preserved, got %v", result["model"])
	}
}

func TestMergeHooksConfig_NullSuppressHookType(t *testing.T) {
	payloads := []string{
		`{"hooks":{"PreToolUse":[{"matcher":"Bash","command":"echo pre"}],"PostToolUse":[{"matcher":"*","command":"echo post"}]}}`,
		`{"hooks":{"PreToolUse":null}}`,
	}

	result, err := MergeHooksConfig(payloads)
	if err != nil {
		t.Fatalf("MergeHooksConfig() error: %v", err)
	}

	hooks := result["hooks"].(map[string]interface{})

	if _, ok := hooks["PreToolUse"]; ok {
		t.Error("expected PreToolUse to be suppressed by null")
	}

	// PostToolUse should be preserved
	postToolUse, ok := hooks["PostToolUse"].([]interface{})
	if !ok {
		t.Fatal("expected PostToolUse to be preserved")
	}
	if len(postToolUse) != 1 {
		t.Errorf("expected 1 PostToolUse hook, got %d", len(postToolUse))
	}
}

func TestMergeHooksConfig_NullSuppressViaJSONUnmarshal(t *testing.T) {
	// Verify that JSON null is properly handled through the unmarshal path
	payloads := []string{
		`{"permissions":["read"],"hooks":{"PreToolUse":[{"matcher":"Bash","command":"echo"}]}}`,
		`{"permissions":null,"hooks":{"PreToolUse":null}}`,
	}

	result, err := MergeHooksConfig(payloads)
	if err != nil {
		t.Fatalf("MergeHooksConfig() error: %v", err)
	}

	if _, ok := result["permissions"]; ok {
		t.Error("expected permissions to be removed via JSON null")
	}

	hooks := result["hooks"].(map[string]interface{})
	if _, ok := hooks["PreToolUse"]; ok {
		t.Error("expected PreToolUse to be removed via JSON null")
	}
}

func TestMergeHooksConfig_Mixed3Layer(t *testing.T) {
	payloads := []string{
		// Layer 1: Global - base permissions and hooks
		`{"permissions":["read"],"model":"sonnet","hooks":{"PreToolUse":[{"matcher":"Bash","command":"echo global-pre"}],"PostToolUse":[{"matcher":"*","command":"echo global-post"}]}}`,
		// Layer 2: Rig - override model, append hook, suppress PostToolUse
		`{"model":"opus","hooks":{"PreToolUse":[{"matcher":"Write","command":"echo rig-pre"}],"PostToolUse":null}}`,
		// Layer 3: Agent - add new top-level key, add new hook type
		`{"customKey":"agentValue","hooks":{"Notification":[{"matcher":"*","command":"echo notify"}]}}`,
	}

	result, err := MergeHooksConfig(payloads)
	if err != nil {
		t.Fatalf("MergeHooksConfig() error: %v", err)
	}

	// Top-level: permissions from layer 1 (not overridden)
	perms, ok := result["permissions"].([]interface{})
	if !ok {
		t.Fatal("expected permissions array")
	}
	if len(perms) != 1 {
		t.Errorf("expected 1 permission, got %d", len(perms))
	}

	// Top-level: model overridden by layer 2
	if result["model"] != "opus" {
		t.Errorf("expected model 'opus', got %v", result["model"])
	}

	// Top-level: customKey from layer 3
	if result["customKey"] != "agentValue" {
		t.Errorf("expected customKey 'agentValue', got %v", result["customKey"])
	}

	hooks := result["hooks"].(map[string]interface{})

	// PreToolUse: global + rig (appended)
	preToolUse := hooks["PreToolUse"].([]interface{})
	if len(preToolUse) != 2 {
		t.Errorf("expected 2 PreToolUse hooks, got %d", len(preToolUse))
	}

	// PostToolUse: suppressed by layer 2 null
	if _, ok := hooks["PostToolUse"]; ok {
		t.Error("expected PostToolUse to be suppressed by rig layer null")
	}

	// Notification: added by layer 3
	notification := hooks["Notification"].([]interface{})
	if len(notification) != 1 {
		t.Errorf("expected 1 Notification hook, got %d", len(notification))
	}
}

func TestMergeHooksConfig_EmptyPayloads(t *testing.T) {
	payloads := []string{"", "", ""}

	result, err := MergeHooksConfig(payloads)
	if err != nil {
		t.Fatalf("MergeHooksConfig() error: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected empty result from empty payloads, got %v", result)
	}
}

func TestMergeHooksConfig_NilPayloads(t *testing.T) {
	result, err := MergeHooksConfig(nil)
	if err != nil {
		t.Fatalf("MergeHooksConfig() error: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected empty result from nil payloads, got %v", result)
	}
}

func TestMergeHooksConfig_InvalidJSON(t *testing.T) {
	payloads := []string{
		`{"valid":"data"}`,
		`not valid json at all`,
	}

	_, err := MergeHooksConfig(payloads)
	if err == nil {
		t.Fatal("expected error for invalid JSON payload")
	}
}

func TestMergeHooksConfig_HooksNonMapValue(t *testing.T) {
	// If hooks value is not a map, it should override entirely
	payloads := []string{
		`{"hooks":"string-not-map"}`,
	}

	result, err := MergeHooksConfig(payloads)
	if err != nil {
		t.Fatalf("MergeHooksConfig() error: %v", err)
	}

	if result["hooks"] != "string-not-map" {
		t.Errorf("expected hooks to be 'string-not-map', got %v", result["hooks"])
	}
}

func TestMergeHooksConfig_HookTypeNonArray(t *testing.T) {
	// If a hook type value is not an array, it should override entirely
	payloads := []string{
		`{"hooks":{"PreToolUse":"not-an-array"}}`,
	}

	result, err := MergeHooksConfig(payloads)
	if err != nil {
		t.Fatalf("MergeHooksConfig() error: %v", err)
	}

	hooks := result["hooks"].(map[string]interface{})
	if hooks["PreToolUse"] != "not-an-array" {
		t.Errorf("expected PreToolUse to be 'not-an-array', got %v", hooks["PreToolUse"])
	}
}

func TestMergeHooksConfig_AppendToNonExistingHookType(t *testing.T) {
	// When appending to a hook type that doesn't exist yet
	payloads := []string{
		`{"hooks":{"PreToolUse":[{"matcher":"Bash","command":"echo pre"}]}}`,
		`{"hooks":{"PostToolUse":[{"matcher":"*","command":"echo post"}]}}`,
	}

	result, err := MergeHooksConfig(payloads)
	if err != nil {
		t.Fatalf("MergeHooksConfig() error: %v", err)
	}

	hooks := result["hooks"].(map[string]interface{})

	preToolUse := hooks["PreToolUse"].([]interface{})
	if len(preToolUse) != 1 {
		t.Errorf("expected 1 PreToolUse hook, got %d", len(preToolUse))
	}

	postToolUse := hooks["PostToolUse"].([]interface{})
	if len(postToolUse) != 1 {
		t.Errorf("expected 1 PostToolUse hook, got %d", len(postToolUse))
	}
}

// --- MaterializeSettings tests ---

func TestMaterializeSettings_WithLayers(t *testing.T) {
	workDir := t.TempDir()

	payloads := []string{
		`{"permissions":["read"],"hooks":{"PreToolUse":[{"matcher":"Bash","command":"echo global"}]}}`,
		`{"permissions":["read","write"],"hooks":{"PreToolUse":[{"matcher":"Write","command":"echo rig"}]}}`,
	}

	err := MaterializeSettings(workDir, "polecat", payloads)
	if err != nil {
		t.Fatalf("MaterializeSettings() error: %v", err)
	}

	// Verify file was written
	settingsPath := filepath.Join(workDir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("reading settings.json: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}

	// permissions should be overridden by second layer
	perms, ok := config["permissions"].([]interface{})
	if !ok {
		t.Fatal("expected permissions array")
	}
	if len(perms) != 2 {
		t.Errorf("expected 2 permissions, got %d", len(perms))
	}

	// hooks should be appended
	hooks := config["hooks"].(map[string]interface{})
	preToolUse := hooks["PreToolUse"].([]interface{})
	if len(preToolUse) != 2 {
		t.Errorf("expected 2 PreToolUse hooks (appended), got %d", len(preToolUse))
	}
}

func TestMaterializeSettings_EmptyFallback(t *testing.T) {
	workDir := t.TempDir()

	// No metadata payloads - should fall back to embedded template
	err := MaterializeSettings(workDir, "polecat", nil)
	if err != nil {
		t.Fatalf("MaterializeSettings() error: %v", err)
	}

	// Verify settings file was created (from embedded template)
	settingsPath := filepath.Join(workDir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("reading settings.json: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}

	// Embedded template should have some content
	if len(config) == 0 {
		t.Error("expected non-empty settings from embedded template")
	}
}

func TestMaterializeSettings_EmptyPayloadsFallback(t *testing.T) {
	workDir := t.TempDir()

	// All-empty payloads produce empty merge result, which triggers WriteSettings
	// with an empty map (not fallback to EnsureSettings since payloads slice is non-nil)
	payloads := []string{"", ""}

	err := MaterializeSettings(workDir, "polecat", payloads)
	if err != nil {
		t.Fatalf("MaterializeSettings() error: %v", err)
	}

	// File should exist (written by WriteSettings with empty merged result)
	settingsPath := filepath.Join(workDir, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Fatal("expected settings.json to exist")
	}
}

func TestMaterializeSettings_FileVerification(t *testing.T) {
	workDir := t.TempDir()

	payloads := []string{
		`{"permissions":["read","write"],"model":"opus","hooks":{"PreToolUse":[{"matcher":"Bash","command":"validate"}]}}`,
	}

	err := MaterializeSettings(workDir, "polecat", payloads)
	if err != nil {
		t.Fatalf("MaterializeSettings() error: %v", err)
	}

	// Verify directory structure
	claudeDir := filepath.Join(workDir, ".claude")
	info, err := os.Stat(claudeDir)
	if err != nil {
		t.Fatalf("expected .claude directory to exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected .claude to be a directory")
	}

	// Verify file exists and is readable
	settingsPath := filepath.Join(claudeDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("reading settings.json: %v", err)
	}

	// Verify file is valid JSON
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}

	// Verify all expected keys are present
	if config["model"] != "opus" {
		t.Errorf("expected model 'opus', got %v", config["model"])
	}

	perms := config["permissions"].([]interface{})
	if len(perms) != 2 {
		t.Errorf("expected 2 permissions, got %d", len(perms))
	}

	hooks := config["hooks"].(map[string]interface{})
	preToolUse := hooks["PreToolUse"].([]interface{})
	if len(preToolUse) != 1 {
		t.Errorf("expected 1 PreToolUse hook, got %d", len(preToolUse))
	}

	h := preToolUse[0].(map[string]interface{})
	if h["matcher"] != "Bash" {
		t.Errorf("expected hook matcher 'Bash', got %v", h["matcher"])
	}
	if h["command"] != "validate" {
		t.Errorf("expected hook command 'validate', got %v", h["command"])
	}

	// Verify file permissions (should be 0600)
	info, err = os.Stat(settingsPath)
	if err != nil {
		t.Fatalf("stat settings.json: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("expected file permission 0600, got %04o", perm)
	}

	// Verify file ends with newline
	if len(data) > 0 && data[len(data)-1] != '\n' {
		t.Error("expected settings.json to end with newline")
	}
}

func TestMaterializeSettings_OverwritesExisting(t *testing.T) {
	workDir := t.TempDir()

	// Create pre-existing settings file
	claudeDir := filepath.Join(workDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("creating .claude dir: %v", err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"old":"data"}`), 0600); err != nil {
		t.Fatalf("writing old settings: %v", err)
	}

	// MaterializeSettings with config beads should overwrite
	payloads := []string{`{"new":"data","hooks":{}}`}
	err := MaterializeSettings(workDir, "polecat", payloads)
	if err != nil {
		t.Fatalf("MaterializeSettings() error: %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("reading settings.json: %v", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}

	if _, ok := config["old"]; ok {
		t.Error("expected old data to be overwritten")
	}
	if config["new"] != "data" {
		t.Errorf("expected new data, got %v", config["new"])
	}
}
