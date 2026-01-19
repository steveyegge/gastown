// Package agent provides the Agents manager for agent processes.
package agent

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/ids"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
)

// AgentID is the logical address of an agent.
// Re-exported from ids package for convenience.
type AgentID = ids.AgentID

// Re-export address values from ids package.
var (
	MayorAddress  = ids.MayorAddress
	DeaconAddress = ids.DeaconAddress
	BootAddress   = ids.BootAddress
)

// Re-export address constructors from ids package.
var (
	WitnessAddress  = ids.WitnessAddress
	RefineryAddress = ids.RefineryAddress
	PolecatAddress  = ids.PolecatAddress
	CrewAddress     = ids.CrewAddress
)

// ErrUnknownRole is returned when the agent role cannot be determined.
var ErrUnknownRole = errors.New("unknown or missing GT_ROLE")

// Self returns the AgentID for the current process based on GT_* environment variables.
// This allows an agent to identify itself without querying tmux.
//
// Required env vars by role:
//   - mayor: GT_ROLE=mayor
//   - deacon: GT_ROLE=deacon
//   - witness: GT_ROLE=witness, GT_RIG
//   - refinery: GT_ROLE=refinery, GT_RIG
//   - crew: GT_ROLE=crew, GT_RIG, GT_CREW
//   - polecat: GT_ROLE=polecat, GT_RIG, GT_POLECAT
func Self() (AgentID, error) {
	role := os.Getenv("GT_ROLE")
	rig := os.Getenv("GT_RIG")

	switch role {
	case constants.RoleMayor:
		return ids.MayorAddress, nil
	case constants.RoleDeacon:
		return ids.DeaconAddress, nil
	case constants.RoleBoot:
		return ids.BootAddress, nil
	case constants.RoleWitness:
		if rig == "" {
			return AgentID{}, fmt.Errorf("%w: witness requires GT_RIG", ErrUnknownRole)
		}
		return ids.WitnessAddress(rig), nil
	case constants.RoleRefinery:
		if rig == "" {
			return AgentID{}, fmt.Errorf("%w: refinery requires GT_RIG", ErrUnknownRole)
		}
		return ids.RefineryAddress(rig), nil
	case constants.RoleCrew:
		name := os.Getenv("GT_CREW")
		if rig == "" || name == "" {
			return AgentID{}, fmt.Errorf("%w: crew requires GT_RIG and GT_CREW", ErrUnknownRole)
		}
		return ids.CrewAddress(rig, name), nil
	case constants.RolePolecat:
		name := os.Getenv("GT_POLECAT")
		if rig == "" || name == "" {
			return AgentID{}, fmt.Errorf("%w: polecat requires GT_RIG and GT_POLECAT", ErrUnknownRole)
		}
		return ids.PolecatAddress(rig, name), nil
	case "":
		return AgentID{}, ErrUnknownRole
	default:
		return AgentID{}, fmt.Errorf("%w: %s", ErrUnknownRole, role)
	}
}

// ErrAlreadyRunning is returned when trying to start an already running agent.
var ErrAlreadyRunning = errors.New("agent already running")

// ErrNotRunning is returned when trying to operate on a non-running agent.
var ErrNotRunning = errors.New("agent not running")

// Agents is the full interface for managing agent processes.
// It composes all the smaller interfaces for consumers that need full control.
//
// Most consumers should depend on smaller interfaces instead:
//   - AgentObserver: for status checks (Exists, GetInfo, List)
//   - AgentStopper: for cleanup (Stop)
//   - AgentStarter: for lifecycle (StartWithConfig, WaitReady)
//   - AgentCommunicator: for interaction (Nudge, Capture, Attach)
//   - AgentRespawner: for handoff (Respawn)
//
// See interfaces.go for the smaller interface definitions.
type Agents interface {
	AgentObserver
	AgentStopper
	AgentStarter
	AgentCommunicator
	AgentRespawner
}

// Implementation is the concrete implementation of the Agents interface.
// It provides runtime-aware lifecycle management.
type Implementation struct {
	sess   session.Sessions
	config *Config
}

// Ensure Implementation implements Agents
var _ Agents = (*Implementation)(nil)

// New creates a new Agents implementation.
// The Sessions handles address-to-session mapping (typically TownSessions with town-specific hashing).
func New(sess session.Sessions, config *Config) *Implementation {
	if config == nil {
		config = Claude()
	}
	return &Implementation{
		sess:   sess,
		config: config,
	}
}

// timeout returns the effective timeout for readiness detection.
func (a *Implementation) timeout() time.Duration {
	if a.config.Timeout > 0 {
		return a.config.Timeout
	}
	return 30 * time.Second // Default fallback
}

// prependEnvVars prepends environment variables to a command.
// Returns a command like "VAR1=val1 VAR2=val2 original-command".
// Precondition: envVars is non-empty (caller checks before calling).
func prependEnvVars(envVars map[string]string, command string) string {
	// Sort keys for deterministic output
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, envVars[k]))
	}

	return strings.Join(parts, " ") + " " + command
}

// doWaitForReady implements the readiness wait logic.
func (a *Implementation) doWaitForReady(id AgentID) error {
	sessionID := a.sess.SessionIDForAgent(id)

	// Run startup hook if defined (e.g., dismiss dialogs)
	if a.config.StartupHook != nil {
		_ = a.config.StartupHook(a.sess, sessionID) // Non-fatal
	}

	// Use checker if available
	if a.config.Checker != nil {
		return a.waitForReady(sessionID, a.timeout(), a.config.Checker)
	}

	// Fall back to startup delay
	if a.config.StartupDelay > 0 {
		time.Sleep(a.config.StartupDelay)
	}

	return nil
}

