package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/runtime"
	"github.com/steveyegge/gastown/internal/workspace"
)

// hookInput represents the JSON input from LLM runtime hooks.
// Claude Code sends this on stdin for SessionStart hooks.
type hookInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Source         string `json:"source"` // startup, resume, clear, compact
}

// readHookSessionID reads session ID from available sources in hook mode.
// Priority: stdin JSON, GT_SESSION_ID env, CLAUDE_SESSION_ID env, auto-generate.
func readHookSessionID() (sessionID, source string) {
	// 1. Try reading stdin JSON (Claude Code format)
	if input := readStdinJSON(); input != nil {
		if input.SessionID != "" {
			return input.SessionID, input.Source
		}
	}

	// 2. Environment variables
	if id := os.Getenv("GT_SESSION_ID"); id != "" {
		return id, ""
	}
	if id := os.Getenv("CLAUDE_SESSION_ID"); id != "" {
		return id, ""
	}

	// 3. Auto-generate
	return uuid.New().String(), ""
}

// readStdinJSON attempts to read and parse JSON from stdin.
// Returns nil if stdin is empty, not a pipe, or invalid JSON.
func readStdinJSON() *hookInput {
	// Check if stdin has data (non-blocking)
	stat, err := os.Stdin.Stat()
	if err != nil {
		return nil
	}

	// Only read if stdin is a pipe or has data
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		// stdin is a terminal, not a pipe - no data to read
		return nil
	}

	// Read first line (JSON should be on one line)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return nil
	}

	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}

	var input hookInput
	if err := json.Unmarshal([]byte(line), &input); err != nil {
		return nil
	}

	return &input
}

// persistSessionID writes the session ID to .runtime/session_id
// This allows subsequent gt prime calls to find the session ID.
func persistSessionID(dir, sessionID string) {
	runtimeDir := filepath.Join(dir, ".runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		return // Non-fatal
	}

	sessionFile := filepath.Join(runtimeDir, "session_id")
	content := fmt.Sprintf("%s\n%s\n", sessionID, time.Now().Format(time.RFC3339))
	_ = os.WriteFile(sessionFile, []byte(content), 0644) // Non-fatal
}

// ReadPersistedSessionID reads a previously persisted session ID.
// Checks cwd first, then town root.
// Returns empty string if not found.
func ReadPersistedSessionID() string {
	// Try cwd first
	cwd, err := os.Getwd()
	if err == nil {
		if id := readSessionFile(cwd); id != "" {
			return id
		}
	}

	// Try town root
	townRoot, err := workspace.FindFromCwd()
	if err == nil && townRoot != "" {
		if id := readSessionFile(townRoot); id != "" {
			return id
		}
	}

	return ""
}

func readSessionFile(dir string) string {
	sessionFile := filepath.Join(dir, ".runtime", "session_id")
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}
	return ""
}

// resolveSessionIDForPrime finds the session ID from available sources.
// Priority: GT_SESSION_ID env, CLAUDE_SESSION_ID env, persisted file, fallback.
func resolveSessionIDForPrime(actor string) string {
	// 1. Try runtime's session ID lookup (checks GT_SESSION_ID_ENV, then CLAUDE_SESSION_ID)
	if id := runtime.SessionIDFromEnv(); id != "" {
		return id
	}

	// 2. Persisted session file (from gt prime --hook)
	if id := ReadPersistedSessionID(); id != "" {
		return id
	}

	// 3. Fallback to generated identifier
	return fmt.Sprintf("%s-%d", actor, os.Getpid())
}

// emitSessionEvent emits a session_start event for seance discovery.
// The event is written to ~/gt/.events.jsonl and can be queried via gt seance.
// Session ID resolution order: GT_SESSION_ID, CLAUDE_SESSION_ID, persisted file, fallback.
func emitSessionEvent(ctx RoleContext) {
	if ctx.Role == RoleUnknown {
		return
	}

	// Get agent identity for the actor field
	actor := getAgentIdentity(ctx)
	if actor == "" {
		return
	}

	// Get session ID from multiple sources
	sessionID := resolveSessionIDForPrime(actor)

	// Determine topic from hook state or default
	topic := ""
	if ctx.Role == RoleWitness || ctx.Role == RoleRefinery || ctx.Role == RoleDeacon {
		topic = "patrol"
	}

	// Emit the event
	payload := events.SessionPayload(sessionID, actor, topic, ctx.WorkDir)
	_ = events.LogFeed(events.TypeSessionStart, actor, payload)
}

// outputSessionMetadata prints a structured metadata line for seance discovery.
// Format: [GAS TOWN] role:<role> pid:<pid> session:<session_id>
// This enables gt seance to discover sessions from gt prime output.
func outputSessionMetadata(ctx RoleContext) {
	if ctx.Role == RoleUnknown {
		return
	}

	// Get agent identity for the role field
	actor := getAgentIdentity(ctx)
	if actor == "" {
		return
	}

	// Get session ID from multiple sources
	sessionID := resolveSessionIDForPrime(actor)

	// Output structured metadata line
	fmt.Printf("[GAS TOWN] role:%s pid:%d session:%s\n", actor, os.Getpid(), sessionID)
}
