package agent

import (
	"fmt"
)

// StartConfig holds configuration for starting an agent.
// This allows per-Start configuration instead of per-Agents configuration.
// The key benefit is callbacks can capture context via closures rather than
// requiring managers to stash mutable state.
type StartConfig struct {
	// WorkDir is the working directory for the agent.
	WorkDir string

	// Command is the command to execute.
	Command string

	// EnvVars are environment variables to prepend to the command.
	// These are merged with any EnvVars configured at the Agents level.
	EnvVars map[string]string

	// OnCreated is called immediately after session creation.
	// This is per-start (unlike Config.OnSessionCreated which is per-Agents).
	// Use for setup that depends on start-time context like agent name.
	OnCreated OnSessionCreated
}

// mergeEnvVars combines Agents-level and Start-level env vars.
// Start-level vars override Agents-level vars with the same key.
func mergeEnvVars(agentsVars, startVars map[string]string) map[string]string {
	if len(agentsVars) == 0 && len(startVars) == 0 {
		return nil
	}
	result := make(map[string]string)
	for k, v := range agentsVars {
		result[k] = v
	}
	for k, v := range startVars {
		result[k] = v
	}
	return result
}

// StartWithConfig launches an agent process with explicit configuration.
// This is more testable than Start() because all configuration is explicit.
//
// Differences from Start():
//   - WorkDir and Command are in StartConfig instead of separate parameters
//   - EnvVars from StartConfig are merged with Agents-level EnvVars
//   - OnCreated callback is per-start (can capture context via closure)
func (a *Implementation) StartWithConfig(id AgentID, cfg StartConfig) error {
	sessionID := a.sess.SessionIDForAgent(id)

	// Check for existing session and handle zombie detection
	exists, _ := a.sess.Exists(sessionID)
	if exists {
		// Session exists - check if agent is actually running (healthy vs zombie)
		if a.sess.IsRunning(sessionID, a.config.ProcessNames...) {
			return ErrAlreadyRunning
		}
		// Zombie - session alive but agent dead. Kill and recreate.
		if err := a.sess.Stop(sessionID); err != nil {
			return fmt.Errorf("killing zombie session: %w", err)
		}
	}

	// Merge env vars: Agents-level + Start-level (Start-level wins on conflict)
	envVars := mergeEnvVars(a.config.EnvVars, cfg.EnvVars)

	// Build final command with env vars prepended
	command := cfg.Command
	if len(envVars) > 0 {
		command = prependEnvVars(envVars, command)
	}

	// Create the session
	if _, err := a.sess.Start(string(sessionID), cfg.WorkDir, command); err != nil {
		return fmt.Errorf("starting session: %w", err)
	}

	// Run Agents-level callback first (if any)
	if a.config.OnSessionCreated != nil {
		if err := a.config.OnSessionCreated(sessionID); err != nil {
			_ = a.sess.Stop(sessionID)
			return fmt.Errorf("session setup: %w", err)
		}
	}

	// Run per-start callback (if any)
	if cfg.OnCreated != nil {
		if err := cfg.OnCreated(sessionID); err != nil {
			_ = a.sess.Stop(sessionID)
			return fmt.Errorf("session setup: %w", err)
		}
	}

	// Wait for agent to be ready (non-blocking)
	go func() { _ = a.doWaitForReady(id) }()

	return nil
}
