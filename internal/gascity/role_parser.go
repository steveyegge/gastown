package gascity

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/steveyegge/gastown/internal/config"
)

const CurrentRoleSpecVersion = 1

var roleNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// RoleSpec is a provisional declarative Gas City role definition.
type RoleSpec struct {
	Version        int               `toml:"version" json:"version"`
	Role           string            `toml:"role" json:"role"`
	Scope          string            `toml:"scope" json:"scope"`
	Provider       string            `toml:"provider" json:"provider"`
	Description    string            `toml:"description,omitempty" json:"description,omitempty"`
	PromptTemplate string            `toml:"prompt_template,omitempty" json:"prompt_template,omitempty"`
	Nudge          string            `toml:"nudge,omitempty" json:"nudge,omitempty"`
	Session        SessionSpec       `toml:"session" json:"session"`
	Capabilities   Capabilities      `toml:"capabilities" json:"capabilities"`
	Env            map[string]string `toml:"env,omitempty" json:"env,omitempty"`
}

// SessionSpec contains process/session launch details for a role.
type SessionSpec struct {
	Pattern      string `toml:"pattern" json:"pattern"`
	WorkDir      string `toml:"work_dir" json:"work_dir"`
	StartCommand string `toml:"start_command,omitempty" json:"start_command,omitempty"`
	NeedsPreSync bool   `toml:"needs_pre_sync" json:"needs_pre_sync"`
}

// Capabilities is the normalized provider capability surface for a role.
type Capabilities struct {
	Hooks         bool   `toml:"hooks" json:"hooks"`
	Resume        bool   `toml:"resume" json:"resume"`
	ForkSession   bool   `toml:"fork_session" json:"fork_session"`
	Exec          bool   `toml:"exec" json:"exec"`
	ReadyStrategy string `toml:"ready_strategy" json:"ready_strategy"`
}

type rawRoleSpec struct {
	Version        int               `toml:"version"`
	Role           string            `toml:"role"`
	Scope          string            `toml:"scope"`
	Provider       string            `toml:"provider"`
	Description    string            `toml:"description,omitempty"`
	PromptTemplate string            `toml:"prompt_template,omitempty"`
	Nudge          string            `toml:"nudge,omitempty"`
	Session        SessionSpec       `toml:"session"`
	Capabilities   rawCapabilities   `toml:"capabilities"`
	Env            map[string]string `toml:"env,omitempty"`
}

type rawCapabilities struct {
	Hooks         *bool  `toml:"hooks"`
	Resume        *bool  `toml:"resume"`
	ForkSession   *bool  `toml:"fork_session"`
	Exec          *bool  `toml:"exec"`
	ReadyStrategy string `toml:"ready_strategy"`
}

// LoadRoleSpec reads and parses a role spec from disk.
func LoadRoleSpec(path string) (*RoleSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading role spec %s: %w", path, err)
	}

	spec, err := ParseRoleSpec(data)
	if err != nil {
		return nil, fmt.Errorf("parsing role spec %s: %w", path, err)
	}
	return spec, nil
}

// ParseRoleSpec parses and validates a declarative Gas City role definition.
func ParseRoleSpec(data []byte) (*RoleSpec, error) {
	var raw rawRoleSpec
	meta, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&raw)
	if err != nil {
		return nil, fmt.Errorf("decoding TOML: %w", err)
	}
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, key := range undecoded {
			keys = append(keys, key.String())
		}
		sort.Strings(keys)
		return nil, fmt.Errorf("unknown fields: %s", strings.Join(keys, ", "))
	}

	raw.Role = strings.TrimSpace(raw.Role)
	raw.Scope = strings.TrimSpace(raw.Scope)
	raw.Provider = strings.TrimSpace(raw.Provider)

	preset := config.GetAgentPresetByName(raw.Provider)
	if err := validateRawRoleSpec(&raw, preset); err != nil {
		return nil, err
	}

	caps, err := resolveCapabilities(raw.Capabilities, preset)
	if err != nil {
		return nil, err
	}

	spec := &RoleSpec{
		Version:        raw.Version,
		Role:           raw.Role,
		Scope:          raw.Scope,
		Provider:       raw.Provider,
		Description:    raw.Description,
		PromptTemplate: raw.PromptTemplate,
		Nudge:          raw.Nudge,
		Session:        raw.Session,
		Capabilities:   caps,
		Env:            raw.Env,
	}

	if spec.Env == nil {
		spec.Env = make(map[string]string, 2)
	}
	if spec.Env["GT_ROLE"] == "" {
		spec.Env["GT_ROLE"] = spec.Role
	}
	if spec.Env["GT_SCOPE"] == "" {
		spec.Env["GT_SCOPE"] = spec.Scope
	}
	if strings.TrimSpace(spec.Session.StartCommand) == "" {
		spec.Session.StartCommand = buildStartCommand(raw.Provider)
	}

	return spec, nil
}

