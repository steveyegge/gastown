package feed

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// writeTestEvents writes GtEvent JSON lines to a temporary .events.jsonl file
// and returns the directory path (townRoot).
func writeTestEvents(t *testing.T, events []GtEvent) string {
	t.Helper()
	dir := t.TempDir()
	eventsPath := filepath.Join(dir, ".events.jsonl")
	var lines []string
	for _, ev := range events {
		b, err := json.Marshal(ev)
		if err != nil {
			t.Fatalf("marshal event: %v", err)
		}
		lines = append(lines, string(b))
	}
	if err := os.WriteFile(eventsPath, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write events file: %v", err)
	}
	return dir
}

func TestPrintGtEvents_ReadsAndFormats(t *testing.T) {
	now := time.Now()
	townRoot := writeTestEvents(t, []GtEvent{
		{Timestamp: now.Add(-2 * time.Minute).Format(time.RFC3339), Source: "test", Type: "create", Actor: "gastown/witness", Visibility: "feed", Payload: map[string]interface{}{"message": "created issue"}},
		{Timestamp: now.Add(-1 * time.Minute).Format(time.RFC3339), Source: "test", Type: "sling", Actor: "gastown/crew/joe", Visibility: "feed", Payload: map[string]interface{}{"bead": "gt-abc", "target": "polecat-1"}},
		{Timestamp: now.Format(time.RFC3339), Source: "test", Type: "done", Actor: "gastown/crew/joe", Visibility: "feed", Payload: map[string]interface{}{"bead": "gt-abc"}},
	})

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := PrintGtEvents(townRoot, PrintOptions{Limit: 10})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("PrintGtEvents returned error: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	// Should have 3 lines of output (oldest first)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 output lines, got %d: %q", len(lines), output)
	}

	// First line should be the oldest event (create)
	if !strings.Contains(lines[0], "created issue") {
		t.Errorf("first line should contain 'created issue', got: %s", lines[0])
	}
	// Last line should be the newest event (done)
	if !strings.Contains(lines[2], "done") {
		t.Errorf("last line should contain 'done', got: %s", lines[2])
	}
}

func TestPrintGtEvents_LimitApplied(t *testing.T) {
	now := time.Now()
	var events []GtEvent
	for i := 0; i < 20; i++ {
		events = append(events, GtEvent{
			Timestamp:  now.Add(time.Duration(-20+i) * time.Minute).Format(time.RFC3339),
			Source:     "test",
			Type:       "create",
			Actor:      "test",
			Visibility: "feed",
			Payload:    map[string]interface{}{"message": "event"},
		})
	}
	townRoot := writeTestEvents(t, events)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := PrintGtEvents(townRoot, PrintOptions{Limit: 5})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("PrintGtEvents returned error: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 output lines (limit), got %d", len(lines))
	}
}

