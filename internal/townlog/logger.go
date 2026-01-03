// Package townlog provides centralized logging for Gas Town agent lifecycle events.
package townlog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// EventType represents the type of agent lifecycle event.
type EventType string

const (
	// EventSpawn indicates a new agent was created.
	EventSpawn EventType = "spawn"
	// EventWake indicates an agent was resumed.
	EventWake EventType = "wake"
	// EventNudge indicates a message was injected into an agent.
	EventNudge EventType = "nudge"
	// EventHandoff indicates an agent handed off to a fresh session.
	EventHandoff EventType = "handoff"
	// EventDone indicates an agent finished its work.
	EventDone EventType = "done"
	// EventCrash indicates an agent exited unexpectedly.
	EventCrash EventType = "crash"
	// EventKill indicates an agent was killed intentionally.
	EventKill EventType = "kill"
	// EventCallback indicates a callback was processed during patrol.
	EventCallback EventType = "callback"

	// Witness patrol events
	EventPatrolStarted  EventType = "patrol_started"
	EventPolecatChecked EventType = "polecat_checked"
	EventPolecatNudged  EventType = "polecat_nudged"
	EventEscalationSent EventType = "escalation_sent"
	EventPatrolComplete EventType = "patrol_complete"
)

// Event represents a single agent lifecycle event.
type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Type      EventType `json:"type"`
	Agent     string    `json:"agent"`            // e.g., "gastown/crew/max" or "gastown/polecats/Toast"
	Context   string    `json:"context,omitempty"` // Additional context (issue ID, error message, etc.)
}

// Logger handles writing events to the town log file.
type Logger struct {
	logPath string
	mu      sync.Mutex
}

// logDir returns the directory for town logs.
func logDir(townRoot string) string {
	return filepath.Join(townRoot, "logs")
}

// logPath returns the path to the town log file.
func logPath(townRoot string) string {
	return filepath.Join(logDir(townRoot), "town.log")
}

// NewLogger creates a new Logger for the given town root.
func NewLogger(townRoot string) *Logger {
	return &Logger{
		logPath: logPath(townRoot),
	}
}

// LogEvent logs a single event to the town log.
func (l *Logger) LogEvent(event Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Ensure log directory exists
	if err := os.MkdirAll(filepath.Dir(l.logPath), 0755); err != nil {
		return fmt.Errorf("creating log directory: %w", err)
	}

	// Open file for appending
	f, err := os.OpenFile(l.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}
	defer f.Close()

	// Write human-readable log line
	line := formatLogLine(event)
	if _, err := f.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("writing log line: %w", err)
	}

	return nil
}

// Log is a convenience method that creates an Event and logs it.
func (l *Logger) Log(eventType EventType, agent, context string) error {
	return l.LogEvent(Event{
		Timestamp: time.Now(),
		Type:      eventType,
		Agent:     agent,
		Context:   context,
	})
}

// formatLogLine formats an event as a human-readable log line.
// Format: 2025-12-26 15:30:45 [spawn] gastown/crew/max spawned for gt-xyz
func formatLogLine(e Event) string {
	ts := e.Timestamp.Format("2006-01-02 15:04:05")

	var detail string
	switch e.Type {
	case EventSpawn:
		if e.Context != "" {
			detail = fmt.Sprintf("spawned for %s", e.Context)
		} else {
			detail = "spawned"
		}
	case EventWake:
		detail = "resumed"
		if e.Context != "" {
			detail += fmt.Sprintf(" (%s)", e.Context)
		}
	case EventNudge:
		if e.Context != "" {
			detail = fmt.Sprintf("nudged with %q", truncate(e.Context, 50))
		} else {
			detail = "nudged"
		}
	case EventHandoff:
		detail = "handed off"
		if e.Context != "" {
			detail += fmt.Sprintf(" (%s)", e.Context)
		}
	case EventDone:
		if e.Context != "" {
			detail = fmt.Sprintf("completed %s", e.Context)
		} else {
			detail = "completed work"
		}
	case EventCrash:
		if e.Context != "" {
			detail = fmt.Sprintf("exited unexpectedly (%s)", e.Context)
		} else {
			detail = "exited unexpectedly"
		}
	case EventKill:
		if e.Context != "" {
			detail = fmt.Sprintf("killed (%s)", e.Context)
		} else {
			detail = "killed"
		}
	case EventCallback:
		if e.Context != "" {
			detail = fmt.Sprintf("callback: %s", e.Context)
		} else {
			detail = "callback processed"
		}
	case EventPatrolStarted:
		if e.Context != "" {
			detail = fmt.Sprintf("started patrol (%s)", e.Context)
		} else {
			detail = "started patrol"
		}
	case EventPolecatChecked:
		if e.Context != "" {
			detail = fmt.Sprintf("checked polecat %s", e.Context)
		} else {
			detail = "checked polecat"
		}
	case EventPolecatNudged:
		if e.Context != "" {
			detail = fmt.Sprintf("nudged polecat (%s)", e.Context)
		} else {
			detail = "nudged polecat"
		}
	case EventEscalationSent:
		if e.Context != "" {
			detail = fmt.Sprintf("escalated (%s)", e.Context)
		} else {
			detail = "escalated"
		}
	case EventPatrolComplete:
		if e.Context != "" {
			detail = fmt.Sprintf("patrol complete (%s)", e.Context)
		} else {
			detail = "patrol complete"
		}
	default:
		detail = string(e.Type)
		if e.Context != "" {
			detail += fmt.Sprintf(" (%s)", e.Context)
		}
	}

	return fmt.Sprintf("%s [%s] %s %s", ts, e.Type, e.Agent, detail)
}

