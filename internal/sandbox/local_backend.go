package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/steveyegge/gastown/internal/tmux"
)

// LocalBackend implements Backend using local tmux sessions.
// This is the default backend and provides the original GasTown behavior.
type LocalBackend struct {
	tmux   *tmux.Tmux
	config *LocalConfig
}

// NewLocalBackend creates a new local backend.
func NewLocalBackend(config *LocalConfig) *LocalBackend {
	return &LocalBackend{
		tmux:   tmux.NewTmux(),
		config: config,
	}
}

// Type returns the backend type identifier.
func (b *LocalBackend) Type() BackendType {
	return BackendLocal
}

// Create creates a new tmux session.
// For the local backend, this creates a detached tmux session in the working directory.
func (b *LocalBackend) Create(ctx context.Context, opts CreateOptions) (*Session, error) {
	sessionID := opts.Name
	if sessionID == "" {
		return nil, fmt.Errorf("session name is required")
	}

	// Check if session already exists
	exists, err := b.tmux.HasSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("checking session: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("session already exists: %s", sessionID)
	}

	// Create the session
	if err := b.tmux.NewSession(sessionID, opts.WorkDir); err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	// Set environment variables
	for key, value := range opts.Env {
		if err := b.tmux.SetEnvironment(sessionID, key, value); err != nil {
			// Non-fatal - log but continue
			fmt.Printf("Warning: failed to set env %s: %v\n", key, err)
		}
	}

	return &Session{
		ID:        sessionID,
		Backend:   BackendLocal,
		WorkDir:   opts.WorkDir,
		Metadata:  make(map[string]string),
		CreatedAt: time.Now(),
	}, nil
}

// Start starts the agent process in an existing session.
// For the local backend, this sends the startup command to tmux.
func (b *LocalBackend) Start(ctx context.Context, session *Session, command string) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}

	// Check session exists
	exists, err := b.tmux.HasSession(session.ID)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !exists {
		return fmt.Errorf("session not found: %s", session.ID)
	}

	// Send the startup command
	if err := b.tmux.SendKeys(session.ID, command); err != nil {
		return fmt.Errorf("sending command: %w", err)
	}

	return nil
}

// Stop gracefully stops the agent in a session.
// For the local backend, this sends Ctrl-C to the session.
func (b *LocalBackend) Stop(ctx context.Context, session *Session) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}

	// Send Ctrl-C for graceful shutdown
	if err := b.tmux.SendKeysRaw(session.ID, "C-c"); err != nil {
		return fmt.Errorf("sending Ctrl-C: %w", err)
	}

	// Wait a bit for graceful shutdown
	time.Sleep(100 * time.Millisecond)

	return nil
}

// Destroy terminates and removes a session completely.
// For the local backend, this kills the tmux session.
func (b *LocalBackend) Destroy(ctx context.Context, session *Session) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}

	if err := b.tmux.KillSession(session.ID); err != nil {
		// Check if session already doesn't exist
		exists, checkErr := b.tmux.HasSession(session.ID)
		if checkErr == nil && !exists {
			return nil // Already gone
		}
		return fmt.Errorf("killing session: %w", err)
	}

	return nil
}

// IsRunning checks if an agent is actively running in the session.
// For the local backend, this checks if Claude (node) is running in the tmux pane.
func (b *LocalBackend) IsRunning(ctx context.Context, session *Session) (bool, error) {
	if session == nil {
		return false, fmt.Errorf("session is nil")
	}

	// Check if session exists first
	exists, err := b.tmux.HasSession(session.ID)
	if err != nil {
		return false, fmt.Errorf("checking session: %w", err)
	}
	if !exists {
		return false, nil
	}

	// Check if Claude is running (node process)
	return b.tmux.IsClaudeRunning(session.ID), nil
}

// HasSession checks if a session exists (may be stopped but not destroyed).
func (b *LocalBackend) HasSession(ctx context.Context, sessionID string) (bool, error) {
	return b.tmux.HasSession(sessionID)
}

