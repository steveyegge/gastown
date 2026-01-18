package factory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/refinery"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/witness"
)

// =============================================================================
// Singleton Agent Operations
//
// Agents() returns an Agents interface for interacting with any agent in a town.
// Start() starts a singleton agent with full production setup.
// =============================================================================

// Agents returns an Agents interface.
// Use this for operations like Stop, Nudge, Capture that work with any AgentID.
// The returned interface doesn't have role-specific configuration - use Start()
// for starting agents with proper env vars and settings.
func Agents() agent.Agents {
	t := tmux.NewTmux()
	return agent.New(t, agent.Claude())
}

// StartOption configures Start() behavior for specific roles.
type StartOption func(*startConfig)

// startConfig holds optional Start() configuration.
type startConfig struct {
	agent        string            // Agent alias override (--agent flag)
	topic        string            // Crew: startup topic for /resume beacon
	interactive  bool              // Crew: remove --dangerously-skip-permissions
	killExisting bool              // Stop existing session first
	envOverrides map[string]string // Additional env vars (e.g., witness overrides)
}

// WithAgent sets the agent alias override (e.g., from --agent flag).
// If not set, the agent is resolved from config based on the AgentID's role.
// Passing an empty string is a no-op, making it safe to use with optional overrides.
func WithAgent(agent string) StartOption {
	return func(c *startConfig) {
		if agent != "" {
			c.agent = agent
		}
	}
}

// WithTopic sets the startup topic for crew agents.
func WithTopic(topic string) StartOption {
	return func(c *startConfig) {
		c.topic = topic
	}
}

// WithInteractive enables interactive mode (no --dangerously-skip-permissions).
func WithInteractive() StartOption {
	return func(c *startConfig) {
		c.interactive = true
	}
}

// WithKillExisting stops any existing session before starting.
func WithKillExisting() StartOption {
	return func(c *startConfig) {
		c.killExisting = true
	}
}

// WithEnvOverrides adds additional environment variables.
func WithEnvOverrides(overrides map[string]string) StartOption {
	return func(c *startConfig) {
		c.envOverrides = overrides
	}
}

// resolveAgentForID resolves the agent alias for an AgentID from config.
// This is the internal auto-resolution that happens when aiRuntime is empty.
func resolveAgentForID(townRoot string, id agent.AgentID) string {
	role, rigName, _ := id.Parse()
	rigPath := ""
	if rigName != "" {
		rigPath = filepath.Join(townRoot, rigName)
	}
	agentName, _ := config.ResolveRoleAgentName(role, townRoot, rigPath)
	return agentName
}

// Start starts any agent with full production setup.
// Works for all agent types: singletons, rig-level, and named workers.
//
// Start blocks until the agent is ready for input (or times out). This differs
// from the old per-role managers which returned immediately after session creation.
// The blocking behavior ensures callers can interact with the agent immediately.
//
// Agent resolution: If aiRuntime is empty, the agent is automatically resolved
// from config based on the AgentID's role. Use WithAgent() to override.
//
// Usage:
//
//	factory.Start(townRoot, agent.MayorAddress, "")                              // Auto-resolve
//	factory.Start(townRoot, agent.WitnessAddress("myrig"), "")                   // Auto-resolve
//	factory.Start(townRoot, agent.PolecatAddress("myrig", "toast"), "")          // Auto-resolve
//	factory.Start(townRoot, agent.CrewAddress("myrig", "joe"), "", WithTopic("patrol"))
//	factory.Start(townRoot, agent.MayorAddress, "", WithAgent("custom"))         // Override
func Start(townRoot string, id agent.AgentID, aiRuntime string, opts ...StartOption) (agent.AgentID, error) {
	// Apply options first to check for WithAgent override
	cfg := &startConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Resolve agent name: WithAgent() > explicit aiRuntime > auto-resolve from config
	if cfg.agent != "" {
		aiRuntime = cfg.agent
	} else if aiRuntime == "" {
		aiRuntime = resolveAgentForID(townRoot, id)
	}

	// Build env vars for production agent creation
	role, rigName, worker := id.Parse()
	envCfg := config.AgentEnvConfig{
		Role:     role,
		TownRoot: townRoot,
	}
	if rigName != "" {
		envCfg.Rig = rigName
		envCfg.BeadsNoDaemon = true
	}
	if worker != "" {
		envCfg.AgentName = worker
	}
	envVars := config.AgentEnv(envCfg)

	// Create tmux
	t := tmux.NewTmux()
	agents := agent.New(t, agent.FromPreset(aiRuntime).WithEnvVars(envVars))

	// Build session configurer callback - passed to StartWithAgents for OnCreated
	themer := buildSessionConfigurer(id, envVars, t)

	return StartWithAgents(agents, themer, townRoot, id, aiRuntime, opts...)
}

