// Package config provides configuration types and serialization for Gas Town.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// AgentPreset identifies a supported LLM agent runtime.
// These presets provide sensible defaults that can be overridden in config.
type AgentPreset string

// Supported agent presets (built-in, E2E tested).
const (
	// AgentClaude is Claude Code (default).
	AgentClaude AgentPreset = "claude"
	// AgentGemini is Gemini CLI.
	AgentGemini AgentPreset = "gemini"
	// AgentCodex is OpenAI Codex.
	AgentCodex AgentPreset = "codex"
	// AgentCursor is Cursor Agent.
	AgentCursor AgentPreset = "cursor"
	// AgentAuggie is Auggie CLI.
	AgentAuggie AgentPreset = "auggie"
	// AgentAmp is Sourcegraph AMP.
	AgentAmp AgentPreset = "amp"
)

// AgentPresetInfo contains the configuration details for an agent preset.
// This extends the basic RuntimeConfig with agent-specific metadata.
type AgentPresetInfo struct {
	// Name is the preset identifier (e.g., "claude", "gemini", "codex", "cursor", "auggie", "amp").
	Name AgentPreset `json:"name"`

	// Command is the CLI binary to invoke.
	Command string `json:"command"`

	// Args are the default command-line arguments for autonomous mode.
	Args []string `json:"args"`

	// ProcessNames are the process names to look for when detecting if the agent is running.
	// Used by tmux.IsAgentRunning to check pane_current_command.
	// E.g., ["node"] for Claude, ["cursor-agent"] for Cursor.
	ProcessNames []string `json:"process_names,omitempty"`

	// SessionIDEnv is the environment variable for session ID.
	// Used for resuming sessions across restarts.
	SessionIDEnv string `json:"session_id_env,omitempty"`

	// ResumeFlag is the flag/subcommand for resuming sessions.
	// For claude/gemini: "--resume"
	// For codex: "resume" (subcommand)
	ResumeFlag string `json:"resume_flag,omitempty"`

	// ResumeStyle indicates how to invoke resume:
	// "flag" - pass as --resume <id> argument
	// "subcommand" - pass as 'codex resume <id>'
	ResumeStyle string `json:"resume_style,omitempty"`

	// SupportsHooks indicates if the agent supports hooks system.
	SupportsHooks bool `json:"supports_hooks,omitempty"`

	// SupportsForkSession indicates if --fork-session is available.
	// Claude-only feature for seance command.
	SupportsForkSession bool `json:"supports_fork_session,omitempty"`

	// NonInteractive contains settings for non-interactive mode.
	NonInteractive *NonInteractiveConfig `json:"non_interactive,omitempty"`
}

// NonInteractiveConfig contains settings for running agents non-interactively.
type NonInteractiveConfig struct {
	// Subcommand is the subcommand for non-interactive execution (e.g., "exec" for codex).
	Subcommand string `json:"subcommand,omitempty"`

	// PromptFlag is the flag for passing prompts (e.g., "-p" for gemini).
	PromptFlag string `json:"prompt_flag,omitempty"`

	// OutputFlag is the flag for structured output (e.g., "--json", "--output-format json").
	OutputFlag string `json:"output_flag,omitempty"`
}

// AgentRegistry contains all known agent presets.
// Can be loaded from JSON config or use built-in defaults.
type AgentRegistry struct {
	// Version is the schema version for the registry.
	Version int `json:"version"`

	// Agents maps agent names to their configurations.
	Agents map[string]*AgentPresetInfo `json:"agents"`
}

// CurrentAgentRegistryVersion is the current schema version.
const CurrentAgentRegistryVersion = 1

