package crew

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
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
	if err := validateCrewName(name); err != nil {
		return nil, err
	}
	if m.exists(name) {
		return nil, ErrCrewExists
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
	if createBranch {
		branchName = fmt.Sprintf("crew/%s", name)
		if err := crewGit.CreateBranch(branchName); err != nil {
			_ = os.RemoveAll(crewPath) // best-effort cleanup
			return nil, fmt.Errorf("creating branch: %w", err)
		}
		if err := crewGit.Checkout(branchName); err != nil {
			_ = os.RemoveAll(crewPath) // best-effort cleanup
			return nil, fmt.Errorf("checking out branch: %w", err)
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

	// NOTE: Slash commands (.claude/commands/) are provisioned at town level by gt install.
	// All agents inherit them via Claude's directory traversal - no per-workspace copies needed.

	// NOTE: We intentionally do NOT write to CLAUDE.md here.
	// Gas Town context is injected ephemerally via SessionStart hook (gt prime).
	// Writing to CLAUDE.md would overwrite project instructions and leak
	// Gas Town internals into the project repo when workers commit/push.

	// Create crew worker state
	now := time.Now()
	crew := &CrewWorker{
		Name:      name,
		Rig:       m.rig.Name,
		ClonePath: crewPath,
		Branch:    branchName,
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

	// Backfill essential fields if missing (handles empty or incomplete state.json)
	if crew.Name == "" {
		crew.Name = name
	}
	if crew.Rig == "" {
		crew.Rig = m.rig.Name
	}
	if crew.ClonePath == "" {
		crew.ClonePath = m.crewDir(name)
	}

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
// It runs git pull --rebase and bd sync.
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

	// Run bd sync
	if err := m.runBdSync(crewPath); err != nil {
		result.SyncError = err.Error()
	} else {
		result.Synced = true
	}

	return result, nil
}

// runBdSync runs bd sync in the given directory.
func (m *Manager) runBdSync(dir string) error {
	cmd := exec.Command("bd", "sync")
	cmd.Dir = dir
	return cmd.Run()
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
			// Normal start - session exists, check if Claude is actually running
			if t.IsClaudeRunning(sessionID) {
				return fmt.Errorf("%w: %s", ErrSessionRunning, sessionID)
			}
			// Zombie session - kill and recreate
			if err := t.KillSession(sessionID); err != nil {
				return fmt.Errorf("killing zombie session: %w", err)
			}
		}
	}

	// Ensure Claude settings exist in crew/ (not crew/<name>/) so we don't
	// write into the source repo. Claude walks up the tree to find settings.
	// All crew members share the same settings file.
	crewBaseDir := filepath.Join(m.rig.Path, "crew")
	if err := claude.EnsureSettingsForRole(crewBaseDir, "crew"); err != nil {
		return fmt.Errorf("ensuring Claude settings: %w", err)
	}

	// Build the startup beacon for predecessor discovery via /resume
	// Pass it as Claude's initial prompt - processed when Claude is ready
	address := fmt.Sprintf("%s/crew/%s", m.rig.Name, name)
	topic := opts.Topic
	if topic == "" {
		topic = "start"
	}
	beacon := session.FormatStartupNudge(session.StartupNudgeConfig{
		Recipient: address,
		Sender:    "human",
		Topic:     topic,
	})

	// Build startup command first
	// SessionStart hook handles context loading (gt prime --hook)
	claudeCmd, err := config.BuildCrewStartupCommandWithAgentOverride(m.rig.Name, name, m.rig.Path, beacon, opts.AgentOverride)
	if err != nil {
		return fmt.Errorf("building startup command: %w", err)
	}

	// For interactive/refresh mode, remove --dangerously-skip-permissions
	if opts.Interactive {
		claudeCmd = strings.Replace(claudeCmd, " --dangerously-skip-permissions", "", 1)
	}

	// Create session with command directly to avoid send-keys race condition.
	// See: https://github.com/anthropics/gastown/issues/280
	if err := t.NewSessionWithCommand(sessionID, worker.ClonePath, claudeCmd); err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	// Set environment variables (non-fatal: session works without these)
	// Use centralized AgentEnv for consistency across all role startup paths
	townRoot := filepath.Dir(m.rig.Path)
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:             "crew",
		Rig:              m.rig.Name,
		AgentName:        name,
		TownRoot:         townRoot,
		RuntimeConfigDir: opts.ClaudeConfigDir,
		BeadsNoDaemon:    true,
	})
	for k, v := range envVars {
		_ = t.SetEnvironment(sessionID, k, v)
	}

	// Apply rig-based theming (non-fatal: theming failure doesn't affect operation)
	theme := tmux.AssignTheme(m.rig.Name)
	_ = t.ConfigureGasTownSession(sessionID, theme, m.rig.Name, name, "crew")

	// Set up C-b n/p keybindings for crew session cycling (non-fatal)
	_ = t.SetCrewCycleBindings(sessionID)

	// Note: We intentionally don't wait for Claude to start here.
	// The session is created in detached mode, and blocking for 60 seconds
	// serves no purpose. If the caller needs to know when Claude is ready,
	// they can check with IsClaudeRunning().

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
