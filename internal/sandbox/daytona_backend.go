package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/steveyegge/gastown/internal/sandbox/daytona"
)

// Metadata keys for session tracking
const (
	MetaSandboxID   = "sandbox_id"
	MetaPtyID       = "pty_id"
	MetaSandboxName = "sandbox_name"
)

// DaytonaBackend implements Backend using Daytona sandboxes.
// This enables running agents in isolated cloud sandboxes for scalability.
//
// See: https://www.daytona.io/docs/en/claude-code-run-tasks-stream-logs-sandbox/
type DaytonaBackend struct {
	config *DaytonaConfig
	client *daytona.Client

	// activeSessions tracks currently active sandbox sessions
	activeSessions map[string]*Session
	sessionsMu     sync.RWMutex

	// ptyHandles tracks WebSocket PTY connections per session
	ptyHandles   map[string]*daytona.PtyHandle
	ptyHandlesMu sync.RWMutex
}

// NewDaytonaBackend creates a new Daytona backend.
func NewDaytonaBackend(config *DaytonaConfig) *DaytonaBackend {
	if config == nil {
		config = DefaultDaytonaConfig()
	}

	return &DaytonaBackend{
		config:         config,
		activeSessions: make(map[string]*Session),
		ptyHandles:     make(map[string]*daytona.PtyHandle),
	}
}

// ensureClient initializes the Daytona API client if not already done.
func (b *DaytonaBackend) ensureClient() error {
	if b.client != nil {
		return nil
	}

	apiKey := os.Getenv(b.config.APIKeyEnv)
	if apiKey == "" {
		return fmt.Errorf("Daytona API key not found in environment variable %s", b.config.APIKeyEnv)
	}

	var opts []daytona.ClientOption
	// Add any custom options from config here

	client, err := daytona.NewClient(apiKey, opts...)
	if err != nil {
		return fmt.Errorf("creating Daytona client: %w", err)
	}

	b.client = client
	return nil
}

// Type returns the backend type identifier.
func (b *DaytonaBackend) Type() BackendType {
	return BackendDaytona
}

// Create creates a new Daytona sandbox.
// This provisions a sandbox and optionally installs Claude Code.
func (b *DaytonaBackend) Create(ctx context.Context, opts CreateOptions) (*Session, error) {
	if err := b.ensureClient(); err != nil {
		return nil, err
	}

	// Build sandbox create request
	req := &daytona.CreateSandboxRequest{
		Name: opts.Name,
		Env:  opts.Env,
		Labels: map[string]string{
			"gastown":     "true",
			"gastown-rig": opts.Name, // Use Name as rig identifier
		},
	}

	// Use snapshot if configured
	if b.config.Snapshot != "" {
		req.Snapshot = b.config.Snapshot
	}

	// Set target location (us, eu)
	if b.config.Target != "" {
		req.Target = b.config.Target
	}

	// Set auto-stop/archive/delete intervals
	// These are always passed - 0 and negative values have specific meanings:
	// - auto_stop: 0 = disabled
	// - auto_archive: 0 = use maximum interval
	// - auto_delete: -1 = disabled, 0 = delete immediately
	req.AutoStopInterval = b.config.AutoStopMinutes
	req.AutoArchiveInterval = b.config.AutoArchiveMinutes
	req.AutoDeleteInterval = b.config.AutoDeleteMinutes

	// Create the sandbox
	sandbox, err := b.client.CreateSandbox(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("creating sandbox: %w", err)
	}

	// Wait for sandbox to be started
	sandbox, err = b.client.WaitForSandboxStarted(ctx, sandbox.ID, 5*time.Minute)
	if err != nil {
		// Try to clean up the sandbox
		_, _ = b.client.DeleteSandbox(ctx, sandbox.ID)
		return nil, fmt.Errorf("waiting for sandbox to start: %w", err)
	}

	// Install Claude Code if not already in snapshot
	if !b.config.SnapshotHasClaudeCode {
		if err := b.installClaudeCode(ctx, sandbox.ID); err != nil {
			// Non-fatal warning
			fmt.Printf("Warning: could not install Claude Code: %v\n", err)
		}
	}

	// Create session object
	// Note: File upload (SyncToSession) should be called after Create() to
	// transfer local worktree files to the sandbox. Git clone is not used
	// because the sandbox cannot access the local filesystem.
	workDir := opts.WorkDir
	if workDir == "" {
		workDir = "/home/daytona"
	}
	session := &Session{
		ID:      opts.Name, // Use the provided name as the session ID
		Backend: BackendDaytona,
		WorkDir: workDir,
		Metadata: map[string]string{
			MetaSandboxID:   sandbox.ID,
			MetaSandboxName: sandbox.Name,
		},
		CreatedAt: time.Now(),
	}

	// Track the session
	b.trackSession(session)

	return session, nil
}

