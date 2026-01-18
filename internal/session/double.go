package session

import (
	"errors"
	"sync"
	"time"

	"github.com/steveyegge/gastown/internal/ids"
)

// Double is a FAKE with SPY capabilities for the Sessions interface.
//
// Test Double Taxonomy (Meszaros/Fowler):
//   - FAKE: Working in-memory implementation (no real tmux subprocess)
//   - SPY: Records method calls for verification (ControlLog, NudgeLog)
//
// Use conformance tests to verify it matches real tmux behavior.
// For error injection, wrap with a stub that intercepts specific methods.
type Double struct {
	mu             sync.RWMutex
	sessions       map[string]*doubleSession
	currentSession string // simulates being "inside" a session (for SwitchTo/CurrentSession)
}

type doubleSession struct {
	name       string // session name
	workDir    string
	command    string
	env        map[string]string
	buffer     []string // captured output lines
	running    bool     // simulates process running
	controlLog []string // log of control sequences sent
	nudgeLog   []string // log of nudge messages sent
}

// NewDouble creates a new in-memory Sessions test double.
func NewDouble() *Double {
	return &Double{
		sessions: make(map[string]*doubleSession),
	}
}

// Ensure Double implements Sessions
var _ Sessions = (*Double)(nil)

// --- Lifecycle ---

// Start creates a new session. Fails if session name already exists.
func (d *Double) Start(name, workDir, command string) (SessionID, error) {
	if name == "" {
		return "", errors.New("session name cannot be empty")
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.sessions[name]; exists {
		return "", errors.New("duplicate session: " + name)
	}

	d.sessions[name] = &doubleSession{
		name:    name,
		workDir: workDir,
		command: command,
		env:     make(map[string]string),
		buffer:  []string{"> "}, // Simulate ready prompt for Claude-like agents
		running: true,
	}

	return SessionID(name), nil
}

// Stop terminates a session. Returns nil if session doesn't exist (idempotent).
func (d *Double) Stop(id SessionID) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.sessions, string(id))
	return nil
}

// Exists checks if a session exists.
func (d *Double) Exists(id SessionID) (bool, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	_, exists := d.sessions[string(id)]
	return exists, nil
}

// Respawn atomically kills the session's process and starts a new one.
// In the double, this clears the buffer and updates the command.
func (d *Double) Respawn(id SessionID, command string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	sess, exists := d.sessions[string(id)]
	if !exists {
		return errors.New("session not found: " + string(id))
	}

	// Clear buffer (simulates clearing scrollback)
	sess.buffer = []string{"> "}
	// Update command
	sess.command = command
	// Reset running state
	sess.running = true
	// Clear logs
	sess.controlLog = nil
	sess.nudgeLog = nil

	return nil
}

// --- Communication ---

// Send sends text to a session. Appends to session buffer.
func (d *Double) Send(id SessionID, text string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	sess, exists := d.sessions[string(id)]
	if !exists {
		return errors.New("session not found: " + string(id))
	}

	// Simulate sending text - append to buffer
	sess.buffer = append(sess.buffer, text)
	return nil
}

// SendControl sends a control sequence. Logs the key for verification.
func (d *Double) SendControl(id SessionID, key string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	sess, exists := d.sessions[string(id)]
	if !exists {
		return errors.New("session not found: " + string(id))
	}

	sess.controlLog = append(sess.controlLog, key)
	return nil
}

// Nudge sends a message to a running agent reliably.
// In the double, this behaves like Send but logs to nudgeLog for verification.
func (d *Double) Nudge(id SessionID, message string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	sess, exists := d.sessions[string(id)]
	if !exists {
		return errors.New("session not found: " + string(id))
	}

	sess.nudgeLog = append(sess.nudgeLog, message)
	sess.buffer = append(sess.buffer, message) // Also add to buffer like Send
	return nil
}

// --- Observation ---

// Capture returns the session buffer content.
func (d *Double) Capture(id SessionID, lines int) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	sess, exists := d.sessions[string(id)]
	if !exists {
		return "", errors.New("session not found: " + string(id))
	}

	// Return last N lines from buffer
	start := 0
	if len(sess.buffer) > lines {
		start = len(sess.buffer) - lines
	}

	result := ""
	for i := start; i < len(sess.buffer); i++ {
		if result != "" {
			result += "\n"
		}
		result += sess.buffer[i]
	}
	return result, nil
}

// CaptureAll returns the entire session buffer content.
func (d *Double) CaptureAll(id SessionID) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	sess, exists := d.sessions[string(id)]
	if !exists {
		return "", errors.New("session not found: " + string(id))
	}

	result := ""
	for i := 0; i < len(sess.buffer); i++ {
		if result != "" {
			result += "\n"
		}
		result += sess.buffer[i]
	}
	return result, nil
}

