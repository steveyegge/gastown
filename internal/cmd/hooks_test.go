package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/hooks"
)

func TestParseHooksFile(t *testing.T) {
	// Create a temp directory with a test settings file
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}

	settings := hooks.SettingsJSON{
		Hooks: hooks.HooksConfig{
			SessionStart: []hooks.HookEntry{
				{
					Matcher: "",
					Hooks: []hooks.Hook{
						{Type: "command", Command: "gt prime"},
					},
				},
			},
			UserPromptSubmit: []hooks.HookEntry{
				{
					Matcher: "*.go",
					Hooks: []hooks.Hook{
						{Type: "command", Command: "go fmt"},
						{Type: "command", Command: "go vet"},
					},
				},
			},
		},
	}

	data, err := hooks.MarshalSettings(&settings)
	if err != nil {
		t.Fatalf("failed to marshal settings: %v", err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatalf("failed to write settings: %v", err)
	}

	// Parse the file
	hookInfos, err := parseHooksFile(settingsPath, "test/agent")
	if err != nil {
		t.Fatalf("parseHooksFile failed: %v", err)
	}

	// Verify results
	if len(hookInfos) != 2 {
		t.Errorf("expected 2 hooks, got %d", len(hookInfos))
	}

	// Find the SessionStart hook
	var sessionStart, userPrompt *HookInfo
	for i := range hookInfos {
		switch hookInfos[i].Type {
		case "SessionStart":
			sessionStart = &hookInfos[i]
		case "UserPromptSubmit":
			userPrompt = &hookInfos[i]
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
	// parseHooksFile now returns empty results for missing files (via LoadSettings),
	// not an error. This matches the updated semantics.
	infos, err := parseHooksFile("/nonexistent/settings.json", "test")
	if err != nil {
		t.Errorf("unexpected error for missing file: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("expected 0 hooks for missing file, got %d", len(infos))
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

	settings := hooks.SettingsJSON{}

	data, _ := json.Marshal(settings)
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	hookInfos, err := parseHooksFile(settingsPath, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hookInfos) != 0 {
		t.Errorf("expected 0 hooks, got %d", len(hookInfos))
	}
}

func TestDiscoverHooksCrewLevel(t *testing.T) {
	// Create a temp directory structure simulating a Gas Town workspace
	tmpDir := t.TempDir()

	// Create rig structure with individual crew member and polecat worktree settings
	// (DiscoverTargets only targets individual worktrees, not parent crew/ or polecats/ dirs)
	rigName := "testrig"
	rigDir := filepath.Join(tmpDir, rigName)

	// Create individual crew member settings (crew/alice/.claude/settings.local.json)
	crewMemberClaudeDir := filepath.Join(rigDir, "crew", "alice", ".claude")
	if err := os.MkdirAll(crewMemberClaudeDir, 0755); err != nil {
		t.Fatalf("failed to create crew/alice/.claude dir: %v", err)
	}

	crewSettings := hooks.SettingsJSON{
		Hooks: hooks.HooksConfig{
			SessionStart: []hooks.HookEntry{
				{
					Matcher: "",
					Hooks: []hooks.Hook{
						{Type: "command", Command: "crew-level-hook"},
					},
				},
			},
		},
	}
	crewData, _ := hooks.MarshalSettings(&crewSettings)
	if err := os.WriteFile(filepath.Join(crewMemberClaudeDir, "settings.local.json"), crewData, 0644); err != nil {
		t.Fatalf("failed to write crew settings: %v", err)
	}

	// Create individual polecat worktree settings (polecats/toast/.claude/settings.local.json)
	polecatClaudeDir := filepath.Join(rigDir, "polecats", "toast", ".claude")
	if err := os.MkdirAll(polecatClaudeDir, 0755); err != nil {
		t.Fatalf("failed to create polecats/toast/.claude dir: %v", err)
	}

	polecatsSettings := hooks.SettingsJSON{
		Hooks: hooks.HooksConfig{
			PreToolUse: []hooks.HookEntry{
				{
					Matcher: "",
					Hooks: []hooks.Hook{
						{Type: "command", Command: "polecats-level-hook"},
					},
				},
			},
		},
	}
	polecatsData, _ := hooks.MarshalSettings(&polecatsSettings)
	if err := os.WriteFile(filepath.Join(polecatClaudeDir, "settings.local.json"), polecatsData, 0644); err != nil {
		t.Fatalf("failed to write polecats settings: %v", err)
	}

	// Discover hooks
	hookInfos, err := discoverHooks(tmpDir)
	if err != nil {
		t.Fatalf("discoverHooks failed: %v", err)
	}

	// Verify individual crew member and polecat hooks were discovered
	var foundCrewLevel, foundPolecatsLevel bool
	for _, h := range hookInfos {
		if h.Agent == "testrig/crew" && len(h.Commands) > 0 && h.Commands[0] == "crew-level-hook" {
			foundCrewLevel = true
		}
		if h.Agent == "testrig/polecats" && len(h.Commands) > 0 && h.Commands[0] == "polecats-level-hook" {
			foundPolecatsLevel = true
		}
	}

	if !foundCrewLevel {
		t.Error("expected crew member hook to be discovered (testrig/crew)")
	}
	if !foundPolecatsLevel {
		t.Error("expected polecat hook to be discovered (testrig/polecats)")
	}
}
