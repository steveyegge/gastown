package archive

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Event types for the journal.
const (
	EventTypeAppend  = "APPEND"  // New lines added to the transcript
	EventTypeReplace = "REPLACE" // Lines replaced due to scroll/redraw
	EventTypeCommit  = "COMMIT"  // Checkpoint marking a stable state
)

// JournalEvent is the interface for all journal event types.
type JournalEvent interface {
	// Type returns the event type (APPEND, REPLACE, COMMIT).
	Type() string

	// Encode returns the JSON-encoded event.
	Encode() (string, error)
}

// AppendEvent records new lines added to the transcript.
type AppendEvent struct {
	EventType string    `json:"type"`
	Timestamp time.Time `json:"ts"`
	Lines     []string  `json:"lines"`
}

// Type returns the event type.
func (e *AppendEvent) Type() string {
	return EventTypeAppend
}

// Encode returns the JSON-encoded event.
func (e *AppendEvent) Encode() (string, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ReplaceEvent records lines that were replaced due to scroll or redraw.
type ReplaceEvent struct {
	EventType string    `json:"type"`
	Timestamp time.Time `json:"ts"`
	StartLine int       `json:"start"` // 0-indexed line number
	Lines     []string  `json:"lines"` // Replacement content
}

// Type returns the event type.
func (e *ReplaceEvent) Type() string {
	return EventTypeReplace
}

// Encode returns the JSON-encoded event.
func (e *ReplaceEvent) Encode() (string, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// CommitEvent marks a checkpoint in the transcript.
// This indicates a stable state that can be used for replay.
type CommitEvent struct {
	EventType  string    `json:"type"`
	Timestamp  time.Time `json:"ts"`
	LineCount  int       `json:"line_count"`  // Total lines at this point
	Checksum   string    `json:"checksum"`    // Hash of committed content
}

// Type returns the event type.
func (e *CommitEvent) Type() string {
	return EventTypeCommit
}

// Encode returns the JSON-encoded event.
func (e *CommitEvent) Encode() (string, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Journal manages reading and writing journal events to a file.
type Journal struct {
	path   string
	file   *os.File
	writer *bufio.Writer
}

// OpenJournal opens or creates a journal file for writing.
func OpenJournal(path string) (*Journal, error) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening journal file: %w", err)
	}

	return &Journal{
		path:   path,
		file:   file,
		writer: bufio.NewWriter(file),
	}, nil
}

// Write writes an event to the journal.
func (j *Journal) Write(event JournalEvent) error {
	encoded, err := event.Encode()
	if err != nil {
		return fmt.Errorf("encoding event: %w", err)
	}

	if _, err := j.writer.WriteString(encoded + "\n"); err != nil {
		return fmt.Errorf("writing event: %w", err)
	}

	return j.writer.Flush()
}

// WriteAppend writes an append event with the given lines.
func (j *Journal) WriteAppend(lines []string) error {
	return j.Write(&AppendEvent{
		EventType: EventTypeAppend,
		Timestamp: time.Now(),
		Lines:     lines,
	})
}

// WriteReplace writes a replace event.
func (j *Journal) WriteReplace(startLine int, lines []string) error {
	return j.Write(&ReplaceEvent{
		EventType: EventTypeReplace,
		Timestamp: time.Now(),
		StartLine: startLine,
		Lines:     lines,
	})
}

// WriteCommit writes a commit checkpoint.
func (j *Journal) WriteCommit(lineCount int, checksum string) error {
	return j.Write(&CommitEvent{
		EventType: EventTypeCommit,
		Timestamp: time.Now(),
		LineCount: lineCount,
		Checksum:  checksum,
	})
}

// Close flushes and closes the journal file.
func (j *Journal) Close() error {
	if err := j.writer.Flush(); err != nil {
		return fmt.Errorf("flushing journal: %w", err)
	}
	return j.file.Close()
}

// Path returns the journal file path.
func (j *Journal) Path() string {
	return j.path
}

// ReadJournal reads all events from a journal file.
func ReadJournal(path string) ([]JournalEvent, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening journal file: %w", err)
	}
	defer file.Close()

	var events []JournalEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		event, err := decodeEvent(line)
		if err != nil {
			return nil, fmt.Errorf("decoding event: %w", err)
		}
		events = append(events, event)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading journal: %w", err)
	}

	return events, nil
}

// decodeEvent parses a JSON line into the appropriate event type.
func decodeEvent(line string) (JournalEvent, error) {
	// First, decode just the type field
	var typeOnly struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(line), &typeOnly); err != nil {
		return nil, fmt.Errorf("parsing event type: %w", err)
	}

	// Then decode the full event based on type
	switch typeOnly.Type {
	case EventTypeAppend:
		var event AppendEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, fmt.Errorf("parsing append event: %w", err)
		}
		return &event, nil
	case EventTypeReplace:
		var event ReplaceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, fmt.Errorf("parsing replace event: %w", err)
		}
		return &event, nil
	case EventTypeCommit:
		var event CommitEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, fmt.Errorf("parsing commit event: %w", err)
		}
		return &event, nil
	default:
		return nil, fmt.Errorf("unknown event type: %s", typeOnly.Type)
	}
}
