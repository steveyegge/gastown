package crew

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/util"
)

// Common errors
var (
	ErrCrewExists      = errors.New("crew worker already exists")
	ErrCrewNotFound    = errors.New("crew worker not found")
	ErrHasChanges      = errors.New("crew worker has uncommitted changes")
	ErrInvalidCrewName = errors.New("invalid crew name")
	ErrSessionRunning  = errors.New("session already running")
	ErrSessionNotFound = errors.New("session not found")
)

// StartOptions configures crew session startup.
type StartOptions struct {
	// Account specifies the account handle to use (overrides default).
	Account string

	// ClaudeConfigDir is resolved CLAUDE_CONFIG_DIR for the account.
	// If set, this is injected as an environment variable.
	ClaudeConfigDir string

	// KillExisting kills any existing session before starting (for restart operations).
	// If false and a session is running, Start() returns ErrSessionRunning.
	KillExisting bool

	// Topic is the startup nudge topic (e.g., "start", "restart", "refresh").
	// Defaults to "start" if empty.
	Topic string

	// Interactive removes --dangerously-skip-permissions for interactive/refresh mode.
	Interactive bool

	// AgentOverride specifies an alternate agent alias (e.g., for testing).
	AgentOverride string

	// Model specifies the model name to use (if supported).
	Model string

	// Args are extra runtime CLI args to append.
	Args []string

	// Env contains additional environment variables to set.
	Env map[string]string
}

// AddOptions configures crew workspace creation.
type AddOptions struct {
	CreateBranch bool
	BranchName   string
	Agent        string
	Model        string
	Account      string
	Args         []string
	Env          map[string]string
}

// validateCrewName checks that a crew name is safe and valid.
// Rejects path traversal attempts and characters that break agent ID parsing.
func validateCrewName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: name cannot be empty", ErrInvalidCrewName)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("%w: %q is not allowed", ErrInvalidCrewName, name)
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("%w: %q contains path separators", ErrInvalidCrewName, name)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("%w: %q contains path traversal sequence", ErrInvalidCrewName, name)
	}
	// Reject characters that break agent ID parsing (same as rig names)
	if strings.ContainsAny(name, "-. ") {
		sanitized := strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(name)
		sanitized = strings.ToLower(sanitized)
		return fmt.Errorf("%w: %q contains invalid characters; hyphens, dots, and spaces are reserved for agent ID parsing. Try %q instead", ErrInvalidCrewName, name, sanitized)
	}
	return nil
}

// Manager handles crew worker lifecycle.
type Manager struct {
	rig *rig.Rig
	git *git.Git
}

// NewManager creates a new crew manager.
func NewManager(r *rig.Rig, g *git.Git) *Manager {
	return &Manager{
		rig: r,
		git: g,
	}
}

// crewDir returns the directory for a crew worker.
func (m *Manager) crewDir(name string) string {
	return filepath.Join(m.rig.Path, "crew", name)
}

// stateFile returns the state file path for a crew worker.
func (m *Manager) stateFile(name string) string {
	return filepath.Join(m.crewDir(name), "state.json")
}

// mailDir returns the mail directory path for a crew worker.
func (m *Manager) mailDir(name string) string {
	return filepath.Join(m.crewDir(name), "mail")
}

// exists checks if a crew worker exists.
func (m *Manager) exists(name string) bool {
	_, err := os.Stat(m.crewDir(name))
	return err == nil
}

// Add creates a new crew worker with a clone of the rig.
func (m *Manager) Add(name string, createBranch bool) (*CrewWorker, error) {
	return m.AddWithOptions(name, AddOptions{CreateBranch: createBranch})
}

