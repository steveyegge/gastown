// Package session provides session lifecycle management for Gas Town agents.
package session

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/tmux"
)

// StartSessionOpts configures agent session startup.
type StartSessionOpts struct {
	// SessionID is the tmux session identifier.
	SessionID string

	// WorkDir is the working directory for the session.
	WorkDir string

	// Role is the agent role: mayor, deacon, witness, refinery, polecat, crew.
	Role string

	// Rig is the rig name (empty for town-level roles like mayor/deacon).
	Rig string

	// AgentName is the specific agent name for polecats/crew (empty for singletons).
	AgentName string

	// TownRoot is the Gas Town root directory.
	TownRoot string

	// RigPath is the path to the rig directory (empty for town-level roles).
	RigPath string

	// Beacon is the startup beacon message to send to the agent.
	// If SupportsMultilinePrompt is true, embedded in command; otherwise sent via NudgeSession.
	Beacon string

	// AgentOverride optionally specifies an agent alias to use instead of defaults.
	AgentOverride string

	// RuntimeConfigDir is the optional CLAUDE_CONFIG_DIR path.
	RuntimeConfigDir string

	// BeadsNoDaemon sets BEADS_NO_DAEMON=1 if true.
	BeadsNoDaemon bool

	// WaitForReady controls whether to wait for the agent to be ready.
	// Default is true for PTY agents (needed before sending beacon).
	WaitForReady *bool

	// Interactive removes --dangerously-skip-permissions for human-attended sessions.
	// This is primarily used for crew members where the user will be interacting directly.
	Interactive bool

	// ExtraEnv contains additional environment variables to include in the session.
	// These are merged with the standard agent env vars (ExtraEnv takes precedence).
	ExtraEnv map[string]string
}

// StartSession creates a tmux session for an agent using the appropriate startup path.
// For agents that need PTY access (like cursor-agent), uses NewSessionWithEnvAndCommand.
// For standard agents, uses NewSessionWithCommand with shell wrapper.
// Handles beacon delivery based on agent's SupportsMultilinePrompt flag.
func StartSession(t *tmux.Tmux, opts StartSessionOpts) error {
	// Resolve agent configuration and name
	_, rc, err := resolveAgent(opts)
	if err != nil {
		return fmt.Errorf("resolving agent: %w", err)
	}

	// Get agent capability flags
	// Use rc.Command (e.g., "cursor-agent") not agentName (e.g., "composer-1")
	// because PTY requirements are determined by the actual binary, not the alias
	needsPTY := config.GetNeedsPTY(rc.Command)
	supportsMultiline := config.GetSupportsMultilinePrompt(rc.Command)

	// Build environment variables
	env := buildEnv(opts)

	// Determine beacon handling
	beaconInCommand := opts.Beacon != "" && supportsMultiline
	beaconDeferred := opts.Beacon != "" && !supportsMultiline

	// Build agent command
	var agentCmd string
	if beaconInCommand {
		agentCmd = rc.BuildCommandWithPrompt(opts.Beacon)
	} else {
		agentCmd = rc.BuildCommand()
	}

	// For interactive mode, remove --dangerously-skip-permissions
	// This allows Claude to prompt for confirmation on dangerous operations
	if opts.Interactive {
		agentCmd = strings.Replace(agentCmd, " --dangerously-skip-permissions", "", 1)
	}

	// For PTY agents, resolve command to full path
	// This is necessary because exec runs before the shell sources RC files,
	// so PATH may not include directories like ~/.local/bin
	if needsPTY {
		agentCmd = resolveCommandPath(agentCmd)
	}

	// Create session using appropriate path
	if needsPTY {
		// PTY-aware path: use shell wrapper with exec to replace shell with agent
		// This ensures the agent IS the main pane process with proper PTY access.
		// Using exec in the shell command is more reliable than send-keys approach.
		fullCmd := buildPTYCommand(env, agentCmd)
		if err := t.NewSessionWithCommand(opts.SessionID, opts.WorkDir, fullCmd); err != nil {
			return fmt.Errorf("creating PTY session: %w", err)
		}
	} else {
		// Standard path: shell wrapper with env exports
		fullCmd := config.BuildStartupCommandWithEnv(env, agentCmd, "")
		if err := t.NewSessionWithCommand(opts.SessionID, opts.WorkDir, fullCmd); err != nil {
			return fmt.Errorf("creating session: %w", err)
		}
	}

	// Set tmux environment variables (for new panes)
	for k, v := range env {
		_ = t.SetEnvironment(opts.SessionID, k, v)
	}

	// Handle deferred beacon delivery
	if beaconDeferred {
		// Wait for agent to be ready before sending beacon
		waitForReady := opts.WaitForReady == nil || *opts.WaitForReady
		if waitForReady {
			if err := t.WaitForCommand(opts.SessionID, constants.SupportedShells, constants.ClaudeStartTimeout); err != nil {
				// Non-fatal - try to send beacon anyway
			}
			// Additional delay for agent to fully initialize
			// cursor-agent needs more time than Claude for UI to be ready
			time.Sleep(2 * time.Second)
		}

		// Send beacon via NudgeSession
		if err := t.NudgeSession(opts.SessionID, opts.Beacon); err != nil {
			// Non-fatal - session is created, beacon delivery failed
		}
	}

	return nil
}

