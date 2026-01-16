package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isClaudeCmd checks if a command is claude (either "claude" or a path ending in "/claude").
// Note: Named differently from loader_test.go's isClaudeCommand to avoid redeclaration.
func isClaudeCmd(cmd string) bool {
	return cmd == "claude" || strings.HasSuffix(cmd, "/claude")
}

func TestBuiltinPresets(t *testing.T) {
	t.Parallel()
	// Ensure all built-in presets are accessible
	presets := []AgentPreset{AgentClaude, AgentGemini, AgentCodex, AgentCursor, AgentAuggie, AgentAmp}

	for _, preset := range presets {
		info := GetAgentPreset(preset)
		if info == nil {
			t.Errorf("GetAgentPreset(%s) returned nil", preset)
			continue
		}

		if info.Command == "" {
			t.Errorf("preset %s has empty Command", preset)
		}

		// All presets should have ProcessNames for agent detection
		if len(info.ProcessNames) == 0 {
			t.Errorf("preset %s has empty ProcessNames", preset)
		}
	}
}

func TestGetAgentPresetByName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		want    AgentPreset
		wantNil bool
	}{
		{"claude", AgentClaude, false},
		{"gemini", AgentGemini, false},
		{"codex", AgentCodex, false},
		{"cursor", AgentCursor, false},
		{"auggie", AgentAuggie, false},
		{"amp", AgentAmp, false},
		{"aider", "", true},               // Not built-in, can be added via config
		{"opencode", AgentOpenCode, false}, // Built-in multi-model CLI agent
		{"unknown", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetAgentPresetByName(tt.name)
			if tt.wantNil && got != nil {
				t.Errorf("GetAgentPresetByName(%s) = %v, want nil", tt.name, got)
			}
			if !tt.wantNil && got == nil {
				t.Errorf("GetAgentPresetByName(%s) = nil, want preset", tt.name)
			}
			if !tt.wantNil && got != nil && got.Name != tt.want {
				t.Errorf("GetAgentPresetByName(%s).Name = %v, want %v", tt.name, got.Name, tt.want)
			}
		})
	}
}

func TestSupportsModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		wantOK   bool
		wantFlag string
	}{
		{"claude", true, "--model"},
		{"codex", true, "--model"},
		{"gemini", false, ""},
		{"unknown", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, flag := SupportsModel(tt.name)
			if ok != tt.wantOK {
				t.Errorf("SupportsModel(%q) ok = %v, want %v", tt.name, ok, tt.wantOK)
			}
			if flag != tt.wantFlag {
				t.Errorf("SupportsModel(%q) flag = %q, want %q", tt.name, flag, tt.wantFlag)
			}
		})
	}
}

func TestRuntimeConfigFromPreset(t *testing.T) {
	t.Parallel()
	tests := []struct {
		preset      AgentPreset
		wantCommand string
	}{
		{AgentClaude, "claude"}, // Note: claude may resolve to full path
		{AgentGemini, "gemini"},
		{AgentCodex, "codex"},
		{AgentCursor, "cursor-agent"},
		{AgentAuggie, "auggie"},
		{AgentAmp, "amp"},
	}

	for _, tt := range tests {
		t.Run(string(tt.preset), func(t *testing.T) {
			rc := RuntimeConfigFromPreset(tt.preset)
			// For claude, command may be full path due to resolveClaudePath
			if tt.preset == AgentClaude {
				if !isClaudeCmd(rc.Command) {
					t.Errorf("RuntimeConfigFromPreset(%s).Command = %v, want claude or path ending in /claude",
						tt.preset, rc.Command)
				}
			} else if rc.Command != tt.wantCommand {
				t.Errorf("RuntimeConfigFromPreset(%s).Command = %v, want %v",
					tt.preset, rc.Command, tt.wantCommand)
			}
		})
	}
}

