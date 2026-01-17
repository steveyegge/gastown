// Package tmux provides a wrapper for tmux session operations via subprocess.
package tmux

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
)

// sessionNudgeLocks serializes nudges to the same session.
// This prevents interleaving when multiple nudges arrive concurrently,
// which can cause garbled input and missed Enter keys.
var sessionNudgeLocks sync.Map // map[string]*sync.Mutex

// versionPattern matches Claude Code version numbers like "2.0.76"
var versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// validSessionNameRe validates session names to prevent shell injection
var validSessionNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// Common errors
var (
	ErrNoServer        = errors.New("no tmux server running")
	ErrSessionExists   = errors.New("session already exists")
	ErrSessionNotFound = errors.New("session not found")
)

// Tmux wraps tmux operations.
type Tmux struct{}

// NewTmux creates a new Tmux wrapper.
func NewTmux() *Tmux {
	return &Tmux{}
}

// run executes a tmux command and returns stdout.
func (t *Tmux) run(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", t.wrapError(err, stderr.String(), args)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// wrapError wraps tmux errors with context.
func (t *Tmux) wrapError(err error, stderr string, args []string) error {
	stderr = strings.TrimSpace(stderr)

	// Detect specific error types
	if strings.Contains(stderr, "no server running") ||
		strings.Contains(stderr, "error connecting to") {
		return ErrNoServer
	}
	if strings.Contains(stderr, "duplicate session") {
		return ErrSessionExists
	}
	if strings.Contains(stderr, "session not found") ||
		strings.Contains(stderr, "can't find session") {
		return ErrSessionNotFound
	}

	if stderr != "" {
		return fmt.Errorf("tmux %s: %s", args[0], stderr)
	}
	return fmt.Errorf("tmux %s: %w", args[0], err)
}

// NewSession creates a new detached tmux session.
func (t *Tmux) NewSession(name, workDir string) error {
	args := []string{"new-session", "-d", "-s", name}
	if workDir != "" {
		args = append(args, "-c", workDir)
	}
	_, err := t.run(args...)
	return err
}

// NewSessionWithCommand creates a new detached tmux session that immediately runs a command.
// Unlike NewSession + SendKeys, this avoids race conditions where the shell isn't ready
// or the command arrives before the shell prompt. The command runs directly as the
// initial process of the pane.
// See: https://github.com/anthropics/gastown/issues/280
func (t *Tmux) NewSessionWithCommand(name, workDir, command string) error {
	args := []string{"new-session", "-d", "-s", name}
	if workDir != "" {
		args = append(args, "-c", workDir)
	}
	// Add the command as the last argument - tmux runs it as the pane's initial process
	args = append(args, command)
	_, err := t.run(args...)
	return err
}

// EnsureSessionFresh ensures a session is available and healthy.
// If the session exists but is a zombie (Claude not running), it kills the session first.
// This prevents "session already exists" errors when trying to restart dead agents.
//
// A session is considered a zombie if:
// - The tmux session exists
// - But Claude (node process) is not running in it
//
// Returns nil if session was created successfully.
func (t *Tmux) EnsureSessionFresh(name, workDir string) error {
	// Check if session already exists
	exists, err := t.HasSession(name)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}

	if exists {
		// Session exists - check if it's a zombie
		if !t.IsAgentRunning(name) {
			// Zombie session: tmux alive but Claude dead
			// Kill it so we can create a fresh one
			if err := t.KillSession(name); err != nil {
				return fmt.Errorf("killing zombie session: %w", err)
			}
		} else {
			// Session is healthy (Claude running) - nothing to do
			return nil
		}
	}

	// Create fresh session
	return t.NewSession(name, workDir)
}

// KillSession terminates a tmux session.
func (t *Tmux) KillSession(name string) error {
	_, err := t.run("kill-session", "-t", name)
	return err
}

// KillSessionWithProcesses explicitly kills all processes in a session before terminating it.
// This prevents orphan processes that survive tmux kill-session due to SIGHUP being ignored.
//
// Process:
// 1. Get the pane's main process PID
// 2. Find all descendant processes recursively (not just direct children)
// 3. Send SIGTERM to all descendants (deepest first)
// 4. Wait 100ms for graceful shutdown
// 5. Send SIGKILL to any remaining descendants
// 6. Kill the tmux session
//
// This ensures Claude processes and all their children are properly terminated.
func (t *Tmux) KillSessionWithProcesses(name string) error {
	// Get the pane PID
	pid, err := t.GetPanePID(name)
	if err != nil {
		// Session might not exist or be in bad state, try direct kill
		return t.KillSession(name)
	}

	if pid != "" {
		// Get all descendant PIDs recursively (returns deepest-first order)
		descendants := getAllDescendants(pid)

		// Send SIGTERM to all descendants (deepest first to avoid orphaning)
		for _, dpid := range descendants {
			_ = exec.Command("kill", "-TERM", dpid).Run()
		}

		// Wait for graceful shutdown
		time.Sleep(100 * time.Millisecond)

		// Send SIGKILL to any remaining descendants
		for _, dpid := range descendants {
			_ = exec.Command("kill", "-KILL", dpid).Run()
		}

		// Kill the pane process itself (may have called setsid() and detached)
		_ = exec.Command("kill", "-TERM", pid).Run()
		time.Sleep(100 * time.Millisecond)
		_ = exec.Command("kill", "-KILL", pid).Run()
	}

	// Kill the tmux session
	return t.KillSession(name)
}