// installClaudeCode installs Claude Code in a sandbox.
func (b *DaytonaBackend) installClaudeCode(ctx context.Context, sandboxID string) error {
	resp, err := b.client.ExecuteCommand(ctx, sandboxID, &daytona.ExecuteRequest{
		Command: "npm install -g @anthropic-ai/claude-code",
		Timeout: 120, // 2 minutes for npm install
	})
	if err != nil {
		return err
	}
	if resp.ExitCode != 0 {
		return fmt.Errorf("npm install failed (exit %d): %s", resp.ExitCode, resp.Result)
	}
	return nil
}

// Start starts the agent process in an existing sandbox.
// This creates a PTY session, connects via WebSocket, and launches Claude Code.
func (b *DaytonaBackend) Start(ctx context.Context, session *Session, command string) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}
	if err := b.ensureClient(); err != nil {
		return err
	}

	sandboxID := session.Metadata[MetaSandboxID]
	if sandboxID == "" {
		return fmt.Errorf("session missing sandbox ID")
	}

	// Generate a unique PTY session ID
	ptyID := fmt.Sprintf("claude-%s-%d", session.ID, time.Now().Unix())

	// Get Anthropic API key to pass to the sandbox
	anthropicKey := os.Getenv(b.config.AnthropicAPIKeyEnv)

	// Create PTY session with environment variables
	envs := map[string]string{
		"TERM": "xterm-256color",
	}
	if anthropicKey != "" {
		envs["ANTHROPIC_API_KEY"] = anthropicKey
	}

	// Create PTY session via REST API
	ptyResp, err := b.client.CreatePtySession(ctx, sandboxID, &daytona.PtyCreateRequest{
		ID:   ptyID,
		Cwd:  session.WorkDir,
		Envs: envs,
		Cols: 120,
		Rows: 40,
	})
	if err != nil {
		return fmt.Errorf("creating PTY session: %w", err)
	}

	// Store PTY ID in session metadata
	session.Metadata[MetaPtyID] = ptyResp.ID

	// Connect to PTY via WebSocket for interactive I/O
	ptyHandle, err := b.client.ConnectPty(ctx, sandboxID, ptyResp.ID)
	if err != nil {
		// Clean up PTY session on failure
		_ = b.client.DeletePtySession(ctx, sandboxID, ptyResp.ID)
		return fmt.Errorf("connecting to PTY websocket: %w", err)
	}

	// Wait for WebSocket connection to be established
	if err := ptyHandle.WaitForConnection(10 * time.Second); err != nil {
		ptyHandle.Disconnect()
		_ = b.client.DeletePtySession(ctx, sandboxID, ptyResp.ID)
		return fmt.Errorf("waiting for PTY connection: %w", err)
	}

	// Store PTY handle for later use
	b.setPtyHandle(session.ID, ptyHandle)

	// Build the command to execute
	if command == "" {
		command = "claude --dangerously-skip-permissions"
	}

	// Send the command to the PTY (with newline to execute)
	if err := ptyHandle.SendInput(command + "\n"); err != nil {
		ptyHandle.Disconnect()
		_ = b.client.DeletePtySession(ctx, sandboxID, ptyResp.ID)
		b.deletePtyHandle(session.ID)
		return fmt.Errorf("sending command to PTY: %w", err)
	}

	// Note: The PtyHandle.readMessages goroutine started in ConnectPty
	// handles reading and buffering output automatically

	return nil
}

// Stop gracefully stops the agent in a sandbox.
func (b *DaytonaBackend) Stop(ctx context.Context, session *Session) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}
	if err := b.ensureClient(); err != nil {
		return err
	}

	sandboxID := session.Metadata[MetaSandboxID]
	if sandboxID == "" {
		return fmt.Errorf("session missing sandbox ID")
	}

	// Disconnect PTY WebSocket handle
	if handle := b.getPtyHandle(session.ID); handle != nil {
		handle.Disconnect()
		b.deletePtyHandle(session.ID)
	}

	// Delete PTY session if exists (this terminates the Claude process)
	ptyID := session.Metadata[MetaPtyID]
	if ptyID != "" {
		if err := b.client.DeletePtySession(ctx, sandboxID, ptyID); err != nil {
			// Non-fatal - PTY may already be terminated
			fmt.Printf("Warning: could not delete PTY session: %v\n", err)
		}
		delete(session.Metadata, MetaPtyID)
	}

	// Stop the sandbox (but don't delete it)
	_, err := b.client.StopSandbox(ctx, sandboxID)
	if err != nil {
		return fmt.Errorf("stopping sandbox: %w", err)
	}

	return nil
}

