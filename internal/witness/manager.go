package witness

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// Common errors
var (
	ErrNotRunning     = errors.New("witness not running")
	ErrAlreadyRunning = errors.New("witness already running")
)

// Manager handles witness lifecycle and monitoring operations.
// ZFC-compliant: tmux session is the source of truth for running state.
type Manager struct {
	rig *rig.Rig
}

// NewManager creates a new witness manager for a rig.
func NewManager(r *rig.Rig) *Manager {
	return &Manager{
		rig: r,
	}
}

// IsRunning checks if the witness session is active.
// ZFC: tmux session existence is the source of truth.
func (m *Manager) IsRunning() (bool, error) {
	t := tmux.NewTmux()
	return t.HasSession(m.SessionName())
}

// SessionName returns the tmux session name for this witness.
func (m *Manager) SessionName() string {
	return session.WitnessSessionName(session.PrefixFor(m.rig.Name))
}

// Status returns information about the witness session.
// ZFC-compliant: tmux session is the source of truth.
func (m *Manager) Status() (*tmux.SessionInfo, error) {
	t := tmux.NewTmux()
	sessionID := m.SessionName()

	running, err := t.HasSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return nil, ErrNotRunning
	}

	return t.GetSessionInfo(sessionID)
}

// witnessDir returns the working directory for the witness.
// Prefers witness/rig/, falls back to witness/, then rig root.
func (m *Manager) witnessDir() string {
	witnessRigDir := filepath.Join(m.rig.Path, "witness", "rig")
	if _, err := os.Stat(witnessRigDir); err == nil {
		return witnessRigDir
	}

	witnessDir := filepath.Join(m.rig.Path, "witness")
	if _, err := os.Stat(witnessDir); err == nil {
		return witnessDir
	}

	return m.rig.Path
}

// Start starts the witness.
// If foreground is true, returns an error (foreground mode deprecated).
// Otherwise, spawns a Claude agent in a tmux session.
// agentOverride optionally specifies a different agent alias to use.
// envOverrides are KEY=VALUE pairs that override all other env var sources.
// ZFC-compliant: no state file, tmux session is source of truth.
func (m *Manager) Start(foreground bool, agentOverride string, envOverrides []string) error {
	t := tmux.NewTmux()
	sessionID := m.SessionName()

	if foreground {
		// Foreground mode is deprecated - patrol logic moved to mol-witness-patrol
		return fmt.Errorf("foreground mode is deprecated; use background mode (remove --foreground flag)")
	}

	// Check if session already exists
	running, _ := t.HasSession(sessionID)
	if running {
		// Session exists - check if Claude is actually running (healthy vs zombie)
		if t.IsAgentAlive(sessionID) {
			// Healthy - Claude is running
			return ErrAlreadyRunning
		}
		// Zombie - tmux alive but Claude dead. Kill and recreate.
		if err := t.KillSession(sessionID); err != nil {
			return fmt.Errorf("killing zombie session: %w", err)
		}
	}

	// Note: No PID check per ZFC - tmux session is the source of truth

	// Working directory
	witnessDir := m.witnessDir()

	// Ensure runtime settings exist in the shared witness parent directory.
	// Settings are passed to Claude Code via --settings flag.
	// ResolveRoleAgentConfig is internally serialized (resolveConfigMu in
	// package config) to prevent concurrent rig starts from corrupting the
	// global agent registry.
	townRoot := m.townRoot()
	runtimeConfig := config.ResolveRoleAgentConfig("witness", townRoot, m.rig.Path)
	witnessSettingsDir := config.RoleSettingsDir("witness", m.rig.Path)
	if err := runtime.EnsureSettingsForRole(witnessSettingsDir, witnessDir, "witness", runtimeConfig); err != nil {
		return fmt.Errorf("ensuring runtime settings: %w", err)
	}

	// Ensure .gitignore has required Gas Town patterns
	if err := rig.EnsureGitignorePatterns(witnessDir); err != nil {
		fmt.Printf("Warning: could not update witness .gitignore: %v\n", err)
	}

	roleConfig, err := m.roleConfig()
	if err != nil {
		return err
	}

	// Build startup command first
	// NOTE: No gt prime injection needed - SessionStart hook handles it automatically
	// Export GT_ROLE and BD_ACTOR in the command since tmux SetEnvironment only affects new panes
	// Pass m.rig.Path so rig agent settings are honored (not town-level defaults)
	command, err := buildWitnessStartCommand(m.rig.Path, m.rig.Name, townRoot, agentOverride, roleConfig)
	if err != nil {
		return err
	}

	// Build extra env vars from role config + CLI overrides.
	extraEnv := make(map[string]string)
	for key, value := range roleConfigEnvVars(roleConfig, townRoot, m.rig.Name) {
		extraEnv[key] = value
	}
	for _, override := range envOverrides {
		if key, value, ok := strings.Cut(override, "="); ok {
			extraEnv[key] = value
		}
	}

	// Create session using unified StartSession (benefits from tmuxinator).
	theme := tmux.AssignTheme(m.rig.Name)
	_, err = session.StartSession(t, session.SessionConfig{
		SessionID:    sessionID,
		WorkDir:      witnessDir,
		Role:         "witness",
		TownRoot:     townRoot,
		RigPath:      m.rig.Path,
		RigName:      m.rig.Name,
		AgentName:    "witness",
		Command:      command,
		Theme:        &theme,
		ExtraEnv:     extraEnv,
		WaitForAgent: true,
		WaitFatal:    true,
		AcceptBypass: true,
		TrackPID:     true,
	})
	if err != nil {
		return err
	}

	time.Sleep(constants.ShutdownNotifyDelay)

	return nil
}

