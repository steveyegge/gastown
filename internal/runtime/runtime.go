// Package runtime provides helpers for runtime-specific integration.
package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/opencode"
	"github.com/steveyegge/gastown/internal/tmux"
)

// EnsureSettingsForRole installs runtime hook settings when supported.
// Deprecated: Use EnsureSettingsForRoleWithAccount for account-aware settings.
func EnsureSettingsForRole(workDir, role string, rc *config.RuntimeConfig) error {
	return EnsureSettingsForRoleWithAccount(workDir, role, "", rc)
}

// EnsureSettingsForRoleWithAccount installs runtime hook settings when supported.
// If accountConfigDir is provided, settings are installed per-account (shared across workspaces).
// If accountConfigDir is empty, settings are installed per-workspace (legacy behavior).
func EnsureSettingsForRoleWithAccount(workDir, role, accountConfigDir string, rc *config.RuntimeConfig) error {
	if rc == nil {
		rc = config.DefaultRuntimeConfig()
	}

	if rc.Hooks == nil {
		return nil
	}

	switch rc.Hooks.Provider {
	case "claude":
		return claude.EnsureSettingsForAccount(workDir, role, accountConfigDir)
	case "opencode":
		return opencode.EnsurePluginAt(workDir, rc.Hooks.Dir, rc.Hooks.SettingsFile)
	default:
		return nil
	}
}

// SessionIDFromEnv returns the runtime session ID, if present.
// It checks GT_SESSION_ID_ENV first, then CLAUDE_SESSION_ID env var,
// then falls back to the persisted .runtime/session_id file.
// The file fallback is needed because hook subprocesses (UserPromptSubmit,
// PostToolUse) don't inherit env vars set by gt prime --hook.
func SessionIDFromEnv() string {
	if envName := os.Getenv("GT_SESSION_ID_ENV"); envName != "" {
		if sessionID := os.Getenv(envName); sessionID != "" {
			return sessionID
		}
	}
	if id := os.Getenv("CLAUDE_SESSION_ID"); id != "" {
		return id
	}
	return readPersistedSessionID()
}

// readPersistedSessionID reads the session ID from .runtime/session_id.
// It checks cwd first, then walks up parent directories looking for the file.
func readPersistedSessionID() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	// Check cwd and walk up to find .runtime/session_id
	dir := cwd
	for {
		sessionFile := filepath.Join(dir, ".runtime", "session_id")
		data, err := os.ReadFile(sessionFile)
		if err == nil {
			lines := strings.SplitN(string(data), "\n", 2)
			if len(lines) > 0 {
				id := strings.TrimSpace(lines[0])
				if id != "" {
					return id
				}
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
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

// DefaultPrimeWaitMs is the default wait time in milliseconds for non-hook agents
// to run gt prime before sending work instructions.
const DefaultPrimeWaitMs = 2000

// StartupFallbackInfo describes what fallback actions are needed for agent startup
// based on the agent's hook and prompt capabilities.
//
// Fallback matrix based on agent capabilities:
//
//	| Hooks | Prompt | Beacon Content           | Context Source      | Work Instructions   |
//	|-------|--------|--------------------------|---------------------|---------------------|
//	| ✓     | ✓      | Standard                 | Hook runs gt prime  | In beacon           |
//	| ✓     | ✗      | Standard (via nudge)     | Hook runs gt prime  | Same nudge          |
//	| ✗     | ✓      | "Run gt prime" (prompt)  | Agent runs manually | Delayed nudge       |
//	| ✗     | ✗      | "Run gt prime" (nudge)   | Agent runs manually | Delayed nudge       |
type StartupFallbackInfo struct {
	// IncludePrimeInBeacon indicates the beacon should include "Run gt prime" instruction.
	// True for non-hook agents where gt prime doesn't run automatically.
	IncludePrimeInBeacon bool

	// SendBeaconNudge indicates the beacon must be sent via nudge (agent has no prompt support).
	// True for agents with PromptMode "none".
	SendBeaconNudge bool

	// SendStartupNudge indicates work instructions need to be sent via nudge.
	// True when beacon doesn't include work instructions (non-hook agents, or hook agents without prompt).
	SendStartupNudge bool

	// StartupNudgeDelayMs is milliseconds to wait before sending work instructions nudge.
	// Allows gt prime to complete for non-hook agents (where it's not automatic).
	StartupNudgeDelayMs int
}

// GetStartupFallbackInfo returns the fallback actions needed based on agent capabilities.
func GetStartupFallbackInfo(rc *config.RuntimeConfig) *StartupFallbackInfo {
	if rc == nil {
		rc = config.DefaultRuntimeConfig()
	}

	hasHooks := rc.Hooks != nil && rc.Hooks.Provider != "" && rc.Hooks.Provider != "none"
	hasPrompt := rc.PromptMode != "none"

	info := &StartupFallbackInfo{}

	if !hasHooks {
		// Non-hook agents need to be told to run gt prime
		info.IncludePrimeInBeacon = true
		info.SendStartupNudge = true
		info.StartupNudgeDelayMs = DefaultPrimeWaitMs

		if !hasPrompt {
			// No prompt support - beacon must be sent via nudge
			info.SendBeaconNudge = true
		}
	} else if !hasPrompt {
		// Has hooks but no prompt - need to nudge beacon + work instructions together
		// Hook runs gt prime synchronously, so no wait needed
		info.SendBeaconNudge = true
		info.SendStartupNudge = true
		info.StartupNudgeDelayMs = 0
	}
	// else: hooks + prompt - nothing needed, all in CLI prompt + hook

	return info
}

// StartupNudgeContent returns the work instructions to send as a startup nudge.
func StartupNudgeContent() string {
	return "Check your hook with `gt hook`. If work is present, begin immediately."
}

// BeaconPrimeInstruction returns the instruction to add to beacon for non-hook agents.
func BeaconPrimeInstruction() string {
	return "\n\nRun `gt prime` to initialize your context."
}