// Destroy terminates and removes a sandbox completely.
func (b *DaytonaBackend) Destroy(ctx context.Context, session *Session) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}
	if err := b.ensureClient(); err != nil {
		return err
	}

	sandboxID := session.Metadata[MetaSandboxID]
	if sandboxID == "" {
		return fmt.Errorf("session missing sandbox ID")
	}

	// Disconnect PTY WebSocket handle
	if handle := b.getPtyHandle(session.ID); handle != nil {
		handle.Disconnect()
		b.deletePtyHandle(session.ID)
	}

	// Delete the sandbox
	_, err := b.client.DeleteSandbox(ctx, sandboxID)
	if err != nil {
		return fmt.Errorf("deleting sandbox: %w", err)
	}

	// Untrack the session
	b.untrackSession(session.ID)

	return nil
}

// IsRunning checks if an agent is actively running in the sandbox.
func (b *DaytonaBackend) IsRunning(ctx context.Context, session *Session) (bool, error) {
	if session == nil {
		return false, fmt.Errorf("session is nil")
	}
	if err := b.ensureClient(); err != nil {
		return false, err
	}

	sandboxID := session.Metadata[MetaSandboxID]
	if sandboxID == "" {
		// No sandbox ID - try using session ID as sandbox name
		sandboxID = session.ID
	}

	// Get sandbox status
	sandbox, err := b.client.GetSandbox(ctx, sandboxID)
	if err != nil {
		// If we get a 404, the sandbox doesn't exist
		if apiErr, ok := err.(*daytona.APIError); ok && apiErr.StatusCode == 404 {
			return false, nil
		}
		return false, fmt.Errorf("getting sandbox: %w", err)
	}

	// Check if sandbox is in running state
	if !sandbox.State.IsRunning() {
		return false, nil
	}

	// Optionally check if PTY session is active
	ptyID := session.Metadata[MetaPtyID]
	if ptyID != "" {
		ptyInfo, err := b.client.GetPtySession(ctx, sandboxID, ptyID)
		if err != nil {
			// PTY session doesn't exist - process not running
			return false, nil
		}
		// Check PTY status
		return ptyInfo.Status == "running" || ptyInfo.Status == "active", nil
	}

	// Sandbox is running but no PTY tracked - assume running
	return true, nil
}

// HasSession checks if a sandbox exists.
func (b *DaytonaBackend) HasSession(ctx context.Context, sessionID string) (bool, error) {
	if err := b.ensureClient(); err != nil {
		return false, err
	}

	// First check our tracked sessions
	tracked := b.getTrackedSession(sessionID)
	if tracked != nil {
		sandboxID := tracked.Metadata[MetaSandboxID]
		if sandboxID != "" {
			sandbox, err := b.client.GetSandbox(ctx, sandboxID)
			if err != nil {
				if apiErr, ok := err.(*daytona.APIError); ok && apiErr.StatusCode == 404 {
					// Clean up stale tracking
					b.untrackSession(sessionID)
					return false, nil
				}
				return false, err
			}
			return !sandbox.State.IsTerminal(), nil
		}
	}

	// Try using sessionID as sandbox name directly
	sandbox, err := b.client.GetSandbox(ctx, sessionID)
	if err != nil {
		if apiErr, ok := err.(*daytona.APIError); ok && apiErr.StatusCode == 404 {
			return false, nil
		}
		return false, err
	}

	return !sandbox.State.IsTerminal(), nil
}

// SendInput sends text input to the agent via PTY WebSocket.
func (b *DaytonaBackend) SendInput(ctx context.Context, session *Session, message string) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}

	// Get PTY handle for this session
	handle := b.getPtyHandle(session.ID)
	if handle == nil {
		return fmt.Errorf("no PTY connection for session - call Start() first")
	}

	if !handle.IsConnected() {
		return fmt.Errorf("PTY connection is closed")
	}

	// Send input via WebSocket (add newline to execute)
	if err := handle.SendInput(message + "\n"); err != nil {
		return fmt.Errorf("sending input to PTY: %w", err)
	}

	return nil
}

