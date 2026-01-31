// Package tmux provides a wrapper for tmux session operations via subprocess.
package tmux

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
)

// DefaultTimeout is the default timeout for tmux commands.
// Most tmux operations should complete quickly; hangs indicate issues.
const DefaultTimeout = 10 * time.Second

// sessionNudgeLocks serializes nudges to the same session.
// This prevents interleaving when multiple nudges arrive concurrently,
// which can cause garbled input and missed Enter keys.
var sessionNudgeLocks sync.Map // map[string]*sync.Mutex

// validSessionNameRe validates session names to prevent shell injection
var validSessionNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// versionPattern matches Claude Code version strings as process names (e.g., "1.0.16")
var versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

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
// Uses a default timeout to prevent hangs from blocking operations.
func (t *Tmux) run(args ...string) (string, error) {
	return t.runWithTimeout(DefaultTimeout, args...)
}

// runWithTimeout executes a tmux command with a custom timeout.
func (t *Tmux) runWithTimeout(timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "tmux", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Check if the error was due to context timeout
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("tmux command timed out after %v: %v", timeout, args)
		}
		return "", t.wrapError(err, stderr.String(), args)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// wrapError wraps tmux errors with context.
func (t *Tmux) wrapError(err error, stderr string, args []string) error {
	stderr = strings.TrimSpace(stderr)

	// Detect specific error types
	if strings.Contains(stderr, "no server running") ||
		strings.Contains(stderr, "error connecting to") ||
		strings.Contains(stderr, "no current target") {
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
//
// This function ensures remain-on-exit is set BEFORE the actual command runs.
// This prevents a race condition where fast-exiting commands (auth failures, missing
// binaries) could cause the session to be destroyed before remain-on-exit is set,
// losing diagnostic output. See: gt-958e7d
func (t *Tmux) NewSessionWithCommand(name, workDir, command string) error {
	// Strategy: Create session with a brief sleep, set remain-on-exit, then respawn
	// with the actual command. This avoids quoting issues that arise from trying to
	// wrap arbitrary commands in shell strings.
	//
	// 1. Create session with a dummy command (sleep) to hold the session open
	// 2. Set remain-on-exit on the window
	// 3. Respawn the pane with the actual command
	//
	// The sleep ensures the session exists long enough for us to set remain-on-exit.
	// The respawn-pane then runs the actual command with remain-on-exit already set.
	args := []string{"new-session", "-d", "-s", name}
	if workDir != "" {
		args = append(args, "-c", workDir)
	}
	// Start with a sleep command to hold the session open briefly
	args = append(args, "sleep 60")
	_, err := t.run(args...)
	if err != nil {
		return err
	}

	// Set remain-on-exit BEFORE running the actual command
	// This ensures the pane is preserved even if the command exits immediately
	_, _ = t.run("set-window-option", "-t", name, "remain-on-exit", "on")

	// Now respawn the pane with the actual command
	// respawn-pane -k kills the current process (sleep) and starts the new command
	// The -t flag targets the session's default pane
	_, err = t.run("respawn-pane", "-k", "-t", name, command)
	return err
}

// IsPaneDead checks if the pane's command has exited (pane_dead=1).
// With remain-on-exit enabled, the pane stays around after command exit,
// allowing us to capture diagnostic output before cleanup.
func (t *Tmux) IsPaneDead(session string) (bool, error) {
	out, err := t.run("display-message", "-t", session, "-p", "#{pane_dead}")
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) || errors.Is(err, ErrNoServer) {
			return false, err
		}
		return false, err
	}
	return strings.TrimSpace(out) == "1", nil
}

// CaptureDeadPaneOutput captures output from a dead pane for diagnostics.
// Returns empty string if capture fails (non-fatal since this is for debugging).
func (t *Tmux) CaptureDeadPaneOutput(session string, lines int) string {
	output, err := t.CapturePane(session, lines)
	if err != nil {
		return ""
	}
	return output
}