// getAllDescendants recursively finds all descendant PIDs of a process.
// Returns PIDs in deepest-first order so killing them doesn't orphan grandchildren.
func getAllDescendants(pid string) []string {
	var result []string

	// Get direct children using pgrep
	out, err := exec.Command("pgrep", "-P", pid).Output()
	if err != nil {
		return result
	}

	children := strings.Fields(strings.TrimSpace(string(out)))
	for _, child := range children {
		// First add grandchildren (recursively) - deepest first
		result = append(result, getAllDescendants(child)...)
		// Then add this child
		result = append(result, child)
	}

	return result
}

// isDescendantOf checks if a process is a descendant of an ancestor PID.
// It does this by walking up the process tree using parent PIDs.
func isDescendantOf(pid, ancestorPID string) bool {
	currentPID := pid
	maxIterations := 50 // Prevent infinite loops

	for i := 0; i < maxIterations; i++ {
		if currentPID == ancestorPID {
			return true
		}

		// Get the parent PID
		ppid, err := getParentPID(currentPID)
		if err != nil {
			return false
		}

		// If we reached PID 1 (init), we've gone too far
		if ppid == "1" || ppid == "0" {
			return false
		}

		currentPID = ppid
	}

	return false
}

// getParentPID gets the parent PID of a process using ps.
func getParentPID(pid string) (string, error) {
	out, err := exec.Command("ps", "-o", "ppid=", "-p", pid).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// findClaudeProcessesDescendantsOf finds all Claude processes that are descendants of a PID.
func findClaudeProcessesDescendantsOf(ancestorPID string) []string {
	var result []string

	// Find all processes with "claude" or "node" in their command line
	pgrepCmd := exec.Command("pgrep", "-f", "claude")
	claudeOut, err := pgrepCmd.Output()
	if err != nil {
		// No claude processes found, try "node" as fallback
		pgrepCmd = exec.Command("pgrep", "-f", "node")
		claudeOut, err = pgrepCmd.Output()
		if err != nil {
			return result
		}
	}

	// For each Claude process, check if it's a descendant of the ancestor PID
	claudePIDs := strings.Fields(strings.TrimSpace(string(claudeOut)))
	for _, pid := range claudePIDs {
		if isDescendantOf(pid, ancestorPID) {
			result = append(result, pid)
		}
	}

	return result
}

// KillPaneProcesses kills all Claude processes in a pane without killing the pane itself.
// This is useful before respawn-pane to ensure the old process is actually terminated.
// The pane parameter should be a pane ID (e.g., "%0") or session:window.pane format.
func (t *Tmux) KillPaneProcesses(pane string) error {
	// Get the pane PID
	pid, err := t.run("list-panes", "-t", pane, "-F", "#{pane_pid}")
	if err != nil {
		return err
	}

	panePID := strings.TrimSpace(pid)
	if panePID == "" {
		return nil // No pane PID, nothing to kill
	}

	// Find all Claude/node processes that are descendants of the pane PID
	claudePIDs := findClaudeProcessesDescendantsOf(panePID)

	// Send SIGTERM to all Claude processes
	for _, pid := range claudePIDs {
		_ = exec.Command("kill", "-TERM", pid).Run()
	}

	// Wait for graceful shutdown
	time.Sleep(100 * time.Millisecond)

	// Send SIGKILL to any remaining Claude processes
	for _, pid := range claudePIDs {
		_ = exec.Command("kill", "-KILL", pid).Run()
	}

	return nil
}

// KillServer terminates the entire tmux server and all sessions.
func (t *Tmux) KillServer() error {
	_, err := t.run("kill-server")
	if errors.Is(err, ErrNoServer) {
		return nil // Already dead
	}
	return err
}

// SetExitEmpty controls the tmux exit-empty server option.
// When on (default), the server exits when there are no sessions.
// When off, the server stays running even with no sessions.
// This is useful during shutdown to prevent the server from exiting
// when all Gas Town sessions are killed but the user has no other sessions.
func (t *Tmux) SetExitEmpty(on bool) error {
	value := "on"
	if !on {
		value = "off"
	}
	_, err := t.run("set-option", "-g", "exit-empty", value)
	if errors.Is(err, ErrNoServer) {
		return nil // No server to configure
	}
	return err
}

// IsAvailable checks if tmux is installed and can be invoked.
func (t *Tmux) IsAvailable() bool {
	cmd := exec.Command("tmux", "-V")
	return cmd.Run() == nil
}

// HasSession checks if a session exists (exact match).
// Uses "=" prefix for exact matching, preventing prefix matches
// (e.g., "gt-deacon-boot" won't match when checking for "gt-deacon").
func (t *Tmux) HasSession(name string) (bool, error) {
	_, err := t.run("has-session", "-t", "="+name)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) || errors.Is(err, ErrNoServer) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ListSessions returns all session names.
func (t *Tmux) ListSessions() ([]string, error) {
	out, err := t.run("list-sessions", "-F", "#{session_name}")
	if err != nil {
		if errors.Is(err, ErrNoServer) {
			return nil, nil // No server = no sessions
		}
		return nil, err
	}

	if out == "" {
		return nil, nil
	}

	return strings.Split(out, "\n"), nil
}

// SessionSet provides O(1) session existence checks by caching session names.
// Use this when you need to check multiple sessions to avoid N+1 subprocess calls.
type SessionSet struct {
	sessions map[string]struct{}
}

// GetSessionSet returns a SessionSet containing all current sessions.
// Call this once at the start of an operation, then use Has() for O(1) checks.
// This replaces multiple HasSession() calls with a single ListSessions() call.
//
// Builds the map directly from tmux output to avoid intermediate slice allocation.
func (t *Tmux) GetSessionSet() (*SessionSet, error) {
	out, err := t.run("list-sessions", "-F", "#{session_name}")
	if err != nil {
		if errors.Is(err, ErrNoServer) {
			return &SessionSet{sessions: make(map[string]struct{})}, nil
		}
		return nil, err
	}

	// Count newlines to pre-size map (avoids rehashing during insertion)
	count := strings.Count(out, "\n") + 1
	set := &SessionSet{
		sessions: make(map[string]struct{}, count),
	}

	// Parse directly without intermediate slice allocation
	for len(out) > 0 {
		idx := strings.IndexByte(out, '\n')
		var line string
		if idx >= 0 {
			line = out[:idx]
			out = out[idx+1:]
		} else {
			line = out
			out = ""
		}
		if line != "" {
			set.sessions[line] = struct{}{}
		}
	}
	return set, nil
}

// Has returns true if the session exists in the set.
// This is an O(1) lookup - no subprocess is spawned.
func (s *SessionSet) Has(name string) bool {
	if s == nil {
		return false
	}
	_, ok := s.sessions[name]
	return ok
}

// Names returns all session names in the set.
func (s *SessionSet) Names() []string {
	if s == nil || len(s.sessions) == 0 {
		return nil
	}
	names := make([]string, 0, len(s.sessions))
	for name := range s.sessions {
		names = append(names, name)
	}
	return names
}

// ListSessionIDs returns a map of session name to session ID.
// Session IDs are in the format "$N" where N is a number.
func (t *Tmux) ListSessionIDs() (map[string]string, error) {
	out, err := t.run("list-sessions", "-F", "#{session_name}:#{session_id}")
	if err != nil {
		if errors.Is(err, ErrNoServer) {
			return nil, nil // No server = no sessions
		}
		return nil, err
	}

	if out == "" {
		return nil, nil
	}

	result := make(map[string]string)
	skipped := 0
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		// Parse "name:$id" format
		idx := strings.Index(line, ":")
		if idx > 0 && idx < len(line)-1 {
			name := line[:idx]
			id := line[idx+1:]
			result[name] = id
		} else {
			skipped++
		}
	}
	// Note: skipped lines are silently ignored for backward compatibility
	_ = skipped
	return result, nil
}

