// Package polecat provides polecat workspace and session management.
package polecat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/sandbox"
	"github.com/steveyegge/gastown/internal/sandbox/daytona"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

// debugSession logs non-fatal errors during session startup when GT_DEBUG_SESSION=1.
func debugSession(context string, err error) {
	if os.Getenv("GT_DEBUG_SESSION") != "" && err != nil {
		fmt.Fprintf(os.Stderr, "[session-debug] %s: %v\n", context, err)
	}
}

// Session errors
var (
	ErrSessionRunning  = errors.New("session already running")
	ErrSessionNotFound = errors.New("session not found")
)

// SessionManager handles polecat session lifecycle.
// It supports both local (tmux) and remote (Daytona) execution backends.
type SessionManager struct {
	tmux    *tmux.Tmux
	rig     *rig.Rig
	backend sandbox.Backend // Optional: if set, uses backend abstraction

	// activeSessions tracks sandbox sessions for remote backends.
	// Key: polecat name, Value: sandbox session
	activeSessions map[string]*sandbox.Session
}

// NewSessionManager creates a new polecat session manager for a rig.
// Uses local (tmux) backend by default.
func NewSessionManager(t *tmux.Tmux, r *rig.Rig) *SessionManager {
	return &SessionManager{
		tmux:           t,
		rig:            r,
		activeSessions: make(map[string]*sandbox.Session),
	}
}

// NewSessionManagerWithBackend creates a session manager with a specific backend.
// For local backend, the tmux parameter is used as fallback for advanced operations.
// For remote backends (Daytona), tmux is only used for local operations if needed.
func NewSessionManagerWithBackend(t *tmux.Tmux, r *rig.Rig, backend sandbox.Backend) *SessionManager {
	return &SessionManager{
		tmux:           t,
		rig:            r,
		backend:        backend,
		activeSessions: make(map[string]*sandbox.Session),
	}
}

// NewSessionManagerForRig creates a SessionManager with the appropriate backend for a rig.
// This is a convenience function for callers that don't have a polecat.Manager instance.
// It loads sandbox configuration from the rig and town settings.
func NewSessionManagerForRig(r *rig.Rig, rigsConfig *config.RigsConfig) (*SessionManager, error) {
	t := tmux.NewTmux()

	// Try to load sandbox config from rig, fall back to town
	townRoot, err := workspace.Find(r.Path)
	if err != nil {
		// On error, use local backend (default behavior)
		return NewSessionManager(t, r), nil
	}

	// Load sandbox config
	sandboxCfg, err := sandbox.LoadConfig(townRoot)
	if err != nil {
		// On error, use local backend (default behavior)
		return NewSessionManager(t, r), nil
	}

	// Try to load rig-level overrides
	rigCfg, _ := sandbox.LoadConfig(r.Path)
	if rigCfg != nil {
		sandboxCfg = sandbox.MergeConfigs(sandboxCfg, rigCfg)
	}

	// Get the backend for polecats
	backend, err := sandbox.GetBackendForRole(sandboxCfg, "polecat")
	if err != nil {
		// If we can't get the configured backend, fall back to local
		return NewSessionManager(t, r), nil
	}

	return NewSessionManagerWithBackend(t, r, backend), nil
}

// SetBackend sets the execution backend for this session manager.
// If nil, falls back to direct tmux operations.
func (m *SessionManager) SetBackend(backend sandbox.Backend) {
	m.backend = backend
}

// Backend returns the current execution backend, or nil if using direct tmux.
func (m *SessionManager) Backend() sandbox.Backend {
	return m.backend
}

// IsRemoteBackend returns true if using a remote execution backend (not local tmux).
func (m *SessionManager) IsRemoteBackend() bool {
	if m.backend == nil {
		return false
	}
	return m.backend.Type() != sandbox.BackendLocal
}

