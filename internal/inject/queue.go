// Package inject provides a queue-based injection system for Claude Code hooks.
//
// The injection queue solves API 400 concurrency errors that occur when multiple
// hooks try to inject content simultaneously. Instead of writing to stdout directly,
// hooks queue their content, and a dedicated drain command outputs everything safely.
//
// Architecture:
//   - Queue Storage: .runtime/inject-queue/<session-id>.jsonl
//   - Queue Writers: gt mail check --inject, bd decision check --inject
//   - Queue Consumer: gt inject drain
//
// The queue is designed for single-writer (hook) and single-reader (drain) access.
// Within a single Claude session, hooks fire sequentially, so we don't need
// complex locking - atomic file operations suffice.
package inject

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/steveyegge/gastown/internal/constants"
)

// EntryType identifies the type of queued content.
type EntryType string

const (
	// TypeMail indicates mail notification content.
	TypeMail EntryType = "mail"
	// TypeDecision indicates decision notification content.
	TypeDecision EntryType = "decision"
	// TypeNudge indicates a nudge message from another agent.
	TypeNudge EntryType = "nudge"
)

// Entry represents a single item in the injection queue.
type Entry struct {
	Type      EntryType `json:"type"`
	Content   string    `json:"content"`
	Timestamp int64     `json:"timestamp"`
}

// Queue manages the injection queue for a session.
type Queue struct {
	sessionID string
	queueDir  string
}

// NewQueue creates a queue for the given session.
// workDir should be the rig or workspace directory containing .runtime/.
func NewQueue(workDir, sessionID string) *Queue {
	return &Queue{
		sessionID: sessionID,
		queueDir:  filepath.Join(workDir, constants.DirRuntime, "inject-queue"),
	}
}

// queueFile returns the path to this session's queue file.
func (q *Queue) queueFile() string {
	return filepath.Join(q.queueDir, q.sessionID+".jsonl")
}

// Enqueue adds an entry to the queue.
func (q *Queue) Enqueue(entryType EntryType, content string) error {
	// Ensure queue directory exists
	if err := os.MkdirAll(q.queueDir, 0755); err != nil {
		return fmt.Errorf("creating queue directory: %w", err)
	}

	// Create entry
	entry := Entry{
		Type:      entryType,
		Content:   content,
		Timestamp: time.Now().UnixMilli(),
	}

	// Marshal to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshaling entry: %w", err)
	}

	// Append to queue file with flock
	f, err := os.OpenFile(q.queueFile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening queue file: %w", err)
	}
	defer f.Close()

	// Acquire exclusive lock for write
	if err := lockFileExclusive(f); err != nil {
		return fmt.Errorf("acquiring file lock: %w", err)
	}
	defer func() { _ = unlockFile(f) }()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("writing to queue: %w", err)
	}

	return nil
}

// Drain reads all entries from the queue and removes them.
// Returns the entries in order (oldest first).
func (q *Queue) Drain() ([]Entry, error) {
	// Open file for read/write to get exclusive lock
	f, err := os.OpenFile(q.queueFile(), os.O_RDWR, 0644)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Empty queue
		}
		return nil, fmt.Errorf("opening queue file: %w", err)
	}
	defer f.Close()

	// Acquire exclusive lock
	if err := lockFileExclusive(f); err != nil {
		return nil, fmt.Errorf("acquiring file lock: %w", err)
	}
	defer func() { _ = unlockFile(f) }()

	// Read queue file
	data, err := os.ReadFile(q.queueFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Empty queue
		}
		return nil, fmt.Errorf("reading queue file: %w", err)
	}

	// Parse entries
	var entries []Entry
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			// Log but don't fail on corrupt entries
			continue
		}
		entries = append(entries, entry)
	}

	// Clear the queue file
	if err := os.Remove(q.queueFile()); err != nil && !os.IsNotExist(err) {
		return entries, fmt.Errorf("removing queue file: %w", err)
	}

	return entries, nil
}

