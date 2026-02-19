// Package session provides polecat session lifecycle management.
package session

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/tmuxinator"
)

// SessionConfig describes how to create and start a tmux session.
// This unifies the common startup pattern that was previously duplicated
// across polecat, mayor, boot, deacon, witness, refinery, crew, and dog
// session managers. Each of those managers previously had to coordinate
// 4+ packages (config, runtime, session, tmux) manually.
//
// Usage pattern:
//
//	result, err := session.StartSession(t, session.SessionConfig{
//	    SessionID: "gt-myrig-toast",
//	    WorkDir:   "/path/to/worktree",
//	    Role:      "polecat",
//	    TownRoot:  "/path/to/town",
//	    Beacon:    session.BeaconConfig{...},
//	})
type SessionConfig struct {
	// SessionID is the tmux session name (e.g., "gt-wyvern-Toast", "hq-mayor").
	SessionID string

	// WorkDir is the working directory for the session.
	WorkDir string

	// Role is the agent role (e.g., "polecat", "mayor", "boot", "deacon").
	Role string

	// TownRoot is the root of the Gas Town workspace (e.g., ~/gt).
	TownRoot string

	// RigPath is the rig directory path for config resolution.
	// Empty for town-level agents (mayor, deacon, boot).
	RigPath string

	// RigName is the rig name for environment variables and theming.
	// Empty for town-level agents.
	RigName string

	// AgentName is the specific agent name within a rig.
	// Used for polecats, crew, and dogs. Empty for singletons.
	AgentName string

	// Command is a pre-built startup command. If non-empty, skips command building.
	// If empty, the command is built from Beacon + config.BuildAgentStartupCommand.
	Command string

	// Beacon configures the startup beacon message for session identification.
	// Ignored if Command is non-empty.
	Beacon BeaconConfig

	// Instructions are appended after the beacon in the startup prompt.
	// Used by roles like Boot and Deacon that need explicit instructions.
	// Ignored if Command is non-empty.
	Instructions string

	// AgentOverride optionally specifies a different agent alias (e.g., "opencode").
	AgentOverride string

	// RuntimeConfigDir overrides the config directory for the runtime.
	RuntimeConfigDir string

	// ExtraEnv adds additional environment variables beyond the standard AgentEnv set.
	// These are set in the tmux session environment after the standard vars.
	ExtraEnv map[string]string

	// Theme is the tmux theme to apply. Nil means no theme is applied.
	Theme *tmux.Theme

	// Post-start behavior options.

	// WaitForAgent waits for the agent command to appear in the pane.
	WaitForAgent bool

	// WaitFatal makes WaitForAgent failure fatal — kills the session and returns error.
	// If false, WaitForAgent failure is silently ignored.
	WaitFatal bool

	// AcceptBypass accepts the bypass permissions warning dialog if it appears.
	AcceptBypass bool

	// ReadyDelay sleeps for the runtime's configured readiness delay.
	ReadyDelay bool

	// AutoRespawn sets the auto-respawn hook so the session survives crashes.
	AutoRespawn bool

	// RemainOnExit sets remain-on-exit immediately after session creation.
	RemainOnExit bool

	// TrackPID tracks the pane PID for defense-in-depth orphan cleanup.
	TrackPID bool

	// VerifySurvived checks that the session is still alive after startup.
	VerifySurvived bool
}

// StartResult contains the results of session startup.
type StartResult struct {
	// RuntimeConfig is the resolved runtime config for the role.
	// Callers may need this for role-specific post-startup steps
	// (e.g., handling fallback nudges, legacy fallback).
	RuntimeConfig *config.RuntimeConfig
}