// StartWithAgents is the testable version of Start().
// It accepts an injected Agents implementation and an optional theming callback.
//
// The themer callback can be nil for tests that don't need tmux theming.
//
// Usage in tests:
//
//	agents := agent.NewDouble()
//	id, err := factory.StartWithAgents(agents, nil, townRoot, agent.MayorAddress, "claude")
func StartWithAgents(
	agents agent.Agents,
	themer agent.OnSessionCreated,
	townRoot string,
	id agent.AgentID,
	aiRuntime string,
	opts ...StartOption,
) (agent.AgentID, error) {
	// Apply options
	cfg := &startConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Resolve agent name: WithAgent() > explicit aiRuntime > auto-resolve from config
	if cfg.agent != "" {
		aiRuntime = cfg.agent
	} else if aiRuntime == "" {
		aiRuntime = resolveAgentForID(townRoot, id)
	}

	role, rigName, worker := id.Parse()

	// Compute workDir
	workDir, err := WorkDirForID(townRoot, id)
	if err != nil {
		return agent.AgentID{}, err
	}

	// Build env vars from parsed components
	envCfg := config.AgentEnvConfig{
		Role:     role,
		TownRoot: townRoot,
	}
	if rigName != "" {
		envCfg.Rig = rigName
		envCfg.BeadsNoDaemon = true // Rig-level agents don't use daemon
	}
	if worker != "" {
		envCfg.AgentName = worker
	}
	envVars := config.AgentEnv(envCfg)

	// Apply env overrides
	for k, v := range cfg.envOverrides {
		envVars[k] = v
	}

	// Ensure runtime settings exist
	runtimeConfig := config.LoadRuntimeConfig(townRoot)
	if err := runtime.EnsureSettingsForRole(workDir, role, runtimeConfig); err != nil {
		return agent.AgentID{}, fmt.Errorf("ensuring runtime settings: %w", err)
	}

	// Build startup command
	startupCmd := buildCommand(role, aiRuntime, cfg)

	// Kill existing session if requested.
	// Error is intentionally ignored - the session may not exist, or may
	// already be stopped. We proceed with starting regardless.
	if cfg.killExisting {
		_ = agents.Stop(id, true)
	}

	// Build StartConfig with OnCreated callback for theming (may be nil in tests)
	startCfg := agent.StartConfig{
		WorkDir:   workDir,
		Command:   startupCmd,
		EnvVars:   envVars,
		OnCreated: themer,
	}
	if err := agents.StartWithConfig(id, startCfg); err != nil {
		return agent.AgentID{}, err
	}

	// Wait for agent to be ready - fatal if agent fails to launch.
	// This blocks until the agent is ready for input or times out.
	if err := agents.WaitReady(id); err != nil {
		// Kill the zombie session before returning error
		_ = agents.Stop(id, false)
		return agent.AgentID{}, fmt.Errorf("waiting for agent to start: %w", err)
	}

	return id, nil
}

// buildCommand constructs the startup command for an agent.
func buildCommand(role, aiRuntime string, cfg *startConfig) string {
	beacon := ""
	if cfg.topic != "" {
		beacon = cfg.topic
	}
	cmd := config.BuildAgentCommand(aiRuntime, beacon)

	// For interactive/refresh mode, remove --dangerously-skip-permissions
	if cfg.interactive {
		cmd = strings.Replace(cmd, " --dangerously-skip-permissions", "", 1)
	}

	return cmd
}