// Peek returns all entries without removing them.
func (q *Queue) Peek() ([]Entry, error) {
	// Open file for read with shared lock
	f, err := os.OpenFile(q.queueFile(), os.O_RDONLY, 0644)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("opening queue file: %w", err)
	}
	defer f.Close()

	// Acquire shared lock for read
	if err := lockFileShared(f); err != nil {
		return nil, fmt.Errorf("acquiring file lock: %w", err)
	}
	defer func() { _ = unlockFile(f) }()

	// Read queue file
	data, err := os.ReadFile(q.queueFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading queue file: %w", err)
	}

	// Parse entries
	var entries []Entry
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// Count returns the number of queued entries.
func (q *Queue) Count() (int, error) {
	entries, err := q.Peek()
	if err != nil {
		return 0, err
	}
	return len(entries), nil
}

// Clear removes all entries from the queue.
func (q *Queue) Clear() error {
	if err := os.Remove(q.queueFile()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing queue file: %w", err)
	}
	return nil
}

// splitLines splits data into lines, handling both \n and \r\n.
func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			end := i
			if end > start && data[end-1] == '\r' {
				end--
			}
			lines = append(lines, data[start:end])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

// NudgeQueue manages a town-level nudge queue for cross-agent communication.
// Unlike the session-specific Queue, NudgeQueue is keyed by tmux session name
// and stored at the town level so any agent can queue nudges for any other agent.
type NudgeQueue struct {
	townRoot    string
	sessionName string // tmux session name (e.g., "gt-gastown-witness")
}

// NewNudgeQueue creates a nudge queue for the given tmux session.
func NewNudgeQueue(townRoot, sessionName string) *NudgeQueue {
	return &NudgeQueue{
		townRoot:    townRoot,
		sessionName: sessionName,
	}
}

// queueFile returns the path to this session's nudge queue file.
func (nq *NudgeQueue) queueFile() string {
	return filepath.Join(nq.townRoot, constants.DirRuntime, "nudge-queue", nq.sessionName+".jsonl")
}

// Enqueue adds a nudge message to the queue.
// The content should already be formatted (including any sender prefix).
func (nq *NudgeQueue) Enqueue(content string) error {
	// Ensure queue directory exists
	queueDir := filepath.Dir(nq.queueFile())
	if err := os.MkdirAll(queueDir, 0755); err != nil {
		return fmt.Errorf("creating nudge queue directory: %w", err)
	}

	// Create entry
	entry := Entry{
		Type:      TypeNudge,
		Content:   content,
		Timestamp: time.Now().UnixMilli(),
	}

	// Marshal to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshaling nudge entry: %w", err)
	}

	// Append to queue file with flock
	f, err := os.OpenFile(nq.queueFile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening nudge queue file: %w", err)
	}
	defer f.Close()

	// Acquire exclusive lock for write
	if err := lockFileExclusive(f); err != nil {
		return fmt.Errorf("acquiring nudge queue lock: %w", err)
	}
	defer func() { _ = unlockFile(f) }()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("writing to nudge queue: %w", err)
	}

	return nil
}

// Drain reads all entries from the nudge queue and removes them.
func (nq *NudgeQueue) Drain() ([]Entry, error) {
	f, err := os.OpenFile(nq.queueFile(), os.O_RDWR, 0644)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Empty queue
		}
		return nil, fmt.Errorf("opening nudge queue file: %w", err)
	}
	defer f.Close()

	// Acquire exclusive lock
	if err := lockFileExclusive(f); err != nil {
		return nil, fmt.Errorf("acquiring nudge queue lock: %w", err)
	}
	defer func() { _ = unlockFile(f) }()

	// Read queue file
	data, err := os.ReadFile(nq.queueFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading nudge queue file: %w", err)
	}

	// Parse entries
	var entries []Entry
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	// Clear the queue file
	if err := os.Remove(nq.queueFile()); err != nil && !os.IsNotExist(err) {
		return entries, fmt.Errorf("removing nudge queue file: %w", err)
	}

	return entries, nil
}

// Count returns the number of queued nudges.
func (nq *NudgeQueue) Count() (int, error) {
	data, err := os.ReadFile(nq.queueFile())
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	count := 0
	for _, line := range splitLines(data) {
		if len(line) > 0 {
			count++
		}
	}
	return count, nil
}