// StartSession creates a tmux session following the standard Gas Town lifecycle.
//
// The lifecycle handles:
//  1. Resolve runtime config for the role
//  2. Ensure settings/plugins exist for the agent
//  3. Build startup command (if not provided)
//  4. Create tmux session with command
//  5. Set environment variables (standard + extra)
//  6. Apply theme (if configured)
//  7. Optional post-start: wait for agent, accept bypass, ready delay,
//     auto-respawn, PID tracking, verify survived
//
// Role-specific concerns (issue validation, fallback nudges, pane-died hooks,
// crew cycle bindings, etc.) should be handled by the caller before/after
// calling StartSession.
func StartSession(t *tmux.Tmux, cfg SessionConfig) (*StartResult, error) {
	if cfg.SessionID == "" {
		return nil, fmt.Errorf("SessionID is required")
	}
	if cfg.WorkDir == "" {
		return nil, fmt.Errorf("WorkDir is required")
	}
	if cfg.Role == "" {
		return nil, fmt.Errorf("Role is required")
	}

	// 1. Resolve runtime config.
	runtimeConfig := config.ResolveRoleAgentConfig(cfg.Role, cfg.TownRoot, cfg.RigPath)

	// 2. Ensure settings/plugins exist for the agent.
	settingsDir := config.RoleSettingsDir(cfg.Role, cfg.RigPath)
	if settingsDir == "" {
		settingsDir = cfg.WorkDir
	}
	if err := runtime.EnsureSettingsForRole(settingsDir, cfg.WorkDir, cfg.Role, runtimeConfig); err != nil {
		return nil, fmt.Errorf("ensuring runtime settings: %w", err)
	}

	// 3. Build startup command if not provided.
	command := cfg.Command
	if command == "" {
		prompt := buildPrompt(cfg)
		var err error
		command, err = buildCommand(cfg, prompt)
		if err != nil {
			return nil, fmt.Errorf("building startup command: %w", err)
		}
	}

	// Prepend runtime config dir env if needed.
	if runtimeConfig.Session != nil && runtimeConfig.Session.ConfigDirEnv != "" && cfg.RuntimeConfigDir != "" {
		command = config.PrependEnv(command, map[string]string{
			runtimeConfig.Session.ConfigDirEnv: cfg.RuntimeConfigDir,
		})
	}

	// Prepend extra env vars that need to be in the command (for initial shell inheritance).
	if len(cfg.ExtraEnv) > 0 {
		command = config.PrependEnv(command, cfg.ExtraEnv)
	}

	// 4. Create tmux session — prefer tmuxinator for declarative setup,
	// fall back to raw tmux if tmuxinator is not available.
	if tmuxinator.IsAvailable() {
		if err := startWithTmuxinator(cfg, command); err != nil {
			return nil, fmt.Errorf("creating session via tmuxinator: %w", err)
		}
	} else {
		if err := startWithRawTmux(t, cfg, command); err != nil {
			return nil, err
		}
	}

	// 5. Wait for agent to start.
	if cfg.WaitForAgent {
		if err := t.WaitForCommand(cfg.SessionID, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
			if cfg.WaitFatal {
				_ = t.KillSessionWithProcesses(cfg.SessionID)
				return nil, fmt.Errorf("waiting for %s to start: %w", cfg.Role, err)
			}
		}
	}

	// 6. Accept bypass permissions warning.
	if cfg.AcceptBypass {
		_ = t.AcceptBypassPermissionsWarning(cfg.SessionID)
	}

	// 7. Ready delay.
	if cfg.ReadyDelay {
		runtime.SleepForReadyDelay(runtimeConfig)
	}

	// 8. Verify session survived startup.
	if cfg.VerifySurvived {
		running, err := t.HasSession(cfg.SessionID)
		if err != nil {
			// Clean up session on verification error to prevent orphan
			_ = t.KillSessionWithProcesses(cfg.SessionID)
			return nil, fmt.Errorf("verifying session: %w", err)
		}
		if !running {
			return nil, fmt.Errorf("session %s died during startup (agent command may have failed)", cfg.SessionID)
		}
	}

	// 9. Track PID for defense-in-depth orphan cleanup.
	if cfg.TrackPID && cfg.TownRoot != "" {
		_ = TrackSessionPID(cfg.TownRoot, cfg.SessionID, t)
	}

	return &StartResult{RuntimeConfig: runtimeConfig}, nil
}

// StopSession stops a tmux session with optional graceful shutdown.
//
// If graceful is true, sends Ctrl-C first and waits for the session to exit
// before force-killing. This allows the agent to clean up.
func StopSession(t *tmux.Tmux, sessionID string, graceful bool) error {
	running, err := t.HasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if graceful {
		_ = t.SendKeysRaw(sessionID, "C-c")
		WaitForSessionExit(t, sessionID, constants.GracefulShutdownTimeout)
	}

	if err := t.KillSessionWithProcesses(sessionID); err != nil {
		return fmt.Errorf("killing session: %w", err)
	}

	return nil
}

// KillExistingSession kills an existing session if one is found.
// Returns true if a session was killed.
//
// If checkAlive is true, only kills zombie sessions (tmux alive but agent dead).
// If the session exists and the agent is alive, returns ErrAlreadyRunning.
// If checkAlive is false, kills any existing session unconditionally.
func KillExistingSession(t *tmux.Tmux, sessionID string, checkAlive bool) (bool, error) {
	running, err := t.HasSession(sessionID)
	if err != nil {
		return false, fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return false, nil
	}

	if checkAlive && t.IsAgentAlive(sessionID) {
		return false, fmt.Errorf("session already running: %s", sessionID)
	}

	if err := t.KillSessionWithProcesses(sessionID); err != nil {
		return false, fmt.Errorf("killing session %s: %w", sessionID, err)
	}

	return true, nil
}