// AddWithOptions creates a new crew worker with a clone of the rig.
func (m *Manager) AddWithOptions(name string, opts AddOptions) (*CrewWorker, error) {
	if err := validateCrewName(name); err != nil {
		return nil, err
	}
	if m.exists(name) {
		return nil, ErrCrewExists
	}

	if opts.BranchName != "" {
		opts.CreateBranch = true
	}

	crewPath := m.crewDir(name)

	// Create crew directory if needed
	crewBaseDir := filepath.Join(m.rig.Path, "crew")
	if err := os.MkdirAll(crewBaseDir, 0755); err != nil {
		return nil, fmt.Errorf("creating crew dir: %w", err)
	}

	// Clone the rig repo
	if m.rig.LocalRepo != "" {
		if err := m.git.CloneWithReference(m.rig.GitURL, crewPath, m.rig.LocalRepo); err != nil {
			fmt.Printf("Warning: could not clone with local repo reference: %v\n", err)
			if err := m.git.Clone(m.rig.GitURL, crewPath); err != nil {
				return nil, fmt.Errorf("cloning rig: %w", err)
			}
		}
	} else {
		if err := m.git.Clone(m.rig.GitURL, crewPath); err != nil {
			return nil, fmt.Errorf("cloning rig: %w", err)
		}
	}

	crewGit := git.NewGit(crewPath)
	branchName := m.rig.DefaultBranch()

	// Optionally create a working branch
	if opts.CreateBranch {
		if opts.BranchName != "" {
			branchName = opts.BranchName
		} else {
			branchName = fmt.Sprintf("crew/%s", name)
		}
		if err := crewGit.CreateBranch(branchName); err != nil {
			_ = os.RemoveAll(crewPath) // best-effort cleanup
			return nil, fmt.Errorf("creating branch: %w", err)
		}
		if err := crewGit.Checkout(branchName); err != nil {
			_ = os.RemoveAll(crewPath) // best-effort cleanup
			return nil, fmt.Errorf("checking out branch: %w", err)
		}
	}

	// Add upstream remote if configured and distinct from origin.
	if m.rig.UpstreamURL != "" && m.rig.UpstreamURL != m.rig.GitURL {
		if err := crewGit.SetRemoteURL("upstream", m.rig.UpstreamURL); err != nil {
			fmt.Printf("Warning: could not add upstream remote: %v\n", err)
		}
	}

	// Create mail directory for mail delivery
	mailPath := m.mailDir(name)
	if err := os.MkdirAll(mailPath, 0755); err != nil {
		_ = os.RemoveAll(crewPath) // best-effort cleanup
		return nil, fmt.Errorf("creating mail dir: %w", err)
	}

	// Set up shared beads: crew uses rig's shared beads via redirect file
	if err := m.setupSharedBeads(crewPath); err != nil {
		// Non-fatal - crew can still work, warn but don't fail
		fmt.Printf("Warning: could not set up shared beads: %v\n", err)
	}

	// Provision PRIME.md with Gas Town context for this worker.
	// This is the fallback if SessionStart hook fails - ensures crew workers
	// always have GUPP and essential Gas Town context.
	if err := beads.ProvisionPrimeMDForWorktree(crewPath); err != nil {
		// Non-fatal - crew can still work via hook, warn but don't fail
		fmt.Printf("Warning: could not provision PRIME.md: %v\n", err)
	}

	// Copy overlay files from .runtime/overlay/ to crew root.
	// This allows services to have .env and other config files at their root.
	if err := rig.CopyOverlay(m.rig.Path, crewPath); err != nil {
		// Non-fatal - log warning but continue
		fmt.Printf("Warning: could not copy overlay files: %v\n", err)
	}

	// Ensure .gitignore has required Gas Town patterns
	if err := rig.EnsureGitignorePatterns(crewPath); err != nil {
		// Non-fatal - log warning but continue
		fmt.Printf("Warning: could not update .gitignore: %v\n", err)
	}

	// NOTE: Slash commands (.claude/commands/) are provisioned at town level by gt install.
	// All agents inherit them via Claude's directory traversal - no per-workspace copies needed.

	// NOTE: We intentionally do NOT write to CLAUDE.md here.
	// Gas Town context is injected ephemerally via SessionStart hook (gt prime).
	// Writing to CLAUDE.md would overwrite project instructions and leak
	// Gas Town internals into the project repo when workers commit/push.

	// Create crew worker state
	now := time.Now()
	var argsCopy []string
	if len(opts.Args) > 0 {
		argsCopy = append([]string(nil), opts.Args...)
	}
	var envCopy map[string]string
	if len(opts.Env) > 0 {
		envCopy = make(map[string]string, len(opts.Env))
		for k, v := range opts.Env {
			envCopy[k] = v
		}
	}

	crew := &CrewWorker{
		Name:      name,
		Rig:       m.rig.Name,
		ClonePath: crewPath,
		Branch:    branchName,
		Agent:     opts.Agent,
		Model:     opts.Model,
		Account:   opts.Account,
		Args:      argsCopy,
		Env:       envCopy,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Save state
	if err := m.saveState(crew); err != nil {
		_ = os.RemoveAll(crewPath) // best-effort cleanup
		return nil, fmt.Errorf("saving state: %w", err)
	}

	return crew, nil
}

// Remove deletes a crew worker.
func (m *Manager) Remove(name string, force bool) error {
	if err := validateCrewName(name); err != nil {
		return err
	}
	if !m.exists(name) {
		return ErrCrewNotFound
	}

	crewPath := m.crewDir(name)

	if !force {
		crewGit := git.NewGit(crewPath)
		hasChanges, err := crewGit.HasUncommittedChanges()
		if err == nil && hasChanges {
			return ErrHasChanges
		}
	}

	// Remove directory
	if err := os.RemoveAll(crewPath); err != nil {
		return fmt.Errorf("removing crew dir: %w", err)
	}

	return nil
}

// List returns all crew workers in the rig.
func (m *Manager) List() ([]*CrewWorker, error) {
	crewBaseDir := filepath.Join(m.rig.Path, "crew")

	entries, err := os.ReadDir(crewBaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading crew dir: %w", err)
	}

	var workers []*CrewWorker
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		worker, err := m.Get(entry.Name())
		if err != nil {
			continue // Skip invalid workers
		}
		workers = append(workers, worker)
	}

	return workers, nil
}

