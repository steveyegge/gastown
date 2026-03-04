package cursor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoleTypeFor(t *testing.T) {
	tests := []struct {
		role     string
		expected RoleType
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
			actual := RoleTypeFor(tt.role)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestEnsureHooksAt_CreatesAutonomousHooks(t *testing.T) {
	tmpDir := t.TempDir()
	err := EnsureHooksAt(tmpDir, Autonomous, ".cursor", "hooks.json")
	require.NoError(t, err)
	hooksPath := filepath.Join(tmpDir, ".cursor", "hooks.json")
	content, err := os.ReadFile(hooksPath)
	require.NoError(t, err)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(content, &result))
	assert.Equal(t, float64(1), result["version"])
	hooks, ok := result["hooks"].(map[string]interface{})
	require.True(t, ok)
	sessionStart, ok := hooks["sessionStart"].([]interface{})
	require.True(t, ok)
	require.Len(t, sessionStart, 1)
	cmd := sessionStart[0].(map[string]interface{})["command"].(string)
	assert.Contains(t, cmd, "gt prime --hook")
	assert.Contains(t, cmd, "gt mail check --inject")
}

func TestEnsureHooksAt_CreatesInteractiveHooks(t *testing.T) {
	tmpDir := t.TempDir()
	err := EnsureHooksAt(tmpDir, Interactive, ".cursor", "hooks.json")
	require.NoError(t, err)
	hooksPath := filepath.Join(tmpDir, ".cursor", "hooks.json")
	content, err := os.ReadFile(hooksPath)
	require.NoError(t, err)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(content, &result))
	hooks := result["hooks"].(map[string]interface{})
	sessionStart := hooks["sessionStart"].([]interface{})
	require.Len(t, sessionStart, 1)
	cmd := sessionStart[0].(map[string]interface{})["command"].(string)
	assert.Contains(t, cmd, "gt prime --hook")
	assert.NotContains(t, cmd, "gt mail check --inject")
}

func TestEnsureHooksAt_DoesNotOverwriteExisting(t *testing.T) {
	tmpDir := t.TempDir()
	cursorDir := filepath.Join(tmpDir, ".cursor")
	require.NoError(t, os.MkdirAll(cursorDir, 0755))
	existingContent := []byte(`{"version":1,"hooks":{"custom":true}}`)
	hooksPath := filepath.Join(cursorDir, "hooks.json")
	require.NoError(t, os.WriteFile(hooksPath, existingContent, 0600))
	err := EnsureHooksAt(tmpDir, Autonomous, ".cursor", "hooks.json")
	require.NoError(t, err)
	content, err := os.ReadFile(hooksPath)
	require.NoError(t, err)
	assert.Equal(t, existingContent, content)
}

func TestEnsureHooksForRoleAt(t *testing.T) {
	tmpDir := t.TempDir()
	err := EnsureHooksForRoleAt(tmpDir, "polecat", ".cursor", "hooks.json")
	require.NoError(t, err)
	hooksPath := filepath.Join(tmpDir, ".cursor", "hooks.json")
	content, err := os.ReadFile(hooksPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "gt mail check --inject")
}

func TestEnsureHooksAt_HasAllExpectedEvents(t *testing.T) {
	tmpDir := t.TempDir()
	err := EnsureHooksAt(tmpDir, Autonomous, ".cursor", "hooks.json")
	require.NoError(t, err)
	content, err := os.ReadFile(filepath.Join(tmpDir, ".cursor", "hooks.json"))
	require.NoError(t, err)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(content, &result))
	hooks := result["hooks"].(map[string]interface{})
	expectedEvents := []string{"sessionStart", "preCompact", "beforeSubmitPrompt", "preToolUse", "stop"}
	for _, event := range expectedEvents {
		_, ok := hooks[event]
		assert.True(t, ok, "missing hook event: %s", event)
	}
}
