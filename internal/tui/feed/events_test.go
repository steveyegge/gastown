package feed

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestGtEventsSource_TailsAppendedLines is a regression test for the bug where
// the tail goroutine reused a single bufio.Scanner across poll ticks. Once the
// scanner hit EOF, its internal 'done' flag latched and no subsequent Scan()
// calls re-read the file, so lines appended after the source was started were
// silently dropped.
func TestGtEventsSource_TailsAppendedLines(t *testing.T) {
	dir := t.TempDir()
	eventsPath := filepath.Join(dir, ".events.jsonl")
	if err := os.WriteFile(eventsPath, nil, 0644); err != nil {
		t.Fatalf("create events file: %v", err)
	}

	src, err := NewGtEventsSource(dir)
	if err != nil {
		t.Fatalf("NewGtEventsSource: %v", err)
	}
	defer src.Close()

	// Drain any backlog (should be none for an empty file, but be defensive).
	drain := time.After(200 * time.Millisecond)
draining:
	for {
		select {
		case <-src.Events():
		case <-drain:
			break draining
		}
	}

	// Append a valid JSONL event line after the source is tailing.
	ev := GtEvent{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Source:     "test",
		Type:       "session_start",
		Actor:      "gastown/witness",
		Visibility: "feed",
		Payload:    map[string]interface{}{"rig": "test"},
	}
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	f, err := os.OpenFile(eventsPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		t.Fatalf("append event: %v", err)
	}
	_ = f.Close()

	select {
	case got := <-src.Events():
		if got.Type != "session_start" {
			t.Fatalf("unexpected event type: got %q want %q", got.Type, "session_start")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for appended event; tail goroutine stopped reading after EOF")
	}
}
