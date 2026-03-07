package witness

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
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

// IsRunning checks if the witness session is active and healthy.
// Checks both tmux session existence AND agent process liveness to avoid
// reporting zombie sessions (tmux alive but Claude dead) as "running".
// ZFC: tmux session existence is the source of truth for session state,
// but agent liveness determines if the session is actually functional.
func (m *Manager) IsRunning() (bool, error) {
	t := tmux.NewTmux()
	status := t.CheckSessionHealth(m.SessionName(), 0)
	return status == tmux.SessionHealthy, nil
}

// IsHealthy checks if the witness is running and has been active recently.
// Unlike IsRunning which only checks process liveness, this also detects hung
// sessions where Claude is alive but hasn't produced output in maxInactivity.
// Returns the detailed ZombieStatus for callers that need to distinguish
// between different failure modes.
func (m *Manager) IsHealthy(maxInactivity time.Duration) tmux.ZombieStatus {
	t := tmux.NewTmux()
	return t.CheckSessionHealth(m.SessionName(), maxInactivity)
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
		// Zombie detected — tmux alive but agent dead.
		// Mitigate TOCTOU gap: the agent may be slow to start, appearing
		// dead during initialization. Record session creation time, wait
		// briefly, then re-verify before killing to avoid destroying a
		// session that just became healthy.
		createdAt, _ := t.GetSessionCreatedUnix(sessionID)
		time.Sleep(constants.ZombieKillGracePeriod)

		// Re-check: abort kill if agent started or session was replaced
		if t.IsAgentAlive(sessionID) {
			return ErrAlreadyRunning
		}
		if createdNow, _ := t.GetSessionCreatedUnix(sessionID); createdAt > 0 && createdNow != createdAt {
			// Session was replaced between checks — another process already
			// handled the zombie. Treat as already running; caller can retry.
			return ErrAlreadyRunning
		}

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
		style.PrintWarning("could not update witness .gitignore: %v", err)
	}

	roleConfig, err := m.roleConfig()
	if err != nil {
		// Non-fatal: role config is optional. Log and continue with defaults.
		log.Printf("warning: could not load witness role config for %s: %v", m.rig.Name, err)
		roleConfig = nil
	}

	// Build startup command first
	// NOTE: No gt prime injection needed - SessionStart hook handles it automatically
	// Export GT_ROLE and BD_ACTOR in the command since tmux SetEnvironment only affects new panes
	// Pass m.rig.Path so rig agent settings are honored (not town-level defaults)
	command, err := buildWitnessStartCommand(m.rig.Path, m.rig.Name, townRoot, sessionID, agentOverride, roleConfig)
	if err != nil {
		return err
	}

	// Generate the GASTA run ID for this witness session.
	runID := uuid.New().String()

	// Create session with command directly to avoid send-keys race condition.
	// See: https://github.com/anthropics/gastown/issues/280
	if err := t.NewSessionWithCommand(sessionID, witnessDir, command); err != nil {
		return fmt.Errorf("creating tmux session: %w", err)
	}

	// Set environment variables (non-fatal: session works without these)
	// Use centralized AgentEnv for consistency across all role startup paths
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:        "witness",
		Rig:         m.rig.Name,
		TownRoot:    townRoot,
		Agent:       agentOverride,
		SessionName: sessionID,
	})
	envVars = session.MergeRuntimeLivenessEnv(envVars, runtimeConfig)
	for k, v := range envVars {
		_ = t.SetEnvironment(sessionID, k, v)
	}
	_ = t.SetEnvironment(sessionID, "GT_RUN", runID)
	// Apply role config env vars if present (non-fatal).
	// Skip keys already set by AgentEnv — AgentEnv is the single source of truth
	// for identity vars (GT_ROLE, BD_ACTOR, etc.) and produces fully-qualified
	// values (e.g., "beacon/witness"). TOML [env] sections contain unqualified
	// values (e.g., "witness") that would overwrite the correct ones.
	for key, value := range roleConfigEnvVars(roleConfig, townRoot, m.rig.Name) {
		if _, alreadySet := envVars[key]; alreadySet {
			continue
		}
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

	// Accept startup dialogs (workspace trust + bypass permissions) if they appear.
	if err := t.AcceptStartupDialogs(sessionID); err != nil {
		log.Printf("warning: accepting startup dialogs for %s: %v", sessionID, err)
	}

	// Track PID for defense-in-depth orphan cleanup (non-fatal)
	if err := session.TrackSessionPID(townRoot, sessionID, t); err != nil {
		log.Printf("warning: tracking session PID for %s: %v", sessionID, err)
	}

	// Stream witness's Claude Code JSONL conversation log to VictoriaLogs (opt-in).
	if os.Getenv("GT_LOG_AGENT_OUTPUT") == "true" && os.Getenv("GT_OTEL_LOGS_URL") != "" {
		if err := session.ActivateAgentLogging(sessionID, witnessDir, runID); err != nil {
			log.Printf("warning: agent log watcher setup failed for %s: %v", sessionID, err)
		}
	}

	// Record the agent instantiation event (GASTA root span).
	session.RecordAgentInstantiateFromDir(context.Background(), runID, runtimeConfig.ResolvedAgent,
		"witness", "witness", sessionID, m.rig.Name, townRoot, "", witnessDir)

	time.Sleep(constants.ShutdownNotifyDelay)

	return nil
}

func (m *Manager) roleConfig() (*beads.RoleConfig, error) {
	townRoot := m.townRoot()
	roleDef, err := config.LoadRoleDefinition(townRoot, m.rig.Path, "witness")
	if err != nil {
		return nil, fmt.Errorf("loading witness role config: %w", err)
	}
	return &beads.RoleConfig{
		SessionPattern: roleDef.Session.Pattern,
		WorkDirPattern: roleDef.Session.WorkDir,
		NeedsPreSync:   roleDef.Session.NeedsPreSync,
		StartCommand:   roleDef.Session.StartCommand,
		EnvVars:        roleDef.Env,
	}, nil
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
		expanded[key] = beads.ExpandRolePattern(value, townRoot, rigName, "", "witness", session.PrefixFor(rigName))
	}
	return expanded
}

func buildWitnessStartCommand(rigPath, rigName, townRoot, sessionName, agentOverride string, roleConfig *beads.RoleConfig) (string, error) {
	if agentOverride != "" {
		roleConfig = nil
	}
	if roleConfig != nil && roleConfig.StartCommand != "" {
		return beads.ExpandRolePattern(roleConfig.StartCommand, townRoot, rigName, "", "witness", session.PrefixFor(rigName)), nil
	}
	initialPrompt := session.BuildStartupPrompt(session.BeaconConfig{
		Recipient: session.BeaconRecipient("witness", "", rigName),
		Sender:    "deacon",
		Topic:     "patrol",
	}, "Run `gt prime --hook` and begin patrol.")
	command, err := config.BuildStartupCommandFromConfig(config.AgentEnvConfig{
		Role:        "witness",
		Rig:         rigName,
		TownRoot:    townRoot,
		Prompt:      initialPrompt,
		Topic:       "patrol",
		SessionName: sessionName,
	}, rigPath, initialPrompt, agentOverride)
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
