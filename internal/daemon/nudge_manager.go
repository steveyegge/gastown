package daemon

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/steveyegge/gastown/internal/tmux"
)

const (
	// NudgeMaxAge is the maximum age of a nudge before it expires.
	NudgeMaxAge = 2 * time.Minute

	// NudgeMaxAttempts is the maximum delivery attempts per nudge.
	NudgeMaxAttempts = 5

	// NudgeMaxPerAgent is the maximum queued nudges per target agent.
	NudgeMaxPerAgent = 8

	// NudgeMaxTotal is the maximum total queued nudges.
	NudgeMaxTotal = 1024

	// NudgeMaxLineSize is the maximum size of a serialized nudge line.
	NudgeMaxLineSize = 512

	// NudgeStuckPollInterval is the interval for checking stuck nudges.
	NudgeStuckPollInterval = 2 * time.Second

	// NudgeQueuePollInterval is the interval for checking the queue.
	NudgeQueuePollInterval = 200 * time.Millisecond

	// NudgeEscalationThreshold is consecutive session failures before escalating.
	NudgeEscalationThreshold = 3

	// nudgeDeliveryVerifyDelay is the time to wait after sending before checking if stuck.
	nudgeDeliveryVerifyDelay = 200 * time.Millisecond

	// nudgeInputClearDelay is the time to wait after sending Ctrl-U for input to clear.
	nudgeInputClearDelay = 100 * time.Millisecond
)

// NudgeRequest represents a queued nudge.
// JSON tags use short names to minimize queue file size since messages
// can be up to 512 bytes and we may have 1024 queued.
type NudgeRequest struct {
	ID        string    `json:"id"`
	Target    string    `json:"t"`       // target agent session
	Message   string    `json:"m"`       // message content
	From      string    `json:"f,omitempty"` // sender identity
	Timestamp time.Time `json:"ts"`
	Attempts  int       `json:"a,omitempty"` // delivery attempts
	Error     string    `json:"e,omitempty"` // last error
}

// SessionState tracks delivery state for a target session.
type SessionState struct {
	Failures int // consecutive delivery failures
}

// NudgeManager handles reliable nudge delivery via a file-based queue.
type NudgeManager struct {
	queuePath string
	lockPath  string
	tmux      *tmux.Tmux
	logger    func(format string, args ...interface{})

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	sessions     map[string]*SessionState
	lastQueueMod time.Time
	mu           sync.Mutex
}

// NewNudgeManager creates a new nudge manager.
func NewNudgeManager(townRoot string, t *tmux.Tmux, logger func(format string, args ...interface{})) (*NudgeManager, error) {
	queuePath := filepath.Join(townRoot, "daemon", "nudges.jsonl")
	lockPath := queuePath + ".lock"

	// Ensure daemon directory exists
	if err := os.MkdirAll(filepath.Dir(queuePath), 0755); err != nil {
		return nil, fmt.Errorf("create daemon dir: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &NudgeManager{
		queuePath: queuePath,
		lockPath:  lockPath,
		tmux:      t,
		logger:    logger,
		ctx:       ctx,
		cancel:    cancel,
		sessions:  make(map[string]*SessionState),
	}, nil
}

// Start begins the nudge manager goroutine.
func (m *NudgeManager) Start() error {
	// Initial processing of any queued nudges
	m.processQueue()

	m.wg.Add(1)
	go m.run()
	return nil
}

// Stop gracefully stops the nudge manager.
func (m *NudgeManager) Stop() {
	m.cancel()
	m.wg.Wait()
}

// run is the main manager loop.
func (m *NudgeManager) run() {
	defer m.wg.Done()

	queueTicker := time.NewTicker(NudgeQueuePollInterval)
	stuckTicker := time.NewTicker(NudgeStuckPollInterval)
	defer queueTicker.Stop()
	defer stuckTicker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return

		case <-queueTicker.C:
			// Check if queue file was modified
			if m.queueFileModified() {
				m.processQueue()
			}

		case <-stuckTicker.C:
			m.checkStuckNudges()
		}
	}
}

