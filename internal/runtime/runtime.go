// Package runtime provides helpers for runtime-specific integration.
package runtime

import (
	"os"
	"strings"

	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/cli"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/copilot"
	"github.com/steveyegge/gastown/internal/gemini"
	"github.com/steveyegge/gastown/internal/omp"
	"github.com/steveyegge/gastown/internal/opencode"
	"github.com/steveyegge/gastown/internal/pi"
	"github.com/steveyegge/gastown/internal/templates/commands"
	"github.com/steveyegge/gastown/internal/tmux"
)

func init() {
	// Register hook installers for all agents that support hooks.
	// This replaces the provider switch statement in EnsureSettingsForRole.
	// Adding a new hook-supporting agent = adding a registration here.
	config.RegisterHookInstaller("claude", func(settingsDir, workDir, role, hooksDir, hooksFile string) error {
		return claude.EnsureSettingsForRoleAt(settingsDir, role, hooksDir, hooksFile)
	})
	config.RegisterHookInstaller("gemini", func(settingsDir, workDir, role, hooksDir, hooksFile string) error {
		// Gemini CLI has no --settings flag; install settings in workDir.
		return gemini.EnsureSettingsForRoleAt(workDir, role, hooksDir, hooksFile)
	})
	config.RegisterHookInstaller("opencode", func(settingsDir, workDir, role, hooksDir, hooksFile string) error {
		// OpenCode plugins stay in workDir — no --settings equivalent.
		return opencode.EnsurePluginAt(workDir, hooksDir, hooksFile)
	})
	config.RegisterHookInstaller("copilot", func(settingsDir, workDir, role, hooksDir, hooksFile string) error {
		// Copilot custom instructions stay in workDir — no --settings equivalent.
		return copilot.EnsureSettingsAt(workDir, hooksDir, hooksFile)
	})
	config.RegisterHookInstaller("omp", func(settingsDir, workDir, role, hooksDir, hooksFile string) error {
		// OMP hooks stay in workDir — loaded via --hook flag.
		return omp.EnsureHookAt(workDir, hooksDir, hooksFile)
	})
	config.RegisterHookInstaller("pi", func(settingsDir, workDir, role, hooksDir, hooksFile string) error {
		// Pi extensions stay in workDir — loaded via -e flag.
		return pi.EnsureHookAt(workDir, hooksDir, hooksFile)
	})
}

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

	// 1. Provider-specific settings (settings.json for Claude, plugin for OpenCode, etc.)
	// Hook installers are registered in init() — no switch statement needed.
	if installer := config.GetHookInstaller(provider); installer != nil {
		if err := installer(settingsDir, workDir, role, rc.Hooks.Dir, rc.Hooks.SettingsFile); err != nil {
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

// SessionIDFromEnv returns the runtime session ID, if present.
// It checks GT_SESSION_ID_ENV first, then resolves from the current agent's preset,
// and falls back to CLAUDE_SESSION_ID for backwards compatibility.
func SessionIDFromEnv() string {
	if envName := os.Getenv("GT_SESSION_ID_ENV"); envName != "" {
		if sessionID := os.Getenv(envName); sessionID != "" {
			return sessionID
		}
	}
	// Use the current agent's session ID env var from its preset
	if agentName := os.Getenv("GT_AGENT"); agentName != "" {
		if preset := config.GetAgentPresetByName(agentName); preset != nil && preset.SessionIDEnv != "" {
			if sessionID := os.Getenv(preset.SessionIDEnv); sessionID != "" {
				return sessionID
			}
		}
	}
	// Backwards-compatible fallback for sessions without GT_AGENT
	return os.Getenv("CLAUDE_SESSION_ID")
}

// StartupFallbackCommands returns commands that approximate Claude hooks when hooks are unavailable.
func StartupFallbackCommands(role string, rc *config.RuntimeConfig) []string {
	if rc == nil {
		rc = config.DefaultRuntimeConfig()
	}
	if rc.Hooks != nil && rc.Hooks.Provider != "" && rc.Hooks.Provider != "none" && !rc.Hooks.Informational {
		return nil
	}

	role = strings.ToLower(role)
	command := "gt prime"
	if isAutonomousRole(role) {
		command += " && gt mail check --inject"
	}
	// NOTE: session-started nudge to deacon removed — it interrupted
	// the deacon's await-signal backoff (exponential sleep). The deacon
	// already wakes on beads activity via bd activity --follow.

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
// refinery, deacon, boot) operate without human prompting and need mail injection
// to receive work assignments.
//
// Non-autonomous roles (mayor, crew) are human-guided and should not
// have automatic mail injection to avoid confusion.
func isAutonomousRole(role string) bool {
	switch role {
	case "polecat", "witness", "refinery", "deacon", "boot":
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

	hasHooks := rc.Hooks != nil && rc.Hooks.Provider != "" && rc.Hooks.Provider != "none" && !rc.Hooks.Informational
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
	return "Check your hook with `" + cli.Name() + " hook`. If work is present, begin immediately."
}

// BeaconPrimeInstruction returns the instruction to add to beacon for non-hook agents.
func BeaconPrimeInstruction() string {
	return "\n\nRun `" + cli.Name() + " prime` to initialize your context."
}

// RuntimeConfigWithMinDelay returns a shallow copy of rc with ReadyDelayMs set to
// at least minMs, and ReadyPromptPrefix cleared. This forces WaitForRuntimeReady
// to use the delay-based fallback path, ensuring the minimum wall-clock wait is
// always enforced. Used for the gt prime wait where we need a guaranteed delay for
// the agent to process the beacon and run gt prime — prompt detection would
// short-circuit immediately (seeing the still-present prompt from the initial
// readiness check) and bypass the intended delay floor.
func RuntimeConfigWithMinDelay(rc *config.RuntimeConfig, minMs int) *config.RuntimeConfig {
	if rc == nil {
		return &config.RuntimeConfig{Tmux: &config.RuntimeTmuxConfig{ReadyDelayMs: minMs}}
	}
	cp := *rc
	if cp.Tmux == nil {
		cp.Tmux = &config.RuntimeTmuxConfig{ReadyDelayMs: minMs}
	} else {
		tmuxCp := *cp.Tmux
		if tmuxCp.ReadyDelayMs < minMs {
			tmuxCp.ReadyDelayMs = minMs
		}
		// Clear prompt prefix to force the delay-based path in WaitForRuntimeReady.
		// The prime wait needs a guaranteed wall-clock delay, not prompt detection.
		tmuxCp.ReadyPromptPrefix = ""
		cp.Tmux = &tmuxCp
	}
	return &cp
}
