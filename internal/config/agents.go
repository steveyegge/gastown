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
	// AgentOpenCode is OpenCode multi-model CLI.
	AgentOpenCode AgentPreset = "opencode"
	// AgentCopilot is GitHub Copilot CLI.
	AgentCopilot AgentPreset = "copilot"
	// AgentPi is Pi Coding Agent (extension-based lifecycle).
	AgentPi AgentPreset = "pi"
)

// AgentPresetInfo contains the configuration details for an agent preset.
// This is the single source of truth for all agent-specific behavior.
// Adding a new agent = adding a builtinPresets entry + optional hook installer.
// No provider-string switch statements should exist outside this registry.
type AgentPresetInfo struct {
	// Name is the preset identifier (e.g., "claude", "gemini", "codex", "cursor", "auggie", "amp", "copilot").
	Name AgentPreset `json:"name"`

	// Command is the CLI binary to invoke.
	Command string `json:"command"`

	// Args are the default command-line arguments for autonomous mode.
	Args []string `json:"args"`

	// Env are environment variables to set when starting the agent.
	// These are merged with the standard GT_* variables.
	// Used for agent-specific configuration like OPENCODE_PERMISSION.
	Env map[string]string `json:"env,omitempty"`

	// ProcessNames are the process names to look for when detecting if the agent is running.
	// Used by tmux.IsAgentRunning to check pane_current_command.
	// E.g., ["node"] for Claude, ["cursor-agent"] for Cursor.
	ProcessNames []string `json:"process_names,omitempty"`

	// SessionIDEnv is the environment variable for session ID.
	// Used for resuming sessions across restarts.
	SessionIDEnv string `json:"session_id_env,omitempty"`

	// ResumeFlag is the flag/subcommand for resuming a specific session.
	// For claude/gemini: "--resume"
	// For codex: "resume" (subcommand)
	ResumeFlag string `json:"resume_flag,omitempty"`

	// ContinueFlag is the flag for auto-resuming the most recent session.
	// For claude: "--continue" (--resume without args opens interactive picker)
	// If empty, --resume without a session ID is rejected with a clear error.
	ContinueFlag string `json:"continue_flag,omitempty"`

	// ResumeStyle indicates how to invoke resume:
	// "flag" - pass as --resume <id> argument
	// "subcommand" - pass as 'codex resume <id>'
	ResumeStyle string `json:"resume_style,omitempty"`

	// SupportsHooks indicates if the agent supports hooks system.
	SupportsHooks bool `json:"supports_hooks,omitempty"`

	// SupportsForkSession indicates if --fork-session is available.
	// Used by the seance command for session forking.
	SupportsForkSession bool `json:"supports_fork_session,omitempty"`

	// NonInteractive contains settings for non-interactive mode.
	NonInteractive *NonInteractiveConfig `json:"non_interactive,omitempty"`

	// --- Runtime default fields (replaces scattered default*() switch statements) ---

	// PromptMode controls how the initial prompt is delivered: "arg" or "none".
	// Defaults to "arg" if empty.
	PromptMode string `json:"prompt_mode,omitempty"`

	// ConfigDirEnv is the env var for the agent's config directory (e.g., "CLAUDE_CONFIG_DIR").
	ConfigDirEnv string `json:"config_dir_env,omitempty"`

	// ConfigDir is the top-level config directory (e.g., ".claude", ".opencode").
	// Used for slash command provisioning. Empty means no command provisioning.
	ConfigDir string `json:"config_dir,omitempty"`

	// HooksProvider is the hooks framework provider type (e.g., "claude", "opencode").
	// Empty or "none" means no hooks support.
	HooksProvider string `json:"hooks_provider,omitempty"`

	// HooksDir is the directory for hooks/settings (e.g., ".claude", ".opencode/plugins").
	HooksDir string `json:"hooks_dir,omitempty"`

	// HooksSettingsFile is the settings/plugin filename (e.g., "settings.json", "gastown.js").
	HooksSettingsFile string `json:"hooks_settings_file,omitempty"`

	// HooksInformational indicates hooks are instructions-only (not executable lifecycle hooks).
	// For these providers, Gas Town sends startup fallback commands via nudge.
	HooksInformational bool `json:"hooks_informational,omitempty"`

	// ReadyPromptPrefix is the prompt prefix for tmux readiness detection (e.g., "❯ ").
	// Empty means delay-based detection only.
	ReadyPromptPrefix string `json:"ready_prompt_prefix,omitempty"`

	// ReadyDelayMs is the delay-based readiness fallback in milliseconds.
	ReadyDelayMs int `json:"ready_delay_ms,omitempty"`

	// InstructionsFile is the instructions file for this agent (e.g., "CLAUDE.md", "AGENTS.md").
	// Defaults to "AGENTS.md" if empty.
	InstructionsFile string `json:"instructions_file,omitempty"`

	// EmitsPermissionWarning indicates the agent shows a bypass-permissions warning on startup
	// that needs to be acknowledged via tmux.
	EmitsPermissionWarning bool `json:"emits_permission_warning,omitempty"`
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
// Each preset is the single source of truth for its agent's behavior.
var builtinPresets = map[AgentPreset]*AgentPresetInfo{
	AgentClaude: {
		Name:                AgentClaude,
		Command:             "claude",
		Args:                []string{"--dangerously-skip-permissions"},
		ProcessNames:        []string{"node", "claude"}, // Claude runs as Node.js
		SessionIDEnv:        "CLAUDE_SESSION_ID",
		ResumeFlag:          "--resume",
		ContinueFlag:        "--continue",
		ResumeStyle:         "flag",
		SupportsHooks:       true,
		SupportsForkSession: true,
		NonInteractive:      nil, // Claude is native non-interactive
		// Runtime defaults
		PromptMode:             "arg",
		ConfigDirEnv:           "CLAUDE_CONFIG_DIR",
		ConfigDir:              ".claude",
		HooksProvider:          "claude",
		HooksDir:               ".claude",
		HooksSettingsFile:      "settings.json",
		ReadyPromptPrefix:      "❯ ",
		ReadyDelayMs:           10000,
		InstructionsFile:       "CLAUDE.md",
		EmitsPermissionWarning: true,
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
		// Runtime defaults
		PromptMode:        "arg",
		ConfigDir:         ".gemini",
		HooksProvider:     "gemini",
		HooksDir:          ".gemini",
		HooksSettingsFile: "settings.json",
		ReadyDelayMs:      5000,
		InstructionsFile:  "AGENTS.md",
	},
	AgentCodex: {
		Name:                AgentCodex,
		Command:             "codex",
		Args:                []string{"--dangerously-bypass-approvals-and-sandbox"},
		ProcessNames:        []string{"codex"}, // Codex CLI binary
		SessionIDEnv:        "",                 // Codex captures from JSONL output
		ResumeFlag:          "resume",
		ResumeStyle:         "subcommand",
		SupportsHooks:       false, // Use env/files instead
		SupportsForkSession: false,
		NonInteractive: &NonInteractiveConfig{
			Subcommand: "exec",
			OutputFlag: "--json",
		},
		// Runtime defaults
		PromptMode:       "none",
		ReadyDelayMs:     3000,
		InstructionsFile: "AGENTS.md",
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
		// Runtime defaults
		PromptMode:       "arg",
		InstructionsFile: "AGENTS.md",
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
		// Runtime defaults
		PromptMode:       "arg",
		InstructionsFile: "AGENTS.md",
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
		// Runtime defaults
		PromptMode:       "arg",
		InstructionsFile: "AGENTS.md",
	},
	AgentOpenCode: {
		Name:    AgentOpenCode,
		Command: "opencode",
		Args:    []string{}, // No CLI flags needed, YOLO via OPENCODE_PERMISSION env
		Env: map[string]string{
			// Auto-approve all tool calls (equivalent to --dangerously-skip-permissions)
			"OPENCODE_PERMISSION": `{"*":"allow"}`,
		},
		ProcessNames:        []string{"opencode", "node", "bun"}, // Runs as Node.js or Bun
		SessionIDEnv:        "",                                   // OpenCode manages sessions internally
		ResumeFlag:          "",                                   // No resume support yet
		ResumeStyle:         "",
		SupportsHooks:       true, // Uses .opencode/plugins/gastown.js
		SupportsForkSession: false,
		NonInteractive: &NonInteractiveConfig{
			Subcommand: "run",
			OutputFlag: "--format json",
		},
		// Runtime defaults
		PromptMode:        "arg",
		ConfigDir:         ".opencode",
		HooksProvider:     "opencode",
		HooksDir:          ".opencode/plugins",
		HooksSettingsFile: "gastown.js",
		ReadyDelayMs:      8000,
		InstructionsFile:  "AGENTS.md",
	},
	AgentCopilot: {
		Name:                AgentCopilot,
		Command:             "copilot",
		Args:                []string{"--yolo"},
		ProcessNames:        []string{"copilot"}, // Copilot CLI binary (Node.js but reports as "copilot")
		SessionIDEnv:        "",                   // Session IDs stored on disk, not in env
		ResumeFlag:          "--resume",
		ResumeStyle:         "flag",
		SupportsHooks:       false, // Copilot instructions file is not executable hooks
		SupportsForkSession: false,
		NonInteractive: &NonInteractiveConfig{
			PromptFlag: "-p",
		},
		// Runtime defaults
		PromptMode:         "arg",
		ConfigDir:          ".copilot",
		HooksProvider:      "copilot",
		HooksDir:           ".copilot",
		HooksSettingsFile:  "copilot-instructions.md",
		HooksInformational: true,
		ReadyPromptPrefix:  "❯ ",
		ReadyDelayMs:       5000,
		InstructionsFile:   "AGENTS.md",
	},
	AgentPi: {
		Name:                AgentPi,
		Command:             "pi",
		Args:                []string{}, // Extension loaded via -e flag in town settings
		ProcessNames:        []string{"pi", "node", "bun"}, // Pi runs as Node.js
		SessionIDEnv:        "PI_SESSION_ID",
		ResumeFlag:          "",    // No resume support yet
		ResumeStyle:         "",
		SupportsHooks:       true,  // Uses .pi/extensions/gastown-hooks.js
		HooksProvider:       "pi",
		HooksDir:            ".pi/extensions",
		HooksSettingsFile:   "gastown-hooks.js",
		SupportsForkSession: false,
		NonInteractive: &NonInteractiveConfig{
			PromptFlag: "-p",
			OutputFlag: "--no-session",
		},
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

// loadAgentRegistryFromPath loads agent definitions from a JSON file and merges with built-ins.
// Caller must hold registryMu write lock.
func loadAgentRegistryFromPathLocked(path string) error {
	initRegistryLocked()

	if loadedPaths[path] {
		return nil
	}

	data, err := os.ReadFile(path) //nolint:gosec // G304: path is from config
	if err != nil {
		if os.IsNotExist(err) {
			// Don't cache non-existent paths — the file may be created later
			// and we need to pick it up on the next load call.
			return nil
		}
		return err
	}

	var userRegistry AgentRegistry
	if err := json.Unmarshal(data, &userRegistry); err != nil {
		return err
	}

	for name, preset := range userRegistry.Agents {
		preset.Name = AgentPreset(name)
		globalRegistry.Agents[name] = preset
	}

	loadedPaths[path] = true
	return nil
}

// LoadAgentRegistry loads agent definitions from a JSON file and merges with built-ins.
// User-defined agents override built-in presets with the same name.
// This function caches loaded paths to avoid redundant file reads.
func LoadAgentRegistry(path string) error {
	registryMu.Lock()
	defer registryMu.Unlock()
	return loadAgentRegistryFromPathLocked(path)
}

// DefaultAgentRegistryPath returns the default path for agent registry.
// Located alongside other town settings.
func DefaultAgentRegistryPath(townRoot string) string {
	return filepath.Join(townRoot, "settings", "agents.json")
}

// DefaultRigAgentRegistryPath returns the default path for rig-level agent registry.
// Located in <rig>/settings/agents.json.
func DefaultRigAgentRegistryPath(rigPath string) string {
	return filepath.Join(rigPath, "settings", "agents.json")
}

// RigAgentRegistryPath returns the path for rig-level agent registry.
// Alias for DefaultRigAgentRegistryPath for consistency with other path functions.
func RigAgentRegistryPath(rigPath string) string {
	return DefaultRigAgentRegistryPath(rigPath)
}

// LoadRigAgentRegistry loads agent definitions from a rig-level JSON file and merges with built-ins.
// This function works similarly to LoadAgentRegistry but for rig-level configurations.
func LoadRigAgentRegistry(path string) error {
	registryMu.Lock()
	defer registryMu.Unlock()
	return loadAgentRegistryFromPathLocked(path)
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
// This provides the basic Command/Args/Env; additional fields from AgentPresetInfo
// can be accessed separately for extended functionality.
func RuntimeConfigFromPreset(preset AgentPreset) *RuntimeConfig {
	info := GetAgentPreset(preset)
	if info == nil {
		// Fall back to Claude defaults
		return DefaultRuntimeConfig()
	}

	// Copy Env map to avoid mutation
	var envCopy map[string]string
	if len(info.Env) > 0 {
		envCopy = make(map[string]string, len(info.Env))
		for k, v := range info.Env {
			envCopy[k] = v
		}
	}

	rc := &RuntimeConfig{
		Provider: string(info.Name),
		Command: info.Command,
		Args:    append([]string(nil), info.Args...), // Copy to avoid mutation
		Env:     envCopy,
	}

	// Resolve command path for claude preset (handles alias installations)
	// Uses resolveClaudePath() from types.go which finds ~/.claude/local/claude
	if preset == AgentClaude && rc.Command == "claude" {
		rc.Command = resolveClaudePath()
	}

	return normalizeRuntimeConfig(rc)
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
		// e.g., "codex resume <session_id> --dangerously-bypass-approvals-and-sandbox"
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
		// Default to Claude's process names for backwards compatibility
		return []string{"node", "claude"}
	}
	return info.ProcessNames
}

// ResolveProcessNames determines the correct process names for liveness detection
// given an agent name and the actual command binary. This handles custom agents
// that shadow built-in preset names (e.g., a custom "codex" agent that runs
// "opencode" instead of the built-in "codex" binary).
//
// Resolution order:
//  1. If agentName matches a built-in preset AND the preset's Command matches
//     the actual command → use the preset's ProcessNames (no mismatch).
//  2. Otherwise, find a built-in preset whose Command matches the actual command
//     and use its ProcessNames (custom agent using a known launcher).
//  3. Fallback: [command] (fully custom binary).
func ResolveProcessNames(agentName, command string) []string {
	ensureRegistry()
	registryMu.RLock()
	defer registryMu.RUnlock()

	// Normalize command to basename for comparison. Commands may be
	// path-resolved (e.g., "/home/user/.claude/local/claude" from
	// resolveClaudePath), but built-in presets store bare names ("claude").
	// Process matching (processMatchesNames, pgrep) also uses basenames.
	cmdBase := command
	if command != "" {
		cmdBase = filepath.Base(command)
	}

	// Check if agentName matches a built-in/registered preset with matching command.
	// Compare against both the raw command and basename to handle registry entries
	// that store absolute-path commands (e.g., "/opt/bin/my-tool").
	if info, ok := globalRegistry.Agents[agentName]; ok && len(info.ProcessNames) > 0 {
		if info.Command == command || info.Command == cmdBase || filepath.Base(info.Command) == cmdBase || cmdBase == "" {
			return info.ProcessNames
		}
	}

	// Agent name doesn't match or command differs — look up by command
	if cmdBase != "" {
		for _, info := range globalRegistry.Agents {
			if (info.Command == command || filepath.Base(info.Command) == cmdBase) && len(info.ProcessNames) > 0 {
				return info.ProcessNames
			}
		}
		// Unknown command — use the binary basename itself
		return []string{cmdBase}
	}

	// No command provided, agent not in registry — Claude defaults
	return []string{"node", "claude"}
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

// HookInstallerFunc is the signature for agent-specific hook/settings installers.
// settingsDir is the gastown-managed parent (used by agents with --settings flag).
// workDir is the agent's working directory.
// role is the Gas Town role (e.g., "polecat", "crew", "witness").
// hooksDir and hooksFile come from the preset's HooksDir and HooksSettingsFile.
type HookInstallerFunc func(settingsDir, workDir, role, hooksDir, hooksFile string) error

// hookInstallers maps provider names to their hook installation functions.
// Registration happens via RegisterHookInstaller, typically from agent package init() or runtime init().
var hookInstallers = make(map[string]HookInstallerFunc)

// RegisterHookInstaller registers a hook installation function for an agent provider.
// This replaces the switch statement in runtime.EnsureSettingsForRole.
func RegisterHookInstaller(provider string, fn HookInstallerFunc) {
	hookInstallers[provider] = fn
}

// GetHookInstaller returns the registered hook installer for a provider.
// Returns nil if no installer is registered.
func GetHookInstaller(provider string) HookInstallerFunc {
	return hookInstallers[provider]
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

// RegisterAgentForTesting adds a custom agent preset to the registry.
// The registry is initialized first if needed. Intended for test use only.
func RegisterAgentForTesting(name string, info AgentPresetInfo) {
	ensureRegistry()
	registryMu.Lock()
	defer registryMu.Unlock()
	globalRegistry.Agents[name] = &info
}

// ResetHookInstallersForTesting clears all hook installer registrations.
func ResetHookInstallersForTesting() {
	hookInstallers = make(map[string]HookInstallerFunc)
}
