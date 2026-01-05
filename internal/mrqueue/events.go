// Package mrqueue provides merge request queue storage and events.
package mrqueue

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// EventType represents the type of MQ lifecycle event.
type EventType string

const (
	// EventMergeStarted indicates refinery began processing an MR.
	EventMergeStarted EventType = "merge_started"
	// EventMerged indicates an MR was successfully merged.
	EventMerged EventType = "merged"
	// EventMergeFailed indicates a merge failed (conflict, tests, etc.).
	EventMergeFailed EventType = "merge_failed"
	// EventMergeSkipped indicates an MR was skipped (already merged, etc.).
	EventMergeSkipped EventType = "merge_skipped"
)

// Event represents a single MQ lifecycle event.
type Event struct {
	Timestamp   time.Time `json:"timestamp"`
	Type        EventType `json:"type"`
	MRID        string    `json:"mr_id"`
	Branch      string    `json:"branch"`
	Target      string    `json:"target"`
	Worker      string    `json:"worker,omitempty"`
	SourceIssue string    `json:"source_issue,omitempty"`
	Rig         string    `json:"rig,omitempty"`
	MergeCommit string    `json:"merge_commit,omitempty"` // For merged events
	Reason      string    `json:"reason,omitempty"`       // For failed/skipped events
}

// EventLogger handles writing MQ events to the event log.
type EventLogger struct {
	logPath string
	mu      sync.Mutex
}

// NewEventLogger creates a new EventLogger for the given beads directory.
func NewEventLogger(beadsDir string) *EventLogger {
	return &EventLogger{
		logPath: filepath.Join(beadsDir, "mq_events.jsonl"),
	}
}

// NewEventLoggerFromRig creates an EventLogger for the given rig path.
func NewEventLoggerFromRig(rigPath string) *EventLogger {
	return NewEventLogger(filepath.Join(rigPath, ".beads"))
}

// LogEvent writes an event to the MQ event log.
func (l *EventLogger) LogEvent(event Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Ensure timestamp is set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Ensure log directory exists
	if err := os.MkdirAll(filepath.Dir(l.logPath), 0755); err != nil {
		return fmt.Errorf("creating log directory: %w", err)
	}

	// Marshal event to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshaling event: %w", err)
	}

	// Append to log file
	f, err := os.OpenFile(l.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("opening event log: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("writing event: %w", err)
	}

	return nil
}

// LogMergeStarted logs a merge_started event.
func (l *EventLogger) LogMergeStarted(mr *MR) error {
	return l.LogEvent(Event{
		Type:        EventMergeStarted,
		MRID:        mr.ID,
		Branch:      mr.Branch,
		Target:      mr.Target,
		Worker:      mr.Worker,
		SourceIssue: mr.SourceIssue,
		Rig:         mr.Rig,
	})
}

// LogMerged logs a merged event.
func (l *EventLogger) LogMerged(mr *MR, mergeCommit string) error {
	return l.LogEvent(Event{
		Type:        EventMerged,
		MRID:        mr.ID,
		Branch:      mr.Branch,
		Target:      mr.Target,
		Worker:      mr.Worker,
		SourceIssue: mr.SourceIssue,
		Rig:         mr.Rig,
		MergeCommit: mergeCommit,
	})
}

// LogMergeFailed logs a merge_failed event.
func (l *EventLogger) LogMergeFailed(mr *MR, reason string) error {
	return l.LogEvent(Event{
		Type:        EventMergeFailed,
		MRID:        mr.ID,
		Branch:      mr.Branch,
		Target:      mr.Target,
		Worker:      mr.Worker,
		SourceIssue: mr.SourceIssue,
		Rig:         mr.Rig,
		Reason:      reason,
	})
}

// LogMergeSkipped logs a merge_skipped event.
func (l *EventLogger) LogMergeSkipped(mr *MR, reason string) error {
	return l.LogEvent(Event{
		Type:        EventMergeSkipped,
		MRID:        mr.ID,
		Branch:      mr.Branch,
		Target:      mr.Target,
		Worker:      mr.Worker,
		SourceIssue: mr.SourceIssue,
		Rig:         mr.Rig,
		Reason:      reason,
	})
}

// LogPath returns the path to the event log file.
func (l *EventLogger) LogPath() string {
	return l.logPath
}
