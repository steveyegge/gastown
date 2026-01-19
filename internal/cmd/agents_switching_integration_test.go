//go:build integration

// Package cmd contains integration tests for agent switching functionality.
//
// Run with: go test -tags=integration ./internal/cmd -run TestAgents -v
package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

// TestCategorizeSession verifies that tmux session names are correctly
// categorized into agent types.
func TestCategorizeSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sessionName string
		wantType    AgentType
		wantRig     string
		wantAgent   string
		wantNil     bool
	}{
		// Town-level agents (hq- prefix)
		{
			name:        "mayor session",
			sessionName: "hq-mayor",
			wantType:    AgentMayor,
			wantRig:     "",
			wantAgent:   "",
		},
		{
			name:        "deacon session",
			sessionName: "hq-deacon",
			wantType:    AgentDeacon,
			wantRig:     "",
			wantAgent:   "",
		},
		{
			name:        "unknown hq session",
			sessionName: "hq-unknown",
			wantNil:     true,
		},

		// Rig-level agents (gt-<rig>-<type>)
		{
			name:        "witness session",
			sessionName: "gt-gastown-witness",
			wantType:    AgentWitness,
			wantRig:     "gastown",
			wantAgent:   "",
		},
		{
			name:        "refinery session",
			sessionName: "gt-myrig-refinery",
			wantType:    AgentRefinery,
			wantRig:     "myrig",
			wantAgent:   "",
		},
		{
			name:        "crew session",
			sessionName: "gt-gastown-crew-alice",
			wantType:    AgentCrew,
			wantRig:     "gastown",
			wantAgent:   "alice",
		},
		{
			name:        "crew session with dashes in name",
			sessionName: "gt-myrig-crew-bob-worker",
			wantType:    AgentCrew,
			wantRig:     "myrig",
			wantAgent:   "bob-worker",
		},

		// Polecat sessions (gt-<rig>-<name>)
		{
			name:        "polecat session",
			sessionName: "gt-gastown-rictus",
			wantType:    AgentPolecat,
			wantRig:     "gastown",
			wantAgent:   "rictus",
		},
		{
			name:        "polecat session with dashes",
			sessionName: "gt-myrig-slit-worker",
			wantType:    AgentPolecat,
			wantRig:     "myrig",
			wantAgent:   "slit-worker",
		},

		// Legacy witness format
		{
			name:        "legacy witness session",
			sessionName: "gt-witness-gastown",
			wantType:    AgentWitness,
			wantRig:     "gastown",
			wantAgent:   "",
		},

		// Non-matching sessions
		{
			name:        "non-gt session",
			sessionName: "my-random-session",
			wantNil:     true,
		},
		{
			name:        "gt with no rig",
			sessionName: "gt-",
			wantNil:     true,
		},
		{
			name:        "gt with only rig",
			sessionName: "gt-gastown",
			wantNil:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := categorizeSession(tt.sessionName)

			if tt.wantNil {
				if result != nil {
					t.Errorf("categorizeSession(%q) = %+v, want nil", tt.sessionName, result)
				}
				return
			}

			if result == nil {
				t.Fatalf("categorizeSession(%q) = nil, want non-nil", tt.sessionName)
			}

			if result.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", result.Type, tt.wantType)
			}
			if result.Rig != tt.wantRig {
				t.Errorf("Rig = %q, want %q", result.Rig, tt.wantRig)
			}
			if result.AgentName != tt.wantAgent {
				t.Errorf("AgentName = %q, want %q", result.AgentName, tt.wantAgent)
			}
		})
	}
}