// buildSessionConfigurer returns a callback for tmux setup (env vars, theming, hooks).
// This callback is passed to StartConfig.OnCreated and receives the SessionID
// after the session is created.
//
// The callback captures the tmux instance to perform tmux-specific operations
// like theming, hooks, and bindings using the SessionID.
func buildSessionConfigurer(id agent.AgentID, envVars map[string]string, t *tmux.Tmux) agent.OnSessionCreated {
	role, rigName, worker := id.Parse()

	return func(sessionID session.SessionID) error {
		// Set env vars via the front door (tmux translates internally)
		if err := t.SetEnvVars(sessionID, envVars); err != nil {
			return fmt.Errorf("setting env vars: %w", err)
		}

		// Compute theming parameters:
		// - Town-level (mayor, deacon): rig="", worker=role (capitalized), role=role
		// - Rig singletons (witness, refinery): rig=rigName, worker=role, role=role
		// - Named workers (polecat, crew): rig=rigName, worker=name, role=role
		var themeRig, themeWorker string
		var theme tmux.Theme
		if rigName == "" {
			// Town-level: no rig, worker is the role name, use role-specific theme
			themeRig = ""
			themeWorker = strings.Title(role)
			switch role {
			case constants.RoleMayor:
				theme = tmux.MayorTheme()
			case constants.RoleDeacon:
				theme = tmux.DeaconTheme()
			default:
				theme = tmux.DefaultTheme()
			}
		} else {
			themeRig = rigName
			if worker != "" {
				themeWorker = worker // Named worker
			} else {
				themeWorker = role // Rig singleton
			}
			theme = tmux.AssignTheme(rigName)
		}

		// Apply theming via the front door (tmux translates internally)
		if err := t.ConfigureGasTownSession(sessionID, theme, themeRig, themeWorker, role); err != nil {
			return fmt.Errorf("configuring session: %w", err)
		}

		// Role-specific hooks (only for rig-level named agents)
		switch role {
		case constants.RolePolecat:
			if err := t.SetPaneDiedHook(sessionID, fmt.Sprintf("%s/%s", rigName, worker)); err != nil {
				return fmt.Errorf("setting pane died hook: %w", err)
			}
		case constants.RoleCrew:
			// SetCrewCycleBindings sets global bindings (session param unused)
			if err := t.SetCrewCycleBindings(""); err != nil {
				return fmt.Errorf("setting crew bindings: %w", err)
			}
		}

		return nil
	}
}

// WorkDirForID computes the working directory for any agent based on its ID.
// This is the generalized version of WorkDirForRole that works with all agent types.
//
// For rig-level agents (witness, refinery, polecat, crew), this performs
// filesystem checks to handle legacy vs new directory structures.
func WorkDirForID(townRoot string, id agent.AgentID) (string, error) {
	role, rigName, worker := id.Parse()

	switch role {
	// Town-level singletons
	case constants.RoleMayor:
		return filepath.Join(townRoot, constants.RoleMayor), nil
	case constants.RoleDeacon:
		return filepath.Join(townRoot, constants.RoleDeacon), nil
	case constants.RoleBoot:
		return filepath.Join(townRoot, "deacon", "dogs", constants.RoleBoot), nil

	// Rig-level singletons
	case constants.RoleWitness:
		if rigName == "" {
			return "", fmt.Errorf("witness requires rig name in ID")
		}
		return witnessWorkDir(townRoot, rigName), nil
	case constants.RoleRefinery:
		if rigName == "" {
			return "", fmt.Errorf("refinery requires rig name in ID")
		}
		return refineryWorkDir(townRoot, rigName), nil

	// Named agents
	case constants.RolePolecat:
		if rigName == "" || worker == "" {
			return "", fmt.Errorf("polecat requires rig and worker name in ID")
		}
		return polecatWorkDir(townRoot, rigName, worker), nil
	case constants.RoleCrew:
		if rigName == "" || worker == "" {
			return "", fmt.Errorf("crew requires rig and worker name in ID")
		}
		return crewWorkDir(townRoot, rigName, worker), nil

	default:
		return "", fmt.Errorf("unknown role in AgentID: %s", role)
	}
}

// witnessWorkDir returns the working directory for a witness.
// Prefers witness/rig/, falls back to witness/, then rig root.
func witnessWorkDir(townRoot, rigName string) string {
	rigPath := filepath.Join(townRoot, rigName)

	witnessRigDir := filepath.Join(rigPath, "witness", "rig")
	if _, err := os.Stat(witnessRigDir); err == nil {
		return witnessRigDir
	}

	witnessDir := filepath.Join(rigPath, "witness")
	if _, err := os.Stat(witnessDir); err == nil {
		return witnessDir
	}

	return rigPath
}