// queueFileModified checks if the queue file has been modified since last check.
func (m *NudgeManager) queueFileModified() bool {
	info, err := os.Stat(m.queuePath)
	if err != nil {
		return false // File doesn't exist or error
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	modTime := info.ModTime()
	if modTime.After(m.lastQueueMod) {
		m.lastQueueMod = modTime
		return true
	}
	return false
}

// GenerateNudgeID creates a deterministic ID for dedup.
// Same target + message within 1 second = same ID.
func GenerateNudgeID(target, message string, ts time.Time) string {
	seed := fmt.Sprintf("%s|%s|%d", target, message, ts.Unix())
	hash := sha256.Sum256([]byte(seed))
	num := binary.BigEndian.Uint32(hash[:4])
	return strconv.FormatUint(uint64(num), 36)
}

// QueueNudge adds a nudge to the queue.
// Returns error if nudge is invalid or duplicate.
func (m *NudgeManager) QueueNudge(target, message, from string) error {
	ts := time.Now()
	id := GenerateNudgeID(target, message, ts)

	req := NudgeRequest{
		ID:        id,
		Target:    target,
		Message:   message,
		From:      from,
		Timestamp: ts,
	}

	// Validate line size
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal nudge: %w", err)
	}
	if len(data) > NudgeMaxLineSize {
		return fmt.Errorf("nudge too large (%d bytes, max %d). Use gt mail for longer content", len(data), NudgeMaxLineSize)
	}

	// Acquire lock for write
	lock := flock.New(m.lockPath)
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer func() { _ = lock.Unlock() }()

	// Check for duplicate
	existing, err := m.loadQueue()
	if err != nil {
		return fmt.Errorf("load queue: %w", err)
	}
	for _, e := range existing {
		if e.ID == id {
			m.logger("nudge manager: duplicate nudge %s, skipping", id)
			return nil // Idempotent - not an error
		}
	}

	// Enforce limits
	bySession := make(map[string]int)
	for _, e := range existing {
		bySession[e.Target]++
	}
	if len(existing) >= NudgeMaxTotal {
		return fmt.Errorf("nudge queue full (%d). Retry shortly or use gt mail instead", NudgeMaxTotal)
	}
	if bySession[target] >= NudgeMaxPerAgent {
		return fmt.Errorf("too many nudges for %s (%d). Retry shortly or use gt mail instead", target, NudgeMaxPerAgent)
	}

	// Append to queue
	f, err := os.OpenFile(m.queuePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open queue: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(string(data) + "\n"); err != nil {
		return fmt.Errorf("write nudge: %w", err)
	}

	return nil
}

// loadQueue reads all nudges from the queue file.
// Caller must hold the file lock when modifying the queue after reading.
func (m *NudgeManager) loadQueue() ([]NudgeRequest, error) {
	f, err := os.Open(m.queuePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var nudges []NudgeRequest
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var req NudgeRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			m.logger("nudge manager: invalid line: %s", line)
			continue
		}
		nudges = append(nudges, req)
	}
	return nudges, scanner.Err()
}

// processQueue attempts to deliver queued nudges.
func (m *NudgeManager) processQueue() {
	lock := flock.New(m.lockPath)
	if err := lock.Lock(); err != nil {
		m.logger("nudge manager: lock error: %v", err)
		return
	}
	defer func() { _ = lock.Unlock() }()

	nudges, err := m.loadQueue()
	if err != nil {
		m.logger("nudge manager: load error: %v", err)
		return
	}

	if len(nudges) == 0 {
		return
	}

	now := time.Now()
	var remaining []NudgeRequest
	delivered := make(map[string]bool)

	// Group by target, process oldest first
	byTarget := make(map[string][]NudgeRequest)
	for _, n := range nudges {
		// Skip expired
		if now.Sub(n.Timestamp) > NudgeMaxAge {
			m.logger("nudge manager: expired %s to %s", n.ID, n.Target)
			continue
		}
		// Skip max attempts
		if n.Attempts >= NudgeMaxAttempts {
			m.logger("nudge manager: max attempts %s to %s", n.ID, n.Target)
			continue
		}
		byTarget[n.Target] = append(byTarget[n.Target], n)
	}

	for target, targetNudges := range byTarget {
		// Only deliver one per target per cycle (preserve order)
		nudge := targetNudges[0]

		// Attempt delivery
		if err := m.deliverNudge(&nudge); err != nil {
			m.logger("nudge manager: delivery error %s: %v", nudge.ID, err)
			nudge.Attempts++
			nudge.Error = err.Error()
			remaining = append(remaining, nudge)
			remaining = append(remaining, targetNudges[1:]...)
			m.recordFailure(target)
		} else {
			m.logger("nudge manager: delivered %s to %s", nudge.ID, target)
			delivered[nudge.ID] = true
			remaining = append(remaining, targetNudges[1:]...)
			m.resetFailures(target)
		}
	}

	// Rewrite queue with remaining nudges
	if len(remaining) != len(nudges) || len(delivered) > 0 {
		if err := m.rewriteQueue(remaining); err != nil {
			m.logger("nudge manager: rewrite error: %v", err)
		}
	}
}