// SendKeys sends keystrokes to a session and presses Enter.
// Always sends Enter as a separate command for reliability.
// Uses a debounce delay between paste and Enter to ensure paste completes.
func (t *Tmux) SendKeys(session, keys string) error {
	return t.SendKeysDebounced(session, keys, constants.DefaultDebounceMs) // 100ms default debounce
}

// SendKeysDebounced sends keystrokes with a configurable delay before Enter.
// The debounceMs parameter controls how long to wait after paste before sending Enter.
// This prevents race conditions where Enter arrives before paste is processed.
func (t *Tmux) SendKeysDebounced(session, keys string, debounceMs int) error {
	// Send text using literal mode (-l) to handle special chars
	if _, err := t.run("send-keys", "-t", session, "-l", keys); err != nil {
		return err
	}
	// Wait for paste to be processed
	if debounceMs > 0 {
		time.Sleep(time.Duration(debounceMs) * time.Millisecond)
	}
	// Send Enter separately - more reliable than appending to send-keys
	_, err := t.run("send-keys", "-t", session, "Enter")
	return err
}

// SendKeysRaw sends keystrokes without adding Enter.
func (t *Tmux) SendKeysRaw(session, keys string) error {
	_, err := t.run("send-keys", "-t", session, keys)
	return err
}

// SendKeysReplace sends keystrokes, clearing any pending input first.
// This is useful for "replaceable" notifications where only the latest matters.
// Uses Ctrl-U to clear the input line before sending the new message.
// The delay parameter controls how long to wait after clearing before sending (ms).
func (t *Tmux) SendKeysReplace(session, keys string, clearDelayMs int) error {
	// Send Ctrl-U to clear any pending input on the line
	if _, err := t.run("send-keys", "-t", session, "C-u"); err != nil {
		return err
	}

	// Small delay to let the clear take effect
	if clearDelayMs > 0 {
		time.Sleep(time.Duration(clearDelayMs) * time.Millisecond)
	}

	// Now send the actual message
	return t.SendKeys(session, keys)
}

// SendKeysDelayed sends keystrokes after a delay (in milliseconds).
// Useful for waiting for a process to be ready before sending input.
func (t *Tmux) SendKeysDelayed(session, keys string, delayMs int) error {
	time.Sleep(time.Duration(delayMs) * time.Millisecond)
	return t.SendKeys(session, keys)
}

// SendKeysDelayedDebounced sends keystrokes after a pre-delay, with a custom debounce before Enter.
// Use this when sending input to a process that needs time to initialize AND the message
// needs extra time between paste and Enter (e.g., Claude prompt injection).
// preDelayMs: time to wait before sending text (for process readiness)
// debounceMs: time to wait between text paste and Enter key (for paste completion)
func (t *Tmux) SendKeysDelayedDebounced(session, keys string, preDelayMs, debounceMs int) error {
	if preDelayMs > 0 {
		time.Sleep(time.Duration(preDelayMs) * time.Millisecond)
	}
	return t.SendKeysDebounced(session, keys, debounceMs)
}