// TestAgentDisplayLabel verifies that display labels are generated correctly
// for the tmux menu.
func TestAgentDisplayLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		session *AgentSession
		wantContains []string
	}{
		{
			name: "mayor label",
			session: &AgentSession{
				Name: "hq-mayor",
				Type: AgentMayor,
			},
			wantContains: []string{"Mayor"},
		},
		{
			name: "deacon label",
			session: &AgentSession{
				Name: "hq-deacon",
				Type: AgentDeacon,
			},
			wantContains: []string{"Deacon"},
		},
		{
			name: "witness label",
			session: &AgentSession{
				Name: "gt-gastown-witness",
				Type: AgentWitness,
				Rig:  "gastown",
			},
			wantContains: []string{"gastown", "witness"},
		},
		{
			name: "refinery label",
			session: &AgentSession{
				Name: "gt-myrig-refinery",
				Type: AgentRefinery,
				Rig:  "myrig",
			},
			wantContains: []string{"myrig", "refinery"},
		},
		{
			name: "crew label",
			session: &AgentSession{
				Name:      "gt-gastown-crew-alice",
				Type:      AgentCrew,
				Rig:       "gastown",
				AgentName: "alice",
			},
			wantContains: []string{"gastown", "crew", "alice"},
		},
		{
			name: "polecat label",
			session: &AgentSession{
				Name:      "gt-gastown-rictus",
				Type:      AgentPolecat,
				Rig:       "gastown",
				AgentName: "rictus",
			},
			wantContains: []string{"gastown", "rictus"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label := tt.session.displayLabel()
			for _, s := range tt.wantContains {
				if !containsString(label, s) {
					t.Errorf("displayLabel() = %q, missing %q", label, s)
				}
			}
		})
	}
}

// TestShortcutKeyGeneration verifies that keyboard shortcuts are assigned correctly.
func TestShortcutKeyGeneration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		index int
		want  string
	}{
		{0, "1"},
		{1, "2"},
		{8, "9"},
		{9, "a"},
		{10, "b"},
		{34, "z"},
		{35, ""}, // Beyond alphabet
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := shortcutKey(tt.index)
			if got != tt.want {
				t.Errorf("shortcutKey(%d) = %q, want %q", tt.index, got, tt.want)
			}
		})
	}
}

// TestAgentTypeIcons verifies that all agent types have icons defined.
func TestAgentTypeIcons(t *testing.T) {
	t.Parallel()

	types := []AgentType{
		AgentMayor,
		AgentDeacon,
		AgentWitness,
		AgentRefinery,
		AgentCrew,
		AgentPolecat,
	}

	for _, agentType := range types {
		icon, ok := AgentTypeIcons[agentType]
		if !ok {
			t.Errorf("AgentTypeIcons missing type %v", agentType)
			continue
		}
		if icon == "" {
			t.Errorf("AgentTypeIcons[%v] is empty", agentType)
		}
	}
}

// TestAgentTypeColors verifies that all agent types have colors defined.
func TestAgentTypeColors(t *testing.T) {
	t.Parallel()

	types := []AgentType{
		AgentMayor,
		AgentDeacon,
		AgentWitness,
		AgentRefinery,
		AgentCrew,
		AgentPolecat,
	}

	for _, agentType := range types {
		color, ok := AgentTypeColors[agentType]
		if !ok {
			t.Errorf("AgentTypeColors missing type %v", agentType)
			continue
		}
		if color == "" {
			t.Errorf("AgentTypeColors[%v] is empty", agentType)
		}
	}
}

// TestAgentPresetSwitching verifies that agent presets can be switched correctly.
func TestAgentPresetSwitching(t *testing.T) {
	t.Parallel()

	// Verify all built-in presets are available
	presets := []config.AgentPreset{
		config.AgentClaude,
		config.AgentGemini,
		config.AgentCodex,
		config.AgentCursor,
		config.AgentAuggie,
		config.AgentAmp,
		config.AgentOpenCode,
	}

	for _, preset := range presets {
		t.Run(string(preset), func(t *testing.T) {
			info := config.GetAgentPreset(preset)
			if info == nil {
				t.Fatalf("GetAgentPreset(%s) returned nil", preset)
			}

			// All presets must have a command
			if info.Command == "" {
				t.Errorf("preset %s has empty Command", preset)
			}

			// All presets must have ProcessNames for detection
			if len(info.ProcessNames) == 0 {
				t.Errorf("preset %s has empty ProcessNames", preset)
			}

			// Verify runtime config can be generated
			rc := config.RuntimeConfigFromPreset(preset)
			if rc == nil {
				t.Errorf("RuntimeConfigFromPreset(%s) returned nil", preset)
			}
			if rc.Command != info.Command {
				t.Errorf("RuntimeConfig.Command = %q, want %q", rc.Command, info.Command)
			}
		})
	}
}

