// Package config provides configuration types and serialization for Gas Town.
package config

import (
	"path/filepath"
	"os"
	"strings"
	"time"
)

// TownConfig represents the main town identity (mayor/town.json).
type TownConfig struct {
	Type       string    `json:"type"`                  // "town"
	Version    int       `json:"version"`               // schema version
	Name       string    `json:"name"`                  // town identifier (internal)
	Owner      string    `json:"owner,omitempty"`       // owner email (entity identity)
	PublicName string    `json:"public_name,omitempty"` // public display name
	CreatedAt  time.Time `json:"created_at"`
}

// MayorConfig represents town-level behavioral configuration (mayor/config.json).
// This is separate from TownConfig (identity) to keep configuration concerns distinct.
type MayorConfig struct {
	Type            string           `json:"type"`                        // "mayor-config"
	Version         int              `json:"version"`                     // schema version
	Theme           *TownThemeConfig `json:"theme,omitempty"`             // global theme settings
	Daemon          *DaemonConfig    `json:"daemon,omitempty"`            // daemon settings
	Deacon          *DeaconConfig    `json:"deacon,omitempty"`            // deacon settings
	DefaultCrewName string           `json:"default_crew_name,omitempty"` // default crew name for new rigs
}

// CurrentTownSettingsVersion is the current schema version for TownSettings.
const CurrentTownSettingsVersion = 1

// TownSettings represents town-level behavioral configuration (settings/config.json).
// This contains agent configuration that applies to all rigs unless overridden.
type TownSettings struct {
	Type    string `json:"type"`    // "town-settings"
	Version int    `json:"version"` // schema version

	// DefaultAgent is the name of the agent preset to use by default.
	// Can be a built-in preset ("claude", "gemini", "codex", "cursor", "auggie", "amp")
	// or a custom agent name defined in settings/agents.json.
	// Default: "claude"
	DefaultAgent string `json:"default_agent,omitempty"`

	// Agents defines custom agent configurations or overrides.
	// Keys are agent names that can be referenced by DefaultAgent or rig settings.
	// Values override or extend the built-in presets.
	// Example: {"gemini": {"command": "/custom/path/to/gemini"}}
	Agents map[string]*RuntimeConfig `json:"agents,omitempty"`

	// RoleAgents maps role names to agent aliases for per-role model selection.
	// Keys are role names: "mayor", "deacon", "witness", "refinery", "polecat", "crew".
	// Values are agent names (built-in presets or custom agents defined in Agents).
	// This allows cost optimization by using different models for different roles.
	// Example: {"mayor": "claude-opus", "witness": "claude-haiku", "polecat": "claude-sonnet"}
	RoleAgents map[string]string `json:"role_agents,omitempty"`

	// AgentEmailDomain is the domain used for agent git identity emails.
	// Agent addresses like "gastown/crew/jack" become "gastown.crew.jack@{domain}".
	// Default: "gastown.local"
	AgentEmailDomain string `json:"agent_email_domain,omitempty"`
}

// NewTownSettings creates a new TownSettings with defaults.
func NewTownSettings() *TownSettings {
	return &TownSettings{
		Type:         "town-settings",
		Version:      CurrentTownSettingsVersion,
		DefaultAgent: "claude",
		Agents:       make(map[string]*RuntimeConfig),
		RoleAgents:   make(map[string]string),
	}
}

// DaemonConfig represents daemon process settings.
type DaemonConfig struct {
	HeartbeatInterval string `json:"heartbeat_interval,omitempty"` // e.g., "30s"
	PollInterval      string `json:"poll_interval,omitempty"`      // e.g., "10s"
}

// DaemonPatrolConfig represents the daemon patrol configuration (mayor/daemon.json).
// This configures how patrols are triggered and managed.
type DaemonPatrolConfig struct {
	Type      string                  `json:"type"`                // "daemon-patrol-config"
	Version   int                     `json:"version"`             // schema version
	Heartbeat *HeartbeatConfig        `json:"heartbeat,omitempty"` // heartbeat settings
	Patrols   map[string]PatrolConfig `json:"patrols,omitempty"`   // named patrol configurations
}

