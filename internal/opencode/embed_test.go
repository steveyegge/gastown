package opencode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// EnsurePluginAt Tests - Tests for the plugin installation/embedding functionality
// =============================================================================

func TestEnsurePluginAt_EmptyParameters(t *testing.T) {
	// Test that empty pluginDir or pluginFile returns nil
	t.Run("empty pluginDir", func(t *testing.T) {
		err := EnsurePluginAt("/tmp/work", "", "plugin.js")
		if err != nil {
			t.Errorf("EnsurePluginAt() with empty pluginDir should return nil, got %v", err)
		}
	})

	t.Run("empty pluginFile", func(t *testing.T) {
		err := EnsurePluginAt("/tmp/work", "plugins", "")
		if err != nil {
			t.Errorf("EnsurePluginAt() with empty pluginFile should return nil, got %v", err)
		}
	})

	t.Run("both empty", func(t *testing.T) {
		err := EnsurePluginAt("/tmp/work", "", "")
		if err != nil {
			t.Errorf("EnsurePluginAt() with both empty should return nil, got %v", err)
		}
	})
}

func TestEnsurePluginAt_FileExists(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	// Create the plugin file first
	pluginDir := "plugins"
	pluginFile := "gastown.js"
	pluginPath := filepath.Join(tmpDir, pluginDir, pluginFile)

	if err := os.MkdirAll(filepath.Dir(pluginPath), 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Write a placeholder file
	existingContent := []byte("// existing plugin")
	if err := os.WriteFile(pluginPath, existingContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// EnsurePluginAt should not overwrite existing file
	err := EnsurePluginAt(tmpDir, pluginDir, pluginFile)
	if err != nil {
		t.Fatalf("EnsurePluginAt() error = %v", err)
	}

	// Verify file content is unchanged
	content, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("Failed to read plugin file: %v", err)
	}
	if string(content) != string(existingContent) {
		t.Error("EnsurePluginAt() should not overwrite existing file")
	}
}

func TestEnsurePluginAt_CreatesFile(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	pluginDir := "plugins"
	pluginFile := "gastown.js"
	pluginPath := filepath.Join(tmpDir, pluginDir, pluginFile)

	// Ensure plugin doesn't exist
	if _, err := os.Stat(pluginPath); err == nil {
		t.Fatal("Plugin file should not exist yet")
	}

	// Create the plugin
	err := EnsurePluginAt(tmpDir, pluginDir, pluginFile)
	if err != nil {
		t.Fatalf("EnsurePluginAt() error = %v", err)
	}

	// Verify file was created
	info, err := os.Stat(pluginPath)
	if err != nil {
		t.Fatalf("Plugin file was not created: %v", err)
	}
	if info.IsDir() {
		t.Error("Plugin path should be a file, not a directory")
	}

	// Verify file has content
	content, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("Failed to read plugin file: %v", err)
	}
	if len(content) == 0 {
		t.Error("Plugin file should have content")
	}
}

func TestEnsurePluginAt_CreatesDirectory(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	pluginDir := "nested/plugins/dir"
	pluginFile := "gastown.js"
	pluginPath := filepath.Join(tmpDir, pluginDir, pluginFile)

	// Create the plugin
	err := EnsurePluginAt(tmpDir, pluginDir, pluginFile)
	if err != nil {
		t.Fatalf("EnsurePluginAt() error = %v", err)
	}

	// Verify directory was created
	dirInfo, err := os.Stat(filepath.Dir(pluginPath))
	if err != nil {
		t.Fatalf("Plugin directory was not created: %v", err)
	}
	if !dirInfo.IsDir() {
		t.Error("Plugin parent path should be a directory")
	}
}

func TestEnsurePluginAt_FilePermissions(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	pluginDir := "plugins"
	pluginFile := "gastown.js"
	pluginPath := filepath.Join(tmpDir, pluginDir, pluginFile)

	err := EnsurePluginAt(tmpDir, pluginDir, pluginFile)
	if err != nil {
		t.Fatalf("EnsurePluginAt() error = %v", err)
	}

	info, err := os.Stat(pluginPath)
	if err != nil {
		t.Fatalf("Failed to stat plugin file: %v", err)
	}

	// Check file mode is 0644 (rw-r--r--)
	expectedMode := os.FileMode(0644)
	if info.Mode() != expectedMode {
		t.Errorf("Plugin file mode = %v, want %v", info.Mode(), expectedMode)
	}
}

// =============================================================================
// Plugin Content Tests - Tests that gastown.js has required structure/content
// =============================================================================