// Get returns a specific crew worker by name.
func (m *Manager) Get(name string) (*CrewWorker, error) {
	if err := validateCrewName(name); err != nil {
		return nil, err
	}
	if !m.exists(name) {
		return nil, ErrCrewNotFound
	}

	return m.loadState(name)
}

// saveState persists crew worker state to disk using atomic write.
func (m *Manager) saveState(crew *CrewWorker) error {
	stateFile := m.stateFile(crew.Name)
	if err := util.AtomicWriteJSON(stateFile, crew); err != nil {
		return fmt.Errorf("writing state: %w", err)
	}

	return nil
}

// loadState reads crew worker state from disk.
func (m *Manager) loadState(name string) (*CrewWorker, error) {
	stateFile := m.stateFile(name)

	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Return minimal crew worker if state file missing
			return &CrewWorker{
				Name:      name,
				Rig:       m.rig.Name,
				ClonePath: m.crewDir(name),
			}, nil
		}
		return nil, fmt.Errorf("reading state: %w", err)
	}

	var crew CrewWorker
	if err := json.Unmarshal(data, &crew); err != nil {
		return nil, fmt.Errorf("parsing state: %w", err)
	}

	// Directory name is source of truth for Name and ClonePath.
	// state.json can become stale after directory rename, copy, or corruption.
	crew.Name = name
	if crew.Rig == "" {
		crew.Rig = m.rig.Name
	}
	crew.ClonePath = m.crewDir(name)

	return &crew, nil
}

// Rename renames a crew worker from oldName to newName.
func (m *Manager) Rename(oldName, newName string) error {
	if !m.exists(oldName) {
		return ErrCrewNotFound
	}
	if m.exists(newName) {
		return ErrCrewExists
	}

	oldPath := m.crewDir(oldName)
	newPath := m.crewDir(newName)

	// Rename directory
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("renaming crew dir: %w", err)
	}

	// Update state file with new name and path
	crew, err := m.loadState(newName)
	if err != nil {
		// Rollback on error (best-effort)
		_ = os.Rename(newPath, oldPath)
		return fmt.Errorf("loading state: %w", err)
	}

	crew.Name = newName
	crew.ClonePath = newPath
	crew.UpdatedAt = time.Now()

	if err := m.saveState(crew); err != nil {
		// Rollback on error (best-effort)
		_ = os.Rename(newPath, oldPath)
		return fmt.Errorf("saving state: %w", err)
	}

	return nil
}

