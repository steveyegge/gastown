package witness

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/rig"
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
	return fmt.Sprintf("gt-%s-witness", m.rig.Name)
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
		if t.IsClaudeRunning(sessionID) {
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

	// Ensure Claude settings exist in witness/ (not witness/rig/) so we don't
	// write into the source repo. Claude walks up the tree to find settings.
	witnessParentDir := filepath.Join(m.rig.Path, "witness")
	if err := claude.EnsureSettingsForRole(witnessParentDir, "witness"); err != nil {
		return fmt.Errorf("ensuring Claude settings: %w", err)
	}

	roleConfig, err := m.roleConfig()
	if err != nil {
		return err
	}

	townRoot := m.townRoot()

	// Build startup command first
	// NOTE: No gt prime injection needed - SessionStart hook handles it automatically
	// Export GT_ROLE and BD_ACTOR in the command since tmux SetEnvironment only affects new panes
	// Pass m.rig.Path so rig agent settings are honored (not town-level defaults)
	command, err := buildWitnessStartCommand(m.rig.Path, m.rig.Name, townRoot, agentOverride, roleConfig)
	if err != nil {
		return err
	}

	// Create session with command directly to avoid send-keys race condition.
	// See: https://github.com/anthropics/gastown/issues/280
	if err := t.NewSessionWithCommand(sessionID, witnessDir, command); err != nil {
		return fmt.Errorf("creating tmux session: %w", err)
	}

	// Set environment variables (non-fatal: session works without these)
	// Use centralized AgentEnv for consistency across all role startup paths
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:     "witness",
		Rig:      m.rig.Name,
		TownRoot: townRoot,
	})
	for k, v := range envVars {
		_ = t.SetEnvironment(sessionID, k, v)
	}
	// Apply role config env vars if present (non-fatal).
	for key, value := range roleConfigEnvVars(roleConfig, townRoot, m.rig.Name) {
		_ = t.SetEnvironment(sessionID, key, value)
	}
	// Apply CLI env overrides (highest priority, non-fatal).
	for _, override := range envOverrides {
		if key, value, ok := strings.Cut(override, "="); ok {
			_ = t.SetEnvironment(sessionID, key, value)
		}
	}

	// Apply Gas Town theming (non-fatal: theming failure doesn't affect operation)
	theme := tmux.AssignTheme(m.rig.Name)
	_ = t.ConfigureGasTownSession(sessionID, theme, m.rig.Name, "witness", "witness")

	// Wait for Claude to start - fatal if Claude fails to launch
	if err := t.WaitForCommand(sessionID, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
		// Kill the zombie session before returning error
		_ = t.KillSessionWithProcesses(sessionID)
		return fmt.Errorf("waiting for witness to start: %w", err)
	}

	// Accept bypass permissions warning dialog if it appears.
	_ = t.AcceptBypassPermissionsWarning(sessionID)

	time.Sleep(constants.ShutdownNotifyDelay)

	// Inject startup nudge for predecessor discovery via /resume
	address := fmt.Sprintf("%s/witness", m.rig.Name)
	_ = session.StartupNudge(t, sessionID, session.StartupNudgeConfig{
		Recipient: address,
		Sender:    "deacon",
		Topic:     "patrol",
	}) // Non-fatal

	// GUPP: Gas Town Universal Propulsion Principle
	// Send the propulsion nudge to trigger autonomous patrol execution.
	// Wait for beacon to be fully processed (needs to be separate prompt)
	time.Sleep(2 * time.Second)
	_ = t.NudgeSession(sessionID, session.PropulsionNudgeForRole("witness", witnessDir)) // Non-fatal

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
	// Add initial prompt for autonomous patrol startup.
	// The prompt triggers GUPP: witness starts patrol immediately without waiting for input.
	initialPrompt := "I am Witness for " + rigName + ". Start patrol: check gt hook, if empty create mol-witness-patrol wisp and execute it."
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
