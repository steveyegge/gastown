// Package runtime provides helpers for runtime-specific integration.
package runtime

import (
	"os"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/lifecycle"
	"github.com/steveyegge/gastown/internal/session"
)

type startupNudger interface {
	NudgeSession(session, message string) error
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
	// NOTE: session-started nudge to deacon removed — it interrupted
	// the deacon's await-signal backoff (exponential sleep). The deacon
	// already wakes on beads activity via bd activity --follow.

	return []string{command}
}

// RunStartupFallback sends the startup fallback commands via tmux.
func RunStartupFallback(t startupNudger, sessionID, role string, rc *config.RuntimeConfig) error {
	commands := StartupFallbackCommands(role, rc)
	for _, cmd := range commands {
		if err := t.NudgeSession(sessionID, cmd); err != nil {
			return err
		}
	}
	return nil
}

// StartupBootstrapPlan describes startup actions needed after session launch.
type StartupBootstrapPlan struct {
	// SendPromptNudge means the startup prompt should be sent via nudge because
	// the runtime doesn't support initial prompt args.
	SendPromptNudge bool

	// RunPrimeFallback means no hooks are available and startup fallback commands
	// (gt prime, and mail injection for autonomous roles) must be nudged.
	RunPrimeFallback bool
}

func runStartupBootstrapWithPlan(t startupNudger, sessionID, role, startupPrompt string, rc *config.RuntimeConfig, plan *StartupBootstrapPlan) error {
	if plan == nil {
		plan = GetStartupBootstrapPlan(role, rc)
	}
	if plan.SendPromptNudge && startupPrompt != "" {
		if err := t.NudgeSession(sessionID, startupPrompt); err != nil {
			return err
		}
	}
	if plan.RunPrimeFallback {
		return RunStartupFallback(t, sessionID, role, rc)
	}
	return nil
}

func runtimeHasHooks(rc *config.RuntimeConfig) bool {
	if rc == nil {
		rc = config.DefaultRuntimeConfig()
	}
	return rc.Hooks != nil && rc.Hooks.Provider != "" && rc.Hooks.Provider != "none"
}

func runtimeHasPrompt(rc *config.RuntimeConfig) bool {
	if rc == nil {
		rc = config.DefaultRuntimeConfig()
	}
	return rc.PromptMode != "none"
}

// GetStartupBootstrapPlan computes capability-based startup bootstrap actions.
func GetStartupBootstrapPlan(role string, rc *config.RuntimeConfig) *StartupBootstrapPlan {
	hasHooks := runtimeHasHooks(rc)
	hasPrompt := runtimeHasPrompt(rc)
	return &StartupBootstrapPlan{
		SendPromptNudge: !hasPrompt,
		// Prime fallback nudges are only needed when both hooks and prompt delivery
		// are unavailable. No-hooks+prompt runtimes must not get startup nudges that
		// can interrupt the first turn.
		RunPrimeFallback: !hasHooks && !hasPrompt && len(StartupFallbackCommands(role, rc)) > 0,
	}
}

// RunStartupBootstrap executes the canonical startup bootstrap path.
// It handles prompt delivery fallback for no-prompt runtimes and non-hook
// gt prime fallback commands.
func RunStartupBootstrap(t startupNudger, sessionID, role, startupPrompt string, rc *config.RuntimeConfig) error {
	return runStartupBootstrapWithPlan(t, sessionID, role, startupPrompt, rc, nil)
}

// RunStartupBootstrapIfNeeded runs startup bootstrap only when capability fallbacks are required.
func RunStartupBootstrapIfNeeded(t startupNudger, sessionID, role, startupPrompt string, rc *config.RuntimeConfig) error {
	plan := GetStartupBootstrapPlan(role, rc)
	if !plan.SendPromptNudge && !plan.RunPrimeFallback {
		return nil
	}
	lifecycle.SleepForReadyDelay(rc)
	return runStartupBootstrapWithPlan(t, sessionID, role, startupPrompt, rc, plan)
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

// StartupBeaconConfig applies capability-based startup fallback behavior to beacon config.
func StartupBeaconConfig(cfg session.BeaconConfig, rc *config.RuntimeConfig) session.BeaconConfig {
	// Startup beacon only needs one fallback dimension:
	// include `gt prime` instruction when no session-start hooks are available.
	cfg.IncludePrimeInstruction = !runtimeHasHooks(rc)
	return cfg
}

// StartupBeacon builds the startup beacon with capability-based fallback behavior.
func StartupBeacon(cfg session.BeaconConfig, rc *config.RuntimeConfig) string {
	return session.FormatStartupBeacon(StartupBeaconConfig(cfg, rc))
}

// StartupPrompt builds the startup prompt with capability-based fallback behavior.
func StartupPrompt(cfg session.BeaconConfig, instructions string, rc *config.RuntimeConfig) string {
	return session.BuildStartupPrompt(StartupBeaconConfig(cfg, rc), instructions)
}