// truncate shortens a string to max length with ellipsis.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// ReadEvents reads all events from the log file.
// Useful for filtering and analysis.
func ReadEvents(townRoot string) ([]Event, error) {
	path := logPath(townRoot)

	content, err := os.ReadFile(path) //nolint:gosec // G304: path is constructed from trusted townRoot
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No log file yet
		}
		return nil, fmt.Errorf("reading log file: %w", err)
	}

	return ParseLogLines(string(content))
}

// ParseLogLines parses log lines back into Events.
// This is the inverse of formatLogLine for filtering.
func ParseLogLines(content string) ([]Event, error) {
	var events []Event
	lines := splitLines(content)

	for _, line := range lines {
		if line == "" {
			continue
		}
		event, err := parseLogLine(line)
		if err != nil {
			continue // Skip malformed lines
		}
		events = append(events, event)
	}

	return events, nil
}

// parseLogLine parses a single log line into an Event.
// Format: 2025-12-26 15:30:45 [spawn] gastown/crew/max spawned for gt-xyz
func parseLogLine(line string) (Event, error) {
	var event Event

	// Parse timestamp (first 19 chars: "2006-01-02 15:04:05")
	if len(line) < 19 {
		return event, fmt.Errorf("line too short")
	}
	ts, err := time.Parse("2006-01-02 15:04:05", line[:19])
	if err != nil {
		return event, fmt.Errorf("parsing timestamp: %w", err)
	}
	event.Timestamp = ts

	// Find event type in brackets
	rest := line[20:] // Skip timestamp and space
	if len(rest) < 3 || rest[0] != '[' {
		return event, fmt.Errorf("missing event type")
	}

	closeBracket := -1
	for i, c := range rest {
		if c == ']' {
			closeBracket = i
			break
		}
	}
	if closeBracket < 0 {
		return event, fmt.Errorf("unclosed bracket")
	}

	event.Type = EventType(rest[1:closeBracket])

	// Rest is " agent details"
	rest = rest[closeBracket+1:]
	if len(rest) < 2 || rest[0] != ' ' {
		return event, fmt.Errorf("missing agent")
	}
	rest = rest[1:]

	// Find first space after agent
	spaceIdx := -1
	for i, c := range rest {
		if c == ' ' {
			spaceIdx = i
			break
		}
	}
	if spaceIdx < 0 {
		event.Agent = rest
	} else {
		event.Agent = rest[:spaceIdx]
		// The rest is context info (not worth parsing further)
	}

	return event, nil
}

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

// TailEvents returns the last n events from the log.
func TailEvents(townRoot string, n int) ([]Event, error) {
	events, err := ReadEvents(townRoot)
	if err != nil {
		return nil, err
	}
	if len(events) <= n {
		return events, nil
	}
	return events[len(events)-n:], nil
}

// FilterEvents returns events matching the filter criteria.
type Filter struct {
	Type  EventType // Filter by event type (empty for all)
	Agent string    // Filter by agent prefix (empty for all)
	Since time.Time // Filter by time (zero for all)
}

// FilterEvents applies a filter to events.
func FilterEvents(events []Event, f Filter) []Event {
	var result []Event
	for _, e := range events {
		if f.Type != "" && e.Type != f.Type {
			continue
		}
		if f.Agent != "" && !hasPrefix(e.Agent, f.Agent) {
			continue
		}
		if !f.Since.IsZero() && e.Timestamp.Before(f.Since) {
			continue
		}
		result = append(result, e)
	}
	return result
}

func hasPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return s[:len(prefix)] == prefix
}
