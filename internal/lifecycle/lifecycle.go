// Package lifecycle provides low-level runtime lifecycle helpers shared by
// startup code paths. It must remain independent of higher-level runtime/session
// packages to avoid import cycles.
package lifecycle

import (
	"time"

	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/opencode"
	"github.com/steveyegge/gastown/internal/templates/commands"
)

// EnsureSettingsForRole provisions all agent-specific configuration for a role.
// settingsDir is where provider settings (e.g., .claude/settings.json) are installed.
// workDir is the agent's working directory where slash commands are provisioned.
// For roles like crew/witness/refinery/polecat, settingsDir is a gastown-managed
// parent directory (passed via --settings flag), while workDir is the customer repo.
// For mayor/deacon, settingsDir and workDir are the same.
func EnsureSettingsForRole(settingsDir, workDir, role string, rc *config.RuntimeConfig) error {
	if rc == nil {
		rc = config.DefaultRuntimeConfig()
	}

	if rc.Hooks == nil {
		return nil
	}

	provider := rc.Hooks.Provider
	if provider == "" || provider == "none" {
		return nil
	}

	// 1. Provider-specific settings (settings.json for Claude, plugin for OpenCode)
	// Settings are installed to settingsDir (gastown-managed parent for rig roles).
	switch provider {
	case "claude":
		if err := claude.EnsureSettingsForRoleAt(settingsDir, role, rc.Hooks.Dir, rc.Hooks.SettingsFile); err != nil {
			return err
		}
	case "opencode":
		// OpenCode plugins stay in workDir — OpenCode has no --settings equivalent
		// for path redirection, so it discovers plugins from the working directory.
		if err := opencode.EnsurePluginAt(workDir, rc.Hooks.Dir, rc.Hooks.SettingsFile); err != nil {
			return err
		}
	}

	// 2. Slash commands (agent-agnostic, uses shared body with provider-specific frontmatter)
	// Only provision for known agents to maintain backwards compatibility
	if commands.IsKnownAgent(provider) {
		if err := commands.ProvisionFor(workDir, provider); err != nil {
			return err
		}
	}

	return nil
}

// SleepForReadyDelay sleeps for the runtime's configured readiness delay.
func SleepForReadyDelay(rc *config.RuntimeConfig) {
	if rc == nil || rc.Tmux == nil {
		return
	}
	if rc.Tmux.ReadyDelayMs <= 0 {
		return
	}
	time.Sleep(time.Duration(rc.Tmux.ReadyDelayMs) * time.Millisecond)
}