func TestRuntimeConfigFromPresetReturnsNilEnvForPresetsWithoutEnv(t *testing.T) {
	t.Parallel()
	// Built-in presets like Claude don't have Env set
	// This verifies nil Env handling in RuntimeConfigFromPreset
	rc := RuntimeConfigFromPreset(AgentClaude)
	if rc == nil {
		t.Fatal("RuntimeConfigFromPreset returned nil")
	}

	// Claude preset doesn't have Env, so it should be nil
	if rc.Env != nil && len(rc.Env) > 0 {
		t.Errorf("Expected nil/empty Env for Claude preset, got %v", rc.Env)
	}
}

func TestIsKnownPreset(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		want bool
	}{
		{"claude", true},
		{"gemini", true},
		{"codex", true},
		{"cursor", true},
		{"auggie", true},
		{"amp", true},
		{"aider", false},    // Not built-in, can be added via config
		{"opencode", true},  // Built-in multi-model CLI agent
		{"unknown", false},
		{"chatgpt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsKnownPreset(tt.name); got != tt.want {
				t.Errorf("IsKnownPreset(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestLoadAgentRegistry(t *testing.T) {
	// Create temp directory for test config
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "agents.json")

	// Write custom agent config
	customRegistry := AgentRegistry{
		Version: CurrentAgentRegistryVersion,
		Agents: map[string]*AgentPresetInfo{
			"my-agent": {
				Name:    "my-agent",
				Command: "my-agent-bin",
				Args:    []string{"--auto"},
			},
		},
	}

	data, err := json.Marshal(customRegistry)
	if err != nil {
		t.Fatalf("failed to marshal test config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	// Reset global registry for test isolation
	ResetRegistryForTesting()

	// Load should succeed
	if err := LoadAgentRegistry(configPath); err != nil {
		t.Fatalf("LoadAgentRegistry failed: %v", err)
	}

	// Check custom agent is available
	myAgent := GetAgentPresetByName("my-agent")
	if myAgent == nil {
		t.Fatal("custom agent 'my-agent' not found after loading registry")
	}

	if myAgent.Command != "my-agent-bin" {
		t.Errorf("my-agent.Command = %v, want my-agent-bin", myAgent.Command)
	}

	// Check built-ins still accessible
	claude := GetAgentPresetByName("claude")
	if claude == nil {
		t.Fatal("built-in 'claude' not found after loading registry")
	}

	// Reset for other tests
	ResetRegistryForTesting()
}

func TestAgentPresetYOLOFlags(t *testing.T) {
	t.Parallel()
	// Verify YOLO flags are set correctly for each E2E tested agent
	tests := []struct {
		preset  AgentPreset
		wantArg string // At least this arg should be present
	}{
		{AgentClaude, "--dangerously-skip-permissions"},
		{AgentGemini, "yolo"}, // Part of "--approval-mode yolo"
		{AgentCodex, "--yolo"},
	}

	for _, tt := range tests {
		t.Run(string(tt.preset), func(t *testing.T) {
			info := GetAgentPreset(tt.preset)
			if info == nil {
				t.Fatalf("preset %s not found", tt.preset)
			}

			found := false
			for _, arg := range info.Args {
				if arg == tt.wantArg || (tt.preset == AgentGemini && arg == "yolo") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("preset %s args %v missing expected %s", tt.preset, info.Args, tt.wantArg)
			}
		})
	}
}

func TestMergeWithPreset(t *testing.T) {
	t.Parallel()
	// Test that user config overrides preset defaults
	userConfig := &RuntimeConfig{
		Command: "/custom/claude",
		Args:    []string{"--custom-arg"},
	}

	merged := userConfig.MergeWithPreset(AgentClaude)

	if merged.Command != "/custom/claude" {
		t.Errorf("merged command should be user value, got %s", merged.Command)
	}

	if len(merged.Args) != 1 || merged.Args[0] != "--custom-arg" {
		t.Errorf("merged args should be user value, got %v", merged.Args)
	}

	// Test nil config gets preset defaults
	var nilConfig *RuntimeConfig
	merged = nilConfig.MergeWithPreset(AgentClaude)

	if !isClaudeCmd(merged.Command) {
		t.Errorf("nil config merge should get preset command (claude or path), got %s", merged.Command)
	}

	// Test empty config gets preset defaults
	emptyConfig := &RuntimeConfig{}
	merged = emptyConfig.MergeWithPreset(AgentGemini)

	if merged.Command != "gemini" {
		t.Errorf("empty config merge should get preset command, got %s", merged.Command)
	}
}

func TestBuildResumeCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		agentName string
		sessionID string
		wantEmpty bool
		contains  []string // strings that should appear in result
	}{
		{
			name:      "claude with session",
			agentName: "claude",
			sessionID: "session-123",
			wantEmpty: false,
			contains:  []string{"claude", "--dangerously-skip-permissions", "--resume", "session-123"},
		},
		{
			name:      "gemini with session",
			agentName: "gemini",
			sessionID: "gemini-sess-456",
			wantEmpty: false,
			contains:  []string{"gemini", "--approval-mode", "yolo", "--resume", "gemini-sess-456"},
		},
		{
			name:      "codex subcommand style",
			agentName: "codex",
			sessionID: "codex-sess-789",
			wantEmpty: false,
			contains:  []string{"codex", "resume", "codex-sess-789", "--yolo"},
		},
		{
			name:      "empty session ID",
			agentName: "claude",
			sessionID: "",
			wantEmpty: true,
			contains:  []string{"claude"},
		},
		{
			name:      "unknown agent",
			agentName: "unknown-agent",
			sessionID: "session-123",
			wantEmpty: true,
			contains:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildResumeCommand(tt.agentName, tt.sessionID)
			if tt.wantEmpty {
				if result != "" {
					t.Errorf("BuildResumeCommand(%s, %s) = %q, want empty", tt.agentName, tt.sessionID, result)
				}
				return
			}
			for _, s := range tt.contains {
				if !strings.Contains(result, s) {
					t.Errorf("BuildResumeCommand(%s, %s) = %q, missing %q", tt.agentName, tt.sessionID, result, s)
				}
			}
		})
	}
}

