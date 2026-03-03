// Package agentlog provides a pluggable interface for watching AI agent conversation logs
// and emitting normalized OTEL telemetry events.
//
// Design: AgentAdapter is the extension point. Adding support for a new agent
// (OpenCode, Kiro, etc.) means implementing this interface. The gt agent-log command
// selects the adapter via --agent flag and defaults to "claudecode".
package agentlog

import (
	"context"
	"time"
)

// AgentEvent is a normalized event extracted from an AI agent's conversation log.
// All adapters emit this type so downstream telemetry is agent-agnostic.
type AgentEvent struct {
	AgentType       string    // "claudecode", "opencode", …
	SessionID       string    // Gas Town tmux session name (e.g. "hq-mayor", "gt-wyvern-toast")
	NativeSessionID string    // agent-native session UUID (e.g. Claude Code session UUID from JSONL filename)
	EventType       string    // "text", "tool_use", "tool_result", "thinking", "usage"
	Role            string    // "assistant" or "user"
	Content         string    // text content; empty for "usage" events
	Timestamp       time.Time // original timestamp from the conversation log

	// Token usage fields — non-zero only for EventType == "usage".
	// One "usage" event is emitted per assistant turn (not per content block).
	InputTokens         int // input_tokens from Claude API usage
	OutputTokens        int // output_tokens from Claude API usage
	CacheReadTokens     int // cache_read_input_tokens
	CacheCreationTokens int // cache_creation_input_tokens
}

// AgentAdapter watches an agent's conversation log and streams normalized events.
// Implement this interface to support a new agent type.
type AgentAdapter interface {
	// AgentType returns the adapter identifier (e.g., "claudecode").
	AgentType() string

	// Watch starts watching and returns a channel of events.
	// The channel is closed when ctx is canceled or a fatal error occurs.
	// sessionID is the Gas Town tmux session name used as a log tag.
	// workDir is the agent's working directory (used to locate log files).
	// since filters out JSONL files last modified before this time; use zero
	// to disable filtering (picks up any file regardless of age).
	Watch(ctx context.Context, sessionID, workDir string, since time.Time) (<-chan AgentEvent, error)
}

// NewAdapter returns the AgentAdapter for the given agent type name.
// Returns nil if the agent type is unknown.
func NewAdapter(agentType string) AgentAdapter {
	switch agentType {
	case "claudecode", "":
		return &ClaudeCodeAdapter{}
	case "opencode":
		return &OpenCodeAdapter{}
	default:
		return nil
	}
}