// getSessionNudgeLock returns the mutex for serializing nudges to a session.
// Creates a new mutex if one doesn't exist for this session.
func getSessionNudgeLock(session string) *sync.Mutex {
	actual, _ := sessionNudgeLocks.LoadOrStore(session, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

// NudgeSession sends a message to a Claude Code session reliably.
// This is the canonical way to send messages to Claude sessions.
// Uses: literal mode + 500ms debounce + ESC (for vim mode) + separate Enter.
// Verification is the Witness's job (AI), not this function.
//
// IMPORTANT: Nudges to the same session are serialized to prevent interleaving.
// If multiple goroutines try to nudge the same session concurrently, they will
// queue up and execute one at a time. This prevents garbled input when
// SessionStart hooks and nudges arrive simultaneously.
func (t *Tmux) NudgeSession(session, message string) error {
	// Serialize nudges to this session to prevent interleaving
	lock := getSessionNudgeLock(session)
	lock.Lock()
	defer lock.Unlock()

	// 1. Send text in literal mode (handles special characters)
	if _, err := t.run("send-keys", "-t", session, "-l", message); err != nil {
		return err
	}

	// 2. Wait 500ms for paste to complete (tested, required)
	time.Sleep(500 * time.Millisecond)

	// 3. Send Escape to exit vim INSERT mode if enabled (harmless in normal mode)
	// See: https://github.com/anthropics/gastown/issues/307
	_, _ = t.run("send-keys", "-t", session, "Escape")
	time.Sleep(100 * time.Millisecond)

	// 4. Send Enter with retry (critical for message submission)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(200 * time.Millisecond)
		}
		if _, err := t.run("send-keys", "-t", session, "Enter"); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("failed to send Enter after 3 attempts: %w", lastErr)
}

// NudgePane sends a message to a specific pane reliably.
// Same pattern as NudgeSession but targets a pane ID (e.g., "%9") instead of session name.
// Nudges to the same pane are serialized to prevent interleaving.
func (t *Tmux) NudgePane(pane, message string) error {
	// Serialize nudges to this pane to prevent interleaving
	lock := getSessionNudgeLock(pane)
	lock.Lock()
	defer lock.Unlock()

	// 1. Send text in literal mode (handles special characters)
	if _, err := t.run("send-keys", "-t", pane, "-l", message); err != nil {
		return err
	}

	// 2. Wait 500ms for paste to complete (tested, required)
	time.Sleep(500 * time.Millisecond)

	// 3. Send Escape to exit vim INSERT mode if enabled (harmless in normal mode)
	// See: https://github.com/anthropics/gastown/issues/307
	_, _ = t.run("send-keys", "-t", pane, "Escape")
	time.Sleep(100 * time.Millisecond)

	// 4. Send Enter with retry (critical for message submission)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(200 * time.Millisecond)
		}
		if _, err := t.run("send-keys", "-t", pane, "Enter"); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("failed to send Enter after 3 attempts: %w", lastErr)
}

// AcceptBypassPermissionsWarning dismisses the Claude Code bypass permissions warning dialog.
// When Claude starts with --dangerously-skip-permissions, it shows a warning dialog that
// requires pressing Down arrow to select "Yes, I accept" and then Enter to confirm.
// This function checks if the warning is present before sending keys to avoid interfering
// with sessions that don't show the warning (e.g., already accepted or different config).
//
// Call this after starting Claude and waiting for it to initialize (WaitForCommand),
// but before sending any prompts.
func (t *Tmux) AcceptBypassPermissionsWarning(session string) error {
	// Wait for the dialog to potentially render
	time.Sleep(1 * time.Second)

	// Check if the bypass permissions warning is present
	content, err := t.CapturePane(session, 30)
	if err != nil {
		return err
	}

	// Look for the characteristic warning text
	if !strings.Contains(content, "Bypass Permissions mode") {
		// Warning not present, nothing to do
		return nil
	}

	// Press Down to select "Yes, I accept" (option 2)
	if _, err := t.run("send-keys", "-t", session, "Down"); err != nil {
		return err
	}

	// Small delay to let selection update
	time.Sleep(200 * time.Millisecond)

	// Press Enter to confirm
	if _, err := t.run("send-keys", "-t", session, "Enter"); err != nil {
		return err
	}

	return nil
}