// refineryWorkDir returns the working directory for a refinery.
// Prefers refinery/rig/, falls back to mayor/rig (legacy).
func refineryWorkDir(townRoot, rigName string) string {
	rigPath := filepath.Join(townRoot, rigName)

	refineryRigDir := filepath.Join(rigPath, "refinery", "rig")
	if _, err := os.Stat(refineryRigDir); err == nil {
		return refineryRigDir
	}

	// Fall back to mayor/rig (legacy architecture)
	return filepath.Join(rigPath, "mayor", "rig")
}

// polecatWorkDir returns the working directory for a polecat.
// New structure: polecats/<name>/<rigname>/ - falls back to old: polecats/<name>/
func polecatWorkDir(townRoot, rigName, polecatName string) string {
	rigPath := filepath.Join(townRoot, rigName)

	// New structure: polecats/<name>/<rigname>/
	newPath := filepath.Join(rigPath, "polecats", polecatName, rigName)
	if info, err := os.Stat(newPath); err == nil && info.IsDir() {
		return newPath
	}

	// Old structure: polecats/<name>/
	return filepath.Join(rigPath, "polecats", polecatName)
}

// crewWorkDir returns the working directory for a crew member.
func crewWorkDir(townRoot, rigName, crewName string) string {
	rigPath := filepath.Join(townRoot, rigName)
	return filepath.Join(rigPath, "crew", crewName)
}


// =============================================================================
// Factory with Dependency Injection (for rig-level agents)
//
// The Factory struct is used for rig-level agents that need tmux callbacks
// for theming and hooks. Singleton agents use Create() instead.
// =============================================================================

// Factory creates properly configured agent managers.
// It holds shared dependencies and configuration.
type Factory struct {
	sess     session.Sessions // Core session operations
	townRoot string
}

// New creates a production Factory with real tmux and sessions.
func New(townRoot string) *Factory {
	return &Factory{
		sess:     tmux.NewTmux(),
		townRoot: townRoot,
	}
}


// =============================================================================
// Rig-Level Agent Creation
// =============================================================================

// WitnessManager creates a properly configured witness.Manager.
func (f *Factory) WitnessManager(r *rig.Rig, aiRuntime string, envOverrides ...string) *witness.Manager {
	overrides := parseEnvOverrides(envOverrides)
	agents := f.agentsForRole(constants.RoleWitness, r.Name, aiRuntime, overrides)
	return witness.NewManager(agents, r)
}

// RefineryManager creates a properly configured refinery.Manager.
func (f *Factory) RefineryManager(r *rig.Rig, aiRuntime string) *refinery.Manager {
	agents := f.agentsForRole(constants.RoleRefinery, r.Name, aiRuntime, nil)
	return refinery.NewManager(agents, r)
}

// PolecatSessionManager creates a properly configured polecat.SessionManager.
func (f *Factory) PolecatSessionManager(r *rig.Rig, aiRuntime string) *polecat.SessionManager {
	agents := agent.New(f.sess, agent.FromPreset(aiRuntime))
	return polecat.NewSessionManager(agents, r, f.townRoot)
}

// CrewManager creates a properly configured crew.Manager.
// Note: Lifecycle operations (Start) should use factory.Start().
func (f *Factory) CrewManager(r *rig.Rig, aiRuntime string) *crew.Manager {
	g := git.NewGit(r.Path)
	agents := agent.New(f.sess, agent.FromPreset(aiRuntime))
	return crew.NewManager(agents, r, g, f.townRoot)
}

// =============================================================================
// Factory Helpers
// =============================================================================

// agentsForRole creates a configured agent.Agents for a given role.
func (f *Factory) agentsForRole(role, rigName, aiRuntime string, envOverrides map[string]string) agent.Agents {
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:     role,
		Rig:      rigName,
		TownRoot: f.townRoot,
	})
	for k, v := range envOverrides {
		envVars[k] = v
	}

	return agent.New(f.sess, agent.FromPreset(aiRuntime).WithEnvVars(envVars))
}

// =============================================================================
// Utility Functions
// =============================================================================

// parseEnvOverrides converts a list of KEY=VALUE strings to a map.
func parseEnvOverrides(overrides []string) map[string]string {
	result := make(map[string]string)
	for _, override := range overrides {
		parts := strings.SplitN(override, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}