// builtinPresets contains the default presets for supported agents.
var builtinPresets = map[AgentPreset]*AgentPresetInfo{
	AgentClaude: {
		Name:                AgentClaude,
		Command:             "claude",
		Args:                []string{"--dangerously-skip-permissions"},
		ProcessNames:        []string{"node"}, // Claude runs as Node.js
		SessionIDEnv:        "CLAUDE_SESSION_ID",
		ResumeFlag:          "--resume",
		ResumeStyle:         "flag",
		SupportsHooks:       true,
		SupportsForkSession: true,
		NonInteractive:      nil, // Claude is native non-interactive
	},
	AgentGemini: {
		Name:                AgentGemini,
		Command:             "gemini",
		Args:                []string{"--approval-mode", "yolo"},
		ProcessNames:        []string{"gemini"}, // Gemini CLI binary
		SessionIDEnv:        "GEMINI_SESSION_ID",
		ResumeFlag:          "--resume",
		ResumeStyle:         "flag",
		SupportsHooks:       true,
		SupportsForkSession: false,
		NonInteractive: &NonInteractiveConfig{
			PromptFlag: "-p",
			OutputFlag: "--output-format json",
		},
	},
	AgentCodex: {
		Name:                AgentCodex,
		Command:             "codex",
		Args:                []string{"--yolo"},
		ProcessNames:        []string{"codex"}, // Codex CLI binary
		SessionIDEnv:        "", // Codex captures from JSONL output
		ResumeFlag:          "resume",
		ResumeStyle:         "subcommand",
		SupportsHooks:       false, // Use env/files instead
		SupportsForkSession: false,
		NonInteractive: &NonInteractiveConfig{
			Subcommand: "exec",
			OutputFlag: "--json",
		},
	},
	AgentCursor: {
		Name:                AgentCursor,
		Command:             "cursor-agent",
		Args:                []string{"-f"}, // Force mode (YOLO equivalent), -p requires prompt
		ProcessNames:        []string{"cursor-agent"},
		SessionIDEnv:        "", // Uses --resume with chatId directly
		ResumeFlag:          "--resume",
		ResumeStyle:         "flag",
		SupportsHooks:       false, // TODO: verify hooks support
		SupportsForkSession: false,
		NonInteractive: &NonInteractiveConfig{
			PromptFlag: "-p",
			OutputFlag: "--output-format json",
		},
	},
	AgentAuggie: {
		Name:                AgentAuggie,
		Command:             "auggie",
		Args:                []string{"--allow-indexing"},
		ProcessNames:        []string{"auggie"},
		SessionIDEnv:        "",
		ResumeFlag:          "--resume",
		ResumeStyle:         "flag",
		SupportsHooks:       false,
		SupportsForkSession: false,
	},
	AgentAmp: {
		Name:                AgentAmp,
		Command:             "amp",
		Args:                []string{"--dangerously-allow-all", "--no-ide"},
		ProcessNames:        []string{"amp"},
		SessionIDEnv:        "",
		ResumeFlag:          "threads continue",
		ResumeStyle:         "subcommand", // 'amp threads continue <threadId>'
		SupportsHooks:       false,
		SupportsForkSession: false,
	},
}

// Registry state with proper synchronization.
var (
	// registryMu protects all registry state.
	registryMu sync.RWMutex
	// globalRegistry is the merged registry of built-in and user-defined agents.
	globalRegistry *AgentRegistry
	// loadedPaths tracks which config files have been loaded to avoid redundant reads.
	loadedPaths = make(map[string]bool)
	// registryInitialized tracks if builtins have been copied.
	registryInitialized bool
)

// initRegistry initializes the global registry with built-in presets.
// Caller must hold registryMu write lock.
func initRegistryLocked() {
	if registryInitialized {
		return
	}
	globalRegistry = &AgentRegistry{
		Version: CurrentAgentRegistryVersion,
		Agents:  make(map[string]*AgentPresetInfo),
	}
	// Copy built-in presets
	for name, preset := range builtinPresets {
		globalRegistry.Agents[string(name)] = preset
	}
	registryInitialized = true
}

// ensureRegistry ensures the registry is initialized for read operations.
func ensureRegistry() {
	registryMu.Lock()
	defer registryMu.Unlock()
	initRegistryLocked()
}

// LoadAgentRegistry loads agent definitions from a JSON file and merges with built-ins.
// User-defined agents override built-in presets with the same name.
// This function caches loaded paths to avoid redundant file reads.
func LoadAgentRegistry(path string) error {
	registryMu.Lock()
	defer registryMu.Unlock()

	initRegistryLocked()

	// Check if already loaded from this path
	if loadedPaths[path] {
		return nil
	}

	data, err := os.ReadFile(path) //nolint:gosec // G304: path is from config
	if err != nil {
		if os.IsNotExist(err) {
			loadedPaths[path] = true // Mark as "loaded" (no file)
			return nil               // No custom config, use built-ins only
		}
		return err
	}

	var userRegistry AgentRegistry
	if err := json.Unmarshal(data, &userRegistry); err != nil {
		return err
	}

	// Merge user-defined agents (override built-ins)
	for name, preset := range userRegistry.Agents {
		preset.Name = AgentPreset(name)
		globalRegistry.Agents[name] = preset
	}

	loadedPaths[path] = true
	return nil
}

// DefaultAgentRegistryPath returns the default path for agent registry.
// Located alongside other town settings.
func DefaultAgentRegistryPath(townRoot string) string {
	return filepath.Join(townRoot, "settings", "agents.json")
}

// GetAgentPreset returns the preset info for a given agent name.
// Returns nil if the preset is not found.
func GetAgentPreset(name AgentPreset) *AgentPresetInfo {
	ensureRegistry()
	registryMu.RLock()
	defer registryMu.RUnlock()
	return globalRegistry.Agents[string(name)]
}

// GetAgentPresetByName returns the preset info by string name.
// Returns nil if not found, allowing caller to fall back to defaults.
func GetAgentPresetByName(name string) *AgentPresetInfo {
	ensureRegistry()
	registryMu.RLock()
	defer registryMu.RUnlock()
	return globalRegistry.Agents[name]
}

// ListAgentPresets returns all known agent preset names.
func ListAgentPresets() []string {
	ensureRegistry()
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(globalRegistry.Agents))
	for name := range globalRegistry.Agents {
		names = append(names, name)
	}
	return names
}