// HeartbeatConfig represents heartbeat settings for daemon.
type HeartbeatConfig struct {
	Enabled  bool   `json:"enabled"`            // whether heartbeat is enabled
	Interval string `json:"interval,omitempty"` // e.g., "3m"
}

// PatrolConfig represents a single patrol configuration.
type PatrolConfig struct {
	Enabled  bool   `json:"enabled"`            // whether this patrol is enabled
	Interval string `json:"interval,omitempty"` // e.g., "5m"
	Agent    string `json:"agent,omitempty"`    // agent that runs this patrol
}

// CurrentDaemonPatrolConfigVersion is the current schema version for DaemonPatrolConfig.
const CurrentDaemonPatrolConfigVersion = 1

// DaemonPatrolConfigFileName is the filename for daemon patrol configuration.
const DaemonPatrolConfigFileName = "daemon.json"

// NewDaemonPatrolConfig creates a new DaemonPatrolConfig with sensible defaults.
func NewDaemonPatrolConfig() *DaemonPatrolConfig {
	return &DaemonPatrolConfig{
		Type:    "daemon-patrol-config",
		Version: CurrentDaemonPatrolConfigVersion,
		Heartbeat: &HeartbeatConfig{
			Enabled:  true,
			Interval: "3m",
		},
		Patrols: map[string]PatrolConfig{
			"deacon": {
				Enabled:  true,
				Interval: "5m",
				Agent:    "deacon",
			},
			"witness": {
				Enabled:  true,
				Interval: "5m",
				Agent:    "witness",
			},
			"refinery": {
				Enabled:  true,
				Interval: "5m",
				Agent:    "refinery",
			},
		},
	}
}

// DeaconConfig represents deacon process settings.
type DeaconConfig struct {
	PatrolInterval string `json:"patrol_interval,omitempty"` // e.g., "5m"
}

// CurrentMayorConfigVersion is the current schema version for MayorConfig.
const CurrentMayorConfigVersion = 1

// DefaultCrewName is the default name for crew workspaces when not overridden.
const DefaultCrewName = "max"

// RigsConfig represents the rigs registry (mayor/rigs.json).
type RigsConfig struct {
	Version int                 `json:"version"`
	Rigs    map[string]RigEntry `json:"rigs"`
}

// RigEntry represents a single rig in the registry.
type RigEntry struct {
	GitURL      string       `json:"git_url"`
	LocalRepo   string       `json:"local_repo,omitempty"`
	AddedAt     time.Time    `json:"added_at"`
	BeadsConfig *BeadsConfig `json:"beads,omitempty"`
}

// BeadsConfig represents beads configuration for a rig.
type BeadsConfig struct {
	Repo   string `json:"repo"`   // "local" | path | git-url
	Prefix string `json:"prefix"` // issue prefix
}

// CurrentTownVersion is the current schema version for TownConfig.
// Version 2: Added Owner and PublicName fields for federation identity.
const CurrentTownVersion = 2

// CurrentRigsVersion is the current schema version for RigsConfig.
const CurrentRigsVersion = 1

// CurrentRigConfigVersion is the current schema version for RigConfig.
const CurrentRigConfigVersion = 1

// CurrentRigSettingsVersion is the current schema version for RigSettings.
const CurrentRigSettingsVersion = 1

// RigConfig represents per-rig identity (rig/config.json).
// This contains only identity - behavioral config is in settings/config.json.
type RigConfig struct {
	Type      string       `json:"type"`    // "rig"
	Version   int          `json:"version"` // schema version
	Name      string       `json:"name"`    // rig name
	GitURL    string       `json:"git_url"` // git repository URL
	LocalRepo string       `json:"local_repo,omitempty"`
	CreatedAt time.Time    `json:"created_at"` // when the rig was created
	Beads     *BeadsConfig `json:"beads,omitempty"`
}

// WorkflowConfig represents workflow settings for a rig.
type WorkflowConfig struct {
	// DefaultFormula is the formula to use when `gt formula run` is called without arguments.
	// If empty, no default is set and a formula name must be provided.
	DefaultFormula string `json:"default_formula,omitempty"`
}