// GetPaneExitStatus returns the exit status of a dead pane.
// Returns empty string if the pane is not dead or if the status cannot be retrieved.
// Note: Requires tmux 2.0+ for pane_dead_status support.
func (t *Tmux) GetPaneExitStatus(session string) string {
	out, err := t.run("display-message", "-t", session, "-p", "#{pane_dead_status}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
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
			// Use KillSessionWithProcesses to ensure all descendant processes are killed
			if err := t.KillSessionWithProcesses(name); err != nil {
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

// processKillGracePeriod is how long to wait after SIGTERM before sending SIGKILL.
// 2 seconds gives processes time to clean up gracefully. The previous 100ms was too short
// and caused Claude processes to become orphans when they couldn't shut down in time.
const processKillGracePeriod = 2 * time.Second

// KillSessionWithProcesses explicitly kills all processes in a session before terminating it.
// This prevents orphan processes that survive tmux kill-session due to SIGHUP being ignored.
//
// Process:
// 1. Get the pane's main process PID and its process group ID (PGID)
// 2. Kill the entire process group (catches reparented processes that stayed in the group)
// 3. Find all descendant processes recursively (catches any stragglers)
// 4. Send SIGTERM/SIGKILL to descendants
// 5. Kill the pane process itself
// 6. Kill the tmux session
//
// The process group kill is critical because:
// - pgrep -P only finds direct children (PPID matching)
// - Processes that reparent to init (PID 1) are missed by pgrep
// - But they typically stay in the same process group unless they call setsid()
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
		// First, kill the entire process group. This catches processes that:
		// - Reparented to init (PID 1) when their parent died
		// - Are not direct children but stayed in the same process group
		// Note: Processes that called setsid() will have a new PGID and won't be killed here
		pgid := getProcessGroupID(pid)
		if pgid != "" && pgid != "0" && pgid != "1" {
			// Kill process group using syscall.Kill() directly, rather than shelling
			// out to /usr/bin/kill which has parsing ambiguity with negative PGIDs.
			// procps-ng kill (v4.0.4+) misparses "-PGID" and can kill ALL processes.
			// syscall.Kill with negative PID targets the process group (POSIX).
			pgidInt, _ := strconv.Atoi(pgid)
			_ = syscall.Kill(-pgidInt, syscall.SIGTERM)
			time.Sleep(100 * time.Millisecond)
			_ = syscall.Kill(-pgidInt, syscall.SIGKILL)
		}

		// Also walk the process tree for any descendants that might have called setsid()
		// and created their own process groups (rare but possible)
		descendants := getAllDescendants(pid)

		// Send SIGTERM to all descendants (deepest first to avoid orphaning)
		for _, dpid := range descendants {
			_ = exec.Command("kill", "-TERM", dpid).Run()
		}

		// Wait for graceful shutdown (2s gives processes time to clean up)
		time.Sleep(processKillGracePeriod)

		// Send SIGKILL to any remaining descendants
		for _, dpid := range descendants {
			_ = exec.Command("kill", "-KILL", dpid).Run()
		}

		// Kill the pane process itself (may have called setsid() and detached)
		_ = exec.Command("kill", "-TERM", pid).Run()
		time.Sleep(processKillGracePeriod)
		_ = exec.Command("kill", "-KILL", pid).Run()
	}

	// Kill the tmux session
	// Ignore "session not found" or "no server" - killing the pane process may have already
	// caused tmux to destroy the session automatically. If it was the last session,
	// the tmux server itself shuts down, resulting in "no server" instead of "session not found".
	err = t.KillSession(name)
	if err == ErrSessionNotFound || err == ErrNoServer {
		return nil
	}
	return err
}

// KillSessionWithProcessesExcluding is like KillSessionWithProcesses but excludes
// specified PIDs from being killed. This is essential for self-kill scenarios where
// the calling process (e.g., gt done) is running inside the session it's terminating.
// Without exclusion, the caller would be killed before completing the cleanup.
func (t *Tmux) KillSessionWithProcessesExcluding(name string, excludePIDs []string) error {
	// Build exclusion set for O(1) lookup
	exclude := make(map[string]bool)
	for _, pid := range excludePIDs {
		exclude[pid] = true
	}

	// Get the pane PID
	pid, err := t.GetPanePID(name)
	if err != nil {
		// Session might not exist or be in bad state, try direct kill
		return t.KillSession(name)
	}

	if pid != "" {
		// Get the process group ID
		pgid := getProcessGroupID(pid)

		// Collect all PIDs to kill (from multiple sources)
		toKill := make(map[string]bool)

		// 1. Get all process group members (catches reparented processes)
		if pgid != "" && pgid != "0" && pgid != "1" {
			for _, member := range getProcessGroupMembers(pgid) {
				if !exclude[member] {
					toKill[member] = true
				}
			}
		}

		// 2. Get all descendant PIDs recursively (catches processes that called setsid())
		descendants := getAllDescendants(pid)
		for _, dpid := range descendants {
			if !exclude[dpid] {
				toKill[dpid] = true
			}
		}

		// Convert to slice for iteration
		var killList []string
		for p := range toKill {
			killList = append(killList, p)
		}

		// Send SIGTERM to all non-excluded processes
		for _, dpid := range killList {
			_ = exec.Command("kill", "-TERM", dpid).Run()
		}

		// Wait for graceful shutdown (2s gives processes time to clean up)
		time.Sleep(processKillGracePeriod)

		// Send SIGKILL to any remaining non-excluded processes
		for _, dpid := range killList {
			_ = exec.Command("kill", "-KILL", dpid).Run()
		}

		// Kill the pane process itself (may have called setsid() and detached)
		// Only if not excluded
		if !exclude[pid] {
			_ = exec.Command("kill", "-TERM", pid).Run()
			time.Sleep(processKillGracePeriod)
			_ = exec.Command("kill", "-KILL", pid).Run()
		}
	}

	// Kill the tmux session - this will terminate the excluded process too
	// Ignore "session not found" or "no server" - if we killed all non-excluded processes,
	// tmux may have already destroyed the session automatically. If it was the last session,
	// the tmux server itself shuts down, resulting in "no server" instead of "session not found".
	err = t.KillSession(name)
	if err == ErrSessionNotFound || err == ErrNoServer {
		return nil
	}
	return err
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

// getProcessGroupID returns the process group ID (PGID) for a given PID.
// Returns empty string if the process doesn't exist or PGID can't be determined.
func getProcessGroupID(pid string) string {
	out, err := exec.Command("ps", "-o", "pgid=", "-p", pid).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// getProcessGroupMembers returns all PIDs in a process group.
// This finds processes that share the same PGID, including those that reparented to init.
func getProcessGroupMembers(pgid string) []string {
	// Use ps to find all processes with this PGID
	// On macOS: ps -axo pid,pgid
	// On Linux: ps -eo pid,pgid
	out, err := exec.Command("ps", "-axo", "pid,pgid").Output()
	if err != nil {
		return nil
	}

	var members []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimSpace(fields[1]) == pgid {
			members = append(members, strings.TrimSpace(fields[0]))
		}
	}
	return members
}

// KillPaneProcesses explicitly kills all processes associated with a tmux pane.
// This prevents orphan processes that survive pane respawn due to SIGHUP being ignored.
//
// Process:
// 1. Get the pane's main process PID and its process group ID (PGID)
// 2. Kill the entire process group (catches reparented processes)
// 3. Find all descendant processes recursively (catches any stragglers)
// 4. Send SIGTERM/SIGKILL to descendants
// 5. Kill the pane process itself
//
// This ensures Claude processes and all their children are properly terminated
// before respawning the pane.
func (t *Tmux) KillPaneProcesses(pane string) error {
	// Get the pane PID
	pid, err := t.GetPanePID(pane)
	if err != nil {
		return fmt.Errorf("getting pane PID: %w", err)
	}

	if pid == "" {
		return fmt.Errorf("pane PID is empty")
	}

	// First, kill the entire process group. This catches processes that:
	// - Reparented to init (PID 1) when their parent died
	// - Are not direct children but stayed in the same process group
	pgid := getProcessGroupID(pid)
	if pgid != "" && pgid != "0" && pgid != "1" {
		// Kill process group using syscall.Kill() directly.
		// See comment in KillSessionWithProcesses for why we avoid exec.Command("kill").
		pgidInt, _ := strconv.Atoi(pgid)
		_ = syscall.Kill(-pgidInt, syscall.SIGTERM)
		time.Sleep(100 * time.Millisecond)
		_ = syscall.Kill(-pgidInt, syscall.SIGKILL)
	}

	// Also walk the process tree for any descendants that might have called setsid()
	descendants := getAllDescendants(pid)

	// Send SIGTERM to all descendants (deepest first to avoid orphaning)
	for _, dpid := range descendants {
		_ = exec.Command("kill", "-TERM", dpid).Run()
	}

	// Wait for graceful shutdown (2s gives processes time to clean up)
	time.Sleep(processKillGracePeriod)

	// Send SIGKILL to any remaining descendants
	for _, dpid := range descendants {
		_ = exec.Command("kill", "-KILL", dpid).Run()
	}

	// Kill the pane process itself (may have called setsid() and detached,
	// or may have no children like Claude Code)
	_ = exec.Command("kill", "-TERM", pid).Run()
	time.Sleep(processKillGracePeriod)
	_ = exec.Command("kill", "-KILL", pid).Run()

	return nil
}

// KillPaneProcessesExcluding is like KillPaneProcesses but excludes specified PIDs
// from being killed. This is essential for self-handoff scenarios where the calling
// process (e.g., gt handoff running inside Claude Code) needs to survive long enough
// to call RespawnPane. Without exclusion, the caller would be killed before completing.
//
// The excluded PIDs should include the calling process and any ancestors that must
// survive. After this function returns, RespawnPane's -k flag will send SIGHUP to
// clean up the remaining processes.
func (t *Tmux) KillPaneProcessesExcluding(pane string, excludePIDs []string) error {
	// Build exclusion set for O(1) lookup
	exclude := make(map[string]bool)
	for _, pid := range excludePIDs {
		exclude[pid] = true
	}

	// Get the pane PID
	pid, err := t.GetPanePID(pane)
	if err != nil {
		return fmt.Errorf("getting pane PID: %w", err)
	}

	if pid == "" {
		return fmt.Errorf("pane PID is empty")
	}

	// Get all descendant PIDs recursively (returns deepest-first order)
	descendants := getAllDescendants(pid)

	// Filter out excluded PIDs
	var filtered []string
	for _, dpid := range descendants {
		if !exclude[dpid] {
			filtered = append(filtered, dpid)
		}
	}

	// Send SIGTERM to all non-excluded descendants (deepest first to avoid orphaning)
	for _, dpid := range filtered {
		_ = exec.Command("kill", "-TERM", dpid).Run()
	}

	// Wait for graceful shutdown
	time.Sleep(100 * time.Millisecond)

	// Send SIGKILL to any remaining non-excluded descendants
	for _, dpid := range filtered {
		_ = exec.Command("kill", "-KILL", dpid).Run()
	}

	// Kill the pane process itself only if not excluded
	if !exclude[pid] {
		_ = exec.Command("kill", "-TERM", pid).Run()
		time.Sleep(100 * time.Millisecond)
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

// GetCurrentSessionName returns the name of the current tmux session.
// This only works when called from within a tmux session.
func (t *Tmux) GetCurrentSessionName() (string, error) {
	out, err := t.run("display-message", "-p", "#{session_name}")
	if err != nil {
		return "", fmt.Errorf("getting current session name: %w", err)
	}
	return out, nil
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

// IsSessionAttached returns true if the session has any clients attached.
func (t *Tmux) IsSessionAttached(target string) bool {
	attached, err := t.run("display-message", "-t", target, "-p", "#{session_attached}")
	return err == nil && attached == "1"
}

// WakePane triggers a SIGWINCH in a pane by resizing it slightly then restoring.
// This wakes up Claude Code's event loop by simulating a terminal resize.
//
// When Claude runs in a detached tmux session, its TUI library may not process
// stdin until a terminal event occurs. Attaching triggers SIGWINCH which wakes
// the event loop. This function simulates that by doing a resize dance.
//
// Note: This always performs the resize. Use WakePaneIfDetached to skip
// attached sessions where the wake is unnecessary.
func (t *Tmux) WakePane(target string) {
	// Shrink pane by 1 row (-D = down/shrink), then grow back by 1 row (-U = up/grow)
	// This triggers SIGWINCH without changing the final pane size
	// Note: -D/-U are for relative adjustments; -y sets absolute height (which doesn't work here)
	_, _ = t.run("resize-pane", "-t", target, "-D", "1")
	time.Sleep(50 * time.Millisecond)
	_, _ = t.run("resize-pane", "-t", target, "-U", "1")
}

// WakePaneIfDetached triggers a SIGWINCH only if the session is detached.
// This avoids unnecessary latency on attached sessions where Claude is
// already processing terminal events.
func (t *Tmux) WakePaneIfDetached(target string) {
	if t.IsSessionAttached(target) {
		return
	}
	t.WakePane(target)
}

// NudgeSession sends a message to a Claude Code session reliably.
// This is the canonical way to send messages to Claude sessions.
// Uses: literal mode + 500ms debounce + ESC (for vim mode) + separate Enter.
// After sending, triggers SIGWINCH to wake Claude in detached sessions.
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
		// 5. Wake the pane to trigger SIGWINCH for detached sessions
		t.WakePaneIfDetached(session)
		return nil
	}
	return fmt.Errorf("failed to send Enter after 3 attempts: %w", lastErr)
}

// NudgePane sends a message to a specific pane reliably.
// Same pattern as NudgeSession but targets a pane ID (e.g., "%9") instead of session name.
// After sending, triggers SIGWINCH to wake Claude in detached sessions.
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
		// 5. Wake the pane to trigger SIGWINCH for detached sessions
		t.WakePaneIfDetached(pane)
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

// hasChildWithNames checks if a process has a descendant matching any of the given names.
// Used when the pane command is a shell (bash, zsh) that launched an agent.
//
// IMPORTANT: This checks ALL descendants, not just direct children.
// Agents are often started via wrapper scripts like:
//
//	bash (pane) -> bash -c 'export ... && claude' -> claude
//
// In this case, the agent is a grandchild, not a direct child.
// Using only pgrep -P (direct children) would miss this case and cause
// CleanupOrphanedSessions to kill sessions with running agents.
func hasChildWithNames(pid string, names []string) bool {
	if len(names) == 0 {
		return false
	}

	// Build a set of names for fast lookup
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	// Get all descendant PIDs recursively
	descendants := getAllDescendants(pid)
	if len(descendants) == 0 {
		return false
	}

	// Check each descendant's command name
	for _, dpid := range descendants {
		// Get the command name for this PID
		out, err := exec.Command("ps", "-o", "comm=", "-p", dpid).Output()
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(out))
		if nameSet[name] {
			return true
		}
		// Also check for version pattern (Claude Code shows version as process name)
		if versionPattern.MatchString(name) {
			return true
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

// IsRuntimeRunning checks if a runtime appears to be running in the session.
// Checks both pane command and child processes (for agents started via shell).
// This is the unified agent detection method for all agent types.
func (t *Tmux) IsRuntimeRunning(session string, processNames []string) bool {
	if len(processNames) == 0 {
		return false
	}
	cmd, err := t.GetPaneCommand(session)
	if err != nil {
		return false
	}
	// Check direct pane command match
	for _, name := range processNames {
		if cmd == name {
			return true
		}
	}
	// If pane command is a shell, check for child processes.
	// This handles agents started with "bash -c 'export ... && agent ...'"
	for _, shell := range constants.SupportedShells {
		if cmd == shell {
			pid, err := t.GetPanePID(session)
			if err == nil && pid != "" {
				return hasChildWithNames(pid, processNames)
			}
			break
		}
	}
	return false
}

// IsAgentAlive checks if an agent is running in the session using agent-agnostic detection.
// It reads GT_AGENT from the session environment to determine which process names to check.
// Falls back to Claude's process names if GT_AGENT is not set (legacy sessions).
// This is the preferred method for zombie detection across all agent types.
func (t *Tmux) IsAgentAlive(session string) bool {
	agentName, _ := t.GetEnvironment(session, "GT_AGENT")
	processNames := config.GetProcessNames(agentName) // Returns Claude defaults if empty
	return t.IsRuntimeRunning(session, processNames)
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
	if err := t.SetAgentsBinding(session); err != nil {
		return fmt.Errorf("setting agents binding: %w", err)
	}
	if err := t.SetCycleBindings(session); err != nil {
		return fmt.Errorf("setting cycle bindings: %w", err)
	}
	if err := t.EnableMouseMode(session); err != nil {
		return fmt.Errorf("enabling mouse mode: %w", err)
	}
	return nil
}

// EnableMouseMode enables mouse support and clipboard integration for a tmux session.
// This allows clicking to select panes/windows, scrolling with mouse wheel,
// and dragging to resize panes. Hold Shift for native terminal text selection.
// Also enables clipboard integration so copied text goes to system clipboard.
func (t *Tmux) EnableMouseMode(session string) error {
	if _, err := t.run("set-option", "-t", session, "mouse", "on"); err != nil {
		return err
	}
	// Enable clipboard integration with terminal (OSC 52)
	// This allows copying text to system clipboard when selecting with mouse
	_, err := t.run("set-option", "-t", session, "set-clipboard", "on")
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

// RespawnPaneWithWorkDir kills all processes in a pane and starts a new command with a working directory.
// Like RespawnPane but allows specifying the working directory for the new process.
func (t *Tmux) RespawnPaneWithWorkDir(pane, workDir, command string) error {
	args := []string{"respawn-pane", "-k", "-t", pane}
	if workDir != "" {
		args = append(args, "-c", workDir)
	}
	args = append(args, command)
	_, err := t.run(args...)
	return err
}

// ClearHistory clears the scrollback history buffer for a pane.
// This resets copy-mode display from [0/N] to [0/0].
// The pane parameter should be a pane ID (e.g., "%0") or session:window.pane format.
func (t *Tmux) ClearHistory(pane string) error {
	_, err := t.run("clear-history", "-t", pane)
	return err
}

// SetRemainOnExit controls whether a pane stays around after its process exits.
// When on, the pane remains with "[Exited]" status, allowing respawn-pane to restart it.
// When off (default), the pane is destroyed when its process exits.
// This is essential for handoff: set on before killing processes, so respawn-pane works.
func (t *Tmux) SetRemainOnExit(pane string, on bool) error {
	value := "on"
	if !on {
		value = "off"
	}
	_, err := t.run("set-option", "-t", pane, "remain-on-exit", value)
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

// SetAgentsBinding configures C-b g to open the agent switcher popup menu.
// This runs `gt agents` which displays a tmux popup with all Gas Town agents.
//
// IMPORTANT: This binding is conditional - it only runs for Gas Town sessions
// (those starting with "gt-" or "hq-"). For non-GT sessions, a help message is shown.
func (t *Tmux) SetAgentsBinding(session string) error {
	// C-b g → gt agents for GT sessions, help message otherwise
	_, err := t.run("bind-key", "-T", "prefix", "g",
		"if-shell", "echo '#{session_name}' | grep -Eq '^(gt|hq)-'",
		"run-shell 'gt agents'",
		"display-message 'C-b g is for Gas Town sessions only'")
	return err
}

// CleanupOrphanedSessions scans for zombie Gas Town sessions and kills them.
// A zombie session is one where tmux is alive but the Claude process has died.
// This runs at `gt start` time to prevent session name conflicts and resource accumulation.
//
// Returns:
//   - cleaned: number of zombie sessions that were killed
//   - err: error if session listing failed (individual kill errors are logged but not returned)
func (t *Tmux) CleanupOrphanedSessions() (cleaned int, err error) {
	sessions, err := t.ListSessions()
	if err != nil {
		return 0, fmt.Errorf("listing sessions: %w", err)
	}

	for _, sess := range sessions {
		// Only process Gas Town sessions (gt-* for rigs, hq-* for town-level)
		if !strings.HasPrefix(sess, "gt-") && !strings.HasPrefix(sess, "hq-") {
			continue
		}

		// Check if the session is a zombie (tmux alive, agent dead)
		if !t.IsAgentAlive(sess) {
			// Kill the zombie session
			if killErr := t.KillSessionWithProcesses(sess); killErr != nil {
				// Log but continue - other sessions may still need cleanup
				fmt.Printf("  warning: failed to kill orphaned session %s: %v\n", sess, killErr)
				continue
			}
			cleaned++
		}
	}

	return cleaned, nil
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