// SendInput sends text input to the agent.
// For the local backend, this uses tmux send-keys.
func (b *LocalBackend) SendInput(ctx context.Context, session *Session, message string) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}

	return b.tmux.NudgeSession(session.ID, message)
}

// CaptureOutput captures recent output from the agent session.
// For the local backend, this uses tmux capture-pane.
func (b *LocalBackend) CaptureOutput(ctx context.Context, session *Session, lines int) (string, error) {
	if session == nil {
		return "", fmt.Errorf("session is nil")
	}

	return b.tmux.CapturePane(session.ID, lines)
}

// SetEnvironment sets an environment variable in the session.
func (b *LocalBackend) SetEnvironment(ctx context.Context, session *Session, key, value string) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}

	return b.tmux.SetEnvironment(session.ID, key, value)
}

// --- Additional methods specific to local backend ---

// Tmux returns the underlying tmux instance for advanced operations.
// This allows callers that need tmux-specific functionality to access it.
func (b *LocalBackend) Tmux() *tmux.Tmux {
	return b.tmux
}

// EnsureSessionFresh creates a new session or kills and recreates if zombie.
// This is useful for restart scenarios.
func (b *LocalBackend) EnsureSessionFresh(ctx context.Context, opts CreateOptions) (*Session, error) {
	sessionID := opts.Name
	if sessionID == "" {
		return nil, fmt.Errorf("session name is required")
	}

	// Use tmux's EnsureSessionFresh which handles zombies
	if err := b.tmux.EnsureSessionFresh(sessionID, opts.WorkDir); err != nil {
		return nil, fmt.Errorf("ensuring fresh session: %w", err)
	}

	// Set environment variables
	for key, value := range opts.Env {
		if err := b.tmux.SetEnvironment(sessionID, key, value); err != nil {
			// Non-fatal
			fmt.Printf("Warning: failed to set env %s: %v\n", key, err)
		}
	}

	return &Session{
		ID:        sessionID,
		Backend:   BackendLocal,
		WorkDir:   opts.WorkDir,
		Metadata:  make(map[string]string),
		CreatedAt: time.Now(),
	}, nil
}

// GetSession retrieves session information for an existing session.
func (b *LocalBackend) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	exists, err := b.tmux.HasSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("checking session: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	// Get working directory
	workDir, _ := b.tmux.GetPaneWorkDir(sessionID)

	return &Session{
		ID:       sessionID,
		Backend:  BackendLocal,
		WorkDir:  workDir,
		Metadata: make(map[string]string),
	}, nil
}

// ApplyTheme applies Gas Town theming to a session.
func (b *LocalBackend) ApplyTheme(ctx context.Context, session *Session, theme tmux.Theme, rig, worker, role string) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}

	return b.tmux.ConfigureGasTownSession(session.ID, theme, rig, worker, role)
}

// WaitForAgentReady waits for the agent to be ready after startup.
func (b *LocalBackend) WaitForAgentReady(ctx context.Context, session *Session, excludeCommands []string, timeout time.Duration) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}

	return b.tmux.WaitForCommand(session.ID, excludeCommands, timeout)
}

// AcceptBypassPermissionsWarning handles Claude's bypass permissions dialog.
func (b *LocalBackend) AcceptBypassPermissionsWarning(ctx context.Context, session *Session) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}

	return b.tmux.AcceptBypassPermissionsWarning(session.ID)
}

// AttachSession attaches to a session interactively.
func (b *LocalBackend) AttachSession(ctx context.Context, session *Session) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}

	return b.tmux.AttachSession(session.ID)
}

// SetPaneDiedHook sets a hook to detect when the pane process dies.
func (b *LocalBackend) SetPaneDiedHook(ctx context.Context, session *Session, agentID string) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}

	return b.tmux.SetPaneDiedHook(session.ID, agentID)
}

// Compile-time interface check
var _ Backend = (*LocalBackend)(nil)