// CaptureOutput captures recent output from the PTY session.
func (b *DaytonaBackend) CaptureOutput(ctx context.Context, session *Session, lines int) (string, error) {
	if session == nil {
		return "", fmt.Errorf("session is nil")
	}

	// Get PTY handle for this session
	handle := b.getPtyHandle(session.ID)
	if handle == nil {
		return "", fmt.Errorf("no PTY connection for session - call Start() first")
	}

	// Get buffered output from the PTY handle
	output := handle.GetBufferedOutput()
	if len(output) == 0 {
		return "", nil
	}

	// Convert to string and optionally limit to last N lines
	outputStr := string(output)
	if lines > 0 {
		outputLines := splitLines(outputStr)
		if len(outputLines) > lines {
			outputLines = outputLines[len(outputLines)-lines:]
		}
		outputStr = joinLines(outputLines)
	}

	return outputStr, nil
}

// splitLines splits a string into lines.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// joinLines joins lines with newlines.
func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	result := lines[0]
	for i := 1; i < len(lines); i++ {
		result += "\n" + lines[i]
	}
	return result
}

// SetEnvironment sets an environment variable in the sandbox.
func (b *DaytonaBackend) SetEnvironment(ctx context.Context, session *Session, key, value string) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}
	if err := b.ensureClient(); err != nil {
		return err
	}

	sandboxID := session.Metadata[MetaSandboxID]
	if sandboxID == "" {
		return fmt.Errorf("session missing sandbox ID")
	}

	// Execute export command in sandbox
	_, err := b.client.ExecuteCommand(ctx, sandboxID, &daytona.ExecuteRequest{
		Command: fmt.Sprintf("export %s=%q", key, value),
		Timeout: 10,
	})
	if err != nil {
		return fmt.Errorf("setting environment: %w", err)
	}

	return nil
}

// --- SyncBackend implementation ---

// SyncToSession uploads local files to the sandbox.
// If srcPath is a directory, it recursively uploads all files.
func (b *DaytonaBackend) SyncToSession(ctx context.Context, session *Session, srcPath, dstPath string) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}
	if err := b.ensureClient(); err != nil {
		return err
	}

	sandboxID := session.Metadata[MetaSandboxID]
	if sandboxID == "" {
		return fmt.Errorf("session missing sandbox ID")
	}

	// Check if srcPath is a file or directory
	info, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	if !info.IsDir() {
		// Single file upload
		return b.client.UploadFile(ctx, sandboxID, srcPath, dstPath)
	}

	// Directory: create destination folder first
	if err := b.client.CreateFolder(ctx, sandboxID, dstPath, ""); err != nil {
		// Ignore error if folder already exists
		if apiErr, ok := err.(*daytona.APIError); !ok || apiErr.StatusCode != 409 {
			return fmt.Errorf("creating destination folder: %w", err)
		}
	}

	// Walk directory and upload all files
	return filepath.Walk(srcPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get relative path
		relPath, err := filepath.Rel(srcPath, path)
		if err != nil {
			return err
		}

		// Skip root
		if relPath == "." {
			return nil
		}

		// Build remote path
		remotePath := filepath.Join(dstPath, relPath)

		if info.IsDir() {
			// Create directory in sandbox
			if err := b.client.CreateFolder(ctx, sandboxID, remotePath, ""); err != nil {
				if apiErr, ok := err.(*daytona.APIError); !ok || apiErr.StatusCode != 409 {
					return fmt.Errorf("creating folder %s: %w", remotePath, err)
				}
			}
			return nil
		}

		// Upload file
		return b.client.UploadFile(ctx, sandboxID, path, remotePath)
	})
}

// SyncFromSession downloads files from the sandbox.
// If srcPath is a directory, it recursively downloads all files.
func (b *DaytonaBackend) SyncFromSession(ctx context.Context, session *Session, srcPath, dstPath string) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}
	if err := b.ensureClient(); err != nil {
		return err
	}

	sandboxID := session.Metadata[MetaSandboxID]
	if sandboxID == "" {
		return fmt.Errorf("session missing sandbox ID")
	}

	// List files at srcPath to determine if it's a file or directory
	files, err := b.client.ListFiles(ctx, sandboxID, srcPath)
	if err != nil {
		// Might be a single file, try downloading directly
		return b.client.DownloadFile(ctx, sandboxID, srcPath, dstPath)
	}

	// If listing returned files, it's a directory
	if err := os.MkdirAll(dstPath, 0755); err != nil {
		return fmt.Errorf("creating local directory: %w", err)
	}

	// Download each file/directory recursively
	return b.syncDirFromSession(ctx, sandboxID, srcPath, dstPath, files)
}

