// Package runtime provides helpers for runtime-specific integration.
package runtime

import (
	"os"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/opencode"
	"github.com/steveyegge/gastown/internal/tmux"
)

// EnsureSettingsForRole installs runtime hook settings when supported.
//
// IMPORTANT: Settings must be installed in workDir (the session's working directory),
// NOT in a parent directory. Neither Claude Code nor OpenCode traverse parent
// directories to find settings:
//
//   - Claude Code: Does NOT walk up parent directories for .claude/settings.json.
//     This is an open feature request: https://github.com/anthropics/claude-code/issues/12962
//     Only CLAUDE.md files support parent directory traversal.
//
//   - OpenCode: Documentation does not specify directory traversal for .opencode/plugins/.
//     Plugins are loaded from project-level (.opencode/plugins/) or global (~/.config/opencode/plugins/).
//
// Therefore, when a session runs in a subdirectory (e.g., polecats/Toast/ or crew/emma/),
// settings must be placed directly in that directory, not in a shared parent.
func EnsureSettingsForRole(workDir, role string, rc *config.RuntimeConfig) error {
	if rc == nil {
		rc = config.DefaultRuntimeConfig()
	}

	// If Hooks not set, fill defaults based on Provider
	if rc.Hooks == nil {
		rc.Hooks = &config.RuntimeHooksConfig{}
		if rc.Hooks.Provider == "" {
			rc.Hooks.Provider = rc.Provider
		}
		if rc.Hooks.Dir == "" {
			switch rc.Provider {
			case "claude":
				rc.Hooks.Dir = ".claude"
			case "opencode":
				rc.Hooks.Dir = ".opencode/plugins"
			default:
				rc.Hooks.Dir = ""
			}
		}
		if rc.Hooks.SettingsFile == "" {
			switch rc.Provider {
			case "claude":
				rc.Hooks.SettingsFile = "settings.json"
			case "opencode":
				rc.Hooks.SettingsFile = "gastown.js"
			default:
				rc.Hooks.SettingsFile = ""
			}
		}
	}

	switch rc.Hooks.Provider {
	case "claude":
		return claude.EnsureSettingsForRoleAt(workDir, role, rc.Hooks.Dir, rc.Hooks.SettingsFile)
	case "opencode":
		return opencode.EnsurePluginAt(workDir, rc.Hooks.Dir, rc.Hooks.SettingsFile)
	default:
		return nil
	}
}

// SessionIDFromEnv returns the runtime session ID, if present.
// It checks GT_SESSION_ID_ENV first, then falls back to CLAUDE_SESSION_ID.
func SessionIDFromEnv() string {
	if envName := os.Getenv("GT_SESSION_ID_ENV"); envName != "" {
		if sessionID := os.Getenv(envName); sessionID != "" {
			return sessionID
		}
	}
	return os.Getenv("CLAUDE_SESSION_ID")
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

// StartupFallbackCommands returns commands that approximate Claude hooks when hooks are unavailable.
func StartupFallbackCommands(role string, rc *config.RuntimeConfig) []string {
	if rc == nil {
		rc = config.DefaultRuntimeConfig()
	}
	if rc.Hooks != nil && rc.Hooks.Provider != "" && rc.Hooks.Provider != "none" {
		return nil
	}

	role = strings.ToLower(role)
	command := "gt prime"
	if isAutonomousRole(role) {
		command += " && gt mail check --inject"
	}
	command += " && gt nudge deacon session-started"

	return []string{command}
}

// RunStartupFallback sends the startup fallback commands via tmux.
func RunStartupFallback(t *tmux.Tmux, sessionID, role string, rc *config.RuntimeConfig) error {
	commands := StartupFallbackCommands(role, rc)
	for _, cmd := range commands {
		if err := t.NudgeSession(sessionID, cmd); err != nil {
			return err
		}
	}
	return nil
}

// isAutonomousRole returns true if the given role should automatically
// inject mail check on startup. Autonomous roles (polecat, witness,
// refinery, deacon) operate without human prompting and need mail injection
// to receive work assignments.
//
// Non-autonomous roles (mayor, crew) are human-guided and should not
// have automatic mail injection to avoid confusion.
func isAutonomousRole(role string) bool {
	switch role {
	case "polecat", "witness", "refinery", "deacon":
		return true
	default:
		return false
	}
}