// TestAgentSwitchingWithCustomRegistry verifies that custom agents can be
// registered and used for switching.
func TestAgentSwitchingWithCustomRegistry(t *testing.T) {
	// Reset registry for test isolation
	config.ResetRegistryForTesting()
	t.Cleanup(config.ResetRegistryForTesting)

	// Create temp directory with custom agent config
	tmpDir := t.TempDir()
	registryPath := filepath.Join(tmpDir, "agents.json")

	registryContent := `{
  "version": 1,
  "agents": {
    "my-custom-llm": {
      "command": "my-llm-cli",
      "args": ["--autonomous", "--no-confirm"],
      "process_names": ["my-llm-cli"],
      "resume_flag": "--resume",
      "resume_style": "flag"
    }
  }
}`
	if err := os.WriteFile(registryPath, []byte(registryContent), 0644); err != nil {
		t.Fatalf("failed to write registry: %v", err)
	}

	// Load the custom registry
	if err := config.LoadAgentRegistry(registryPath); err != nil {
		t.Fatalf("LoadAgentRegistry failed: %v", err)
	}

	// Verify custom agent is available
	customAgent := config.GetAgentPresetByName("my-custom-llm")
	if customAgent == nil {
		t.Fatal("custom agent 'my-custom-llm' not found")
	}

	if customAgent.Command != "my-llm-cli" {
		t.Errorf("custom agent Command = %q, want %q", customAgent.Command, "my-llm-cli")
	}

	// Verify built-in agents are still available
	claude := config.GetAgentPresetByName("claude")
	if claude == nil {
		t.Error("built-in 'claude' agent not found after loading custom registry")
	}

	// Verify session resume can be built for custom agent
	resumeCmd := config.BuildResumeCommand("my-custom-llm", "session-123")
	if resumeCmd == "" {
		t.Error("BuildResumeCommand returned empty for custom agent")
	}
	if !containsString(resumeCmd, "my-llm-cli") {
		t.Errorf("resume command missing cli: %q", resumeCmd)
	}
	if !containsString(resumeCmd, "--resume") {
		t.Errorf("resume command missing --resume: %q", resumeCmd)
	}
	if !containsString(resumeCmd, "session-123") {
		t.Errorf("resume command missing session ID: %q", resumeCmd)
	}
}

// TestAgentProcessDetection verifies that process names are correctly returned
// for agent detection.
func TestAgentProcessDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		agentName string
		wantNames []string
	}{
		{"claude", []string{"node"}},
		{"gemini", []string{"gemini"}},
		{"codex", []string{"codex"}},
		{"cursor", []string{"cursor-agent"}},
		{"auggie", []string{"auggie"}},
		{"amp", []string{"amp"}},
		{"opencode", []string{"opencode"}},
		{"unknown", []string{"node"}}, // Falls back to Claude's process
	}

	for _, tt := range tests {
		t.Run(tt.agentName, func(t *testing.T) {
			got := config.GetProcessNames(tt.agentName)
			if len(got) != len(tt.wantNames) {
				t.Errorf("GetProcessNames(%q) = %v, want %v", tt.agentName, got, tt.wantNames)
				return
			}
			for i := range got {
				if got[i] != tt.wantNames[i] {
					t.Errorf("GetProcessNames(%q)[%d] = %q, want %q",
						tt.agentName, i, got[i], tt.wantNames[i])
				}
			}
		})
	}
}

// TestGuessSessionFromWorkerDir verifies that worker directory paths are
// correctly mapped to tmux session names.
func TestGuessSessionFromWorkerDir(t *testing.T) {
	t.Parallel()

	townRoot := "/home/user/gastown"

	tests := []struct {
		name      string
		workerDir string
		want      string
	}{
		{
			name:      "crew worker",
			workerDir: "/home/user/gastown/myrig/crew/alice",
			want:      "gt-myrig-crew-alice",
		},
		{
			name:      "polecat worker",
			workerDir: "/home/user/gastown/myrig/polecats/rictus",
			want:      "gt-myrig-rictus",
		},
		{
			name:      "invalid path - too short",
			workerDir: "/home/user/gastown/myrig",
			want:      "",
		},
		{
			name:      "unknown worker type",
			workerDir: "/home/user/gastown/myrig/unknown/worker",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := guessSessionFromWorkerDir(tt.workerDir, townRoot)
			if got != tt.want {
				t.Errorf("guessSessionFromWorkerDir(%q, %q) = %q, want %q",
					tt.workerDir, townRoot, got, tt.want)
			}
		})
	}
}

// containsString is a helper to check if a string contains a substring.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
