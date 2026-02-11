// Package config provides role configuration for Gas Town agents.
package config

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

//go:embed roles/*.toml
var defaultRolesFS embed.FS

// RoleDefinition contains all configuration for a role type.
// This replaces the role bead system with config files.
type RoleDefinition struct {
	// Role is the role identifier (mayor, deacon, witness, refinery, polecat, crew, dog).
	Role string `toml:"role"`

	// Scope is "town" or "rig" - determines where the agent runs.
	Scope string `toml:"scope"`

	// Session contains tmux session configuration.
	Session RoleSessionConfig `toml:"session"`

	// Env contains environment variables to set in the session.
	Env map[string]string `toml:"env,omitempty"`

	// Health contains health check configuration.
	Health RoleHealthConfig `toml:"health"`

	// Nudge is the initial prompt sent when starting the agent.
	Nudge string `toml:"nudge,omitempty"`

	// PromptTemplate is the name of the role's prompt template file.
	PromptTemplate string `toml:"prompt_template,omitempty"`
}

// RoleSessionConfig contains session-related configuration.
type RoleSessionConfig struct {
	// Pattern is the tmux session name pattern.
	// Supports placeholders: {rig}, {name}, {role}
	// Examples: "hq-mayor", "gt-{rig}-witness", "gt-{rig}-{name}"
	Pattern string `toml:"pattern"`

	// WorkDir is the working directory pattern.
	// Supports placeholders: {town}, {rig}, {name}, {role}
	// Examples: "{town}", "{town}/{rig}/witness"
	WorkDir string `toml:"work_dir"`

	// NeedsPreSync indicates if workspace needs git sync before starting.
	NeedsPreSync bool `toml:"needs_pre_sync"`

	// StartCommand is the command to run after creating the session.
	// Default: "exec claude --dangerously-skip-permissions"
	StartCommand string `toml:"start_command,omitempty"`
}

// RoleHealthConfig contains health check thresholds.
type RoleHealthConfig struct {
	// PingTimeout is how long to wait for a health check response.
	PingTimeout Duration `toml:"ping_timeout"`

	// ConsecutiveFailures is how many failed health checks before force-kill.
	ConsecutiveFailures int `toml:"consecutive_failures"`

	// KillCooldown is the minimum time between force-kills.
	KillCooldown Duration `toml:"kill_cooldown"`

	// StuckThreshold is how long a wisp can be in_progress before considered stuck.
	StuckThreshold Duration `toml:"stuck_threshold"`
}

// Duration is a wrapper for time.Duration that supports TOML marshaling.
type Duration struct {
	time.Duration
}

// UnmarshalText implements encoding.TextUnmarshaler for Duration.
func (d *Duration) UnmarshalText(text []byte) error {
	parsed, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", string(text), err)
	}
	d.Duration = parsed
	return nil
}

// MarshalText implements encoding.TextMarshaler for Duration.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.Duration.String()), nil
}

// String returns the duration as a string.
func (d Duration) String() string {
	return d.Duration.String()
}

// AllRoles returns the list of all known role names.
func AllRoles() []string {
	return []string{"mayor", "deacon", "dog", "witness", "refinery", "polecat", "crew"}
}

// TownRoles returns roles that operate at town scope.
func TownRoles() []string {
	return []string{"mayor", "deacon", "dog"}
}

// RigRoles returns roles that operate at rig scope.
func RigRoles() []string {
	return []string{"witness", "refinery", "polecat", "crew"}
}

// isValidRoleName checks if the given name is a known role.
func isValidRoleName(name string) bool {
	for _, r := range AllRoles() {
		if r == name {
			return true
		}
	}
	return false
}

// LoadRoleDefinition loads role configuration with override resolution.
// Resolution order (later overrides earlier):
//  1. Built-in defaults (embedded in binary)
//  2. Town-level overrides (<town>/roles/<role>.toml)
//  3. Rig-level overrides (<rig>/roles/<role>.toml)
//
// Each layer merges with (not replaces) the previous. Users only specify
// fields they want to change.
func LoadRoleDefinition(townRoot, rigPath, roleName string) (*RoleDefinition, error) {
	// Validate role name
	if !isValidRoleName(roleName) {
		return nil, fmt.Errorf("unknown role %q - valid roles: %v", roleName, AllRoles())
	}

	// 1. Load built-in defaults
	def, err := loadBuiltinRoleDefinition(roleName)
	if err != nil {
		return nil, fmt.Errorf("loading built-in role %s: %w", roleName, err)
	}

	// 2. Apply town-level overrides if present
	townOverridePath := filepath.Join(townRoot, "roles", roleName+".toml")
	if override, err := loadRoleOverride(townOverridePath); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("town-level role override %s: %w", townOverridePath, err)
		}
	} else {
		mergeRoleDefinition(def, override)
	}

	// 3. Apply rig-level overrides if present (only for rig-scoped roles)
	if rigPath != "" {
		rigOverridePath := filepath.Join(rigPath, "roles", roleName+".toml")
		if override, err := loadRoleOverride(rigOverridePath); err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("rig-level role override %s: %w", rigOverridePath, err)
			}
		} else {
			mergeRoleDefinition(def, override)
		}
	}

	return def, nil
}