// Pristine ensures a crew worker is up-to-date with remote.
// It runs git pull --rebase.
func (m *Manager) Pristine(name string) (*PristineResult, error) {
	if err := validateCrewName(name); err != nil {
		return nil, err
	}
	if !m.exists(name) {
		return nil, ErrCrewNotFound
	}

	crewPath := m.crewDir(name)
	crewGit := git.NewGit(crewPath)

	result := &PristineResult{
		Name: name,
	}

	// Check for uncommitted changes
	hasChanges, err := crewGit.HasUncommittedChanges()
	if err != nil {
		return nil, fmt.Errorf("checking changes: %w", err)
	}
	result.HadChanges = hasChanges

	// Pull latest (use origin and current branch)
	if err := crewGit.Pull("origin", ""); err != nil {
		result.PullError = err.Error()
	} else {
		result.Pulled = true
	}

	// Note: With Dolt backend, beads changes are persisted immediately - no sync needed
	result.Synced = true

	return result, nil
}

// PristineResult captures the results of a pristine operation.
type PristineResult struct {
	Name       string `json:"name"`
	HadChanges bool   `json:"had_changes"`
	Pulled     bool   `json:"pulled"`
	PullError  string `json:"pull_error,omitempty"`
	Synced     bool   `json:"synced"`
	SyncError  string `json:"sync_error,omitempty"`
}

// setupSharedBeads creates a redirect file so the crew worker uses the rig's shared .beads database.
// This eliminates the need for git sync between crew clones - all crew members share one database.
func (m *Manager) setupSharedBeads(crewPath string) error {
	townRoot := filepath.Dir(m.rig.Path)
	return beads.SetupRedirect(townRoot, crewPath)
}

// SessionName returns the tmux session name for a crew member.
func (m *Manager) SessionName(name string) string {
	return fmt.Sprintf("gt-%s-crew-%s", m.rig.Name, name)
}

