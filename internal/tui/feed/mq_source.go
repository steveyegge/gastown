package feed

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/mrqueue"
)

// MQEventSource reads MQ lifecycle events from mq_events.jsonl
type MQEventSource struct {
	file    *os.File
	events  chan Event
	cancel  context.CancelFunc
	logPath string
}

// NewMQEventSource creates a source that tails MQ events from a beads directory.
func NewMQEventSource(beadsDir string) (*MQEventSource, error) {
	logPath := filepath.Join(beadsDir, "mq_events.jsonl")

	// Create file if it doesn't exist
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		// Ensure directory exists
		if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
			return nil, err
		}
		// Create empty file
		f, err := os.Create(logPath)
		if err != nil {
			return nil, err
		}
		_ = f.Close() //nolint:gosec // G104: best-effort close on file creation
	}

	file, err := os.Open(logPath)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	source := &MQEventSource{
		file:    file,
		events:  make(chan Event, 100),
		cancel:  cancel,
		logPath: logPath,
	}

	go source.tail(ctx)

	return source, nil
}

// NewMQEventSourceFromWorkDir creates an MQ event source by finding the beads directory.
func NewMQEventSourceFromWorkDir(workDir string) (*MQEventSource, error) {
	beadsDir, err := FindBeadsDir(workDir)
	if err != nil {
		return nil, err
	}
	return NewMQEventSource(beadsDir)
}

// tail follows the MQ event log file and sends events.
func (s *MQEventSource) tail(ctx context.Context) {
	defer close(s.events)

	// Seek to end for live tailing
	_, _ = s.file.Seek(0, 2)

	scanner := bufio.NewScanner(s.file)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for scanner.Scan() {
				line := scanner.Text()
				if event := parseMQEventLine(line); event != nil {
					select {
					case s.events <- *event:
					default:
						// Drop event if channel full
					}
				}
			}
		}
	}
}

// Events returns the event channel.
func (s *MQEventSource) Events() <-chan Event {
	return s.events
}

// Close stops the source.
func (s *MQEventSource) Close() error {
	s.cancel()
	return s.file.Close()
}

// parseMQEventLine parses a line from mq_events.jsonl into a feed Event.
func parseMQEventLine(line string) *Event {
	if strings.TrimSpace(line) == "" {
		return nil
	}

	var mqEvent mrqueue.Event
	if err := json.Unmarshal([]byte(line), &mqEvent); err != nil {
		return nil
	}

	// Convert MQ event to feed Event
	feedType := mapMQEventType(mqEvent.Type)
	message := formatMQEventMessage(mqEvent)

	return &Event{
		Time:    mqEvent.Timestamp,
		Type:    feedType,
		Actor:   "refinery",
		Target:  mqEvent.MRID,
		Message: message,
		Rig:     mqEvent.Rig,
		Role:    "refinery",
		Raw:     line,
	}
}

// mapMQEventType maps MQ event types to feed event types.
func mapMQEventType(mqType mrqueue.EventType) string {
	switch mqType {
	case mrqueue.EventMergeStarted:
		return "merge_started"
	case mrqueue.EventMerged:
		return "merged"
	case mrqueue.EventMergeFailed:
		return "merge_failed"
	case mrqueue.EventMergeSkipped:
		return "merge_skipped"
	default:
		return string(mqType)
	}
}

// formatMQEventMessage creates a human-readable message for an MQ event.
func formatMQEventMessage(e mrqueue.Event) string {
	branchInfo := e.Branch
	if e.Target != "" {
		branchInfo += " -> " + e.Target
	}

	switch e.Type {
	case mrqueue.EventMergeStarted:
		return "Merge started: " + branchInfo
	case mrqueue.EventMerged:
		msg := "Merged: " + branchInfo
		if e.MergeCommit != "" {
			// Show short commit SHA
			sha := e.MergeCommit
			if len(sha) > 8 {
				sha = sha[:8]
			}
			msg += " (" + sha + ")"
		}
		return msg
	case mrqueue.EventMergeFailed:
		msg := "Merge failed: " + branchInfo
		if e.Reason != "" {
			msg += " - " + e.Reason
		}
		return msg
	case mrqueue.EventMergeSkipped:
		msg := "Merge skipped: " + branchInfo
		if e.Reason != "" {
			msg += " - " + e.Reason
		}
		return msg
	default:
		return string(e.Type) + ": " + branchInfo
	}
}