// persistedSessionState stores Daytona session info on disk for cross-process access.
type persistedSessionState struct {
	SessionID string            `json:"session_id"`
	SandboxID string            `json:"sandbox_id"`
	PtyID     string            `json:"pty_id"`
	Backend   string            `json:"backend"`
	CreatedAt time.Time         `json:"created_at"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// sessionStatePath returns the file path for persisting session state.
func (m *SessionManager) sessionStatePath(polecat string) string {
	return filepath.Join(m.rig.Path, ".runtime", "daytona-sessions", polecat+".json")
}

// saveSessionState persists session metadata to disk for cross-process access.
func (m *SessionManager) saveSessionState(polecat string, session *sandbox.Session) error {
	statePath := m.sessionStatePath(polecat)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	state := persistedSessionState{
		SessionID: session.ID,
		SandboxID: session.Metadata[sandbox.MetaSandboxID],
		PtyID:     session.Metadata[sandbox.MetaPtyID],
		Backend:   string(session.Backend),
		CreatedAt: session.CreatedAt,
		Metadata:  session.Metadata,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	if err := os.WriteFile(statePath, data, 0644); err != nil {
		return fmt.Errorf("writing state file: %w", err)
	}

	return nil
}

// loadSessionState loads persisted session state from disk.
func (m *SessionManager) loadSessionState(polecat string) (*persistedSessionState, error) {
	statePath := m.sessionStatePath(polecat)

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no persisted session state for %s", polecat)
		}
		return nil, fmt.Errorf("reading state file: %w", err)
	}

	var state persistedSessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}

	return &state, nil
}

// deleteSessionState removes persisted session state from disk.
func (m *SessionManager) deleteSessionState(polecat string) error {
	statePath := m.sessionStatePath(polecat)
	if err := os.Remove(statePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing state file: %w", err)
	}
	return nil
}

// SessionStartOptions configures polecat session startup.
type SessionStartOptions struct {
	// WorkDir overrides the default working directory (polecat clone dir).
	WorkDir string

	// Issue is an optional issue ID to work on.
	Issue string

	// Command overrides the default "claude" command.
	Command string

	// Account specifies the account handle to use (overrides default).
	Account string

	// RuntimeConfigDir is resolved config directory for the runtime account.
	// If set, this is injected as an environment variable.
	RuntimeConfigDir string

	// TaskPrompt is an optional task description to send directly to the agent.
	// Used by remote backends (Daytona) where beads are not accessible.
	// If set, this is sent as the initial prompt instead of propulsion nudge.
	TaskPrompt string

	// HookBead is the bead ID that this polecat is working on.
	// Stored in session metadata for retrieval during completion.
	HookBead string
}

// SessionInfo contains information about a running polecat session.
type SessionInfo struct {
	// Polecat is the polecat name.
	Polecat string `json:"polecat"`

	// SessionID is the tmux session identifier (local) or sandbox ID (remote).
	SessionID string `json:"session_id"`

	// Running indicates if the session is currently active.
	Running bool `json:"running"`

	// RigName is the rig this session belongs to.
	RigName string `json:"rig_name"`

	// Backend is the execution backend type ("local" or "daytona").
	Backend string `json:"backend,omitempty"`

	// Attached indicates if someone is attached to the session (local only).
	Attached bool `json:"attached,omitempty"`

	// Created is when the session was created.
	Created time.Time `json:"created,omitempty"`

	// Windows is the number of tmux windows (local only).
	Windows int `json:"windows,omitempty"`

	// LastActivity is when the session last had activity.
	LastActivity time.Time `json:"last_activity,omitempty"`
}

// SessionName generates the tmux session name for a polecat.
func (m *SessionManager) SessionName(polecat string) string {
	return fmt.Sprintf("gt-%s-%s", m.rig.Name, polecat)
}

// polecatDir returns the parent directory for a polecat.
// This is polecats/<name>/ - the polecat's home directory.
func (m *SessionManager) polecatDir(polecat string) string {
	return filepath.Join(m.rig.Path, "polecats", polecat)
}

// clonePath returns the path where the git worktree lives.
// New structure: polecats/<name>/<rigname>/ - gives LLMs recognizable repo context.
// Falls back to old structure: polecats/<name>/ for backward compatibility.
func (m *SessionManager) clonePath(polecat string) string {
	// New structure: polecats/<name>/<rigname>/
	newPath := filepath.Join(m.rig.Path, "polecats", polecat, m.rig.Name)
	if info, err := os.Stat(newPath); err == nil && info.IsDir() {
		return newPath
	}

	// Old structure: polecats/<name>/ (backward compat)
	oldPath := filepath.Join(m.rig.Path, "polecats", polecat)
	if info, err := os.Stat(oldPath); err == nil && info.IsDir() {
		// Check if this is actually a git worktree (has .git file or dir)
		gitPath := filepath.Join(oldPath, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return oldPath
		}
	}

	// Default to new structure for new polecats
	return newPath
}

// hasPolecat checks if the polecat exists in this rig.
func (m *SessionManager) hasPolecat(polecat string) bool {
	polecatPath := m.polecatDir(polecat)
	info, err := os.Stat(polecatPath)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// Start creates and starts a new session for a polecat.
// If a backend is configured, it uses the backend abstraction layer.
// Otherwise, it uses direct tmux operations (original behavior).
func (m *SessionManager) Start(polecat string, opts SessionStartOptions) error {
	// Route to backend if configured and it's a remote backend
	if m.IsRemoteBackend() {
		return m.startWithRemoteBackend(polecat, opts)
	}

	// Use original tmux-based implementation
	return m.startLocal(polecat, opts)
}

// startLocal creates and starts a new local tmux session for a polecat.
// This is the original implementation using direct tmux operations.
func (m *SessionManager) startLocal(polecat string, opts SessionStartOptions) error {
	if !m.hasPolecat(polecat) {
		return fmt.Errorf("%w: %s", ErrPolecatNotFound, polecat)
	}

	sessionID := m.SessionName(polecat)

	// Check if session already exists
	// Note: Orphan sessions are cleaned up by ReconcilePool during AllocateName,
	// so by this point, any existing session should be legitimately in use.
	running, err := m.tmux.HasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if running {
		return fmt.Errorf("%w: %s", ErrSessionRunning, sessionID)
	}

	// Determine working directory
	workDir := opts.WorkDir
	if workDir == "" {
		workDir = m.clonePath(polecat)
	}

	runtimeConfig := config.LoadRuntimeConfig(m.rig.Path)

	// Ensure runtime settings exist in polecats/ (not polecats/<name>/) so we don't
	// write into the source repo. Runtime walks up the tree to find settings.
	polecatsDir := filepath.Join(m.rig.Path, "polecats")
	if err := runtime.EnsureSettingsForRole(polecatsDir, "polecat", runtimeConfig); err != nil {
		return fmt.Errorf("ensuring runtime settings: %w", err)
	}

	// Build startup command first
	command := opts.Command
	if command == "" {
		command = config.BuildPolecatStartupCommand(m.rig.Name, polecat, m.rig.Path, "")
	}
	// Prepend runtime config dir env if needed
	if runtimeConfig.Session != nil && runtimeConfig.Session.ConfigDirEnv != "" && opts.RuntimeConfigDir != "" {
		command = config.PrependEnv(command, map[string]string{runtimeConfig.Session.ConfigDirEnv: opts.RuntimeConfigDir})
	}

	// Create session with command directly to avoid send-keys race condition.
	// See: https://github.com/anthropics/gastown/issues/280
	if err := m.tmux.NewSessionWithCommand(sessionID, workDir, command); err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	// Set environment (non-fatal: session works without these)
	// Use centralized AgentEnv for consistency across all role startup paths
	townRoot := filepath.Dir(m.rig.Path)
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:             "polecat",
		Rig:              m.rig.Name,
		AgentName:        polecat,
		TownRoot:         townRoot,
		RuntimeConfigDir: opts.RuntimeConfigDir,
		BeadsNoDaemon:    true,
	})
	for k, v := range envVars {
		debugSession("SetEnvironment "+k, m.tmux.SetEnvironment(sessionID, k, v))
	}

	// Hook the issue to the polecat if provided via --issue flag
	if opts.Issue != "" {
		agentID := fmt.Sprintf("%s/polecats/%s", m.rig.Name, polecat)
		if err := m.hookIssue(opts.Issue, agentID, workDir); err != nil {
			fmt.Printf("Warning: could not hook issue %s: %v\n", opts.Issue, err)
		}
	}

	// Apply theme (non-fatal)
	theme := tmux.AssignTheme(m.rig.Name)
	debugSession("ConfigureGasTownSession", m.tmux.ConfigureGasTownSession(sessionID, theme, m.rig.Name, polecat, "polecat"))

	// Set pane-died hook for crash detection (non-fatal)
	agentID := fmt.Sprintf("%s/%s", m.rig.Name, polecat)
	debugSession("SetPaneDiedHook", m.tmux.SetPaneDiedHook(sessionID, agentID))

	// Wait for Claude to start (non-fatal)
	debugSession("WaitForCommand", m.tmux.WaitForCommand(sessionID, constants.SupportedShells, constants.ClaudeStartTimeout))

	// Accept bypass permissions warning dialog if it appears
	debugSession("AcceptBypassPermissionsWarning", m.tmux.AcceptBypassPermissionsWarning(sessionID))

	// Wait for runtime to be fully ready at the prompt (not just started)
	runtime.SleepForReadyDelay(runtimeConfig)
	_ = runtime.RunStartupFallback(m.tmux, sessionID, "polecat", runtimeConfig)

	// Inject startup nudge for predecessor discovery via /resume
	address := fmt.Sprintf("%s/polecats/%s", m.rig.Name, polecat)
	debugSession("StartupNudge", session.StartupNudge(m.tmux, sessionID, session.StartupNudgeConfig{
		Recipient: address,
		Sender:    "witness",
		Topic:     "assigned",
		MolID:     opts.Issue,
	}))

	// GUPP: Send propulsion nudge to trigger autonomous work execution
	time.Sleep(2 * time.Second)
	debugSession("NudgeSession PropulsionNudge", m.tmux.NudgeSession(sessionID, session.PropulsionNudge()))

	return nil
}

// startWithRemoteBackend creates and starts a polecat session using a remote backend.
// This is used for Daytona and other remote execution environments.
func (m *SessionManager) startWithRemoteBackend(polecat string, opts SessionStartOptions) error {
	ctx := context.Background()

	// Local polecat worktree directory (source for file sync)
	localWorkDir := opts.WorkDir
	if localWorkDir == "" {
		localWorkDir = m.polecatDir(polecat)
	}

	// Remote working directory inside sandbox (destination for file sync)
	// Get from backend config if available, otherwise use default
	remoteWorkDir := sandbox.DefaultRemoteWorkDir
	if daytonaBackend, ok := m.backend.(*sandbox.DaytonaBackend); ok {
		remoteWorkDir = daytonaBackend.RemoteWorkDir()
	}

	// Build session name
	sessionName := m.SessionName(polecat)

	// Check if already running
	exists, err := m.backend.HasSession(ctx, sessionName)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if exists {
		running, _ := m.backend.IsRunning(ctx, &sandbox.Session{ID: sessionName})
		if running {
			return fmt.Errorf("%w: %s", ErrSessionRunning, sessionName)
		}
	}

	// Build environment variables
	townRoot := filepath.Dir(m.rig.Path)
	beadsDir := filepath.Join(townRoot, ".beads")
	env := map[string]string{
		"GT_RIG":           m.rig.Name,
		"GT_POLECAT":       polecat,
		"BEADS_DIR":        beadsDir,
		"BEADS_NO_DAEMON":  "1",
		"BEADS_AGENT_NAME": fmt.Sprintf("%s/%s", m.rig.Name, polecat),
	}
	if opts.RuntimeConfigDir != "" {
		env["CLAUDE_CONFIG_DIR"] = opts.RuntimeConfigDir
	}

	// Create sandbox session with remote working directory
	createOpts := sandbox.CreateOptions{
		Name:    sessionName,
		WorkDir: remoteWorkDir,
		Env:     env,
	}

	sandboxSession, err := m.backend.Create(ctx, createOpts)
	if err != nil {
		return fmt.Errorf("creating sandbox: %w", err)
	}

	// Track the session
	m.activeSessions[polecat] = sandboxSession

	// Sync local polecat worktree to sandbox
	// This copies the entire worktree so the agent has files to work on.
	// Changes are synced back when the session stops.
	if syncBackend, ok := m.backend.(sandbox.SyncBackend); ok {
		fmt.Printf("Syncing worktree to sandbox: %s -> %s\n", localWorkDir, remoteWorkDir)
		if err := syncBackend.SyncToSession(ctx, sandboxSession, localWorkDir, remoteWorkDir); err != nil {
			// Clean up on failure
			_ = m.backend.Destroy(ctx, sandboxSession)
			delete(m.activeSessions, polecat)
			return fmt.Errorf("syncing worktree to sandbox: %w", err)
		}
		fmt.Printf("✓ Worktree synced to sandbox\n")
	}

	// Store work dirs and hook bead in session metadata for sync back later
	sandboxSession.Metadata["local_work_dir"] = localWorkDir
	sandboxSession.Metadata["remote_work_dir"] = remoteWorkDir
	if opts.HookBead != "" {
		sandboxSession.Metadata["hook_bead"] = opts.HookBead
	}

	// Hook the issue to the polecat if provided
	if opts.Issue != "" {
		agentID := fmt.Sprintf("%s/polecats/%s", m.rig.Name, polecat)
		if err := m.hookIssue(opts.Issue, agentID, localWorkDir); err != nil {
			fmt.Printf("Warning: could not hook issue %s: %v\n", opts.Issue, err)
		}
	}

	// Build startup command
	command := opts.Command
	if command == "" {
		command = config.BuildPolecatStartupCommand(m.rig.Name, polecat, m.rig.Path, "")
	}

	// Start the agent in the sandbox
	if err := m.backend.Start(ctx, sandboxSession, command); err != nil {
		// Clean up on failure
		_ = m.backend.Destroy(ctx, sandboxSession)
		delete(m.activeSessions, polecat)
		return fmt.Errorf("starting agent: %w", err)
	}

	// Persist session state for cross-process access (peek, nudge)
	if err := m.saveSessionState(polecat, sandboxSession); err != nil {
		debugSession("saveSessionState", err)
	}

	// Wait for agent to be ready
	time.Sleep(10 * time.Second)

	// Send task prompt or propulsion nudge to start work
	var prompt string
	if opts.TaskPrompt != "" {
		// Use provided task prompt (for remote backends without beads access)
		prompt = opts.TaskPrompt
	} else {
		// Default to propulsion nudge (agent discovers work via gt prime)
		prompt = session.PropulsionNudge()
	}
	if err := m.backend.SendInput(ctx, sandboxSession, prompt); err != nil {
		debugSession("SendInput prompt", err)
	}

	return nil
}

// Stop terminates a polecat session.
// If a backend is configured, it uses the backend abstraction layer.
func (m *SessionManager) Stop(polecat string, force bool) error {
	// Route to backend if configured and it's a remote backend
	if m.IsRemoteBackend() {
		return m.stopWithRemoteBackend(polecat, force)
	}

	return m.stopLocal(polecat, force)
}

// stopLocal terminates a local tmux polecat session.
func (m *SessionManager) stopLocal(polecat string, force bool) error {
	sessionID := m.SessionName(polecat)

	running, err := m.tmux.HasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return ErrSessionNotFound
	}

	// Sync beads before shutdown (non-fatal)
	if !force {
		polecatDir := m.polecatDir(polecat)
		if err := m.syncBeads(polecatDir); err != nil {
			fmt.Printf("Warning: beads sync failed: %v\n", err)
		}
	}

	// Try graceful shutdown first
	if !force {
		_ = m.tmux.SendKeysRaw(sessionID, "C-c")
		time.Sleep(100 * time.Millisecond)
	}

	if err := m.tmux.KillSession(sessionID); err != nil {
		return fmt.Errorf("killing session: %w", err)
	}

	return nil
}

// stopWithRemoteBackend terminates a remote sandbox polecat session.
func (m *SessionManager) stopWithRemoteBackend(polecat string, force bool) error {
	ctx := context.Background()

	// Get tracked session
	sandboxSession, ok := m.activeSessions[polecat]
	if !ok {
		// Try to find by session name and load persisted state
		sessionName := m.SessionName(polecat)
		exists, err := m.backend.HasSession(ctx, sessionName)
		if err != nil {
			return fmt.Errorf("checking session: %w", err)
		}
		if !exists {
			return ErrSessionNotFound
		}

		// Try to load persisted state to get metadata (local_work_dir, sandbox_id, etc.)
		state, err := m.loadSessionState(polecat)
		if err != nil {
			// Create minimal session if state not found
			sandboxSession = &sandbox.Session{ID: sessionName, Metadata: make(map[string]string)}
		} else {
			sandboxSession = &sandbox.Session{
				ID:       sessionName,
				Backend:  sandbox.BackendType(state.Backend),
				Metadata: state.Metadata,
			}
		}
	}

	// Get work directories for sync back (stored in metadata during start)
	localWorkDir := sandboxSession.Metadata["local_work_dir"]
	if localWorkDir == "" {
		localWorkDir = m.polecatDir(polecat)
	}
	remoteWorkDir := sandboxSession.Metadata["remote_work_dir"]
	if remoteWorkDir == "" {
		// Fallback: get from backend config or use default
		remoteWorkDir = sandbox.DefaultRemoteWorkDir
		if daytonaBackend, ok := m.backend.(*sandbox.DaytonaBackend); ok {
			remoteWorkDir = daytonaBackend.RemoteWorkDir()
		}
	}

	// Sync changes from sandbox back to local worktree (non-fatal)
	// This downloads any file changes the agent made so they appear in the local git worktree.
	if !force {
		if syncBackend, ok := m.backend.(sandbox.SyncBackend); ok {
			fmt.Printf("Syncing changes from sandbox: %s -> %s\n", remoteWorkDir, localWorkDir)
			if err := syncBackend.SyncFromSession(ctx, sandboxSession, remoteWorkDir, localWorkDir); err != nil {
				fmt.Printf("Warning: failed to sync changes from sandbox: %v\n", err)
			} else {
				fmt.Printf("✓ Changes synced from sandbox\n")
			}
		}
	}

	// Sync beads before shutdown (non-fatal)
	if !force {
		if err := m.syncBeads(localWorkDir); err != nil {
			fmt.Printf("Warning: beads sync failed: %v\n", err)
		}
	}

	// Destroy the sandbox
	if err := m.backend.Destroy(ctx, sandboxSession); err != nil {
		return fmt.Errorf("destroying sandbox: %w", err)
	}

	// Remove from tracking and clean up persisted state
	delete(m.activeSessions, polecat)
	_ = m.deleteSessionState(polecat)

	return nil
}

// syncBeads runs bd sync in the given directory.
func (m *SessionManager) syncBeads(workDir string) error {
	cmd := exec.Command("bd", "sync")
	cmd.Dir = workDir
	return cmd.Run()
}

// IsRunning checks if a polecat session is active.
// If a backend is configured, it uses the backend abstraction layer.
func (m *SessionManager) IsRunning(polecat string) (bool, error) {
	// Route to backend if configured and it's a remote backend
	if m.IsRemoteBackend() {
		ctx := context.Background()
		sessionName := m.SessionName(polecat)

		// For remote backends, we need to check if a sandbox exists.
		// First try cached session, then load persisted state to get sandbox_id.
		var session *sandbox.Session
		if cached, ok := m.activeSessions[polecat]; ok {
			session = cached
		} else {
			// Try to load persisted state (cross-process session tracking)
			state, err := m.loadSessionState(polecat)
			if err != nil {
				// No persisted state - session doesn't exist
				return false, nil
			}
			// Build session from persisted state
			session = &sandbox.Session{
				ID:       sessionName,
				Backend:  sandbox.BackendType(state.Backend),
				Metadata: state.Metadata,
			}
		}

		// Check if sandbox exists using sandbox_id from metadata
		sandboxID := session.Metadata[sandbox.MetaSandboxID]
		if sandboxID == "" {
			// No sandbox_id - can't verify, assume not running
			return false, nil
		}

		// Query the backend to check if sandbox is still active
		exists, err := m.backend.HasSession(ctx, sandboxID)
		if err != nil {
			return false, err
		}
		if !exists {
			return false, nil
		}

		running, err := m.backend.IsRunning(ctx, session)
		return running, err
	}

	sessionID := m.SessionName(polecat)
	return m.tmux.HasSession(sessionID)
}

// Status returns detailed status for a polecat session.
// If a backend is configured, it uses the backend abstraction layer.
func (m *SessionManager) Status(polecat string) (*SessionInfo, error) {
	// Route to backend if configured and it's a remote backend
	if m.IsRemoteBackend() {
		return m.statusWithRemoteBackend(polecat)
	}

	return m.statusLocal(polecat)
}

// statusLocal returns status for a local tmux polecat session.
func (m *SessionManager) statusLocal(polecat string) (*SessionInfo, error) {
	sessionID := m.SessionName(polecat)

	running, err := m.tmux.HasSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("checking session: %w", err)
	}

	info := &SessionInfo{
		Polecat:   polecat,
		SessionID: sessionID,
		Running:   running,
		RigName:   m.rig.Name,
		Backend:   string(sandbox.BackendLocal),
	}

	if !running {
		return info, nil
	}

	tmuxInfo, err := m.tmux.GetSessionInfo(sessionID)
	if err != nil {
		return info, nil
	}

	info.Attached = tmuxInfo.Attached
	info.Windows = tmuxInfo.Windows

	if tmuxInfo.Created != "" {
		formats := []string{
			"Mon Jan 2 15:04:05 2006",
			"Mon Jan _2 15:04:05 2006",
			time.ANSIC,
			time.UnixDate,
		}
		for _, format := range formats {
			if t, err := time.Parse(format, tmuxInfo.Created); err == nil {
				info.Created = t
				break
			}
		}
	}

	if tmuxInfo.Activity != "" {
		var activityUnix int64
		if _, err := fmt.Sscanf(tmuxInfo.Activity, "%d", &activityUnix); err == nil && activityUnix > 0 {
			info.LastActivity = time.Unix(activityUnix, 0)
		}
	}

	return info, nil
}

// statusWithRemoteBackend returns status for a remote sandbox polecat session.
// For remote backends, we query the backend directly rather than relying on
// in-memory tracking, since sandboxes persist across process restarts.
func (m *SessionManager) statusWithRemoteBackend(polecat string) (*SessionInfo, error) {
	ctx := context.Background()
	sessionName := m.SessionName(polecat)

	info := &SessionInfo{
		Polecat:   polecat,
		SessionID: sessionName,
		Running:   false,
		RigName:   m.rig.Name,
		Backend:   string(m.backend.Type()),
	}

	// For remote backends, always query the backend directly.
	// Sandboxes persist independently of this process, so in-memory tracking
	// is only a cache - the backend is the source of truth.

	// First check if session exists in the backend
	exists, err := m.backend.HasSession(ctx, sessionName)
	if err != nil {
		return nil, fmt.Errorf("checking session existence: %w", err)
	}

	if !exists {
		// Session doesn't exist in backend
		// Clean up stale in-memory tracking if present
		delete(m.activeSessions, polecat)
		return info, nil
	}

	// Session exists - check if agent is actually running
	// Create a minimal session object to query running state
	session := &sandbox.Session{ID: sessionName}

	// Use cached session if available (has richer metadata like CreatedAt)
	if cached, ok := m.activeSessions[polecat]; ok {
		session = cached
		info.Created = cached.CreatedAt
	}

	running, err := m.backend.IsRunning(ctx, session)
	if err != nil {
		// Session exists but we can't determine running state
		// Be conservative and report it exists
		info.Running = true
		return info, nil
	}

	info.Running = running
	return info, nil
}

// List returns information about all polecat sessions for this rig.
func (m *SessionManager) List() ([]SessionInfo, error) {
	// For remote backends, list from persisted session state directory
	if m.IsRemoteBackend() {
		return m.listRemoteSessions()
	}

	// For local backend, list from tmux sessions
	sessions, err := m.tmux.ListSessions()
	if err != nil {
		return nil, err
	}

	prefix := fmt.Sprintf("gt-%s-", m.rig.Name)
	var infos []SessionInfo

	for _, sessionID := range sessions {
		if !strings.HasPrefix(sessionID, prefix) {
			continue
		}

		polecat := strings.TrimPrefix(sessionID, prefix)
		infos = append(infos, SessionInfo{
			Polecat:   polecat,
			SessionID: sessionID,
			Running:   true,
			RigName:   m.rig.Name,
		})
	}

	return infos, nil
}

// listRemoteSessions lists polecat sessions from persisted state files.
// Remote sessions are stored in {rig}/.runtime/daytona-sessions/{polecat}.json
func (m *SessionManager) listRemoteSessions() ([]SessionInfo, error) {
	sessionsDir := filepath.Join(m.rig.Path, ".runtime", "daytona-sessions")

	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No sessions directory = no sessions
		}
		return nil, fmt.Errorf("reading sessions directory: %w", err)
	}

	var infos []SessionInfo
	ctx := context.Background()

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		// Extract polecat name from filename (e.g., "Toast.json" -> "Toast")
		polecatName := strings.TrimSuffix(entry.Name(), ".json")

		// Load the session state to get details
		state, err := m.loadSessionState(polecatName)
		if err != nil {
			continue // Skip invalid state files
		}

		// Check if session still exists in the backend
		running := false
		if m.backend != nil {
			running, _ = m.backend.HasSession(ctx, state.SessionID)
		}

		infos = append(infos, SessionInfo{
			Polecat:   polecatName,
			SessionID: state.SessionID,
			Running:   running,
			RigName:   m.rig.Name,
			Backend:   state.Backend,
			Created:   state.CreatedAt,
		})
	}

	return infos, nil
}

// Attach attaches to a polecat session.
func (m *SessionManager) Attach(polecat string) error {
	sessionID := m.SessionName(polecat)

	running, err := m.tmux.HasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return ErrSessionNotFound
	}

	return m.tmux.AttachSession(sessionID)
}

// Capture returns the recent output from a polecat session.
func (m *SessionManager) Capture(polecat string, lines int) (string, error) {
	// For remote backends, reconnect to PTY and capture live output
	if m.IsRemoteBackend() {
		return m.captureRemote(polecat, lines)
	}

	sessionID := m.SessionName(polecat)

	running, err := m.tmux.HasSession(sessionID)
	if err != nil {
		return "", fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return "", ErrSessionNotFound
	}

	return m.tmux.CapturePane(sessionID, lines)
}

// captureRemote captures output from a remote polecat session.
// Reads from the script log file where Claude output is captured.
func (m *SessionManager) captureRemote(polecat string, lines int) (string, error) {
	ctx := context.Background()

	// Load persisted session state
	state, err := m.loadSessionState(polecat)
	if err != nil {
		return "", fmt.Errorf("loading session state: %w", err)
	}

	// Check if backend supports execution (Daytona)
	daytonaBackend, ok := m.backend.(*sandbox.DaytonaBackend)
	if !ok {
		return "", fmt.Errorf("remote backend does not support command execution")
	}

	// Get the Daytona client
	client, err := daytonaBackend.GetClient()
	if err != nil {
		return "", fmt.Errorf("getting Daytona client: %w", err)
	}

	// Read from the script log file where Claude output is captured
	// Wrap in sh -c to ensure shell operators work correctly
	cmd := fmt.Sprintf("sh -c 'tail -n %d %s 2>/dev/null || echo \"[No output log yet]\"'", lines, sandbox.DaytonaOutputLog)
	resp, err := client.ExecuteCommand(ctx, state.SandboxID, &daytona.ExecuteRequest{
		Command: cmd,
		Timeout: 60,
	})
	if err != nil {
		return "", fmt.Errorf("reading output log: %w", err)
	}

	return resp.Result, nil
}

// CaptureSession returns the recent output from a session by raw session ID.
func (m *SessionManager) CaptureSession(sessionID string, lines int) (string, error) {
	running, err := m.tmux.HasSession(sessionID)
	if err != nil {
		return "", fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return "", ErrSessionNotFound
	}

	return m.tmux.CapturePane(sessionID, lines)
}

// Inject sends a message to a polecat session.
func (m *SessionManager) Inject(polecat, message string) error {
	// For remote backends, reconnect to PTY and send input
	if m.IsRemoteBackend() {
		return m.injectRemote(polecat, message)
	}

	sessionID := m.SessionName(polecat)

	running, err := m.tmux.HasSession(sessionID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !running {
		return ErrSessionNotFound
	}

	debounceMs := 200 + (len(message)/1024)*100
	if debounceMs > 1500 {
		debounceMs = 1500
	}

	return m.tmux.SendKeysDebounced(sessionID, message, debounceMs)
}

// injectRemote sends a message to a remote polecat session via PTY.
func (m *SessionManager) injectRemote(polecat, message string) error {
	ctx := context.Background()

	// Load persisted session state
	state, err := m.loadSessionState(polecat)
	if err != nil {
		return fmt.Errorf("loading session state: %w", err)
	}

	// Check if backend supports Daytona
	daytonaBackend, ok := m.backend.(*sandbox.DaytonaBackend)
	if !ok {
		return fmt.Errorf("remote backend does not support PTY input")
	}

	// Build session object from persisted state
	session := &sandbox.Session{
		ID:      state.SessionID,
		Backend: sandbox.BackendType(state.Backend),
		Metadata: map[string]string{
			sandbox.MetaSandboxID: state.SandboxID,
			sandbox.MetaPtyID:     state.PtyID,
		},
	}

	// Send the message via PTY
	if err := daytonaBackend.SendInput(ctx, session, message); err != nil {
		return fmt.Errorf("sending message: %w", err)
	}

	return nil
}

// StopAll terminates all polecat sessions for this rig.
func (m *SessionManager) StopAll(force bool) error {
	infos, err := m.List()
	if err != nil {
		return err
	}

	var lastErr error
	for _, info := range infos {
		if err := m.Stop(info.Polecat, force); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// hookIssue pins an issue to a polecat's hook using bd update.
func (m *SessionManager) hookIssue(issueID, agentID, workDir string) error {
	cmd := exec.Command("bd", "update", issueID, "--status=hooked", "--assignee="+agentID) //nolint:gosec
	cmd.Dir = workDir
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bd update failed: %w", err)
	}
	fmt.Printf("✓ Hooked issue %s to %s\n", issueID, agentID)
	return nil
}

// RemoteCompletionResult contains the result of checking remote polecat completion.
type RemoteCompletionResult struct {
	// Completed indicates the polecat has finished its task
	Completed bool

	// Reason describes why we determined the task is complete (or not)
	Reason string

	// Output is the captured output used for analysis (last N lines)
	Output string
}

// TaskCompletedMarker is the exact string that agents output to signal task completion.
// This is specified in the task prompt for remote polecats.
const TaskCompletedMarker = "TASK_COMPLETED"

// CheckRemoteCompletion checks if a remote polecat has completed its task.
// The agent is instructed to output "TASK_COMPLETED" when done.
// Returns a result indicating whether the task is complete.
func (m *SessionManager) CheckRemoteCompletion(polecat string) (*RemoteCompletionResult, error) {
	if !m.IsRemoteBackend() {
		return nil, fmt.Errorf("CheckRemoteCompletion only works for remote backends")
	}

	// Capture recent output (last 200 lines should be enough to see completion marker)
	output, err := m.captureRemote(polecat, 200)
	if err != nil {
		return nil, fmt.Errorf("capturing output: %w", err)
	}

	result := &RemoteCompletionResult{
		Output: output,
	}

	// If output is empty or just "[No output log yet]", task hasn't started
	if output == "" || strings.Contains(output, "[No output log yet]") {
		result.Completed = false
		result.Reason = "no output yet"
		return result, nil
	}

	// Check for the explicit TASK_COMPLETED marker
	if strings.Contains(output, TaskCompletedMarker) {
		result.Completed = true
		result.Reason = "found TASK_COMPLETED marker"
		return result, nil
	}

	result.Completed = false
	result.Reason = "TASK_COMPLETED marker not found"
	return result, nil
}

// CompleteRemotePolecatResult contains the result of the completion protocol.
type CompleteRemotePolecatResult struct {
	// Success indicates the completion protocol succeeded
	Success bool

	// ChangesSynced indicates files were synced from sandbox
	ChangesSynced bool

	// BeadsSynced indicates bd sync was run
	BeadsSynced bool

	// BranchName is the git branch with changes (if any)
	BranchName string

	// Error contains any error that occurred
	Error error
}

// CompleteRemotePolecat executes the completion protocol for a remote polecat.
// This is called when completion is detected and handles:
// 1. Syncing changes from sandbox back to local worktree
// 2. Committing changes with git add + commit
// 3. Running bd sync locally
// 4. Stopping/destroying the sandbox
//
// Note: The MR submission and POLECAT_DONE notification should be handled
// by the caller (daemon) after this method returns, since they require
// access to the witness mail system and merge queue.
func (m *SessionManager) CompleteRemotePolecat(polecat string) (*CompleteRemotePolecatResult, error) {
	if !m.IsRemoteBackend() {
		return nil, fmt.Errorf("CompleteRemotePolecat only works for remote backends")
	}

	result := &CompleteRemotePolecatResult{}
	ctx := context.Background()

	// Load persisted session state to get metadata
	state, err := m.loadSessionState(polecat)
	if err != nil {
		result.Error = fmt.Errorf("loading session state: %w", err)
		return result, result.Error
	}

	// Get tracked session or build from persisted state
	sandboxSession, ok := m.activeSessions[polecat]
	if !ok {
		sandboxSession = &sandbox.Session{
			ID:       state.SessionID,
			Backend:  sandbox.BackendType(state.Backend),
			Metadata: state.Metadata,
		}
	}

	// Get work directories from session metadata
	localWorkDir := sandboxSession.Metadata["local_work_dir"]
	if localWorkDir == "" {
		localWorkDir = m.polecatDir(polecat)
	}
	remoteWorkDir := sandboxSession.Metadata["remote_work_dir"]
	if remoteWorkDir == "" {
		remoteWorkDir = sandbox.DefaultRemoteWorkDir
		if daytonaBackend, ok := m.backend.(*sandbox.DaytonaBackend); ok {
			remoteWorkDir = daytonaBackend.RemoteWorkDir()
		}
	}

	// Step 1: Sync changes from sandbox back to local worktree
	if syncBackend, ok := m.backend.(sandbox.SyncBackend); ok {
		fmt.Printf("Syncing changes from sandbox: %s -> %s\n", remoteWorkDir, localWorkDir)
		if err := syncBackend.SyncFromSession(ctx, sandboxSession, remoteWorkDir, localWorkDir); err != nil {
			fmt.Printf("Warning: failed to sync changes from sandbox: %v\n", err)
		} else {
			fmt.Printf("✓ Changes synced from sandbox\n")
			result.ChangesSynced = true
		}
	}

	// Step 2: Commit changes (git add + commit)
	if result.ChangesSynced {
		// Check if there are any changes to commit
		statusCmd := exec.Command("git", "status", "--porcelain")
		statusCmd.Dir = localWorkDir
		if statusOut, err := statusCmd.Output(); err == nil && len(statusOut) > 0 {
			// Stage all changes
			addCmd := exec.Command("git", "add", ".")
			addCmd.Dir = localWorkDir
			if err := addCmd.Run(); err != nil {
				fmt.Printf("Warning: git add failed: %v\n", err)
			} else {
				// Get issue ID from session metadata if available
				issueID := sandboxSession.Metadata["issue_id"]
				commitMsg := "Remote polecat work"
				if issueID != "" {
					commitMsg = fmt.Sprintf("Remote polecat work (%s)", issueID)
				}

				// Commit the changes
				commitCmd := exec.Command("git", "commit", "-m", commitMsg)
				commitCmd.Dir = localWorkDir
				if err := commitCmd.Run(); err != nil {
					fmt.Printf("Warning: git commit failed: %v\n", err)
				} else {
					fmt.Printf("✓ Changes committed: %s\n", commitMsg)
				}
			}
		}
	}

	// Step 3: Run bd sync locally to update beads
	if err := m.syncBeads(localWorkDir); err != nil {
		fmt.Printf("Warning: beads sync failed: %v\n", err)
	} else {
		result.BeadsSynced = true
	}

	// Step 4: Get the branch name for MR submission
	gitCmd := exec.Command("git", "branch", "--show-current")
	gitCmd.Dir = localWorkDir
	if branchOut, err := gitCmd.Output(); err == nil {
		result.BranchName = strings.TrimSpace(string(branchOut))
	}

	// Step 5: Destroy the sandbox (this also cleans up PTY and session state)
	if err := m.backend.Destroy(ctx, sandboxSession); err != nil {
		result.Error = fmt.Errorf("destroying sandbox: %w", err)
		return result, result.Error
	}

	// Remove from tracking and clean up persisted state
	delete(m.activeSessions, polecat)
	_ = m.deleteSessionState(polecat)

	result.Success = true
	return result, nil
}

// GetLocalWorkDir returns the local working directory for a polecat.
// This is useful for callers that need to run commands in the polecat's worktree.
func (m *SessionManager) GetLocalWorkDir(polecat string) string {
	// Try to get from session metadata first (for remote sessions)
	if m.IsRemoteBackend() {
		if state, err := m.loadSessionState(polecat); err == nil {
			if localDir := state.Metadata["local_work_dir"]; localDir != "" {
				return localDir
			}
		}
	}
	// Fall back to standard polecat directory
	return m.polecatDir(polecat)
}

// GetHookBead returns the hook bead ID for a remote polecat.
// Returns empty string if not found or not a remote backend.
func (m *SessionManager) GetHookBead(polecat string) string {
	if !m.IsRemoteBackend() {
		return ""
	}
	state, err := m.loadSessionState(polecat)
	if err != nil {
		return ""
	}
	return state.Metadata["hook_bead"]
}