// RigSettings represents per-rig behavioral configuration (settings/config.json).
type RigSettings struct {
	Type       string            `json:"type"`                  // "rig-settings"
	Version    int               `json:"version"`               // schema version
	MergeQueue *MergeQueueConfig `json:"merge_queue,omitempty"` // merge queue settings
	Theme      *ThemeConfig      `json:"theme,omitempty"`       // tmux theme settings
	Namepool   *NamepoolConfig   `json:"namepool,omitempty"`    // polecat name pool settings
	Crew       *CrewConfig       `json:"crew,omitempty"`        // crew startup settings
	Workflow   *WorkflowConfig   `json:"workflow,omitempty"`    // workflow settings
	Runtime    *RuntimeConfig    `json:"runtime,omitempty"`     // LLM runtime settings (deprecated: use Agent)

	// Agent selects which agent preset to use for this rig.
	// Can be a built-in preset ("claude", "gemini", "codex", "cursor", "auggie", "amp")
	// or a custom agent defined in settings/agents.json.
	// If empty, uses the town's default_agent setting.
	// Takes precedence over Runtime if both are set.
	Agent string `json:"agent,omitempty"`

	// Agents defines custom agent configurations or overrides for this rig.
	// Similar to TownSettings.Agents but applies to this rig only.
	// Allows per-rig custom agents for polecats and crew members.
	Agents map[string]*RuntimeConfig `json:"agents,omitempty"`

	// RoleAgents maps role names to agent aliases for per-role model selection.
	// Keys are role names: "witness", "refinery", "polecat", "crew".
	// Values are agent names (built-in presets or custom agents).
	// Overrides TownSettings.RoleAgents for this specific rig.
	// Example: {"witness": "claude-haiku", "polecat": "claude-sonnet"}
	RoleAgents map[string]string `json:"role_agents,omitempty"`
}

// CrewConfig represents crew workspace settings for a rig.
type CrewConfig struct {
	// Startup is a natural language instruction for which crew to start on boot.
	// Interpreted by AI during startup. Examples:
	//   "max"                    - start only max
	//   "joe and max"            - start joe and max
	//   "all"                    - start all crew members
	//   "pick one"               - start any one crew member
	//   "none"                   - don't auto-start any crew
	//   "max, but not emma"      - start max, skip emma
	// If empty, defaults to starting no crew automatically.
	Startup string `json:"startup,omitempty"`
}

// RuntimeConfig represents LLM runtime configuration for agent sessions.
// This allows switching between different LLM backends (claude, aider, etc.)
// without modifying startup code.
type RuntimeConfig struct {
	// Provider selects runtime-specific defaults and integration behavior.
	// Known values: "claude", "codex", "generic". Default: "claude".
	Provider string `json:"provider,omitempty"`

	// Command is the CLI command to invoke (e.g., "claude", "aider").
	// Default: "claude"
	Command string `json:"command,omitempty"`

	// Args are additional command-line arguments.
	// Default: ["--dangerously-skip-permissions"] for built-in agents.
	// Empty array [] means no args (not "use defaults").
	Args []string `json:"args"`

	// InitialPrompt is an optional first message to send after startup.
	// For claude, this is passed as the prompt argument.
	// Empty by default (hooks handle context).
	InitialPrompt string `json:"initial_prompt,omitempty"`

	// PromptMode controls how prompts are passed to the runtime.
	// Supported values: "arg" (append prompt arg), "none" (ignore prompt).
	// Default: "arg" for claude/generic, "none" for codex.
	PromptMode string `json:"prompt_mode,omitempty"`

	// Session config controls environment integration for runtime session IDs.
	Session *RuntimeSessionConfig `json:"session,omitempty"`

	// Hooks config controls runtime hook installation (if supported).
	Hooks *RuntimeHooksConfig `json:"hooks,omitempty"`

	// Tmux config controls process detection and readiness heuristics.
	Tmux *RuntimeTmuxConfig `json:"tmux,omitempty"`

	// Instructions controls the per-workspace instruction file name.
	Instructions *RuntimeInstructionsConfig `json:"instructions,omitempty"`
}