// GetPaneCommand returns the current command running in a pane.
// Returns "bash", "zsh", "claude", "node", etc.
func (t *Tmux) GetPaneCommand(session string) (string, error) {
	out, err := t.run("list-panes", "-t", session, "-F", "#{pane_current_command}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// GetPaneID returns the pane identifier for a session's first pane.
// Returns a pane ID like "%0" that can be used with RespawnPane.
func (t *Tmux) GetPaneID(session string) (string, error) {
	out, err := t.run("list-panes", "-t", session, "-F", "#{pane_id}")
	if err != nil {
		return "", err
	}
	lines := strings.Split(out, "\n")
	if len(lines) == 0 || lines[0] == "" {
		return "", fmt.Errorf("no panes found in session %s", session)
	}
	return lines[0], nil
}

// GetPaneWorkDir returns the current working directory of a pane.
func (t *Tmux) GetPaneWorkDir(session string) (string, error) {
	out, err := t.run("list-panes", "-t", session, "-F", "#{pane_current_path}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// GetPanePID returns the PID of the pane's main process.
func (t *Tmux) GetPanePID(session string) (string, error) {
	out, err := t.run("list-panes", "-t", session, "-F", "#{pane_pid}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// hasClaudeChild checks if a process has a child running claude/node.
// Used when the pane command is a shell (bash, zsh) that launched claude.
func hasClaudeChild(pid string) bool {
	// Use pgrep to find child processes
	cmd := exec.Command("pgrep", "-P", pid, "-l")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	// Check if any child is node or claude
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "PID name" e.g., "29677 node"
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			name := parts[1]
			if name == "node" || name == "claude" {
				return true
			}
		}
	}
	return false
}

// FindSessionByWorkDir finds tmux sessions where the pane's current working directory
// matches or is under the target directory. Returns session names that match.
// If processNames is provided, only returns sessions that match those processes.
// If processNames is nil or empty, returns all sessions matching the directory.
func (t *Tmux) FindSessionByWorkDir(targetDir string, processNames []string) ([]string, error) {
	sessions, err := t.ListSessions()
	if err != nil {
		return nil, err
	}

	var matches []string
	for _, session := range sessions {
		if session == "" {
			continue
		}

		workDir, err := t.GetPaneWorkDir(session)
		if err != nil {
			continue // Skip sessions we can't query
		}

		// Check if workdir matches target (exact match or subdir)
		if workDir == targetDir || strings.HasPrefix(workDir, targetDir+"/") {
			if len(processNames) > 0 {
				if t.IsRuntimeRunning(session, processNames) {
					matches = append(matches, session)
				}
				continue
			}
			matches = append(matches, session)
		}
	}

	return matches, nil
}

// CapturePane captures the visible content of a pane.
func (t *Tmux) CapturePane(session string, lines int) (string, error) {
	return t.run("capture-pane", "-p", "-t", session, "-S", fmt.Sprintf("-%d", lines))
}

// CapturePaneAll captures all scrollback history.
func (t *Tmux) CapturePaneAll(session string) (string, error) {
	return t.run("capture-pane", "-p", "-t", session, "-S", "-")
}

// CapturePaneLines captures the last N lines of a pane as a slice.
func (t *Tmux) CapturePaneLines(session string, lines int) ([]string, error) {
	out, err := t.CapturePane(session, lines)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// AttachSession attaches to an existing session.
// Note: This replaces the current process with tmux attach.
func (t *Tmux) AttachSession(session string) error {
	_, err := t.run("attach-session", "-t", session)
	return err
}

// SelectWindow selects a window by index.
func (t *Tmux) SelectWindow(session string, index int) error {
	_, err := t.run("select-window", "-t", fmt.Sprintf("%s:%d", session, index))
	return err
}

// SetEnvironment sets an environment variable in the session.
func (t *Tmux) SetEnvironment(session, key, value string) error {
	_, err := t.run("set-environment", "-t", session, key, value)
	return err
}

// GetEnvironment gets an environment variable from the session.
func (t *Tmux) GetEnvironment(session, key string) (string, error) {
	out, err := t.run("show-environment", "-t", session, key)
	if err != nil {
		return "", err
	}
	// Output format: KEY=value
	parts := strings.SplitN(out, "=", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("unexpected environment format for %s: %q", key, out)
	}
	return parts[1], nil
}

// GetAllEnvironment returns all environment variables for a session.
func (t *Tmux) GetAllEnvironment(session string) (map[string]string, error) {
	out, err := t.run("show-environment", "-t", session)
	if err != nil {
		return nil, err
	}

	env := make(map[string]string)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "-") {
			// Skip empty lines and unset markers (lines starting with -)
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}
	return env, nil
}

// RenameSession renames a session.
func (t *Tmux) RenameSession(oldName, newName string) error {
	_, err := t.run("rename-session", "-t", oldName, newName)
	return err
}

// SessionInfo contains information about a tmux session.
type SessionInfo struct {
	Name         string
	Windows      int
	Created      string
	Attached     bool
	Activity     string // Last activity time
	LastAttached string // Last time the session was attached
}

// DisplayMessage shows a message in the tmux status line.
// This is non-disruptive - it doesn't interrupt the session's input.
// Duration is specified in milliseconds.
func (t *Tmux) DisplayMessage(session, message string, durationMs int) error {
	// Set display time temporarily, show message, then restore
	// Use -d flag for duration in tmux 2.9+
	_, err := t.run("display-message", "-t", session, "-d", fmt.Sprintf("%d", durationMs), message)
	return err
}

// DisplayMessageDefault shows a message with default duration (5 seconds).
func (t *Tmux) DisplayMessageDefault(session, message string) error {
	return t.DisplayMessage(session, message, constants.DefaultDisplayMs)
}

// SendNotificationBanner sends a visible notification banner to a tmux session.
// This interrupts the terminal to ensure the notification is seen.
// Uses echo to print a boxed banner with the notification details.
func (t *Tmux) SendNotificationBanner(session, from, subject string) error {
	// Sanitize inputs to prevent output manipulation
	from = strings.ReplaceAll(from, "\n", " ")
	from = strings.ReplaceAll(from, "\r", " ")
	subject = strings.ReplaceAll(subject, "\n", " ")
	subject = strings.ReplaceAll(subject, "\r", " ")

	// Build the banner text
	banner := fmt.Sprintf(`echo '
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📬 NEW MAIL from %s
Subject: %s
Run: gt mail inbox
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
'`, from, subject)

	return t.SendKeys(session, banner)
}

// IsAgentRunning checks if an agent appears to be running in the session.
//
// If expectedPaneCommands is non-empty, the pane's current command must match one of them.
// If expectedPaneCommands is empty, any non-shell command counts as "agent running".
func (t *Tmux) IsAgentRunning(session string, expectedPaneCommands ...string) bool {
	cmd, err := t.GetPaneCommand(session)
	if err != nil {
		return false
	}

	if len(expectedPaneCommands) > 0 {
		for _, expected := range expectedPaneCommands {
			if expected != "" && cmd == expected {
				return true
			}
		}
		return false
	}

	// Fallback: any non-shell command counts as running.
	for _, shell := range constants.SupportedShells {
		if cmd == shell {
			return false
		}
	}
	return cmd != ""
}

// IsClaudeRunning checks if Claude appears to be running in the session.
// Only trusts the pane command - UI markers in scrollback cause false positives.
// Claude can report as "node", "claude", or a version number like "2.0.76".
// Also checks for child processes when the pane is a shell running claude via "bash -c".
func (t *Tmux) IsClaudeRunning(session string) bool {
	// Check for known command names first
	if t.IsAgentRunning(session, "node", "claude") {
		return true
	}
	// Check for version pattern (e.g., "2.0.76") - Claude Code shows version as pane command
	cmd, err := t.GetPaneCommand(session)
	if err != nil {
		return false
	}
	if versionPattern.MatchString(cmd) {
		return true
	}
	// If pane command is a shell, check for claude/node child processes.
	// This handles the case where sessions are started with "bash -c 'export ... && claude ...'"
	for _, shell := range constants.SupportedShells {
		if cmd == shell {
			pid, err := t.GetPanePID(session)
			if err == nil && pid != "" {
				return hasClaudeChild(pid)
			}
			break
		}
	}
	return false
}

// IsRuntimeRunning checks if a runtime appears to be running in the session.
// Only trusts the pane command - UI markers in scrollback cause false positives.
// This is the runtime-config-aware version of IsAgentRunning.
func (t *Tmux) IsRuntimeRunning(session string, processNames []string) bool {
	if len(processNames) == 0 {
		return false
	}
	cmd, err := t.GetPaneCommand(session)
	if err != nil {
		return false
	}
	for _, name := range processNames {
		if cmd == name {
			return true
		}
	}
	return false
}

// WaitForCommand polls until the pane is NOT running one of the excluded commands.
// Useful for waiting until a shell has started a new process (e.g., claude).
// Returns nil when a non-excluded command is detected, or error on timeout.
func (t *Tmux) WaitForCommand(session string, excludeCommands []string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cmd, err := t.GetPaneCommand(session)
		if err != nil {
			time.Sleep(constants.PollInterval)
			continue
		}
		// Check if current command is NOT in the exclude list
		excluded := false
		for _, exc := range excludeCommands {
			if cmd == exc {
				excluded = true
				break
			}
		}
		if !excluded {
			return nil
		}
		time.Sleep(constants.PollInterval)
	}
	return fmt.Errorf("timeout waiting for command (still running excluded command)")
}

// WaitForShellReady polls until the pane is running a shell command.
// Useful for waiting until a process has exited and returned to shell.
func (t *Tmux) WaitForShellReady(session string, timeout time.Duration) error {
	shells := constants.SupportedShells
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cmd, err := t.GetPaneCommand(session)
		if err != nil {
			time.Sleep(constants.PollInterval)
			continue
		}
		for _, shell := range shells {
			if cmd == shell {
				return nil
			}
		}
		time.Sleep(constants.PollInterval)
	}
	return fmt.Errorf("timeout waiting for shell")
}

// WaitForRuntimeReady polls until the runtime's prompt indicator appears in the pane.
// Runtime is ready when we see the configured prompt prefix at the start of a line.
//
// IMPORTANT: Bootstrap vs Steady-State Observation
//
// This function uses regex to detect runtime prompts - a ZFC violation.
// ZFC (Zero False Commands) principle: AI should observe AI, not regex.
//
// Bootstrap (acceptable):
//
//	During cold startup when no AI agent is running, the daemon uses this
//	function to get the Deacon online. Regex is acceptable here.
//
// Steady-State (use AI observation instead):
//
//	Once any AI agent is running, observation should be AI-to-AI:
//	- Deacon starting polecats → use 'gt deacon pending' + AI analysis
//	- Deacon restarting → Mayor watches via 'gt peek'
//	- Mayor restarting → Deacon watches via 'gt peek'
//
// See: gt deacon pending (ZFC-compliant AI observation)
// See: gt deacon trigger-pending (bootstrap mode, regex-based)
func (t *Tmux) WaitForRuntimeReady(session string, rc *config.RuntimeConfig, timeout time.Duration) error {
	if rc == nil || rc.Tmux == nil {
		return nil
	}

	if rc.Tmux.ReadyPromptPrefix == "" {
		if rc.Tmux.ReadyDelayMs <= 0 {
			return nil
		}
		// Fallback to fixed delay when prompt detection is unavailable.
		delay := time.Duration(rc.Tmux.ReadyDelayMs) * time.Millisecond
		if delay > timeout {
			delay = timeout
		}
		time.Sleep(delay)
		return nil
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		// Capture last few lines of the pane
		lines, err := t.CapturePaneLines(session, 10)
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		// Look for runtime prompt indicator at start of line
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			prefix := strings.TrimSpace(rc.Tmux.ReadyPromptPrefix)
			if strings.HasPrefix(trimmed, rc.Tmux.ReadyPromptPrefix) || (prefix != "" && trimmed == prefix) {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for runtime prompt")
}

// GetSessionInfo returns detailed information about a session.
func (t *Tmux) GetSessionInfo(name string) (*SessionInfo, error) {
	format := "#{session_name}|#{session_windows}|#{session_created_string}|#{session_attached}|#{session_activity}|#{session_last_attached}"
	out, err := t.run("list-sessions", "-F", format, "-f", fmt.Sprintf("#{==:#{session_name},%s}", name))
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, ErrSessionNotFound
	}

	parts := strings.Split(out, "|")
	if len(parts) < 4 {
		return nil, fmt.Errorf("unexpected session info format: %s", out)
	}

	windows := 0
	_, _ = fmt.Sscanf(parts[1], "%d", &windows) // non-fatal: defaults to 0 on parse error

	info := &SessionInfo{
		Name:     parts[0],
		Windows:  windows,
		Created:  parts[2],
		Attached: parts[3] == "1",
	}

	// Activity and last attached are optional (may not be present in older tmux)
	if len(parts) > 4 {
		info.Activity = parts[4]
	}
	if len(parts) > 5 {
		info.LastAttached = parts[5]
	}

	return info, nil
}

// ApplyTheme sets the status bar style for a session.
func (t *Tmux) ApplyTheme(session string, theme Theme) error {
	_, err := t.run("set-option", "-t", session, "status-style", theme.Style())
	return err
}

// roleIcons maps role names to display icons for the status bar.
// Uses centralized emojis from constants package.
// Includes legacy keys ("coordinator", "health-check") for backwards compatibility.
var roleIcons = map[string]string{
	// Standard role names (from constants)
	constants.RoleMayor:    constants.EmojiMayor,
	constants.RoleDeacon:   constants.EmojiDeacon,
	constants.RoleWitness:  constants.EmojiWitness,
	constants.RoleRefinery: constants.EmojiRefinery,
	constants.RoleCrew:     constants.EmojiCrew,
	constants.RolePolecat:  constants.EmojiPolecat,
	// Legacy names (for backwards compatibility)
	"coordinator":  constants.EmojiMayor,
	"health-check": constants.EmojiDeacon,
}

// SetStatusFormat configures the left side of the status bar.
// Shows compact identity: icon + minimal context
func (t *Tmux) SetStatusFormat(session, rig, worker, role string) error {
	// Get icon for role (empty string if not found)
	icon := roleIcons[role]

	// Compact format - icon already identifies role
	// Mayor: 🎩 Mayor
	// Crew:  👷 gastown/crew/max (full path)
	// Polecat: 😺 gastown/Toast
	var left string
	if rig == "" {
		// Town-level agent (Mayor, Deacon)
		left = fmt.Sprintf("%s %s ", icon, worker)
	} else if role == "crew" {
		// Crew member - show full path: rig/crew/name
		left = fmt.Sprintf("%s %s/crew/%s ", icon, rig, worker)
	} else {
		// Rig-level agent - show rig/worker
		left = fmt.Sprintf("%s %s/%s ", icon, rig, worker)
	}

	if _, err := t.run("set-option", "-t", session, "status-left-length", "25"); err != nil {
		return err
	}
	_, err := t.run("set-option", "-t", session, "status-left", left)
	return err
}

// SetDynamicStatus configures the right side with dynamic content.
// Uses a shell command that tmux calls periodically to get current status.
func (t *Tmux) SetDynamicStatus(session string) error {
	// Validate session name to prevent shell injection
	if !validSessionNameRe.MatchString(session) {
		return fmt.Errorf("invalid session name %q: must match %s", session, validSessionNameRe.String())
	}

	// tmux calls this command every status-interval seconds
	// gt status-line reads env vars and mail to build the status
	right := fmt.Sprintf(`#(gt status-line --session=%s 2>/dev/null) %%H:%%M`, session)

	if _, err := t.run("set-option", "-t", session, "status-right-length", "80"); err != nil {
		return err
	}
	// Set faster refresh for more responsive status
	if _, err := t.run("set-option", "-t", session, "status-interval", "5"); err != nil {
		return err
	}
	_, err := t.run("set-option", "-t", session, "status-right", right)
	return err
}

// ConfigureGasTownSession applies full Gas Town theming to a session.
// This is a convenience method that applies theme, status format, and dynamic status.
func (t *Tmux) ConfigureGasTownSession(session string, theme Theme, rig, worker, role string) error {
	if err := t.ApplyTheme(session, theme); err != nil {
		return fmt.Errorf("applying theme: %w", err)
	}
	if err := t.SetStatusFormat(session, rig, worker, role); err != nil {
		return fmt.Errorf("setting status format: %w", err)
	}
	if err := t.SetDynamicStatus(session); err != nil {
		return fmt.Errorf("setting dynamic status: %w", err)
	}
	if err := t.SetMailClickBinding(session); err != nil {
		return fmt.Errorf("setting mail click binding: %w", err)
	}
	if err := t.SetFeedBinding(session); err != nil {
		return fmt.Errorf("setting feed binding: %w", err)
	}
	if err := t.SetCycleBindings(session); err != nil {
		return fmt.Errorf("setting cycle bindings: %w", err)
	}
	if err := t.EnableMouseMode(session); err != nil {
		return fmt.Errorf("enabling mouse mode: %w", err)
	}
	return nil
}

// EnableMouseMode enables mouse support for a tmux session.
// This allows clicking to select panes/windows, scrolling with mouse wheel,
// and dragging to resize panes. Hold Shift for native terminal text selection.
func (t *Tmux) EnableMouseMode(session string) error {
	_, err := t.run("set-option", "-t", session, "mouse", "on")
	return err
}

// IsInsideTmux checks if the current process is running inside a tmux session.
// This is detected by the presence of the TMUX environment variable.
func IsInsideTmux() bool {
	return os.Getenv("TMUX") != ""
}

// SetMailClickBinding configures left-click on status-right to show mail preview.
// This creates a popup showing the first unread message when clicking the mail icon area.
func (t *Tmux) SetMailClickBinding(session string) error {
	// Bind left-click on status-right to show mail popup
	// The popup runs gt mail peek and closes on any key
	_, err := t.run("bind-key", "-T", "root", "MouseDown1StatusRight",
		"display-popup", "-E", "-w", "60", "-h", "15", "gt mail peek || echo 'No unread mail'")
	return err
}

// RespawnPane kills all processes in a pane and starts a new command.
// This is used for "hot reload" of agent sessions - instantly restart in place.
// The pane parameter should be a pane ID (e.g., "%0") or session:window.pane format.
func (t *Tmux) RespawnPane(pane, command string) error {
	_, err := t.run("respawn-pane", "-k", "-t", pane, command)
	return err
}

// ClearHistory clears the scrollback history buffer for a pane.
// This resets copy-mode display from [0/N] to [0/0].
// The pane parameter should be a pane ID (e.g., "%0") or session:window.pane format.
func (t *Tmux) ClearHistory(pane string) error {
	_, err := t.run("clear-history", "-t", pane)
	return err
}

// SwitchClient switches the current tmux client to a different session.
// Used after remote recycle to move the user's view to the recycled session.
func (t *Tmux) SwitchClient(targetSession string) error {
	_, err := t.run("switch-client", "-t", targetSession)
	return err
}

// SetCrewCycleBindings sets up C-b n/p to cycle through sessions.
// This is now an alias for SetCycleBindings - the unified command detects
// session type automatically.
//
// IMPORTANT: We pass #{session_name} to the command because run-shell doesn't
// reliably preserve the session context. tmux expands #{session_name} at binding
// resolution time (when the key is pressed), giving us the correct session.
func (t *Tmux) SetCrewCycleBindings(session string) error {
	return t.SetCycleBindings(session)
}

// SetTownCycleBindings sets up C-b n/p to cycle through sessions.
// This is now an alias for SetCycleBindings - the unified command detects
// session type automatically.
func (t *Tmux) SetTownCycleBindings(session string) error {
	return t.SetCycleBindings(session)
}

// SetCycleBindings sets up C-b n/p to cycle through related sessions.
// The gt cycle command automatically detects the session type and cycles
// within the appropriate group:
// - Town sessions: Mayor ↔ Deacon
// - Crew sessions: All crew members in the same rig
//
// IMPORTANT: These bindings are conditional - they only run gt cycle for
// Gas Town sessions (those starting with "gt-" or "hq-"). For non-GT sessions,
// the default tmux behavior (next-window/previous-window) is preserved.
// See: https://github.com/steveyegge/gastown/issues/13
//
// IMPORTANT: We pass #{session_name} to the command because run-shell doesn't
// reliably preserve the session context. tmux expands #{session_name} at binding
// resolution time (when the key is pressed), giving us the correct session.
func (t *Tmux) SetCycleBindings(session string) error {
	// C-b n → gt cycle next for GT sessions, next-window otherwise
	// The if-shell checks if session name starts with "gt-" or "hq-"
	if _, err := t.run("bind-key", "-T", "prefix", "n",
		"if-shell", "echo '#{session_name}' | grep -Eq '^(gt|hq)-'",
		"run-shell 'gt cycle next --session #{session_name}'",
		"next-window"); err != nil {
		return err
	}
	// C-b p → gt cycle prev for GT sessions, previous-window otherwise
	if _, err := t.run("bind-key", "-T", "prefix", "p",
		"if-shell", "echo '#{session_name}' | grep -Eq '^(gt|hq)-'",
		"run-shell 'gt cycle prev --session #{session_name}'",
		"previous-window"); err != nil {
		return err
	}
	return nil
}

// SetFeedBinding configures C-b a to jump to the activity feed window.
// This creates the feed window if it doesn't exist, or switches to it if it does.
// Uses `gt feed --window` which handles both creation and switching.
//
// IMPORTANT: This binding is conditional - it only runs for Gas Town sessions
// (those starting with "gt-" or "hq-"). For non-GT sessions, a help message is shown.
// See: https://github.com/steveyegge/gastown/issues/13
func (t *Tmux) SetFeedBinding(session string) error {
	// C-b a → gt feed --window for GT sessions, help message otherwise
	_, err := t.run("bind-key", "-T", "prefix", "a",
		"if-shell", "echo '#{session_name}' | grep -Eq '^(gt|hq)-'",
		"run-shell 'gt feed --window'",
		"display-message 'C-b a is for Gas Town sessions only'")
	return err
}

// SetPaneDiedHook sets a pane-died hook on a session to detect crashes.
// When the pane exits, tmux runs the hook command with exit status info.
// The agentID is used to identify the agent in crash logs (e.g., "gastown/Toast").
func (t *Tmux) SetPaneDiedHook(session, agentID string) error {
	// Sanitize inputs to prevent shell injection
	session = strings.ReplaceAll(session, "'", "'\\''")
	agentID = strings.ReplaceAll(agentID, "'", "'\\''")

	// Hook command logs the crash with exit status
	// #{pane_dead_status} is the exit code of the process that died
	// We run gt log crash which records to the town log
	hookCmd := fmt.Sprintf(`run-shell "gt log crash --agent '%s' --session '%s' --exit-code #{pane_dead_status}"`,
		agentID, session)

	// Set the hook on this specific session
	_, err := t.run("set-hook", "-t", session, "pane-died", hookCmd)
	return err
}

// CleanupOrphanedSessions kills any Gas Town sessions that have zombie agents.
// This prevents session accumulation when gt is restarted without proper shutdown.
//
// A session is cleaned up if:
// - Its name starts with "gt-" or "hq-" (Gas Town naming convention)
// - The agent (Claude) is not running in it (zombie session)
//
// Returns the number of sessions cleaned up and any errors encountered.
func (t *Tmux) CleanupOrphanedSessions() (int, error) {
	sessions, err := t.ListSessions()
	if err != nil {
		return 0, err
	}

	cleaned := 0
	for _, session := range sessions {
		if session == "" {
			continue
		}

		// Only clean up Gas Town sessions (both gt-* and hq-* prefixes)
		if !strings.HasPrefix(session, "gt-") && !strings.HasPrefix(session, "hq-") {
			continue
		}

		// Check if Claude is running - if so, leave it alone
		if t.IsClaudeRunning(session) {
			continue
		}

		// Zombie session: tmux alive but Claude dead
		// Kill it with process cleanup to prevent orphan processes
		if err := t.KillSessionWithProcesses(session); err != nil {
			// Log but continue - don't fail the whole cleanup
			continue
		}
		cleaned++
	}

	return cleaned, nil
}