// resolveAgent resolves the agent configuration and returns the agent name and RuntimeConfig.
func resolveAgent(opts StartSessionOpts) (string, *config.RuntimeConfig, error) {
	townRoot := opts.TownRoot
	rigPath := opts.RigPath

	// Derive townRoot from rigPath if not provided
	if townRoot == "" && rigPath != "" {
		townRoot = filepath.Dir(rigPath)
	}

	// Use override if provided
	if opts.AgentOverride != "" {
		rc, agentName, err := config.ResolveAgentConfigWithOverride(townRoot, rigPath, opts.AgentOverride)
		if err != nil {
			return "", nil, err
		}
		// If agentName is empty (backwards compat path), use the override name
		if agentName == "" {
			agentName = opts.AgentOverride
		}
		return agentName, rc, nil
	}

	// Use role-based resolution
	agentName, _ := config.ResolveRoleAgentName(opts.Role, townRoot, rigPath)
	rc := config.ResolveRoleAgentConfig(opts.Role, townRoot, rigPath)
	return agentName, rc, nil
}

// buildEnv builds the environment variables for the session.
func buildEnv(opts StartSessionOpts) map[string]string {
	env := config.AgentEnv(config.AgentEnvConfig{
		Role:             opts.Role,
		Rig:              opts.Rig,
		AgentName:        opts.AgentName,
		TownRoot:         opts.TownRoot,
		RuntimeConfigDir: opts.RuntimeConfigDir,
		BeadsNoDaemon:    opts.BeadsNoDaemon,
	})
	// Merge extra env vars (takes precedence)
	for k, v := range opts.ExtraEnv {
		env[k] = v
	}
	return env
}

// resolveCommandPath resolves the first word of a command to its full path.
// This is needed for PTY agents where exec runs before shell RC files are sourced.
// e.g., "cursor-agent -f" -> "/Users/foo/.local/bin/cursor-agent -f"
func resolveCommandPath(command string) string {
	parts := strings.SplitN(command, " ", 2)
	if len(parts) == 0 {
		return command
	}

	binary := parts[0]
	// If already an absolute path, return as-is
	if strings.HasPrefix(binary, "/") {
		return command
	}

	// Try to resolve the full path
	fullPath, err := exec.LookPath(binary)
	if err != nil {
		// Couldn't resolve, return original
		return command
	}

	// Reconstruct command with full path
	if len(parts) == 1 {
		return fullPath
	}
	return fullPath + " " + parts[1]
}

// buildPTYCommand builds a shell command for PTY-sensitive agents.
// Uses exec to replace the shell with the agent, ensuring the agent is the main pane process.
// Format: export VAR1="val1" VAR2="val2" && exec /full/path/to/agent args
func buildPTYCommand(env map[string]string, agentCmd string) string {
	// Resolve command to full path
	agentCmd = resolveCommandPath(agentCmd)

	// Build export prefix
	var exports []string
	for k, v := range env {
		exports = append(exports, fmt.Sprintf("%s=%q", k, v))
	}

	// Sort for deterministic output
	sort.Strings(exports)

	if len(exports) == 0 {
		return "exec " + agentCmd
	}
	return "export " + strings.Join(exports, " ") + " && exec " + agentCmd
}
