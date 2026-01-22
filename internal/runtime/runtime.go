// Package runtime provides helpers for runtime-specific integration.
package runtime

import (
	"os"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/hooks"
	"github.com/steveyegge/gastown/internal/tmux"
)

// EnsureSettingsForRole installs runtime hook settings when supported.
// It uses the Provider interface to delegate to the appropriate hook provider.
func EnsureSettingsForRole(workDir, role string, rc *config.RuntimeConfig) error {
	if rc == nil {
		rc = config.DefaultRuntimeConfig()
	}

	if rc.Hooks == nil {
		return nil
	}

	provider := hooks.Get(rc.Hooks.Provider)
	if provider == nil {
		// Unknown provider - treat as no-op
		return nil
	}

	return provider.EnsureHooks(workDir, role, rc.Hooks)
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

// StartupFallbackCommands returns commands to emulate hooks when native hooks are unavailable.
// It uses the Provider interface to get provider-specific fallback commands.
func StartupFallbackCommands(role string, rc *config.RuntimeConfig) []string {
	if rc == nil {
		rc = config.DefaultRuntimeConfig()
	}

	role = strings.ToLower(role)

	// If a provider is configured, check if it supports native hooks
	if rc.Hooks != nil && rc.Hooks.Provider != "" {
		provider := hooks.Get(rc.Hooks.Provider)
		if provider != nil {
			if provider.SupportsHooks() {
				// Provider has native hooks, no fallback needed
				return nil
			}
			// Use provider-specific fallback commands
			fallbackCmds := provider.GetHooksFallback(role)
			if len(fallbackCmds) > 0 {
				// Add deacon notification to the last command
				return appendDeaconNotification(fallbackCmds, role)
			}
		}
	}

	// Default fallback when no provider is configured
	command := "gt prime"
	if isAutonomousRole(role) {
		command += " && gt mail check --inject"
	}
	command += " && gt nudge deacon session-started"

	return []string{command}
}

// appendDeaconNotification appends deacon notification to fallback commands.
// Combines all commands into a single chained command for tmux.
func appendDeaconNotification(cmds []string, role string) []string {
	if len(cmds) == 0 {
		return cmds
	}
	// Chain all commands together with deacon notification at the end
	combined := strings.Join(cmds, " && ") + " && gt nudge deacon session-started"
	return []string{combined}
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
