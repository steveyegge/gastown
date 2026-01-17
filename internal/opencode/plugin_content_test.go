package opencode

import (
	"strings"
	"testing"
)

// TestPluginContentStructure tests the embedded plugin has all required components
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
		"session.created",  // SessionStart equivalent
		"message.updated",  // UserPromptSubmit equivalent  
		"session.idle",     // Stop equivalent
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

// TestPluginHasErrorHandling tests plugin includes error handling
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

// TestPluginHasDebouncing tests plugin includes debouncing for idle events
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

// TestPluginInitializationGuard tests plugin guards against double initialization
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

// TestPluginUsesCorrectDirectory tests plugin uses directory parameter
func TestPluginUsesCorrectDirectory(t *testing.T) {
	content, err := pluginFS.ReadFile("plugin/gastown.js")
	if err != nil {
		t.Fatalf("Failed to read plugin file: %v", err)
	}
	contentStr := string(content)

	// Test directory parameter is used
	if !strings.Contains(contentStr, "{ $, directory }") {
		t.Error("Plugin should accept $ and directory parameters")
	}

	// Test cwd is set to directory
	if !strings.Contains(contentStr, ".cwd(directory)") {
		t.Error("Plugin should set cwd to directory for commands")
	}
}

// TestPluginEventStructure tests plugin properly handles event structure
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

// TestPluginMessageRoleFiltering tests plugin filters messages by role
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