// deliverNudge sends a nudge to the target session.
func (m *NudgeManager) deliverNudge(nudge *NudgeRequest) error {
	// Format message with sentinel
	msg := fmt.Sprintf("%s-[from %s] %s", nudge.ID, nudge.From, nudge.Message)

	// Use tmux NudgeSession for delivery
	if err := m.tmux.NudgeSession(nudge.Target, msg); err != nil {
		return err
	}

	// Brief wait then verify not stuck
	time.Sleep(nudgeDeliveryVerifyDelay)

	if m.isStuckInInput(nudge.Target, nudge.ID) {
		return fmt.Errorf("stuck in input")
	}

	return nil
}

// isStuckInInput checks if our nudge is stuck in the input line.
func (m *NudgeManager) isStuckInInput(session, nudgeID string) bool {
	inputLine := m.captureInputLine(session)
	prefix := nudgeID + "-[from"
	return strings.Contains(inputLine, prefix)
}

// captureInputLine gets the current input line from a tmux session.
// Uses the last line of the captured pane as an approximation.
func (m *NudgeManager) captureInputLine(session string) string {
	lines, err := m.tmux.CapturePaneLines(session, 5)
	if err != nil || len(lines) == 0 {
		return ""
	}

	// Return the last non-empty line
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

// checkStuckNudges looks for stuck nudges and attempts recovery.
func (m *NudgeManager) checkStuckNudges() {
	// Process queued nudges under lock
	m.processStuckQueuedNudges()

	// Check for fallback nudges (legacy format without ID) outside lock
	// since adoptFallbackNudges may call QueueNudge which needs the lock
	m.adoptFallbackNudges()
}

// processStuckQueuedNudges handles stuck nudges in the queue.
func (m *NudgeManager) processStuckQueuedNudges() {
	lock := flock.New(m.lockPath)
	if err := lock.Lock(); err != nil {
		return
	}
	defer func() { _ = lock.Unlock() }()

	nudges, err := m.loadQueue()
	if err != nil {
		return
	}

	var modified bool
	for i := range nudges {
		if m.isStuckInInput(nudges[i].Target, nudges[i].ID) {
			// Try to send it first by pressing Enter
			m.logger("nudge manager: stuck nudge %s, attempting to send", nudges[i].ID)
			_ = m.tmux.SendKeysRaw(nudges[i].Target, "Enter")
			time.Sleep(nudgeDeliveryVerifyDelay)

			// Check if still stuck
			if m.isStuckInInput(nudges[i].Target, nudges[i].ID) {
				// Still stuck - clear and mark for retry
				m.logger("nudge manager: still stuck after Enter, clearing %s", nudges[i].ID)
				m.clearInput(nudges[i].Target)
				nudges[i].Attempts++
				nudges[i].Error = "stuck"
				modified = true
			} else {
				// Successfully sent - remove from queue
				m.logger("nudge manager: unstuck %s with Enter", nudges[i].ID)
				nudges[i].Attempts = NudgeMaxAttempts // Mark as done (will be filtered out)
				modified = true
			}
		}
	}

	if modified {
		if err := m.rewriteQueue(nudges); err != nil {
			m.logger("nudge manager: rewrite error: %v", err)
		}
	}
}

// clearInput sends Ctrl-U to clear the input line.
func (m *NudgeManager) clearInput(session string) {
	_ = m.tmux.SendKeysRaw(session, "C-u")
	time.Sleep(nudgeInputClearDelay)
}

// adoptFallbackNudges looks for stuck legacy nudges (without ID prefix) and handles them.
// Uses the same flow as regular stuck nudges: try Enter first, then clear and re-queue.
func (m *NudgeManager) adoptFallbackNudges() {
	// Check all known sessions for stuck [from X] without ID prefix
	m.mu.Lock()
	sessions := make([]string, 0, len(m.sessions))
	for s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.mu.Unlock()

	for _, session := range sessions {
		inputLine := m.captureInputLine(session)
		// Look for [from without ID prefix (legacy format)
		if !strings.Contains(inputLine, "[from ") || strings.Contains(inputLine, "-[from") {
			continue
		}

		m.logger("nudge manager: found legacy nudge in %s, attempting to send", session)

		// Try Enter first (same as regular stuck nudges)
		_ = m.tmux.SendKeysRaw(session, "Enter")
		time.Sleep(nudgeDeliveryVerifyDelay)

		// Check if still stuck
		inputLine = m.captureInputLine(session)
		if !strings.Contains(inputLine, "[from ") {
			m.logger("nudge manager: legacy nudge sent successfully in %s", session)
			continue
		}

		// Still stuck - extract message info and re-queue with proper ID
		m.logger("nudge manager: legacy nudge still stuck in %s, clearing and re-queuing", session)

		// Parse "[from sender] message" format
		fromIdx := strings.Index(inputLine, "[from ")
		if fromIdx == -1 {
			continue
		}
		rest := inputLine[fromIdx+6:] // skip "[from "
		endIdx := strings.Index(rest, "]")
		if endIdx == -1 {
			m.clearInput(session)
			continue
		}
		sender := rest[:endIdx]
		message := strings.TrimSpace(rest[endIdx+1:])

		// Clear the input
		m.clearInput(session)

		// Re-queue with proper ID (will be delivered on next cycle)
		if message != "" {
			if err := m.QueueNudge(session, message, sender); err != nil {
				m.logger("nudge manager: failed to re-queue legacy nudge: %v", err)
			}
		}
	}
}

// recordFailure increments failure count for a session.
func (m *NudgeManager) recordFailure(session string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.sessions[session]
	if !ok {
		state = &SessionState{}
		m.sessions[session] = state
	}
	state.Failures++

	if state.Failures >= NudgeEscalationThreshold {
		m.escalateToMayor(session, state.Failures)
		state.Failures = 0
	}
}

// resetFailures resets failure count for a session.
func (m *NudgeManager) resetFailures(session string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, ok := m.sessions[session]; ok {
		state.Failures = 0
	}
}

// escalateToMayor sends an alert via mail (not nudge).
func (m *NudgeManager) escalateToMayor(session string, failures int) {
	m.logger("nudge manager: escalating to mayor - %s has %d consecutive failures", session, failures)

	// Use gt mail, not nudge (nudges are broken!)
	msg := fmt.Sprintf("Nudge delivery failing for session %s.\nConsecutive failures: %d\nQueue may be drained.", session, failures)

	// Fire and forget - don't block on mail
	go func() {
		cmd := exec.Command("gt", "mail", "send", "mayor", "-s", "ALERT: Nudge delivery broken", "-m", msg)
		if err := cmd.Run(); err != nil {
			m.logger("nudge manager: failed to mail mayor: %v", err)
		}
	}()
}

// rewriteQueue atomically rewrites the queue file.
func (m *NudgeManager) rewriteQueue(nudges []NudgeRequest) error {
	tmpPath := m.queuePath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	var writeErr error
	for _, n := range nudges {
		data, err := json.Marshal(n)
		if err != nil {
			writeErr = err
			break
		}
		if _, err := f.WriteString(string(data) + "\n"); err != nil {
			writeErr = err
			break
		}
	}

	if closeErr := f.Close(); closeErr != nil && writeErr == nil {
		writeErr = closeErr
	}

	if writeErr != nil {
		_ = os.Remove(tmpPath)
		return writeErr
	}

	return os.Rename(tmpPath, m.queuePath)
}
