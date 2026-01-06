package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinPresets(t *testing.T) {
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
		{"aider", "", true},    // Not built-in, can be added via config
		{"opencode", "", true}, // Not built-in, can be added via config
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

func TestRuntimeConfigFromPreset(t *testing.T) {
	tests := []struct {
		preset      AgentPreset
		wantCommand string
	}{
		{AgentClaude, "claude"},
		{AgentGemini, "gemini"},
		{AgentCodex, "codex"},
		{AgentCursor, "cursor-agent"},
		{AgentAuggie, "auggie"},
		{AgentAmp, "amp"},
	}

	for _, tt := range tests {
		t.Run(string(tt.preset), func(t *testing.T) {
			rc := RuntimeConfigFromPreset(tt.preset)
			if rc.Command != tt.wantCommand {
				t.Errorf("RuntimeConfigFromPreset(%s).Command = %v, want %v",
					tt.preset, rc.Command, tt.wantCommand)
			}
		})
	}
}

func TestIsKnownPreset(t *testing.T) {
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
		{"opencode", false}, // Not built-in, can be added via config
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

	// Load the custom registry
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

	if merged.Command != "claude" {
		t.Errorf("nil config merge should get preset command, got %s", merged.Command)
	}

	// Test empty config gets preset defaults
	emptyConfig := &RuntimeConfig{}
	merged = emptyConfig.MergeWithPreset(AgentGemini)

	if merged.Command != "gemini" {
		t.Errorf("empty config merge should get preset command, got %s", merged.Command)
	}
}

func TestBuildResumeCommand(t *testing.T) {
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
		},
		{
			name:      "unknown agent",
			agentName: "unknown-agent",
			sessionID: "session-123",
			wantEmpty: true,
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
	tests := []struct {
		agentName string
		want      string
	}{
		{"claude", "CLAUDE_SESSION_ID"},
		{"gemini", "GEMINI_SESSION_ID"},
		{"codex", ""},    // Codex uses JSONL output instead
		{"cursor", ""},   // Cursor uses --resume with chatId directly
		{"auggie", ""},   // Auggie uses --resume directly
		{"amp", ""},      // AMP uses 'threads continue' subcommand
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
	tests := []struct {
		agentName string
		want      []string
	}{
		{"claude", []string{"node"}},
		{"gemini", []string{"gemini"}},
		{"codex", []string{"codex"}},
		{"cursor", []string{"cursor-agent"}},
		{"auggie", []string{"auggie"}},
		{"amp", []string{"amp"}},
		{"unknown", []string{"node"}}, // Falls back to Claude's process
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

			if rc.Command != tt.wantCommand {
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
