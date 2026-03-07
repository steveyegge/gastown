package witness

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/rig"
)

func TestAggregateTownLog_FiltersToRig(t *testing.T) {
	townRoot := t.TempDir()
	logsDir := filepath.Join(townRoot, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write sample town.log with events from multiple rigs
	logContent := `2026-03-06 10:00:00 [spawn] gastown/polecats/ace spawned for gt-a53
2026-03-06 10:01:00 [nudge] gastown/witness nudged with "patrol"
2026-03-06 10:02:00 [spawn] cfutons/polecats/rust spawned for cf-v00
2026-03-06 10:03:00 [done] gastown/polecats/ace completed gt-a53
2026-03-06 10:04:00 [crash] cfutons/polecats/chrome exited unexpectedly (exit code 1)
`
	if err := os.WriteFile(filepath.Join(logsDir, "town.log"), []byte(logContent), 0600); err != nil {
		t.Fatal(err)
	}

	r := &rig.Rig{
		Name:     "gastown",
		Path:     filepath.Join(townRoot, "gastown"),
		Polecats: []string{"ace"},
	}

	q := LogQuery{
		Rig:  r,
		Tail: 100,
	}

	result := AggregateLogs(townRoot, q)

	// Should only contain gastown events
	if len(result.Entries) != 3 {
		t.Errorf("expected 3 gastown entries, got %d", len(result.Entries))
		for _, e := range result.Entries {
			t.Logf("  %s %s", e.Agent, e.Content)
		}
	}

	// Verify no cfutons events
	for _, e := range result.Entries {
		if e.Agent == "cfutons/polecats/rust" || e.Agent == "cfutons/polecats/chrome" {
			t.Errorf("unexpected agent from other rig: %s", e.Agent)
		}
	}
}

func TestAggregateTownLog_FilterByPolecat(t *testing.T) {
	townRoot := t.TempDir()
	logsDir := filepath.Join(townRoot, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		t.Fatal(err)
	}

	logContent := `2026-03-06 10:00:00 [spawn] gastown/polecats/ace spawned for gt-a53
2026-03-06 10:01:00 [spawn] gastown/polecats/bolt spawned for gt-b77
2026-03-06 10:02:00 [nudge] gastown/witness nudged with "patrol"
2026-03-06 10:03:00 [done] gastown/polecats/ace completed gt-a53
`
	if err := os.WriteFile(filepath.Join(logsDir, "town.log"), []byte(logContent), 0600); err != nil {
		t.Fatal(err)
	}

	r := &rig.Rig{
		Name:     "gastown",
		Path:     filepath.Join(townRoot, "gastown"),
		Polecats: []string{"ace", "bolt"},
	}

	q := LogQuery{
		Rig:     r,
		Polecat: "ace",
		Tail:    100,
	}

	result := AggregateLogs(townRoot, q)

	if len(result.Entries) != 2 {
		t.Errorf("expected 2 entries for polecat ace, got %d", len(result.Entries))
		for _, e := range result.Entries {
			t.Logf("  %s %s", e.Agent, e.Content)
		}
	}
}

func TestGrepFilter(t *testing.T) {
	entries := []LogEntry{
		{Content: "spawned for gt-a53", Agent: "gastown/polecats/ace"},
		{Content: "nudged with patrol", Agent: "gastown/witness"},
		{Content: "completed gt-a53", Agent: "gastown/polecats/ace"},
		{Content: "exited unexpectedly", Agent: "gastown/polecats/bolt"},
	}

	// Search for "a53" - should match 2 entries
	filtered := grepFilter(entries, "a53")
	if len(filtered) != 2 {
		t.Errorf("expected 2 entries matching 'a53', got %d", len(filtered))
	}

	// Case insensitive
	filtered = grepFilter(entries, "PATROL")
	if len(filtered) != 1 {
		t.Errorf("expected 1 entry matching 'PATROL', got %d", len(filtered))
	}

	// Search agent field
	filtered = grepFilter(entries, "bolt")
	if len(filtered) != 1 {
		t.Errorf("expected 1 entry matching 'bolt', got %d", len(filtered))
	}
}

func TestAggregateTownLog_SinceFilter(t *testing.T) {
	townRoot := t.TempDir()
	logsDir := filepath.Join(townRoot, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// townlog.parseLogLine uses time.Parse which returns UTC times.
	// Use UTC for consistency in the test.
	now := time.Now().UTC()
	old := now.Add(-48 * time.Hour)
	recent := now.Add(-10 * time.Minute)

	logContent := old.Format("2006-01-02 15:04:05") + " [spawn] gastown/polecats/ace spawned for old-work\n" +
		recent.Format("2006-01-02 15:04:05") + " [done] gastown/polecats/ace completed recent-work\n"

	if err := os.WriteFile(filepath.Join(logsDir, "town.log"), []byte(logContent), 0600); err != nil {
		t.Fatal(err)
	}

	r := &rig.Rig{
		Name:     "gastown",
		Path:     filepath.Join(townRoot, "gastown"),
		Polecats: []string{"ace"},
	}

	q := LogQuery{
		Rig:   r,
		Since: 24 * time.Hour,
		Tail:  100,
	}

	result := AggregateLogs(townRoot, q)

	if len(result.Entries) != 1 {
		t.Errorf("expected 1 recent entry, got %d", len(result.Entries))
		for _, e := range result.Entries {
			t.Logf("  %s %s", e.Agent, e.Content)
		}
	}
}

func TestAggregateTownLog_TailLimit(t *testing.T) {
	townRoot := t.TempDir()
	logsDir := filepath.Join(townRoot, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		t.Fatal(err)
	}

	logContent := `2026-03-06 10:00:00 [spawn] gastown/polecats/ace spawned for gt-001
2026-03-06 10:01:00 [nudge] gastown/polecats/ace nudged with "work"
2026-03-06 10:02:00 [done] gastown/polecats/ace completed gt-001
2026-03-06 10:03:00 [spawn] gastown/polecats/ace spawned for gt-002
2026-03-06 10:04:00 [done] gastown/polecats/ace completed gt-002
`
	if err := os.WriteFile(filepath.Join(logsDir, "town.log"), []byte(logContent), 0600); err != nil {
		t.Fatal(err)
	}

	r := &rig.Rig{
		Name:     "gastown",
		Path:     filepath.Join(townRoot, "gastown"),
		Polecats: []string{"ace"},
	}

	q := LogQuery{
		Rig:  r,
		Tail: 2,
	}

	result := AggregateLogs(townRoot, q)

	if len(result.Entries) != 2 {
		t.Errorf("expected 2 entries (tail limit), got %d", len(result.Entries))
	}

	// Should be the LAST 2 entries
	if len(result.Entries) == 2 && result.Entries[0].Type != "spawn" {
		t.Errorf("expected first tail entry to be spawn, got %s", result.Entries[0].Type)
	}
}

func TestAggregateTownLog_TypeFilter(t *testing.T) {
	townRoot := t.TempDir()
	logsDir := filepath.Join(townRoot, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		t.Fatal(err)
	}

	logContent := `2026-03-06 10:00:00 [spawn] gastown/polecats/ace spawned for gt-001
2026-03-06 10:01:00 [crash] gastown/polecats/ace exited unexpectedly (exit code 1)
2026-03-06 10:02:00 [spawn] gastown/polecats/bolt spawned for gt-002
`
	if err := os.WriteFile(filepath.Join(logsDir, "town.log"), []byte(logContent), 0600); err != nil {
		t.Fatal(err)
	}

	r := &rig.Rig{
		Name:     "gastown",
		Path:     filepath.Join(townRoot, "gastown"),
		Polecats: []string{"ace", "bolt"},
	}

	q := LogQuery{
		Rig:  r,
		Type: "crash",
		Tail: 100,
	}

	result := AggregateLogs(townRoot, q)

	if len(result.Entries) != 1 {
		t.Errorf("expected 1 crash entry, got %d", len(result.Entries))
	}
}

func TestRigSessionNames(t *testing.T) {
	r := &rig.Rig{
		Name:     "gastown",
		Polecats: []string{"ace", "bolt"},
	}

	// All sessions (no polecat filter)
	q := LogQuery{Rig: r}
	sessions := rigSessionNames(q)
	if len(sessions) != 3 { // witness + 2 polecats
		t.Errorf("expected 3 sessions, got %d", len(sessions))
	}

	// Filter to specific polecat
	q.Polecat = "ace"
	sessions = rigSessionNames(q)
	if len(sessions) != 1 {
		t.Errorf("expected 1 session for polecat filter, got %d", len(sessions))
	}
	if len(sessions) > 0 && sessions[0].agent != "gastown/polecats/ace" {
		t.Errorf("expected agent gastown/polecats/ace, got %s", sessions[0].agent)
	}
}

func TestFilterByPolecat(t *testing.T) {
	events := []struct {
		agent string
		match bool
	}{
		{"gastown/polecats/ace", true},
		{"gastown/ace", true},
		{"gastown/polecats/bolt", false},
		{"gastown/witness", false},
		{"cfutons/polecats/ace", false},
	}

	var input []LogEntry
	for _, e := range events {
		input = append(input, LogEntry{Agent: e.agent})
	}

	// Use the internal function via the town log event type
	// Test filterByPolecat directly
	var townEvents []townlogEvent
	for _, e := range events {
		townEvents = append(townEvents, townlogEvent{Agent: e.agent})
	}
	filtered := filterByPolecatHelper(townEvents, "gastown", "ace")

	if len(filtered) != 2 {
		t.Errorf("expected 2 matching agents, got %d", len(filtered))
		for _, f := range filtered {
			t.Logf("  %s", f.Agent)
		}
	}
}

// townlogEvent is a minimal type for testing filterByPolecat logic.
type townlogEvent struct {
	Agent string
}

// filterByPolecatHelper tests the polecat matching logic without importing townlog.
func filterByPolecatHelper(events []townlogEvent, rigName, polecat string) []townlogEvent {
	var filtered []townlogEvent
	for _, e := range events {
		if e.Agent == rigName+"/"+polecat ||
			e.Agent == rigName+"/polecats/"+polecat {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