// DefaultAgentPreset returns the default agent preset (Claude).
func DefaultAgentPreset() AgentPreset {
	return AgentClaude
}

// RuntimeConfigFromPreset creates a RuntimeConfig from an agent preset.
// This provides the basic Command/Args; additional fields from AgentPresetInfo
// can be accessed separately for extended functionality.
func RuntimeConfigFromPreset(preset AgentPreset) *RuntimeConfig {
	info := GetAgentPreset(preset)
	if info == nil {
		// Fall back to Claude defaults
		return DefaultRuntimeConfig()
	}

	return &RuntimeConfig{
		Command: info.Command,
		Args:    append([]string(nil), info.Args...), // Copy to avoid mutation
	}
}

// BuildResumeCommand builds a command to resume an agent session.
// Returns the full command string including any YOLO/autonomous flags.
// If sessionID is empty or the agent doesn't support resume, returns empty string.
func BuildResumeCommand(agentName, sessionID string) string {
	if sessionID == "" {
		return ""
	}

	info := GetAgentPresetByName(agentName)
	if info == nil || info.ResumeFlag == "" {
		return ""
	}

	// Build base command with args
	args := append([]string(nil), info.Args...)

	// Add resume based on style
	switch info.ResumeStyle {
	case "subcommand":
		// e.g., "codex resume <session_id> --yolo"
		return info.Command + " " + info.ResumeFlag + " " + sessionID + " " + strings.Join(args, " ")
	case "flag":
		fallthrough
	default:
		// e.g., "claude --dangerously-skip-permissions --resume <session_id>"
		args = append(args, info.ResumeFlag, sessionID)
		return info.Command + " " + strings.Join(args, " ")
	}
}

// SupportsSessionResume checks if an agent supports session resumption.
func SupportsSessionResume(agentName string) bool {
	info := GetAgentPresetByName(agentName)
	return info != nil && info.ResumeFlag != ""
}

// GetSessionIDEnvVar returns the environment variable name for storing session IDs
// for a given agent. Returns empty string if the agent doesn't use env vars for this.
func GetSessionIDEnvVar(agentName string) string {
	info := GetAgentPresetByName(agentName)
	if info == nil {
		return ""
	}
	return info.SessionIDEnv
}

// GetProcessNames returns the process names used to detect if an agent is running.
// Used by tmux.IsAgentRunning to check pane_current_command.
// Returns ["node"] for Claude (default) if agent is not found or has no ProcessNames.
func GetProcessNames(agentName string) []string {
	info := GetAgentPresetByName(agentName)
	if info == nil || len(info.ProcessNames) == 0 {
		// Default to Claude's process name for backwards compatibility
		return []string{"node"}
	}
	return info.ProcessNames
}

// MergeWithPreset applies preset defaults to a RuntimeConfig.
// User-specified values take precedence over preset defaults.
// Returns a new RuntimeConfig without modifying the original.
func (rc *RuntimeConfig) MergeWithPreset(preset AgentPreset) *RuntimeConfig {
	if rc == nil {
		return RuntimeConfigFromPreset(preset)
	}

	info := GetAgentPreset(preset)
	if info == nil {
		return rc
	}

	result := &RuntimeConfig{
		Command:       rc.Command,
		Args:          append([]string(nil), rc.Args...),
		InitialPrompt: rc.InitialPrompt,
	}

	// Apply preset defaults only if not overridden
	if result.Command == "" {
		result.Command = info.Command
	}
	if len(result.Args) == 0 {
		result.Args = append([]string(nil), info.Args...)
	}

	return result
}

// IsKnownPreset checks if a string is a known agent preset name.
func IsKnownPreset(name string) bool {
	ensureRegistry()
	registryMu.RLock()
	defer registryMu.RUnlock()
	_, ok := globalRegistry.Agents[name]
	return ok
}

// SaveAgentRegistry writes the agent registry to a file.
func SaveAgentRegistry(path string, registry *AgentRegistry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644) //nolint:gosec // G306: config file
}

// NewExampleAgentRegistry creates an example registry with comments.
func NewExampleAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		Version: CurrentAgentRegistryVersion,
		Agents: map[string]*AgentPresetInfo{
			// Include one example custom agent
			"my-custom-agent": {
				Name:         "my-custom-agent",
				Command:      "my-agent-cli",
				Args:         []string{"--autonomous", "--no-confirm"},
				SessionIDEnv: "MY_AGENT_SESSION_ID",
				ResumeFlag:   "--resume",
				ResumeStyle:  "flag",
				NonInteractive: &NonInteractiveConfig{
					PromptFlag: "-m",
					OutputFlag: "--json",
				},
			},
		},
	}
}

// ResetRegistryForTesting clears all registry state.
// This is intended for use in tests only to ensure test isolation.
func ResetRegistryForTesting() {
	registryMu.Lock()
	defer registryMu.Unlock()
	globalRegistry = nil
	loadedPaths = make(map[string]bool)
	registryInitialized = false
}