// waitForReady polls until the agent is ready or times out.
func (a *Implementation) waitForReady(id session.SessionID, timeout time.Duration, checker ReadinessChecker) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		output, err := a.sess.Capture(id, 50)
		if err == nil && checker.IsReady(output) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for agent ready")
}

// WaitReady blocks until the agent is ready for input or times out.
func (a *Implementation) WaitReady(id AgentID) error {
	sessionID := a.sess.SessionIDForAgent(id)
	exists, _ := a.sess.Exists(sessionID)
	if !exists {
		return ErrNotRunning
	}
	return a.doWaitForReady(id)
}

// Stop terminates an agent process.
func (a *Implementation) Stop(id AgentID, graceful bool) error {
	sessionID := a.sess.SessionIDForAgent(id)
	exists, _ := a.sess.Exists(sessionID)
	if !exists {
		return nil // Idempotent - nothing to stop
	}

	if graceful {
		_ = a.sess.SendControl(sessionID, "C-c")
		time.Sleep(100 * time.Millisecond)
	}

	if err := a.sess.Stop(sessionID); err != nil {
		return fmt.Errorf("stopping session: %w", err)
	}

	return nil
}

// Respawn atomically kills the agent process and starts a new one.
// Clears scrollback history before respawning for a clean start.
// This is used for handoff - an agent can respawn itself or another agent.
//
// The original command (including all env vars and beacon) is reused.
// Work discovery happens through hooks, not through the beacon.
//
// For self-handoff: sess.Respawn() terminates the calling process, so nothing
// after that call executes. The new process starts fresh.
//
// For remote handoff: the caller survives, so we launch the readiness wait.
func (a *Implementation) Respawn(id AgentID) error {
	sessionID := a.sess.SessionIDForAgent(id)
	exists, _ := a.sess.Exists(sessionID)
	if !exists {
		return ErrNotRunning
	}

	// Get the original start command (includes all env vars and beacon)
	command, err := a.sess.GetStartCommand(sessionID)
	if err != nil {
		return fmt.Errorf("getting start command: %w", err)
	}

	// For self-handoff, this call terminates the current process.
	// For remote handoff, execution continues below.
	if err := a.sess.Respawn(sessionID, command); err != nil {
		return fmt.Errorf("respawning session: %w", err)
	}

	// Only reached for remote handoff (respawning a different agent)
	go func() { _ = a.doWaitForReady(id) }()

	return nil
}

// Exists checks if an agent is running (session exists AND process is alive).
// Returns false for zombie sessions (tmux exists but agent process died).
// If ProcessNames is not configured, falls back to session existence only.
func (a *Implementation) Exists(id AgentID) bool {
	sessionID := a.sess.SessionIDForAgent(id)
	exists, _ := a.sess.Exists(sessionID)
	if !exists {
		return false
	}
	// If no process names configured, can't check - assume session exists = agent exists
	if len(a.config.ProcessNames) == 0 {
		return true
	}
	// Check if agent process is actually running within the session
	return a.sess.IsRunning(sessionID, a.config.ProcessNames...)
}

// GetInfo returns information about an agent's session.
func (a *Implementation) GetInfo(id AgentID) (*session.Info, error) {
	return a.sess.GetInfo(a.sess.SessionIDForAgent(id))
}

// Nudge sends a message to a running agent reliably.
func (a *Implementation) Nudge(id AgentID, message string) error {
	return a.sess.Nudge(a.sess.SessionIDForAgent(id), message)
}

// Capture returns the recent output from an agent's session.
func (a *Implementation) Capture(id AgentID, lines int) (string, error) {
	return a.sess.Capture(a.sess.SessionIDForAgent(id), lines)
}

// CaptureAll returns the entire scrollback history from an agent's session.
func (a *Implementation) CaptureAll(id AgentID) (string, error) {
	return a.sess.CaptureAll(a.sess.SessionIDForAgent(id))
}

// List returns all agent addresses.
func (a *Implementation) List() ([]AgentID, error) {
	sessionIDs, err := a.sess.List()
	if err != nil {
		return nil, err
	}
	result := make([]AgentID, 0, len(sessionIDs))
	for _, sid := range sessionIDs {
		// If no process names configured, include all sessions
		// Otherwise only include if process is running (not zombie)
		if len(a.config.ProcessNames) == 0 || a.sess.IsRunning(sid, a.config.ProcessNames...) {
			// Convert SessionID to AgentID via parsing
			agentID, err := session.ParseSessionName(string(sid))
			if err != nil {
				continue // Skip sessions we can't parse (non-GT sessions)
			}
			result = append(result, agentID)
		}
	}
	return result, nil
}

// Attach attaches to a running agent's session.
// Smart context detection:
//   - Inside tmux → switch-client (no-op if already in target)
//   - Outside tmux → blocking attach
func (a *Implementation) Attach(id AgentID) error {
	sessionID := a.sess.SessionIDForAgent(id)

	// Try to switch (works if inside tmux, no-op if already in target)
	if err := a.sess.SwitchTo(sessionID); err == nil {
		return nil
	}

	// Not inside tmux - do a blocking attach
	return a.sess.Attach(sessionID)
}

// =============================================================================
// Factory Functions
// =============================================================================

// Default creates an Agents interface with default settings.
func Default() Agents {
	t := tmux.NewTmux()
	return New(t, nil)
}

// WithConfig creates an Agents interface with the specified config.
// Use Claude() for Claude-specific behavior (zombie filtering, readiness).
func WithConfig(cfg *Config) Agents {
	t := tmux.NewTmux()
	return New(t, cfg)
}
