// Package keepalive provides agent activity signaling via file touch.
//
// This package uses a best-effort design: all write operations silently ignore
// errors. This is intentional because:
//
//  1. Keepalive signals are non-critical - the system works without them
//  2. Failures (disk full, permissions, etc.) should not interrupt gt commands
//  3. The daemon tolerates missing signals by using timeouts
//
// Functions in this package write JSON files to .runtime/ or daemon/ directories.
// These files are used by the daemon to detect agent activity and implement
// features like exponential backoff during idle periods.
package keepalive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/workspace"
)

// State represents the keepalive file contents.
type State struct {
	LastCommand string    `json:"last_command"`
	Timestamp   time.Time `json:"timestamp"`
}

// Touch updates the keepalive file in the workspace's .runtime directory.
// It silently ignores errors (best-effort signaling).
func Touch(command string) {
	TouchWithArgs(command, nil)
}

// TouchWithArgs updates the keepalive file with the full command including args.
// It silently ignores errors (best-effort signaling).
func TouchWithArgs(command string, args []string) {
	root, err := workspace.FindFromCwd()
	if err != nil || root == "" {
		return // Not in a workspace, nothing to do
	}

	// Build full command string
	fullCmd := command
	if len(args) > 0 {
		fullCmd = command + " " + strings.Join(args, " ")
	}

	TouchInWorkspace(root, fullCmd)
}

// TouchInWorkspace updates the keepalive file in a specific workspace.
// It silently ignores errors (best-effort signaling).
func TouchInWorkspace(workspaceRoot, command string) {
	runtimeDir := filepath.Join(workspaceRoot, ".runtime")

	// Ensure .runtime directory exists
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		return
	}

	state := State{
		LastCommand: command,
		Timestamp:   time.Now().UTC(),
	}

	data, err := json.Marshal(state)
	if err != nil {
		return
	}

	keepalivePath := filepath.Join(runtimeDir, "keepalive.json")
	_ = os.WriteFile(keepalivePath, data, 0644) // non-fatal: status file for debugging
}

// Read returns the current keepalive state for the workspace.
// Returns nil if the file doesn't exist or can't be read.
func Read(workspaceRoot string) *State {
	keepalivePath := filepath.Join(workspaceRoot, ".runtime", "keepalive.json")

	data, err := os.ReadFile(keepalivePath)
	if err != nil {
		return nil
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil
	}

	return &state
}

// Age returns how old the keepalive signal is.
// Returns a very large duration if the state is nil.
func (s *State) Age() time.Duration {
	if s == nil {
		return 24 * time.Hour * 365 // No keepalive
	}
	return time.Since(s.Timestamp)
}