// RuntimeSessionConfig configures how Gas Town discovers runtime session IDs.
type RuntimeSessionConfig struct {
	// SessionIDEnv is the environment variable set by the runtime to identify a session.
	// Default: "CLAUDE_SESSION_ID" for claude, empty for codex/generic.
	SessionIDEnv string `json:"session_id_env,omitempty"`

	// ConfigDirEnv is the environment variable that selects a runtime account/config dir.
	// Default: "CLAUDE_CONFIG_DIR" for claude, empty for codex/generic.
	ConfigDirEnv string `json:"config_dir_env,omitempty"`
}

// RuntimeHooksConfig configures runtime hook installation.
type RuntimeHooksConfig struct {
	// Provider controls which hook templates to install: "claude", "opencode", or "none".
	Provider string `json:"provider,omitempty"`

	// Dir is the settings directory (e.g., ".claude").
	Dir string `json:"dir,omitempty"`

	// SettingsFile is the settings file name (e.g., "settings.json").
	SettingsFile string `json:"settings_file,omitempty"`
}

// RuntimeTmuxConfig controls tmux heuristics for detecting runtime readiness.
type RuntimeTmuxConfig struct {
	// ProcessNames are tmux pane commands that indicate the runtime is running.
	ProcessNames []string `json:"process_names,omitempty"`

	// ReadyPromptPrefix is the prompt prefix to detect readiness (e.g., "> ").
	ReadyPromptPrefix string `json:"ready_prompt_prefix,omitempty"`

	// ReadyDelayMs is a fixed delay used when prompt detection is unavailable.
	ReadyDelayMs int `json:"ready_delay_ms,omitempty"`
}

// RuntimeInstructionsConfig controls the name of the role instruction file.
type RuntimeInstructionsConfig struct {
	// File is the instruction filename (e.g., "CLAUDE.md", "AGENTS.md").
	File string `json:"file,omitempty"`
}

// DefaultRuntimeConfig returns a RuntimeConfig with sensible defaults.
func DefaultRuntimeConfig() *RuntimeConfig {
	return normalizeRuntimeConfig(&RuntimeConfig{Provider: "claude"})
}

// BuildCommand returns the full command line string.
// For use with tmux SendKeys.
func (rc *RuntimeConfig) BuildCommand() string {
	resolved := normalizeRuntimeConfig(rc)

	cmd := resolved.Command
	args := resolved.Args

	// Combine command and args
	if len(args) > 0 {
		return cmd + " " + strings.Join(args, " ")
	}
	return cmd
}

// BuildCommandWithPrompt returns the full command line with an initial prompt.
// If the config has an InitialPrompt, it's appended as a quoted argument.
// If prompt is provided, it overrides the config's InitialPrompt.
func (rc *RuntimeConfig) BuildCommandWithPrompt(prompt string) string {
	resolved := normalizeRuntimeConfig(rc)
	base := resolved.BuildCommand()

	// Use provided prompt or fall back to config
	p := prompt
	if p == "" {
		p = resolved.InitialPrompt
	}

	if p == "" || resolved.PromptMode == "none" {
		return base
	}

	// Quote the prompt for shell safety
	return base + " " + quoteForShell(p)
}

// BuildArgsWithPrompt returns the runtime command and args suitable for exec.
func (rc *RuntimeConfig) BuildArgsWithPrompt(prompt string) []string {
	resolved := normalizeRuntimeConfig(rc)
	args := append([]string{resolved.Command}, resolved.Args...)

	p := prompt
	if p == "" {
		p = resolved.InitialPrompt
	}

	if p != "" && resolved.PromptMode != "none" {
		args = append(args, p)
	}

	return args
}