// loadBuiltinRoleDefinition loads a role definition from embedded defaults.
func loadBuiltinRoleDefinition(roleName string) (*RoleDefinition, error) {
	data, err := defaultRolesFS.ReadFile("roles/" + roleName + ".toml")
	if err != nil {
		return nil, fmt.Errorf("role %s not found in defaults: %w", roleName, err)
	}

	var def RoleDefinition
	if err := toml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parsing role %s: %w", roleName, err)
	}

	return &def, nil
}

// loadRoleOverride loads a role override from a file path.
// Returns nil, nil if file doesn't exist.
func loadRoleOverride(path string) (*RoleDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, err // Signal no override exists
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var def RoleDefinition
	if err := toml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	return &def, nil
}

// mergeRoleDefinition merges override into base.
// Only non-zero values in override are applied.
func mergeRoleDefinition(base, override *RoleDefinition) {
	if override == nil {
		return
	}

	// Role and Scope are immutable
	// (can't change a witness to a mayor via override)

	// Session config
	if override.Session.Pattern != "" {
		base.Session.Pattern = override.Session.Pattern
	}
	if override.Session.WorkDir != "" {
		base.Session.WorkDir = override.Session.WorkDir
	}
	// NeedsPreSync can only be enabled via override, not disabled.
	// This is intentional: if a role's builtin requires pre-sync (e.g., refinery),
	// disabling it would break the role's assumptions about workspace state.
	if override.Session.NeedsPreSync {
		base.Session.NeedsPreSync = true
	}
	if override.Session.StartCommand != "" {
		base.Session.StartCommand = override.Session.StartCommand
	}

	// Env vars (merge, don't replace)
	if override.Env != nil {
		if base.Env == nil {
			base.Env = make(map[string]string)
		}
		for k, v := range override.Env {
			base.Env[k] = v
		}
	}

	// Health config
	if override.Health.PingTimeout.Duration != 0 {
		base.Health.PingTimeout = override.Health.PingTimeout
	}
	if override.Health.ConsecutiveFailures != 0 {
		base.Health.ConsecutiveFailures = override.Health.ConsecutiveFailures
	}
	if override.Health.KillCooldown.Duration != 0 {
		base.Health.KillCooldown = override.Health.KillCooldown
	}
	if override.Health.StuckThreshold.Duration != 0 {
		base.Health.StuckThreshold = override.Health.StuckThreshold
	}

	// Prompts
	if override.Nudge != "" {
		base.Nudge = override.Nudge
	}
	if override.PromptTemplate != "" {
		base.PromptTemplate = override.PromptTemplate
	}
}

// ExpandPattern expands placeholders in a pattern string.
// Supported placeholders: {town}, {rig}, {name}, {role}
func ExpandPattern(pattern, townRoot, rig, name, role string) string {
	result := pattern
	result = strings.ReplaceAll(result, "{town}", townRoot)
	result = strings.ReplaceAll(result, "{rig}", rig)
	result = strings.ReplaceAll(result, "{name}", name)
	result = strings.ReplaceAll(result, "{role}", role)
	return result
}

// ToLegacyRoleConfig converts a RoleDefinition to the legacy RoleConfig format
// for backward compatibility with existing daemon code.
func (rd *RoleDefinition) ToLegacyRoleConfig() *LegacyRoleConfig {
	return &LegacyRoleConfig{
		SessionPattern:      rd.Session.Pattern,
		WorkDirPattern:      rd.Session.WorkDir,
		NeedsPreSync:        rd.Session.NeedsPreSync,
		StartCommand:        rd.Session.StartCommand,
		EnvVars:             rd.Env,
		PingTimeout:         rd.Health.PingTimeout.String(),
		ConsecutiveFailures: rd.Health.ConsecutiveFailures,
		KillCooldown:        rd.Health.KillCooldown.String(),
		StuckThreshold:      rd.Health.StuckThreshold.String(),
	}
}

// LegacyRoleConfig matches the old beads.RoleConfig struct for compatibility.
// This allows gradual migration without breaking existing code.
type LegacyRoleConfig struct {
	SessionPattern      string
	WorkDirPattern      string
	NeedsPreSync        bool
	StartCommand        string
	EnvVars             map[string]string
	PingTimeout         string
	ConsecutiveFailures int
	KillCooldown        string
	StuckThreshold      string
}