func validateRawRoleSpec(raw *rawRoleSpec, preset *config.AgentPresetInfo) error {
	switch raw.Version {
	case 0:
		return fmt.Errorf("missing version (expected %d)", CurrentRoleSpecVersion)
	case CurrentRoleSpecVersion:
	default:
		return fmt.Errorf("unsupported version %d (expected %d)", raw.Version, CurrentRoleSpecVersion)
	}

	if !roleNamePattern.MatchString(raw.Role) {
		return fmt.Errorf("invalid role %q: use lowercase letters, numbers, '_' or '-'", raw.Role)
	}

	switch raw.Scope {
	case "town", "rig":
	default:
		return fmt.Errorf("invalid scope %q: expected \"town\" or \"rig\"", raw.Scope)
	}

	if preset == nil {
		return fmt.Errorf("unknown provider %q", raw.Provider)
	}

	if strings.TrimSpace(raw.Session.Pattern) == "" {
		return fmt.Errorf("session.pattern is required")
	}
	if strings.TrimSpace(raw.Session.WorkDir) == "" {
		return fmt.Errorf("session.work_dir is required")
	}
	if raw.Scope == "town" && strings.Contains(raw.Session.WorkDir, "{rig}") {
		return fmt.Errorf("town-scoped roles cannot include {rig} in session.work_dir")
	}
	if raw.Scope == "rig" && !strings.Contains(raw.Session.WorkDir, "{rig}") {
		return fmt.Errorf("rig-scoped roles must include {rig} in session.work_dir")
	}

	return nil
}

func resolveCapabilities(raw rawCapabilities, preset *config.AgentPresetInfo) (Capabilities, error) {
	caps := providerDefaultCapabilities(preset)

	if raw.Hooks != nil {
		if *raw.Hooks && !caps.Hooks {
			return Capabilities{}, fmt.Errorf("provider %q does not support hooks", preset.Name)
		}
		caps.Hooks = *raw.Hooks
	}
	if raw.Resume != nil {
		if *raw.Resume && !caps.Resume {
			return Capabilities{}, fmt.Errorf("provider %q does not support session resume", preset.Name)
		}
		caps.Resume = *raw.Resume
	}
	if raw.ForkSession != nil {
		if *raw.ForkSession && !caps.ForkSession {
			return Capabilities{}, fmt.Errorf("provider %q does not support session forking", preset.Name)
		}
		caps.ForkSession = *raw.ForkSession
	}
	if raw.Exec != nil {
		if *raw.Exec && !caps.Exec {
			return Capabilities{}, fmt.Errorf("provider %q does not support non-interactive exec", preset.Name)
		}
		caps.Exec = *raw.Exec
	}
	if raw.ReadyStrategy != "" {
		switch raw.ReadyStrategy {
		case "delay":
			if preset.ReadyDelayMs <= 0 {
				return Capabilities{}, fmt.Errorf("provider %q does not define delay-based readiness", preset.Name)
			}
		case "prompt":
			if preset.ReadyPromptPrefix == "" {
				return Capabilities{}, fmt.Errorf("provider %q does not define prompt-based readiness", preset.Name)
			}
		case "provider":
		default:
			return Capabilities{}, fmt.Errorf("invalid ready strategy %q", raw.ReadyStrategy)
		}
		caps.ReadyStrategy = raw.ReadyStrategy
	}

	return caps, nil
}

func providerDefaultCapabilities(preset *config.AgentPresetInfo) Capabilities {
	caps := Capabilities{
		Hooks:       preset.SupportsHooks,
		Resume:      preset.ResumeFlag != "",
		ForkSession: preset.SupportsForkSession,
		Exec:        preset.NonInteractive != nil || preset.Name == config.AgentClaude,
	}

	switch {
	case preset.ReadyPromptPrefix != "":
		caps.ReadyStrategy = "prompt"
	case preset.ReadyDelayMs > 0:
		caps.ReadyStrategy = "delay"
	default:
		caps.ReadyStrategy = "provider"
	}

	return caps
}

func buildStartCommand(provider string) string {
	rc := config.RuntimeConfigFromPreset(config.AgentPreset(provider))
	return "exec " + rc.BuildCommand()
}