// Start creates and starts a tmux session for a crew member.
// If the crew member doesn't exist, it will be created first.
func (m *Manager) Start(name string, opts StartOptions) error {
	if err := validateCrewName(name); err != nil {
		return err
	}

	// Get or create the crew worker
	worker, err := m.Get(name)
	if err == ErrCrewNotFound {
		worker, err = m.Add(name, false) // No feature branch for crew
		if err != nil {
			return fmt.Errorf("creating crew workspace: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("getting crew worker: %w", err)
	}

	t := tmux.NewTmux()
	sessionID := m.SessionName(name)

	// Check if session already exists
	running, err := t.HasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if running {
		if opts.KillExisting {
			// Restart mode - kill existing session
			if err := t.KillSession(sessionID); err != nil {
				return fmt.Errorf("killing existing session: %w", err)
			}
		} else {
			// Normal start - session exists, check if agent is actually running
			if t.IsAgentAlive(sessionID) {
				return fmt.Errorf("%w: %s", ErrSessionRunning, sessionID)
			}
			// Zombie session - kill and recreate
			if err := t.KillSession(sessionID); err != nil {
				return fmt.Errorf("killing zombie session: %w", err)
			}
		}
	}

	// Build the startup beacon for predecessor discovery via /resume
	// Pass it as Claude's initial prompt - processed when Claude is ready
	address := fmt.Sprintf("%s/crew/%s", m.rig.Name, name)
	topic := opts.Topic
	if topic == "" {
		topic = "start"
	}
	beacon := session.FormatStartupBeacon(session.BeaconConfig{
		Recipient: address,
		Sender:    "human",
		Topic:     topic,
	})

	account := opts.Account
	if account == "" {
		account = worker.Account
	}
	agentOverride := opts.AgentOverride
	if agentOverride == "" {
		agentOverride = worker.Agent
	}
	model := opts.Model
	if model == "" {
		model = worker.Model
	}

	extraArgs := append([]string(nil), worker.Args...)
	if len(opts.Args) > 0 {
		extraArgs = append(extraArgs, opts.Args...)
	}
	envOverrides := config.MergeEnv(worker.Env, opts.Env)

	townRoot := filepath.Dir(m.rig.Path)
	claudeConfigDir := opts.ClaudeConfigDir
	if claudeConfigDir == "" {
		accountsPath := constants.MayorAccountsPath(townRoot)
		claudeConfigDir, _, err = config.ResolveAccountConfigDir(accountsPath, account)
		if err != nil {
			return fmt.Errorf("resolving account: %w", err)
		}
	}

	rc, agentName, err := config.ResolveAgentConfigWithOverride(townRoot, m.rig.Path, agentOverride)
	if err != nil {
		return fmt.Errorf("resolving agent: %w", err)
	}
	resolved := cloneRuntimeConfig(rc)
	if len(extraArgs) > 0 {
		resolved.Args = append(resolved.Args, extraArgs...)
	}
	if model != "" {
		if ok, flag := config.SupportsModel(agentName); ok {
			if flag == "" {
				flag = "--model"
			}
			resolved.Args = append(resolved.Args, flag, model)
		} else {
			fmt.Printf("Warning: agent %q does not support models; skipping %q\n", agentName, model)
		}
	}
	if opts.Interactive {
		resolved.Args = filterArgs(resolved.Args, "--dangerously-skip-permissions")
	}

	// Ensure runtime settings exist in crew/ (not crew/<name>/) so we don't
	// write into the source repo. Runtimes walk up the tree to find settings.
	crewBaseDir := filepath.Join(m.rig.Path, "crew")
	if err := runtime.EnsureSettingsForRole(crewBaseDir, "crew", resolved); err != nil {
		return fmt.Errorf("ensuring runtime settings: %w", err)
	}

	agentCmd := resolved.BuildCommand()
	if beacon != "" {
		agentCmd = resolved.BuildCommandWithPrompt(beacon)
	}

	envCfg := config.AgentEnvConfig{
		Role:          "crew",
		Rig:           m.rig.Name,
		AgentName:     name,
		TownRoot:      townRoot,
		BeadsNoDaemon: true,
	}
	if resolved.Session != nil {
		envCfg.SessionIDEnv = resolved.Session.SessionIDEnv
		if resolved.Session.ConfigDirEnv == "" || resolved.Session.ConfigDirEnv == "CLAUDE_CONFIG_DIR" {
			envCfg.RuntimeConfigDir = claudeConfigDir
		}
	}
	envVars := config.AgentEnv(envCfg)
	if resolved.Session != nil && resolved.Session.ConfigDirEnv != "" && resolved.Session.ConfigDirEnv != "CLAUDE_CONFIG_DIR" && claudeConfigDir != "" {
		envVars[resolved.Session.ConfigDirEnv] = claudeConfigDir
	}
	envVars = config.MergeEnv(envVars, envOverrides)

	claudeCmd := config.PrependEnv(agentCmd, envVars)

	// Create session with command directly to avoid send-keys race condition.
	// See: https://github.com/anthropics/gastown/issues/280
	if err := t.NewSessionWithCommand(sessionID, worker.ClonePath, claudeCmd); err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	// Set environment variables (non-fatal: session works without these)
	// Use centralized AgentEnv for consistency across all role startup paths
	for k, v := range envVars {
		_ = t.SetEnvironment(sessionID, k, v)
	}

	// Apply rig-based theming (non-fatal: theming failure doesn't affect operation)
	theme := tmux.AssignTheme(m.rig.Name)
	_ = t.ConfigureGasTownSession(sessionID, theme, m.rig.Name, name, "crew")

	// Set up C-b n/p keybindings for crew session cycling (non-fatal)
	_ = t.SetCrewCycleBindings(sessionID)

	// Note: We intentionally don't wait for the agent to start here.
	// The session is created in detached mode, and blocking for 60 seconds
	// serves no purpose. If the caller needs to know when the agent is ready,
	// they can check with IsAgentAlive().

	return nil
}

// Stop terminates a crew member's tmux session.
func (m *Manager) Stop(name string) error {
	if err := validateCrewName(name); err != nil {
		return err
	}

	t := tmux.NewTmux()
	sessionID := m.SessionName(name)

	// Check if session exists
	running, err := t.HasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return ErrSessionNotFound
	}

	// Kill the session
	if err := t.KillSession(sessionID); err != nil {
		return fmt.Errorf("killing session: %w", err)
	}

	return nil
}

// IsRunning checks if a crew member's session is active.
func (m *Manager) IsRunning(name string) (bool, error) {
	t := tmux.NewTmux()
	sessionID := m.SessionName(name)
	return t.HasSession(sessionID)
}

func cloneRuntimeConfig(rc *config.RuntimeConfig) *config.RuntimeConfig {
	if rc == nil {
		return config.DefaultRuntimeConfig()
	}
	copy := *rc
	if rc.Args != nil {
		copy.Args = append([]string(nil), rc.Args...)
	}
	return &copy
}

func filterArgs(args []string, flag string) []string {
	if len(args) == 0 {
		return args
	}
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == flag {
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered
}