func normalizeRuntimeConfig(rc *RuntimeConfig) *RuntimeConfig {
	if rc == nil {
		rc = &RuntimeConfig{}
	}

	if rc.Provider == "" {
		rc.Provider = "claude"
	}

	if rc.Command == "" {
		rc.Command = defaultRuntimeCommand(rc.Provider)
	}

	if rc.Args == nil {
		rc.Args = defaultRuntimeArgs(rc.Provider)
	}

	if rc.PromptMode == "" {
		rc.PromptMode = defaultPromptMode(rc.Provider)
	}

	if rc.Session == nil {
		rc.Session = &RuntimeSessionConfig{}
	}

	if rc.Session.SessionIDEnv == "" {
		rc.Session.SessionIDEnv = defaultSessionIDEnv(rc.Provider)
	}

	if rc.Session.ConfigDirEnv == "" {
		rc.Session.ConfigDirEnv = defaultConfigDirEnv(rc.Provider)
	}

	if rc.Hooks == nil {
		rc.Hooks = &RuntimeHooksConfig{}
	}

	if rc.Hooks.Provider == "" {
		rc.Hooks.Provider = defaultHooksProvider(rc.Provider)
	}

	if rc.Hooks.Dir == "" {
		rc.Hooks.Dir = defaultHooksDir(rc.Provider)
	}

	if rc.Hooks.SettingsFile == "" {
		rc.Hooks.SettingsFile = defaultHooksFile(rc.Provider)
	}

	if rc.Tmux == nil {
		rc.Tmux = &RuntimeTmuxConfig{}
	}

	if rc.Tmux.ProcessNames == nil {
		rc.Tmux.ProcessNames = defaultProcessNames(rc.Provider, rc.Command)
	}

	if rc.Tmux.ReadyPromptPrefix == "" {
		rc.Tmux.ReadyPromptPrefix = defaultReadyPromptPrefix(rc.Provider)
	}

	if rc.Tmux.ReadyDelayMs == 0 {
		rc.Tmux.ReadyDelayMs = defaultReadyDelayMs(rc.Provider)
	}

	if rc.Instructions == nil {
		rc.Instructions = &RuntimeInstructionsConfig{}
	}

	if rc.Instructions.File == "" {
		rc.Instructions.File = defaultInstructionsFile(rc.Provider)
	}

	return rc
}

func defaultRuntimeCommand(provider string) string {
	switch provider {
	case "codex":
		return "codex"
	case "opencode":
		return "opencode"
	case "generic":
		return ""
	default:
		return "claude"
	}
}

func defaultRuntimeArgs(provider string) []string {
	switch provider {
	case "claude":
		return []string{"--dangerously-skip-permissions"}
	default:
		return nil
	}
}

func defaultPromptMode(provider string) string {
	switch provider {
	case "codex":
		return "none"
	case "opencode":
		return "none"
	default:
		return "arg"
	}
}

func defaultSessionIDEnv(provider string) string {
	if provider == "claude" {
		return "CLAUDE_SESSION_ID"
	}
	return ""
}

func defaultConfigDirEnv(provider string) string {
	if provider == "claude" {
		return "CLAUDE_CONFIG_DIR"
	}
	return ""
}

func defaultHooksProvider(provider string) string {
	switch provider {
	case "claude":
		return "claude"
	case "opencode":
		return "opencode"
	default:
		return "none"
	}
}

func defaultHooksDir(provider string) string {
	switch provider {
	case "claude":
		return ".claude"
	case "opencode":
		return ".opencode/plugin"
	default:
		return ""
	}
}

func defaultHooksFile(provider string) string {
	switch provider {
	case "claude":
		return "settings.json"
	case "opencode":
		return "gastown.js"
	default:
		return ""
	}
}

func defaultProcessNames(provider, command string) []string {
	if provider == "claude" {
		return []string{"node"}
	}
	if command != "" {
		return []string{filepath.Base(command)}
	}
	return nil
}

func defaultReadyPromptPrefix(provider string) string {
	if provider == "claude" {
		return "> "
	}
	return ""
}

func defaultReadyDelayMs(provider string) int {
	if provider == "claude" {
		return 10000
	}
	if provider == "codex" {
		return 3000
	}
	return 0
}