func TestSupportsSessionResume(t *testing.T) {
	t.Parallel()
	tests := []struct {
		agentName string
		want      bool
	}{
		{"claude", true},
		{"gemini", true},
		{"codex", true},
		{"cursor", true},
		{"auggie", true},
		{"amp", true},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.agentName, func(t *testing.T) {
			if got := SupportsSessionResume(tt.agentName); got != tt.want {
				t.Errorf("SupportsSessionResume(%s) = %v, want %v", tt.agentName, got, tt.want)
			}
		})
	}
}

func TestGetSessionIDEnvVar(t *testing.T) {
	t.Parallel()
	tests := []struct {
		agentName string
		want      string
	}{
		{"claude", "CLAUDE_SESSION_ID"},
		{"gemini", "GEMINI_SESSION_ID"},
		{"codex", ""},  // Codex uses JSONL output instead
		{"cursor", ""}, // Cursor uses --resume with chatId directly
		{"auggie", ""}, // Auggie uses --resume directly
		{"amp", ""},    // AMP uses 'threads continue' subcommand
		{"unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.agentName, func(t *testing.T) {
			if got := GetSessionIDEnvVar(tt.agentName); got != tt.want {
				t.Errorf("GetSessionIDEnvVar(%s) = %q, want %q", tt.agentName, got, tt.want)
			}
		})
	}
}

