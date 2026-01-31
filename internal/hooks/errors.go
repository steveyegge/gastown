// Package hooks provides hook error logging with deduplication for Claude Code hooks.
package hooks

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/workspace"
)

// HookError represents a hook execution error.
type HookError struct {
	Timestamp string `json:"ts"`
	HookType  string `json:"hook_type"`
	Command   string `json:"command"`
	ExitCode  int    `json:"exit_code"`
	Stderr    string `json:"stderr,omitempty"`
	Role      string `json:"role"`
	Hash      string `json:"hash"` // Deduplication key
	Count     int    `json:"count"` // Number of occurrences
}

// ErrorLog manages hook error logging with deduplication.
type ErrorLog struct {
	townRoot    string
	dedupWindow time.Duration
	mu          sync.Mutex
}

const (
	// HookErrorsFile is the name of the hook errors log file.
	HookErrorsFile = ".hook-errors.jsonl"

	// DefaultDedupWindow is the default deduplication window.
	DefaultDedupWindow = 60 * time.Second

	// MaxErrorsToKeep is the maximum number of error entries to keep.
	MaxErrorsToKeep = 100
)

// NewErrorLog creates a new hook error log.
func NewErrorLog(townRoot string) *ErrorLog {
	return &ErrorLog{
		townRoot:    townRoot,
		dedupWindow: DefaultDedupWindow,
	}
}

// computeHash computes a deduplication hash for an error.
// Uses hook type, command, and role to identify duplicate errors.
func computeHash(hookType, command, role string) string {
	h := sha256.New()
	h.Write([]byte(hookType))
	h.Write([]byte("|"))
	h.Write([]byte(command))
	h.Write([]byte("|"))
	h.Write([]byte(role))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// ReportError logs a hook error with deduplication.
// Returns true if the error was logged, false if it was deduplicated.
func (el *ErrorLog) ReportError(hookType, command string, exitCode int, stderr, role string) (bool, error) {
	el.mu.Lock()
	defer el.mu.Unlock()

	hash := computeHash(hookType, command, role)
	now := time.Now().UTC()

	// Read existing errors
	errors, err := el.readErrors()
	if err != nil {
		// If we can't read, start fresh
		errors = []HookError{}
	}

	// Check for recent duplicate
	dedupCutoff := now.Add(-el.dedupWindow)
	for i := len(errors) - 1; i >= 0; i-- {
		e := errors[i]
		if e.Hash == hash {
			ts, err := time.Parse(time.RFC3339, e.Timestamp)
			if err == nil && ts.After(dedupCutoff) {
				// Duplicate within window - update count but don't log
				errors[i].Count++
				errors[i].Timestamp = now.Format(time.RFC3339)
				if err := el.writeErrors(errors); err != nil {
					return false, err
				}
				return false, nil
			}
		}
	}

	// Truncate stderr
	if len(stderr) > 500 {
		stderr = stderr[:500] + "..."
	}

	// Create new error entry
	newError := HookError{
		Timestamp: now.Format(time.RFC3339),
		HookType:  hookType,
		Command:   command,
		ExitCode:  exitCode,
		Stderr:    stderr,
		Role:      role,
		Hash:      hash,
		Count:     1,
	}

	// Append and trim to max size
	errors = append(errors, newError)
	if len(errors) > MaxErrorsToKeep {
		errors = errors[len(errors)-MaxErrorsToKeep:]
	}

	// Write back
	if err := el.writeErrors(errors); err != nil {
		return false, err
	}

	// Also log to events feed
	_ = events.LogAudit(events.TypeHookError, role, events.HookErrorPayload(hookType, command, exitCode, stderr, role))

	return true, nil
}

// GetRecentErrors returns recent hook errors.
func (el *ErrorLog) GetRecentErrors(limit int) ([]HookError, error) {
	el.mu.Lock()
	defer el.mu.Unlock()

	errors, err := el.readErrors()
	if err != nil {
		return nil, err
	}

	if limit > 0 && len(errors) > limit {
		errors = errors[len(errors)-limit:]
	}

	// Return in reverse chronological order
	reversed := make([]HookError, len(errors))
	for i, e := range errors {
		reversed[len(errors)-1-i] = e
	}

	return reversed, nil
}

// GetErrorsSince returns errors since a given time.
func (el *ErrorLog) GetErrorsSince(since time.Time) ([]HookError, error) {
	el.mu.Lock()
	defer el.mu.Unlock()

	errors, err := el.readErrors()
	if err != nil {
		return nil, err
	}

	var filtered []HookError
	for _, e := range errors {
		ts, err := time.Parse(time.RFC3339, e.Timestamp)
		if err == nil && ts.After(since) {
			filtered = append(filtered, e)
		}
	}

	return filtered, nil
}

// ClearErrors clears all logged errors.
func (el *ErrorLog) ClearErrors() error {
	el.mu.Lock()
	defer el.mu.Unlock()

	path := el.errorsPath()
	return os.Remove(path)
}

// errorsPath returns the path to the hook errors file.
func (el *ErrorLog) errorsPath() string {
	return filepath.Join(el.townRoot, ".runtime", HookErrorsFile)
}

// readErrors reads the errors from the log file.
func (el *ErrorLog) readErrors() ([]HookError, error) {
	path := el.errorsPath()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []HookError{}, nil
	}
	if err != nil {
		return nil, err
	}

	var errors []HookError
	lines := splitLines(data)
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var e HookError
		if err := json.Unmarshal(line, &e); err != nil {
			continue // Skip malformed entries
		}
		errors = append(errors, e)
	}

	return errors, nil
}

// writeErrors writes the errors to the log file.
func (el *ErrorLog) writeErrors(errors []HookError) error {
	path := el.errorsPath()

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// Write all errors
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, e := range errors {
		data, err := json.Marshal(e)
		if err != nil {
			continue
		}
		if _, err := f.Write(data); err != nil {
			return err
		}
		if _, err := f.Write([]byte("\n")); err != nil {
			return err
		}
	}

	return nil
}

// splitLines splits data by newlines.
func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

// ReportHookError is a convenience function for reporting hook errors.
// It finds the town root automatically.
func ReportHookError(hookType, command string, exitCode int, stderr, role string) (bool, error) {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return false, fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	log := NewErrorLog(townRoot)
	return log.ReportError(hookType, command, exitCode, stderr, role)
}

// GetRecentHookErrors is a convenience function for getting recent errors.
func GetRecentHookErrors(limit int) ([]HookError, error) {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return nil, fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	log := NewErrorLog(townRoot)
	return log.GetRecentErrors(limit)
}