func defaultInstructionsFile(provider string) string {
	if provider == "codex" {
		return "AGENTS.md"
	}
	if provider == "opencode" {
		return "AGENTS.md"
	}
	return "CLAUDE.md"
}

// quoteForShell quotes a string for safe shell usage.
func quoteForShell(s string) string {
	// Simple quoting: wrap in double quotes, escape internal quotes
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

// ThemeConfig represents tmux theme settings for a rig.
type ThemeConfig struct {
	// Name picks from the default palette (e.g., "ocean", "forest").
	// If empty, a theme is auto-assigned based on rig name.
	Name string `json:"name,omitempty"`

	// Custom overrides the palette with specific colors.
	Custom *CustomTheme `json:"custom,omitempty"`

	// RoleThemes overrides themes for specific roles in this rig.
	// Keys: "witness", "refinery", "crew", "polecat"
	RoleThemes map[string]string `json:"role_themes,omitempty"`
}

// CustomTheme allows specifying exact colors for the status bar.
type CustomTheme struct {
	BG string `json:"bg"` // Background color (hex or tmux color name)
	FG string `json:"fg"` // Foreground color (hex or tmux color name)
}

// TownThemeConfig represents global theme settings (mayor/config.json).
type TownThemeConfig struct {
	// RoleDefaults sets default themes for roles across all rigs.
	// Keys: "witness", "refinery", "crew", "polecat"
	RoleDefaults map[string]string `json:"role_defaults,omitempty"`
}

// BuiltinRoleThemes returns the default themes for each role.
// These are used when no explicit configuration is provided.
func BuiltinRoleThemes() map[string]string {
	return map[string]string{
		"witness":  "rust", // Red/rust - watchful, alert
		"refinery": "plum", // Purple - processing, refining
		// crew and polecat use rig theme by default (no override)
	}
}

// MergeQueueConfig represents merge queue settings for a rig.
type MergeQueueConfig struct {
	// Enabled controls whether the merge queue is active.
	Enabled bool `json:"enabled"`

	// TargetBranch is the default branch to merge into (usually "main").
	TargetBranch string `json:"target_branch"`

	// IntegrationBranches enables integration branch workflow for epics.
	IntegrationBranches bool `json:"integration_branches"`

	// IntegrationBranchTemplate is the pattern for integration branch names.
	// Supports variables: {epic}, {prefix}, {user}
	// - {epic}: Full epic ID (e.g., "RA-123")
	// - {prefix}: Epic prefix before first hyphen (e.g., "RA")
	// - {user}: Git user.name (e.g., "klauern")
	// Default: "integration/{epic}"
	IntegrationBranchTemplate string `json:"integration_branch_template,omitempty"`

	// OnConflict specifies conflict resolution strategy: "assign_back" or "auto_rebase".
	OnConflict string `json:"on_conflict"`

	// RunTests controls whether to run tests before merging.
	RunTests bool `json:"run_tests"`

	// TestCommand is the command to run for tests.
	TestCommand string `json:"test_command,omitempty"`

	// DeleteMergedBranches controls whether to delete branches after merging.
	DeleteMergedBranches bool `json:"delete_merged_branches"`

	// RetryFlakyTests is the number of times to retry flaky tests.
	RetryFlakyTests int `json:"retry_flaky_tests"`

	// PollInterval is how often to poll for new merge requests (e.g., "30s").
	PollInterval string `json:"poll_interval"`

	// MaxConcurrent is the maximum number of concurrent merges.
	MaxConcurrent int `json:"max_concurrent"`
}

// OnConflict strategy constants.
const (
	OnConflictAssignBack = "assign_back"
	OnConflictAutoRebase = "auto_rebase"
)

// DefaultMergeQueueConfig returns a MergeQueueConfig with sensible defaults.
func DefaultMergeQueueConfig() *MergeQueueConfig {
	return &MergeQueueConfig{
		Enabled:              true,
		TargetBranch:         "main",
		IntegrationBranches:  true,
		OnConflict:           OnConflictAssignBack,
		RunTests:             true,
		TestCommand:          "go test ./...",
		DeleteMergedBranches: true,
		RetryFlakyTests:      1,
		PollInterval:         "30s",
		MaxConcurrent:        1,
	}
}

// NamepoolConfig represents namepool settings for themed polecat names.
type NamepoolConfig struct {
	// Style picks from a built-in theme (e.g., "mad-max", "minerals", "wasteland").
	// If empty, defaults to "mad-max".
	Style string `json:"style,omitempty"`

	// Names is a custom list of names to use instead of a built-in theme.
	// If provided, overrides the Style setting.
	Names []string `json:"names,omitempty"`

	// MaxBeforeNumbering is when to start appending numbers.
	// Default is 50. After this many polecats, names become name-01, name-02, etc.
	MaxBeforeNumbering int `json:"max_before_numbering,omitempty"`
}

// DefaultNamepoolConfig returns a NamepoolConfig with sensible defaults.
func DefaultNamepoolConfig() *NamepoolConfig {
	return &NamepoolConfig{
		Style:              "mad-max",
		MaxBeforeNumbering: 50,
	}
}

// AccountsConfig represents Claude Code account configuration (mayor/accounts.json).
// This enables Gas Town to manage multiple Claude Code accounts with easy switching.
type AccountsConfig struct {
	Version  int                `json:"version"`  // schema version
	Accounts map[string]Account `json:"accounts"` // handle -> account details
	Default  string             `json:"default"`  // default account handle
}

// Account represents a single Claude Code account.
type Account struct {
	Email       string `json:"email"`                 // account email
	Description string `json:"description,omitempty"` // human description
	ConfigDir   string `json:"config_dir"`            // path to CLAUDE_CONFIG_DIR
}

// CurrentAccountsVersion is the current schema version for AccountsConfig.
const CurrentAccountsVersion = 1

// DefaultAccountsConfigDir returns the default base directory for account configs.
func DefaultAccountsConfigDir() string {
	home, _ := os.UserHomeDir()
	return home + "/.claude-accounts"
}

// MessagingConfig represents the messaging configuration (config/messaging.json).
// This defines mailing lists, work queues, and announcement channels.
type MessagingConfig struct {
	Type    string `json:"type"`    // "messaging"
	Version int    `json:"version"` // schema version

	// Lists are static mailing lists. Messages are fanned out to all recipients.
	// Each recipient gets their own copy of the message.
	// Example: {"oncall": ["mayor/", "gastown/witness"]}
	Lists map[string][]string `json:"lists,omitempty"`

	// Queues are shared work queues. Only one copy exists; workers claim messages.
	// Messages sit in the queue until explicitly claimed by a worker.
	// Example: {"work/gastown": ["gastown/polecats/*"]}
	Queues map[string]QueueConfig `json:"queues,omitempty"`

	// Announces are bulletin boards. One copy exists; anyone can read, no claiming.
	// Used for broadcast announcements that don't need acknowledgment.
	// Example: {"alerts": {"readers": ["@town"]}}
	Announces map[string]AnnounceConfig `json:"announces,omitempty"`

	// NudgeChannels are named groups for real-time nudge fan-out.
	// Like mailing lists but for tmux send-keys instead of durable mail.
	// Example: {"workers": ["gastown/polecats/*", "gastown/crew/*"], "witnesses": ["*/witness"]}
	NudgeChannels map[string][]string `json:"nudge_channels,omitempty"`
}

// QueueConfig represents a work queue configuration.
type QueueConfig struct {
	// Workers lists addresses eligible to claim from this queue.
	// Supports wildcards: "gastown/polecats/*" matches all polecats in gastown.
	Workers []string `json:"workers"`

	// MaxClaims is the maximum number of concurrent claims (0 = unlimited).
	MaxClaims int `json:"max_claims,omitempty"`
}

// AnnounceConfig represents a bulletin board configuration.
type AnnounceConfig struct {
	// Readers lists addresses eligible to read from this announce channel.
	// Supports @group syntax: "@town", "@rig/gastown", "@witnesses".
	Readers []string `json:"readers"`

	// RetainCount is the number of messages to retain (0 = unlimited).
	RetainCount int `json:"retain_count,omitempty"`
}

// CurrentMessagingVersion is the current schema version for MessagingConfig.
const CurrentMessagingVersion = 1

// NewMessagingConfig creates a new MessagingConfig with defaults.
func NewMessagingConfig() *MessagingConfig {
	return &MessagingConfig{
		Type:          "messaging",
		Version:       CurrentMessagingVersion,
		Lists:         make(map[string][]string),
		Queues:        make(map[string]QueueConfig),
		Announces:     make(map[string]AnnounceConfig),
		NudgeChannels: make(map[string][]string),
	}
}

// EscalationConfig represents escalation routing configuration (settings/escalation.json).
// This defines severity-based routing for escalations to different channels.
type EscalationConfig struct {
	Type    string `json:"type"`    // "escalation"
	Version int    `json:"version"` // schema version

	// Routes maps severity levels to action lists.
	// Actions are executed in order for each escalation.
	// Action formats:
	//   - "bead"        → Create escalation bead (always first, implicit)
	//   - "mail:<target>" → Send gt mail to target (e.g., "mail:mayor")
	//   - "email:human" → Send email to contacts.human_email
	//   - "sms:human"   → Send SMS to contacts.human_sms
	//   - "slack"       → Post to contacts.slack_webhook
	//   - "log"         → Write to escalation log file
	Routes map[string][]string `json:"routes"`

	// Contacts contains contact information for external notification actions.
	Contacts EscalationContacts `json:"contacts"`

	// StaleThreshold is how long before an unacknowledged escalation
	// is considered stale and gets re-escalated.
	// Format: Go duration string (e.g., "4h", "30m", "24h")
	// Default: "4h"
	StaleThreshold string `json:"stale_threshold,omitempty"`

	// MaxReescalations limits how many times an escalation can be
	// re-escalated. Default: 2 (low→medium→high, then stops)
	MaxReescalations int `json:"max_reescalations,omitempty"`
}

// EscalationContacts contains contact information for external notification channels.
type EscalationContacts struct {
	HumanEmail   string `json:"human_email,omitempty"`   // email address for email:human action
	HumanSMS     string `json:"human_sms,omitempty"`     // phone number for sms:human action
	SlackWebhook string `json:"slack_webhook,omitempty"` // webhook URL for slack action
}

// CurrentEscalationVersion is the current schema version for EscalationConfig.
const CurrentEscalationVersion = 1

// Escalation severity level constants.
const (
	SeverityCritical = "critical" // P0: immediate attention required
	SeverityHigh     = "high"     // P1: urgent, needs attention soon
	SeverityMedium   = "medium"   // P2: standard escalation (default)
	SeverityLow      = "low"      // P3: informational, can wait
)

// ValidSeverities returns the list of valid severity levels in order of priority.
func ValidSeverities() []string {
	return []string{SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical}
}

// IsValidSeverity checks if a severity level is valid.
func IsValidSeverity(severity string) bool {
	switch severity {
	case SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical:
		return true
	default:
		return false
	}
}

// NextSeverity returns the next higher severity level for re-escalation.
// Returns the same level if already at critical.
func NextSeverity(severity string) string {
	switch severity {
	case SeverityLow:
		return SeverityMedium
	case SeverityMedium:
		return SeverityHigh
	case SeverityHigh:
		return SeverityCritical
	default:
		return SeverityCritical
	}
}

// NewEscalationConfig creates a new EscalationConfig with sensible defaults.
func NewEscalationConfig() *EscalationConfig {
	return &EscalationConfig{
		Type:    "escalation",
		Version: CurrentEscalationVersion,
		Routes: map[string][]string{
			SeverityLow:      {"bead"},
			SeverityMedium:   {"bead", "mail:mayor"},
			SeverityHigh:     {"bead", "mail:mayor", "email:human"},
			SeverityCritical: {"bead", "mail:mayor", "email:human", "sms:human"},
		},
		Contacts:         EscalationContacts{},
		StaleThreshold:   "4h",
		MaxReescalations: 2,
	}
}