// IsRunning checks if the session is running specified processes.
// In the double, returns true if session exists and has running=true.
func (d *Double) IsRunning(id SessionID, processNames ...string) bool {
	if len(processNames) == 0 {
		return false
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	sess, exists := d.sessions[string(id)]
	if !exists {
		return false
	}

	return sess.running
}

// WaitFor waits for processes to start. Returns immediately in double.
func (d *Double) WaitFor(id SessionID, timeout time.Duration, processNames ...string) error {
	if len(processNames) == 0 {
		return nil
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	sess, exists := d.sessions[string(id)]
	if !exists {
		return errors.New("session not found: " + string(id))
	}

	if !sess.running {
		return errors.New("timeout waiting for process")
	}

	return nil
}

// GetStartCommand returns the command that started the session.
// In the double, this returns the command passed to Start or the last Respawn.
func (d *Double) GetStartCommand(id SessionID) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	sess, exists := d.sessions[string(id)]
	if !exists {
		return "", errors.New("session not found: " + string(id))
	}

	return sess.command, nil
}

// --- Management ---

// List returns all SessionIDs.
func (d *Double) List() ([]SessionID, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var ids []SessionID
	for name := range d.sessions {
		ids = append(ids, SessionID(name))
	}
	return ids, nil
}

// SetEnv sets an environment variable in a session.
func (d *Double) SetEnv(id SessionID, key, value string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	sess, exists := d.sessions[string(id)]
	if !exists {
		return errors.New("session not found: " + string(id))
	}

	sess.env[key] = value
	return nil
}

// SetEnvVars sets multiple environment variables in a session.
func (d *Double) SetEnvVars(id SessionID, vars map[string]string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	sess, exists := d.sessions[string(id)]
	if !exists {
		return errors.New("session not found: " + string(id))
	}

	for k, v := range vars {
		sess.env[k] = v
	}
	return nil
}

// GetEnv returns an environment variable from a session.
func (d *Double) GetEnv(id SessionID, key string) (string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	sess, exists := d.sessions[string(id)]
	if !exists {
		return "", errors.New("session not found: " + string(id))
	}

	return sess.env[key], nil
}

// GetInfo returns session information.
func (d *Double) GetInfo(id SessionID) (*Info, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	sess, exists := d.sessions[string(id)]
	if !exists {
		return nil, errors.New("session not found: " + string(id))
	}

	return &Info{
		Name:    sess.name,
		Created: time.Now().Format(time.RFC3339),
		Windows: 1,
	}, nil
}

// Attach is a no-op in the test double (can't actually attach in tests).
func (d *Double) Attach(id SessionID) error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	_, exists := d.sessions[string(id)]
	if !exists {
		return errors.New("session not found: " + string(id))
	}
	// In tests, we just verify the session exists
	return nil
}

// SwitchTo switches the "current" session in the test double.
// This simulates being inside a tmux session and switching to another.
func (d *Double) SwitchTo(id SessionID) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Can only switch if we're "inside" a session
	if d.currentSession == "" {
		return errors.New("not inside a session, cannot switch")
	}

	_, exists := d.sessions[string(id)]
	if !exists {
		return errors.New("session not found: " + string(id))
	}

	d.currentSession = string(id)
	return nil
}

// --- Test helpers (not part of Session interface) ---

// SetBuffer sets the capture buffer for a session (for test setup).
func (d *Double) SetBuffer(id SessionID, lines []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	sess, exists := d.sessions[string(id)]
	if !exists {
		return errors.New("session not found: " + string(id))
	}

	sess.buffer = lines
	return nil
}

// SetRunning sets the running state for a session (for test setup).
func (d *Double) SetRunning(id SessionID, running bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	sess, exists := d.sessions[string(id)]
	if !exists {
		return errors.New("session not found: " + string(id))
	}

	sess.running = running
	return nil
}

// Clear removes all sessions (for test cleanup).
func (d *Double) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.sessions = make(map[string]*doubleSession)
}

// SessionCount returns the number of sessions (for test verification).
func (d *Double) SessionCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return len(d.sessions)
}

// ControlLog returns the control sequences sent to a session (for test verification).
func (d *Double) ControlLog(id SessionID) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	sess, exists := d.sessions[string(id)]
	if !exists {
		return nil
	}

	// Return a copy to prevent mutation
	result := make([]string, len(sess.controlLog))
	copy(result, sess.controlLog)
	return result
}

// NudgeLog returns the nudge messages sent to a session (for test verification).
func (d *Double) NudgeLog(id SessionID) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	sess, exists := d.sessions[string(id)]
	if !exists {
		return nil
	}

	// Return a copy to prevent mutation
	result := make([]string, len(sess.nudgeLog))
	copy(result, sess.nudgeLog)
	return result
}

// GetCommand returns the command that was passed to Start (for test verification).
func (d *Double) GetCommand(id SessionID) string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	sess, exists := d.sessions[string(id)]
	if !exists {
		return ""
	}

	return sess.command
}

// SetCurrentSession sets the current session (simulates being "inside" a session).
// Pass empty string to simulate being outside all sessions.
func (d *Double) SetCurrentSession(id SessionID) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.currentSession = string(id)
}

// GetCurrentSession returns the current session for test verification.
func (d *Double) GetCurrentSession() SessionID {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return SessionID(d.currentSession)
}

// SessionIDForAgent converts an agent address to its SessionID.
func (d *Double) SessionIDForAgent(id ids.AgentID) SessionID {
	return SessionID(SessionNameFromAgentID(id))
}