func (m *Manager) roleConfig() (*beads.RoleConfig, error) {
	// Role beads use hq- prefix and live in town-level beads, not rig beads
	townRoot := m.townRoot()
	bd := beads.NewWithBeadsDir(townRoot, beads.ResolveBeadsDir(townRoot))
	roleConfig, err := bd.GetRoleConfig(beads.RoleBeadIDTown("witness"))
	if err != nil {
		return nil, fmt.Errorf("loading witness role config: %w", err)
	}
	return roleConfig, nil
}

func (m *Manager) townRoot() string {
	townRoot, err := workspace.Find(m.rig.Path)
	if err != nil || townRoot == "" {
		return m.rig.Path
	}
	return townRoot
}

func roleConfigEnvVars(roleConfig *beads.RoleConfig, townRoot, rigName string) map[string]string {
	if roleConfig == nil || len(roleConfig.EnvVars) == 0 {
		return nil
	}
	expanded := make(map[string]string, len(roleConfig.EnvVars))
	for key, value := range roleConfig.EnvVars {
		expanded[key] = beads.ExpandRolePattern(value, townRoot, rigName, "", "witness")
	}
	return expanded
}

func buildWitnessStartCommand(rigPath, rigName, townRoot, agentOverride string, roleConfig *beads.RoleConfig) (string, error) {
	if agentOverride != "" {
		roleConfig = nil
	}
	if roleConfig != nil && roleConfig.StartCommand != "" {
		return beads.ExpandRolePattern(roleConfig.StartCommand, townRoot, rigName, "", "witness"), nil
	}
	initialPrompt := session.BuildStartupPrompt(session.BeaconConfig{
		Recipient: fmt.Sprintf("%s/witness", rigName),
		Sender:    "deacon",
		Topic:     "patrol",
	}, "Run `gt prime --hook` and begin patrol.")
	command, err := config.BuildAgentStartupCommandWithAgentOverride("witness", rigName, townRoot, rigPath, initialPrompt, agentOverride)
	if err != nil {
		return "", fmt.Errorf("building startup command: %w", err)
	}
	return command, nil
}

// Stop stops the witness.
// ZFC-compliant: tmux session is the source of truth.
func (m *Manager) Stop() error {
	t := tmux.NewTmux()
	sessionID := m.SessionName()

	// Check if tmux session exists
	running, _ := t.HasSession(sessionID)
	if !running {
		return ErrNotRunning
	}

	// Kill the tmux session
	return t.KillSession(sessionID)
}