func TestPluginContentStructure(t *testing.T) {
	content, err := pluginFS.ReadFile("plugin/gastown.js")
	if err != nil {
		t.Fatalf("Failed to read plugin file: %v", err)
	}
	contentStr := string(content)

	// Test essential exports
	if !strings.Contains(contentStr, "export const GasTown") {
		t.Error("Plugin should export GasTown function")
	}

	// Test event handlers are present
	requiredHandlers := []string{
		"session.created",                 // SessionStart equivalent
		"message.updated",                 // UserPromptSubmit equivalent
		"session.idle",                    // Stop equivalent
		"experimental.session.compacting", // PreCompact equivalent
	}

	for _, handler := range requiredHandlers {
		if !strings.Contains(contentStr, handler) {
			t.Errorf("Plugin should handle %s event", handler)
		}
	}

	// Test Gastown commands are present
	requiredCommands := []string{
		"gt prime",
		"gt mail check --inject",
		"gt nudge deacon session-started",
		"gt costs record",
	}

	for _, cmd := range requiredCommands {
		if !strings.Contains(contentStr, cmd) {
			t.Errorf("Plugin should execute command: %s", cmd)
		}
	}

	// Test role detection
	if !strings.Contains(contentStr, "GT_ROLE") {
		t.Error("Plugin should check GT_ROLE environment variable")
	}

	// Test autonomous role handling
	autonomousRoles := []string{"polecat", "witness", "refinery", "deacon"}
	for _, role := range autonomousRoles {
		if !strings.Contains(contentStr, role) {
			t.Errorf("Plugin should handle autonomous role: %s", role)
		}
	}

	// Test interactive role handling
	interactiveRoles := []string{"mayor", "crew"}
	for _, role := range interactiveRoles {
		if !strings.Contains(contentStr, role) {
			t.Errorf("Plugin should handle interactive role: %s", role)
		}
	}
}

func TestPluginHasErrorHandling(t *testing.T) {
	content, err := pluginFS.ReadFile("plugin/gastown.js")
	if err != nil {
		t.Fatalf("Failed to read plugin file: %v", err)
	}
	contentStr := string(content)

	// Test try-catch or error handling
	if !strings.Contains(contentStr, "catch") || !strings.Contains(contentStr, "err") {
		t.Error("Plugin should include error handling (try-catch)")
	}

	// Test error logging
	if !strings.Contains(contentStr, "console.error") {
		t.Error("Plugin should log errors to console")
	}
}

func TestPluginHasDebouncing(t *testing.T) {
	content, err := pluginFS.ReadFile("plugin/gastown.js")
	if err != nil {
		t.Fatalf("Failed to read plugin file: %v", err)
	}
	contentStr := string(content)

	// Test debouncing logic for session.idle
	if !strings.Contains(contentStr, "lastIdleTime") {
		t.Error("Plugin should implement debouncing with lastIdleTime")
	}

	// Test time comparison for debouncing
	if !strings.Contains(contentStr, "Date.now()") {
		t.Error("Plugin should use Date.now() for debouncing")
	}
}

func TestPluginInitializationGuard(t *testing.T) {
	content, err := pluginFS.ReadFile("plugin/gastown.js")
	if err != nil {
		t.Fatalf("Failed to read plugin file: %v", err)
	}
	contentStr := string(content)

	// Test initialization flag
	if !strings.Contains(contentStr, "didInit") {
		t.Error("Plugin should have didInit flag to prevent double initialization")
	}

	// Test guard check
	if !strings.Contains(contentStr, "if (didInit)") {
		t.Error("Plugin should check didInit before initializing")
	}
}

func TestPluginUsesCorrectDirectory(t *testing.T) {
	content, err := pluginFS.ReadFile("plugin/gastown.js")
	if err != nil {
		t.Fatalf("Failed to read plugin file: %v", err)
	}
	contentStr := string(content)

	// Test directory parameter is used
	if !strings.Contains(contentStr, "{ $, directory, client }") {
		t.Error("Plugin should accept $, directory and client parameters")
	}

	// Test cwd is set to directory
	if !strings.Contains(contentStr, ".cwd(directory)") {
		t.Error("Plugin should set cwd to directory for commands")
	}
}

func TestPluginEventStructure(t *testing.T) {
	content, err := pluginFS.ReadFile("plugin/gastown.js")
	if err != nil {
		t.Fatalf("Failed to read plugin file: %v", err)
	}
	contentStr := string(content)

	// Test event parameter
	if !strings.Contains(contentStr, "event: async ({ event })") {
		t.Error("Plugin should have event handler with event parameter")
	}

	// Test event type checking
	if !strings.Contains(contentStr, "event?.type") {
		t.Error("Plugin should check event.type with safe navigation")
	}

	// Test event properties access
	if !strings.Contains(contentStr, "event.properties") {
		t.Error("Plugin should access event.properties")
	}
}

func TestPluginMessageRoleFiltering(t *testing.T) {
	content, err := pluginFS.ReadFile("plugin/gastown.js")
	if err != nil {
		t.Fatalf("Failed to read plugin file: %v", err)
	}
	contentStr := string(content)

	// Test role checking for user messages
	if !strings.Contains(contentStr, "role === \"user\"") {
		t.Error("Plugin should filter messages by role === 'user'")
	}

	// Test safe navigation for role property
	if !strings.Contains(contentStr, "?.role") || !strings.Contains(contentStr, "?.info?.role") {
		t.Error("Plugin should use safe navigation for role property")
	}
}