func TestPrintGtEvents_SinceFilter(t *testing.T) {
	now := time.Now()
	townRoot := writeTestEvents(t, []GtEvent{
		{Timestamp: now.Add(-2 * time.Hour).Format(time.RFC3339), Source: "test", Type: "create", Actor: "old", Visibility: "feed", Payload: map[string]interface{}{"message": "old event"}},
		{Timestamp: now.Add(-30 * time.Second).Format(time.RFC3339), Source: "test", Type: "create", Actor: "new", Visibility: "feed", Payload: map[string]interface{}{"message": "new event"}},
	})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := PrintGtEvents(townRoot, PrintOptions{Limit: 100, Since: "5m"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("PrintGtEvents returned error: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 event after --since 5m, got %d: %q", len(lines), output)
	}
	if !strings.Contains(lines[0], "new event") {
		t.Errorf("expected recent event, got: %s", lines[0])
	}
}

func TestPrintGtEvents_TypeFilter(t *testing.T) {
	now := time.Now()
	townRoot := writeTestEvents(t, []GtEvent{
		{Timestamp: now.Add(-2 * time.Minute).Format(time.RFC3339), Source: "test", Type: "create", Actor: "a", Visibility: "feed", Payload: map[string]interface{}{"message": "created"}},
		{Timestamp: now.Add(-1 * time.Minute).Format(time.RFC3339), Source: "test", Type: "sling", Actor: "b", Visibility: "feed", Payload: map[string]interface{}{"bead": "gt-1", "target": "p1"}},
		{Timestamp: now.Format(time.RFC3339), Source: "test", Type: "create", Actor: "c", Visibility: "feed", Payload: map[string]interface{}{"message": "created again"}},
	})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := PrintGtEvents(townRoot, PrintOptions{Limit: 100, Type: "create"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("PrintGtEvents returned error: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 create events, got %d: %q", len(lines), output)
	}
}

func TestPrintGtEvents_NoEventsFile(t *testing.T) {
	dir := t.TempDir() // no .events.jsonl
	err := PrintGtEvents(dir, PrintOptions{Limit: 10})
	if err == nil {
		t.Fatal("expected error for missing events file")
	}
	if !strings.Contains(err.Error(), "no events file found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPrintGtEvents_VisibilityFiltering(t *testing.T) {
	now := time.Now()
	townRoot := writeTestEvents(t, []GtEvent{
		{Timestamp: now.Format(time.RFC3339), Source: "test", Type: "create", Actor: "a", Visibility: "feed", Payload: map[string]interface{}{"message": "visible"}},
		{Timestamp: now.Format(time.RFC3339), Source: "test", Type: "create", Actor: "b", Visibility: "internal", Payload: map[string]interface{}{"message": "hidden"}},
		{Timestamp: now.Format(time.RFC3339), Source: "test", Type: "create", Actor: "c", Visibility: "both", Payload: map[string]interface{}{"message": "also visible"}},
	})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := PrintGtEvents(townRoot, PrintOptions{Limit: 100})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("PrintGtEvents returned error: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 visible events, got %d: %q", len(lines), output)
	}
}

func TestPrintGtEvents_InvalidSinceDuration(t *testing.T) {
	now := time.Now()
	townRoot := writeTestEvents(t, []GtEvent{
		{Timestamp: now.Format(time.RFC3339), Source: "test", Type: "create", Actor: "a", Visibility: "feed", Payload: map[string]interface{}{"message": "event"}},
	})

	err := PrintGtEvents(townRoot, PrintOptions{Limit: 10, Since: "notaduration"})
	if err == nil {
		t.Fatal("expected error for invalid --since duration")
	}
	if !strings.Contains(err.Error(), "invalid --since duration") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPrintGtEvents_FollowStreamsAppended(t *testing.T) {
	now := time.Now()
	dir := t.TempDir()
	eventsPath := filepath.Join(dir, ".events.jsonl")

	// Write initial event
	initial, _ := json.Marshal(GtEvent{
		Timestamp: now.Format(time.RFC3339), Source: "test", Type: "create",
		Actor: "a", Visibility: "feed", Payload: map[string]interface{}{"message": "initial"},
	})
	os.WriteFile(eventsPath, append(initial, '\n'), 0644)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var printErr error

	// Run PrintGtEvents with Follow in a goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		printErr = PrintGtEvents(dir, PrintOptions{Limit: 100, Follow: true, Ctx: ctx})
	}()

	// Wait for initial event to be printed, then append a second event
	time.Sleep(500 * time.Millisecond)

	appended, _ := json.Marshal(GtEvent{
		Timestamp: now.Add(1 * time.Second).Format(time.RFC3339), Source: "test", Type: "sling",
		Actor: "b", Visibility: "feed", Payload: map[string]interface{}{"bead": "gt-1", "target": "p1"},
	})
	f, _ := os.OpenFile(eventsPath, os.O_APPEND|os.O_WRONLY, 0644)
	f.Write(append(appended, '\n'))
	f.Close()

	// Wait for the tail loop to pick it up, then cancel
	time.Sleep(500 * time.Millisecond)
	cancel()
	wg.Wait()

	w.Close()
	os.Stdout = oldStdout

	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if printErr != nil {
		t.Logf("PrintGtEvents returned: %v (expected for follow mode)", printErr)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 output lines (initial + appended), got %d: %q", len(lines), output)
	}

	if !strings.Contains(lines[0], "initial") {
		t.Errorf("first line should contain 'initial', got: %s", lines[0])
	}
	// The appended event should have been picked up by the tail loop
	found := false
	for _, line := range lines[1:] {
		if strings.Contains(line, "slung") || strings.Contains(line, "work slung") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected appended event in output, got: %q", output)
	}
}

func TestPrintGtEvents_RigFilter(t *testing.T) {
	now := time.Now()
	townRoot := writeTestEvents(t, []GtEvent{
		{Timestamp: now.Add(-2 * time.Minute).Format(time.RFC3339), Source: "test", Type: "create", Actor: "greenplace/witness", Visibility: "feed", Payload: map[string]interface{}{"message": "greenplace event", "rig": "greenplace"}},
		{Timestamp: now.Add(-1 * time.Minute).Format(time.RFC3339), Source: "test", Type: "create", Actor: "bluecove/witness", Visibility: "feed", Payload: map[string]interface{}{"message": "bluecove event", "rig": "bluecove"}},
		{Timestamp: now.Format(time.RFC3339), Source: "test", Type: "sling", Actor: "greenplace/crew/joe", Visibility: "feed", Payload: map[string]interface{}{"bead": "gt-1", "target": "p1", "rig": "greenplace"}},
	})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := PrintGtEvents(townRoot, PrintOptions{Limit: 100, Rig: "greenplace"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("PrintGtEvents returned error: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 greenplace events, got %d: %q", len(lines), output)
	}
	for _, line := range lines {
		if strings.Contains(line, "bluecove") {
			t.Errorf("rig filter should exclude bluecove events, got: %s", line)
		}
	}
}

func TestPrintGtEvents_ActorFilter(t *testing.T) {
	now := time.Now()
	townRoot := writeTestEvents(t, []GtEvent{
		{Timestamp: now.Add(-2 * time.Minute).Format(time.RFC3339), Source: "test", Type: "sling", Actor: "greenplace/crew/joe", Visibility: "feed", Payload: map[string]interface{}{"bead": "gt-1", "target": "p1"}},
		{Timestamp: now.Add(-1 * time.Minute).Format(time.RFC3339), Source: "test", Type: "sling", Actor: "greenplace/crew/sam", Visibility: "feed", Payload: map[string]interface{}{"bead": "gt-2", "target": "p2"}},
		{Timestamp: now.Format(time.RFC3339), Source: "test", Type: "done", Actor: "greenplace/crew/joe", Visibility: "feed", Payload: map[string]interface{}{"bead": "gt-1"}},
	})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := PrintGtEvents(townRoot, PrintOptions{Limit: 100, Actor: "CREW/JOE"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("PrintGtEvents returned error: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 joe events, got %d: %q", len(lines), output)
	}
	for _, line := range lines {
		if !strings.Contains(strings.ToLower(line), "joe") {
			t.Errorf("actor filter should only include joe events, got: %s", line)
		}
	}
}

func TestPrintGtEvents_ContainsFilter(t *testing.T) {
	now := time.Now()
	townRoot := writeTestEvents(t, []GtEvent{
		{Timestamp: now.Add(-2 * time.Minute).Format(time.RFC3339), Source: "test", Type: "mail", Actor: "mayor", Visibility: "feed", Payload: map[string]interface{}{"subject": "Convoy hotfix", "to": "greenplace/crew/joe"}},
		{Timestamp: now.Add(-1 * time.Minute).Format(time.RFC3339), Source: "test", Type: "done", Actor: "greenplace/crew/sam", Visibility: "feed", Payload: map[string]interface{}{"bead": "gt-123"}},
		{Timestamp: now.Format(time.RFC3339), Source: "test", Type: "merge_failed", Actor: "greenplace/refinery", Visibility: "feed", Payload: map[string]interface{}{"reason": "conflict"}},
	})

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := PrintGtEvents(townRoot, PrintOptions{Limit: 100, Contains: "HOTFIX"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("PrintGtEvents returned error: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 hotfix event, got %d: %q", len(lines), output)
	}
	if !strings.Contains(strings.ToLower(lines[0]), "hotfix") {
		t.Errorf("expected output to contain hotfix, got: %s", lines[0])
	}
}

func TestMatchesFilters(t *testing.T) {
	now := time.Now()
	event := &Event{
		Time:    now,
		Type:    "create",
		Actor:   "greenplace/crew/joe",
		Target:  "gt-abc-123",
		Message: "created issue mol-42",
		Rig:     "greenplace",
	}

	tests := []struct {
		name      string
		sinceTime time.Time
		mol       string
		eventType string
		rig       string
		actor     string
		contains  string
		want      bool
	}{
		{"no filters", time.Time{}, "", "", "", "", "", true},
		{"since matches", now.Add(-1 * time.Minute), "", "", "", "", "", true},
		{"since excludes", now.Add(1 * time.Minute), "", "", "", "", "", false},
		{"mol matches target", time.Time{}, "gt-abc", "", "", "", "", true},
		{"mol matches message", time.Time{}, "mol-42", "", "", "", "", true},
		{"mol no match", time.Time{}, "nonexistent", "", "", "", "", false},
		{"type matches case-insensitive", time.Time{}, "", "CREATE", "", "", "", true},
		{"type no match", time.Time{}, "", "delete", "", "", "", false},
		{"rig matches case-insensitive", time.Time{}, "", "", "GREENPLACE", "", "", true},
		{"rig no match", time.Time{}, "", "", "otherrig", "", "", false},
		{"actor matches", time.Time{}, "", "", "", "crew/joe", "", true},
		{"actor no match", time.Time{}, "", "", "", "crew/sam", "", false},
		{"contains matches message", time.Time{}, "", "", "", "", "ISSUE", true},
		{"contains matches target", time.Time{}, "", "", "", "", "ABC-123", true},
		{"contains no match", time.Time{}, "", "", "", "", "not-here", false},
		{"combined all match", now.Add(-1 * time.Minute), "gt-abc", "create", "greenplace", "crew/joe", "mol-42", true},
		{"combined actor mismatch", now.Add(-1 * time.Minute), "gt-abc", "create", "greenplace", "crew/sam", "mol-42", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesFilters(event, tc.sinceTime, tc.mol, tc.eventType, tc.rig, tc.actor, tc.contains)
			if got != tc.want {
				t.Errorf("matchesFilters() = %v, want %v", got, tc.want)
			}
		})
	}
}
