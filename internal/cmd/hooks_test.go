package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseHooksFile(t *testing.T) {
	// Create a temp directory with a test settings file
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}

	settings := ClaudeSettings{
		Hooks: map[string][]ClaudeHookMatcher{
			"SessionStart": {
				{
					Matcher: "",
					Hooks: []ClaudeHook{
						{Type: "command", Command: "gt prime"},
					},
				},
			},
			"UserPromptSubmit": {
				{
					Matcher: "*.go",
					Hooks: []ClaudeHook{
						{Type: "command", Command: "go fmt"},
						{Type: "command", Command: "go vet"},
					},
				},
			},
		},
	}

	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("failed to marshal settings: %v", err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatalf("failed to write settings: %v", err)
	}

	// Parse the file
	hooks, err := parseHooksFile(settingsPath, "test/agent")
	if err != nil {
		t.Fatalf("parseHooksFile failed: %v", err)
	}

	// Verify results
	if len(hooks) != 2 {
		t.Errorf("expected 2 hooks, got %d", len(hooks))
	}

	// Find the SessionStart hook
	var sessionStart, userPrompt *HookInfo
	for i := range hooks {
		switch hooks[i].Type {
		case "SessionStart":
			sessionStart = &hooks[i]
		case "UserPromptSubmit":
			userPrompt = &hooks[i]
		}
	}

	if sessionStart == nil {
		t.Fatal("expected SessionStart hook")
	}
	if sessionStart.Agent != "test/agent" {
		t.Errorf("expected agent 'test/agent', got %q", sessionStart.Agent)
	}
	if len(sessionStart.Commands) != 1 || sessionStart.Commands[0] != "gt prime" {
		t.Errorf("unexpected SessionStart commands: %v", sessionStart.Commands)
	}

	if userPrompt == nil {
		t.Fatal("expected UserPromptSubmit hook")
	}
	if userPrompt.Matcher != "*.go" {
		t.Errorf("expected matcher '*.go', got %q", userPrompt.Matcher)
	}
	if len(userPrompt.Commands) != 2 {
		t.Errorf("expected 2 commands, got %d", len(userPrompt.Commands))
	}
}

func TestParseHooksFileMissing(t *testing.T) {
	_, err := parseHooksFile("/nonexistent/settings.json", "test")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestParseHooksFileInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.json")

	if err := os.WriteFile(settingsPath, []byte("not json"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	_, err := parseHooksFile(settingsPath, "test")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseHooksFileEmptyHooks(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.json")

	settings := ClaudeSettings{
		Hooks: map[string][]ClaudeHookMatcher{},
	}

	data, _ := json.Marshal(settings)
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	hooks, err := parseHooksFile(settingsPath, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hooks) != 0 {
		t.Errorf("expected 0 hooks, got %d", len(hooks))
	}
}