// syncDirFromSession recursively downloads a directory from sandbox.
func (b *DaytonaBackend) syncDirFromSession(ctx context.Context, sandboxID, remotePath, localPath string, files []daytona.FileInfo) error {
	for _, file := range files {
		remoteFilePath := filepath.Join(remotePath, file.Name)
		localFilePath := filepath.Join(localPath, file.Name)

		if file.IsDir {
			// Create local directory
			if err := os.MkdirAll(localFilePath, 0755); err != nil {
				return fmt.Errorf("creating directory %s: %w", localFilePath, err)
			}

			// List and download subdirectory contents
			subFiles, err := b.client.ListFiles(ctx, sandboxID, remoteFilePath)
			if err != nil {
				return fmt.Errorf("listing directory %s: %w", remoteFilePath, err)
			}

			if err := b.syncDirFromSession(ctx, sandboxID, remoteFilePath, localFilePath, subFiles); err != nil {
				return err
			}
		} else {
			// Download file
			if err := b.client.DownloadFile(ctx, sandboxID, remoteFilePath, localFilePath); err != nil {
				return fmt.Errorf("downloading file %s: %w", remoteFilePath, err)
			}
		}
	}
	return nil
}

// --- Utility methods ---

// IsAvailable checks if the Daytona API is accessible.
func (b *DaytonaBackend) IsAvailable() bool {
	// Check if API key is set
	apiKey := os.Getenv(b.config.APIKeyEnv)
	if apiKey == "" {
		return false
	}

	// Try to initialize the client
	if err := b.ensureClient(); err != nil {
		return false
	}

	// Try a simple API call
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := b.client.ListSandboxes(ctx)
	return err == nil
}

// trackSession adds a session to the active sessions map.
func (b *DaytonaBackend) trackSession(session *Session) {
	b.sessionsMu.Lock()
	defer b.sessionsMu.Unlock()
	b.activeSessions[session.ID] = session
}

// untrackSession removes a session from the active sessions map.
func (b *DaytonaBackend) untrackSession(sessionID string) {
	b.sessionsMu.Lock()
	defer b.sessionsMu.Unlock()
	delete(b.activeSessions, sessionID)
}

// getTrackedSession retrieves a tracked session.
func (b *DaytonaBackend) getTrackedSession(sessionID string) *Session {
	b.sessionsMu.RLock()
	defer b.sessionsMu.RUnlock()
	return b.activeSessions[sessionID]
}

// ActiveSessionCount returns the number of active sandboxes.
func (b *DaytonaBackend) ActiveSessionCount() int {
	b.sessionsMu.RLock()
	defer b.sessionsMu.RUnlock()
	return len(b.activeSessions)
}

// setPtyHandle stores a PTY handle for a session.
func (b *DaytonaBackend) setPtyHandle(sessionID string, handle *daytona.PtyHandle) {
	b.ptyHandlesMu.Lock()
	defer b.ptyHandlesMu.Unlock()
	b.ptyHandles[sessionID] = handle
}

// getPtyHandle retrieves a PTY handle for a session.
func (b *DaytonaBackend) getPtyHandle(sessionID string) *daytona.PtyHandle {
	b.ptyHandlesMu.RLock()
	defer b.ptyHandlesMu.RUnlock()
	return b.ptyHandles[sessionID]
}

// deletePtyHandle removes a PTY handle for a session.
func (b *DaytonaBackend) deletePtyHandle(sessionID string) {
	b.ptyHandlesMu.Lock()
	defer b.ptyHandlesMu.Unlock()
	delete(b.ptyHandles, sessionID)
}

// GetClient returns the underlying Daytona API client.
// This is useful for advanced operations not exposed by the Backend interface.
func (b *DaytonaBackend) GetClient() (*daytona.Client, error) {
	if err := b.ensureClient(); err != nil {
		return nil, err
	}
	return b.client, nil
}

// Compile-time interface checks
var (
	_ Backend     = (*DaytonaBackend)(nil)
	_ SyncBackend = (*DaytonaBackend)(nil)
)