func TestGetProcessNames(t *testing.T) {
	t.Parallel()
	tests := []struct {
		agentName string
		want      []string
	}{
		{"claude", []string{"node", "claude"}},
		{"gemini", []string{"gemini"}},
		{"codex", []string{"codex"}},
		{"cursor", []string{"cursor-agent"}},
		{"auggie", []string{"auggie"}},
		{"amp", []string{"amp"}},
		{"opencode", []string{"opencode", "node", "bun"}},
		{"unknown", []string{"node", "claude"}}, // Falls back to Claude's process
	}

	for _, tt := range tests {
		t.Run(tt.agentName, func(t *testing.T) {
			got := GetProcessNames(tt.agentName)
			if len(got) != len(tt.want) {
				t.Errorf("GetProcessNames(%s) = %v, want %v", tt.agentName, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("GetProcessNames(%s)[%d] = %q, want %q", tt.agentName, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestListAgentPresetsMatchesConstants(t *testing.T) {
	t.Parallel()
	// Ensure all AgentPreset constants are returned by ListAgentPresets
	allConstants := []AgentPreset{AgentClaude, AgentGemini, AgentCodex, AgentCursor, AgentAuggie, AgentAmp}
	presets := ListAgentPresets()

	// Convert to map for quick lookup
	presetMap := make(map[string]bool)
	for _, p := range presets {
		presetMap[p] = true
	}

	// Verify all constants are in the list
	for _, c := range allConstants {
		if !presetMap[string(c)] {
			t.Errorf("ListAgentPresets() missing constant %q", c)
		}
	}

	// Verify no empty names
	for _, p := range presets {
		if p == "" {
			t.Error("ListAgentPresets() contains empty string")
		}
	}
}

func TestAgentCommandGeneration(t *testing.T) {
	t.Parallel()
	// Test full command line generation for each agent
	tests := []struct {
		preset       AgentPreset
		wantCommand  string
		wantContains []string // Args that should be present
	}{
		{
			preset:       AgentClaude,
			wantCommand:  "claude",
			wantContains: []string{"--dangerously-skip-permissions"},
		},
		{
			preset:       AgentGemini,
			wantCommand:  "gemini",
			wantContains: []string{"--approval-mode", "yolo"},
		},
		{
			preset:       AgentCodex,
			wantCommand:  "codex",
			wantContains: []string{"--yolo"},
		},
		{
			preset:       AgentCursor,
			wantCommand:  "cursor-agent",
			wantContains: []string{"-f"},
		},
		{
			preset:       AgentAuggie,
			wantCommand:  "auggie",
			wantContains: []string{"--allow-indexing"},
		},
		{
			preset:       AgentAmp,
			wantCommand:  "amp",
			wantContains: []string{"--dangerously-allow-all", "--no-ide"},
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.preset), func(t *testing.T) {
			rc := RuntimeConfigFromPreset(tt.preset)
			if rc == nil {
				t.Fatal("RuntimeConfigFromPreset returned nil")
			}

			// For claude, command may be full path due to resolveClaudePath
			if tt.preset == AgentClaude {
				if !isClaudeCmd(rc.Command) {
					t.Errorf("Command = %q, want claude or path ending in /claude", rc.Command)
				}
			} else if rc.Command != tt.wantCommand {
				t.Errorf("Command = %q, want %q", rc.Command, tt.wantCommand)
			}

			// Check required args are present
			argsStr := strings.Join(rc.Args, " ")
			for _, arg := range tt.wantContains {
				found := false
				for _, a := range rc.Args {
					if a == arg {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Args %q missing expected %q", argsStr, arg)
				}
			}
		})
	}
}

func TestCursorAgentPreset(t *testing.T) {
	t.Parallel()
	// Verify cursor agent preset is correctly configured
	info := GetAgentPreset(AgentCursor)
	if info == nil {
		t.Fatal("cursor preset not found")
	}

	// Check command
	if info.Command != "cursor-agent" {
		t.Errorf("cursor command = %q, want cursor-agent", info.Command)
	}

	// Check YOLO-equivalent flag (-f for force mode)
	// Note: -p is for non-interactive mode with prompt, not used for default Args
	hasF := false
	for _, arg := range info.Args {
		if arg == "-f" {
			hasF = true
		}
	}
	if !hasF {
		t.Error("cursor args missing -f (force/YOLO mode)")
	}

	// Check ProcessNames for detection
	if len(info.ProcessNames) == 0 {
		t.Error("cursor ProcessNames is empty")
	}
	if info.ProcessNames[0] != "cursor-agent" {
		t.Errorf("cursor ProcessNames[0] = %q, want cursor-agent", info.ProcessNames[0])
	}

	// Check resume support
	if info.ResumeFlag != "--resume" {
		t.Errorf("cursor ResumeFlag = %q, want --resume", info.ResumeFlag)
	}
	if info.ResumeStyle != "flag" {
		t.Errorf("cursor ResumeStyle = %q, want flag", info.ResumeStyle)
	}
}

// TestDefaultRigAgentRegistryPath verifies that the default rig agent registry path is constructed correctly.
func TestDefaultRigAgentRegistryPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		rigPath      string
		expectedPath string
	}{
		{"/Users/alice/gt/myproject", "/Users/alice/gt/myproject/settings/agents.json"},
		{"/tmp/my-rig", "/tmp/my-rig/settings/agents.json"},
		{"relative/path", "relative/path/settings/agents.json"},
	}

	for _, tt := range tests {
		t.Run(tt.rigPath, func(t *testing.T) {
			got := DefaultRigAgentRegistryPath(tt.rigPath)
			want := tt.expectedPath
			if filepath.ToSlash(got) != filepath.ToSlash(want) {
				t.Errorf("DefaultRigAgentRegistryPath(%s) = %s, want %s", tt.rigPath, got, want)
			}
		})
	}
}

// TestLoadRigAgentRegistry verifies that rig-level agent registry is loaded correctly.
func TestLoadRigAgentRegistry(t *testing.T) {
	// Reset registry for test isolation
	ResetRegistryForTesting()
	t.Cleanup(ResetRegistryForTesting)

	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "settings", "agents.json")
	configDir := filepath.Join(tmpDir, "settings")

	// Create settings directory
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create settings dir: %v", err)
	}

	// Write agent registry
	registryContent := `{
  "version": 1,
  "agents": {
    "opencode": {
      "command": "opencode",
      "args": ["--session"],
      "non_interactive": {
        "subcommand": "run",
        "output_flag": "--format json"
      }
    }
  }
}`

	if err := os.WriteFile(registryPath, []byte(registryContent), 0644); err != nil {
		t.Fatalf("failed to write registry file: %v", err)
	}

	// Test 1: Load should succeed and merge agents
	t.Run("load and merge", func(t *testing.T) {
		if err := LoadRigAgentRegistry(registryPath); err != nil {
			t.Fatalf("LoadRigAgentRegistry(%s) failed: %v", registryPath, err)
		}

		info := GetAgentPresetByName("opencode")
		if info == nil {
			t.Fatal("expected opencode agent to be available after loading rig registry")
		}

		if info.Command != "opencode" {
			t.Errorf("expected opencode agent command to be 'opencode', got %s", info.Command)
		}
	})

	// Test 2: File not found should return nil (no error)
	t.Run("file not found", func(t *testing.T) {
		nonExistentPath := filepath.Join(tmpDir, "other-rig", "settings", "agents.json")
		if err := LoadRigAgentRegistry(nonExistentPath); err != nil {
			t.Errorf("LoadRigAgentRegistry(%s) should not error for non-existent file: %v", nonExistentPath, err)
		}

		// Verify that previously loaded agent (from test 1) is still available
		info := GetAgentPresetByName("opencode")
		if info == nil {
			t.Errorf("expected opencode agent to still be available after loading non-existent path")
			return
		}
		if info.Command != "opencode" {
			t.Errorf("expected opencode agent command to be 'opencode', got %s", info.Command)
		}
	})

	// Test 3: Invalid JSON should error
	t.Run("invalid JSON", func(t *testing.T) {
		invalidRegistryPath := filepath.Join(tmpDir, "bad-rig", "settings", "agents.json")
		badConfigDir := filepath.Join(tmpDir, "bad-rig", "settings")
		if err := os.MkdirAll(badConfigDir, 0755); err != nil {
			t.Fatalf("failed to create bad-rig settings dir: %v", err)
		}

		invalidContent := `{"version": 1, "agents": {invalid json}}`
		if err := os.WriteFile(invalidRegistryPath, []byte(invalidContent), 0644); err != nil {
			t.Fatalf("failed to write invalid registry file: %v", err)
		}

		if err := LoadRigAgentRegistry(invalidRegistryPath); err == nil {
			t.Errorf("LoadRigAgentRegistry(%s) should error for invalid JSON: got nil", invalidRegistryPath)
		}
	})
}

func TestOpenCodeAgentPreset(t *testing.T) {
	t.Parallel()
	// Verify OpenCode agent preset is correctly configured
	info := GetAgentPreset(AgentOpenCode)
	if info == nil {
		t.Fatal("opencode preset not found")
	}

	// Check command
	if info.Command != "opencode" {
		t.Errorf("opencode command = %q, want opencode", info.Command)
	}

	// Check Args (should be empty - YOLO via Env)
	if len(info.Args) != 0 {
		t.Errorf("opencode args = %v, want empty (uses Env for YOLO)", info.Args)
	}

	// Check Env for OPENCODE_PERMISSION
	if info.Env == nil {
		t.Fatal("opencode Env is nil")
	}
	permission, ok := info.Env["OPENCODE_PERMISSION"]
	if !ok {
		t.Error("opencode Env missing OPENCODE_PERMISSION")
	}
	if permission != `{"*":"allow"}` {
		t.Errorf("OPENCODE_PERMISSION = %q, want {\"*\":\"allow\"}", permission)
	}

	// Check ProcessNames for detection (opencode, node, bun)
	if len(info.ProcessNames) != 3 {
		t.Errorf("opencode ProcessNames length = %d, want 3", len(info.ProcessNames))
	}
	expectedNames := []string{"opencode", "node", "bun"}
	for i, want := range expectedNames {
		if i < len(info.ProcessNames) && info.ProcessNames[i] != want {
			t.Errorf("opencode ProcessNames[%d] = %q, want %q", i, info.ProcessNames[i], want)
		}
	}

	// Check hooks support
	if !info.SupportsHooks {
		t.Error("opencode should support hooks")
	}

	// Check fork session (not supported)
	if info.SupportsForkSession {
		t.Error("opencode should not support fork session")
	}

	// Check NonInteractive config
	if info.NonInteractive == nil {
		t.Fatal("opencode NonInteractive is nil")
	}
	if info.NonInteractive.Subcommand != "run" {
		t.Errorf("opencode NonInteractive.Subcommand = %q, want run", info.NonInteractive.Subcommand)
	}
	if info.NonInteractive.OutputFlag != "--format json" {
		t.Errorf("opencode NonInteractive.OutputFlag = %q, want --format json", info.NonInteractive.OutputFlag)
	}
}

func TestOpenCodeProviderDefaults(t *testing.T) {
	t.Parallel()

	// Test defaultReadyDelayMs for opencode
	delay := defaultReadyDelayMs("opencode")
	if delay != 8000 {
		t.Errorf("defaultReadyDelayMs(opencode) = %d, want 8000", delay)
	}

	// Test defaultProcessNames for opencode
	names := defaultProcessNames("opencode", "opencode")
	if len(names) != 2 {
		t.Errorf("defaultProcessNames(opencode) length = %d, want 2", len(names))
	}
	if names[0] != "opencode" || names[1] != "node" {
		t.Errorf("defaultProcessNames(opencode) = %v, want [opencode, node]", names)
	}

	// Test defaultInstructionsFile for opencode
	instFile := defaultInstructionsFile("opencode")
	if instFile != "AGENTS.md" {
		t.Errorf("defaultInstructionsFile(opencode) = %q, want AGENTS.md", instFile)
	}
}

func TestOpenCodeRuntimeConfigFromPreset(t *testing.T) {
	t.Parallel()
	rc := RuntimeConfigFromPreset(AgentOpenCode)
	if rc == nil {
		t.Fatal("RuntimeConfigFromPreset(opencode) returned nil")
	}

	// Check command
	if rc.Command != "opencode" {
		t.Errorf("RuntimeConfig.Command = %q, want opencode", rc.Command)
	}

	// Check Env is copied
	if rc.Env == nil {
		t.Fatal("RuntimeConfig.Env is nil")
	}
	if rc.Env["OPENCODE_PERMISSION"] != `{"*":"allow"}` {
		t.Errorf("RuntimeConfig.Env[OPENCODE_PERMISSION] = %q, want {\"*\":\"allow\"}", rc.Env["OPENCODE_PERMISSION"])
	}

	// Verify Env is a copy (mutation doesn't affect original)
	rc.Env["MUTATED"] = "yes"
	original := GetAgentPreset(AgentOpenCode)
	if _, exists := original.Env["MUTATED"]; exists {
		t.Error("Mutation of RuntimeConfig.Env affected original preset")
	}
}
