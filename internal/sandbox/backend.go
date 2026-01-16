// Package sandbox provides an abstraction layer for agent execution backends.
// It enables running agents locally (via tmux) or remotely (via Daytona sandboxes).
package sandbox

import (
	"context"
	"time"
)

// BackendType identifies the execution backend.
type BackendType string

const (
	// BackendLocal uses local tmux sessions for agent execution.
	BackendLocal BackendType = "local"

	// BackendDaytona uses remote Daytona sandboxes for agent execution.
	BackendDaytona BackendType = "daytona"
)

// Session represents an active agent execution session.
// For local backend, this wraps a tmux session.
// For Daytona backend, this wraps a sandbox instance.
type Session struct {
	// ID is the unique identifier for this session.
	// Local: tmux session name (e.g., "gt-gastown-Toast")
	// Daytona: sandbox ID
	ID string

	// Backend is the type of backend managing this session.
	Backend BackendType

	// WorkDir is the working directory inside the session.
	WorkDir string

	// Metadata contains backend-specific data.
	// Local: may contain pane ID, etc.
	// Daytona: may contain sandbox status, PTY handle, etc.
	Metadata map[string]string

	// CreatedAt is when the session was created.
	CreatedAt time.Time
}

// CreateOptions configures session creation.
type CreateOptions struct {
	// Name is the session identifier (e.g., polecat name).
	Name string

	// WorkDir is the initial working directory.
	WorkDir string

	// Environment variables to set in the session.
	Env map[string]string

	// RepoURL is the git repository to clone (Daytona only).
	// For local backend, the worktree is already set up.
	RepoURL string

	// Branch is the git branch to checkout.
	Branch string

	// Command is the startup command to run (e.g., "claude --dangerously-skip-permissions").
	// If empty, uses the default agent startup command.
	Command string
}

// Backend defines the interface for agent execution backends.
// Implementations must be safe for concurrent use.
type Backend interface {
	// Type returns the backend type identifier.
	Type() BackendType

	// Create creates a new session but does not start the agent.
	// The session is ready for agent startup after this call.
	Create(ctx context.Context, opts CreateOptions) (*Session, error)

	// Start starts the agent process in an existing session.
	// This sends the startup command and waits for the agent to be ready.
	Start(ctx context.Context, session *Session, command string) error

	// Stop gracefully stops the agent in a session.
	// The session remains available for restart.
	Stop(ctx context.Context, session *Session) error

	// Destroy terminates and removes a session completely.
	// For local: kills tmux session.
	// For Daytona: deletes the sandbox.
	Destroy(ctx context.Context, session *Session) error

	// IsRunning checks if an agent is actively running in the session.
	// This checks for the actual agent process, not just session existence.
	IsRunning(ctx context.Context, session *Session) (bool, error)

	// HasSession checks if a session exists (may be stopped but not destroyed).
	HasSession(ctx context.Context, sessionID string) (bool, error)

	// SendInput sends text input to the agent (like typing in a terminal).
	// The message is sent followed by Enter key.
	SendInput(ctx context.Context, session *Session, message string) error

	// CaptureOutput captures recent output from the agent session.
	// lines specifies how many lines of history to capture.
	CaptureOutput(ctx context.Context, session *Session, lines int) (string, error)

	// SetEnvironment sets an environment variable in the session.
	SetEnvironment(ctx context.Context, session *Session, key, value string) error
}

// SyncBackend extends Backend with file synchronization capabilities.
// This is optional and primarily used by the Daytona backend.
type SyncBackend interface {
	Backend

	// SyncToSession uploads local files to the session.
	// srcPath is the local path, dstPath is the path inside the session.
	SyncToSession(ctx context.Context, session *Session, srcPath, dstPath string) error

	// SyncFromSession downloads files from the session to local.
	// srcPath is the path inside the session, dstPath is the local path.
	SyncFromSession(ctx context.Context, session *Session, srcPath, dstPath string) error
}

// PooledBackend extends Backend with session pooling capabilities.
// This enables pre-warming sessions for faster agent startup.
type PooledBackend interface {
	Backend

	// WarmPool ensures at least n sessions are pre-created and ready.
	WarmPool(ctx context.Context, n int, opts CreateOptions) error

	// AcquireFromPool gets a pre-warmed session from the pool.
	// Returns nil if no sessions are available in the pool.
	AcquireFromPool(ctx context.Context) (*Session, error)

	// ReleaseToPool returns a session to the pool for reuse.
	// The session should be in a clean state before release.
	ReleaseToPool(ctx context.Context, session *Session) error

	// PoolSize returns the current number of available sessions in the pool.
	PoolSize(ctx context.Context) (int, error)
}