// buildPrompt creates the startup prompt from beacon + instructions.
func buildPrompt(cfg SessionConfig) string {
	if cfg.Instructions != "" {
		return BuildStartupPrompt(cfg.Beacon, cfg.Instructions)
	}
	return FormatStartupBeacon(cfg.Beacon)
}

// buildCommand creates the startup command using the config package.
func buildCommand(cfg SessionConfig, prompt string) (string, error) {
	if cfg.AgentOverride != "" {
		return config.BuildAgentStartupCommandWithAgentOverride(
			cfg.Role, cfg.RigName, cfg.TownRoot, cfg.RigPath, prompt, cfg.AgentOverride)
	}
	return config.BuildAgentStartupCommand(
		cfg.Role, cfg.RigName, cfg.TownRoot, cfg.RigPath, prompt), nil
}

// ReadyDelay sleeps for the runtime's configured readiness delay.
// Exposed for callers that need to call it independently (e.g., when
// using a pre-built StartResult).
func ReadyDelay(rc *config.RuntimeConfig) {
	runtime.SleepForReadyDelay(rc)
}

// ShutdownDelay is the standard delay after session creation.
// Some roles use this instead of the runtime's ready delay.
func ShutdownDelay() time.Duration {
	return constants.ShutdownNotifyDelay
}

// startWithTmuxinator creates a session using tmuxinator's declarative YAML config.
// The YAML handles: session creation, env vars, theme, bindings, mouse, remain-on-exit,
// auto-respawn hooks, and the agent startup command — all in one atomic step.
func startWithTmuxinator(cfg SessionConfig, command string) error {
	tmuxCfg, err := tmuxinator.FromSessionConfig(tmuxinator.SessionConfig{
		SessionID:        cfg.SessionID,
		WorkDir:          cfg.WorkDir,
		Role:             cfg.Role,
		TownRoot:         cfg.TownRoot,
		RigPath:          cfg.RigPath,
		RigName:          cfg.RigName,
		AgentName:        cfg.AgentName,
		Command:          command,
		Theme:            cfg.Theme,
		ExtraEnv:         cfg.ExtraEnv,
		RemainOnExit:     cfg.RemainOnExit,
		AutoRespawn:      cfg.AutoRespawn,
		RuntimeConfigDir: cfg.RuntimeConfigDir,
	})
	if err != nil {
		return fmt.Errorf("generating tmuxinator config: %w", err)
	}

	// Write config to a temp file in the town's tmp directory
	tmpDir := filepath.Join(cfg.TownRoot, ".tmp")
	if cfg.TownRoot == "" {
		tmpDir = os.TempDir()
	}
	_ = os.MkdirAll(tmpDir, 0755)
	configPath := filepath.Join(tmpDir, fmt.Sprintf("tmuxinator-%s.yml", cfg.SessionID))

	if err := tmuxCfg.WriteToFile(configPath); err != nil {
		return fmt.Errorf("writing tmuxinator config: %w", err)
	}
	// Clean up config file after tmuxinator reads it
	defer os.Remove(configPath)

	if err := tmuxinator.Start(configPath); err != nil {
		return fmt.Errorf("tmuxinator start: %w", err)
	}
	return nil
}

// startWithRawTmux creates a session using direct tmux API calls.
// This is the fallback path when tmuxinator is not installed.
func startWithRawTmux(t *tmux.Tmux, cfg SessionConfig, command string) error {
	// Create tmux session with command.
	if err := t.NewSessionWithCommand(cfg.SessionID, cfg.WorkDir, command); err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	// Set remain-on-exit immediately if requested (before anything else can fail).
	if cfg.RemainOnExit {
		_ = t.SetRemainOnExit(cfg.SessionID, true)
	}

	// Set environment variables.
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:             cfg.Role,
		Rig:              cfg.RigName,
		AgentName:        cfg.AgentName,
		TownRoot:         cfg.TownRoot,
		RuntimeConfigDir: cfg.RuntimeConfigDir,
	})
	for k, v := range envVars {
		_ = t.SetEnvironment(cfg.SessionID, k, v)
	}
	for k, v := range cfg.ExtraEnv {
		_ = t.SetEnvironment(cfg.SessionID, k, v)
	}

	// Apply theme.
	if cfg.Theme != nil {
		_ = t.ConfigureGasTownSession(cfg.SessionID, *cfg.Theme, cfg.RigName, cfg.AgentName, cfg.Role)
	}

	// Auto-respawn hook (in raw tmux path only — tmuxinator handles this via on_project_start).
	if cfg.AutoRespawn {
		if err := t.SetAutoRespawnHook(cfg.SessionID); err != nil {
			fmt.Printf("warning: failed to set auto-respawn hook for %s: %v\n", cfg.Role, err)
		}
	}

	return nil
}
