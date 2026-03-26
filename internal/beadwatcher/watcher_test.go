// Package beadwatcher tests verify bead state transition notification logic.
package beadwatcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/nudge"
	"github.com/steveyegge/gastown/internal/session"
)

// fakeBeads implements BeadsLister for test injection.
type fakeBeads struct {
	issues []*beads.Issue
}

func (f *fakeBeads) List(opts beads.ListOptions) ([]*beads.Issue, error) {
	var out []*beads.Issue
	for _, iss := range f.issues {
		if opts.Status == "" || opts.Status == "all" || opts.Status == iss.Status {
			out = append(out, iss)
		}
	}
	return out, nil
}

// setupTownRoot creates a minimal town root directory structure for nudge tests.
func setupTownRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// mayor/town.json is the marker for the town root
	if err := os.MkdirAll(filepath.Join(dir, "mayor"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mayor", "town.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// readNudgesForSession reads queued nudges from the nudge queue directory.
func readNudgesForSession(t *testing.T, townRoot, session string) []nudge.QueuedNudge {
	t.Helper()
	safe := filepath.Join(townRoot, ".runtime", "nudge_queue", session)
	entries, err := os.ReadDir(safe)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("reading nudge queue: %v", err)
	}
	var out []nudge.QueuedNudge
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(safe, e.Name()))
		if err != nil {
			continue
		}
		var n nudge.QueuedNudge
		if err := json.Unmarshal(data, &n); err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

// TestWatcher_BlockedTransitionNudgesMayor verifies that a bead transitioning
// to "blocked" status causes a nudge to be enqueued for the mayor.
func TestWatcher_BlockedTransitionNudgesMayor(t *testing.T) {
	townRoot := setupTownRoot(t)
	mayorSession := session.MayorSessionName()

	now := time.Now()
	blockedIssue := &beads.Issue{
		ID:        "hq-001",
		Title:     "Some blocked task",
		Status:    string(beads.StatusBlocked),
		UpdatedAt: now.Format(time.RFC3339),
		CreatedAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
	}

	bl := &fakeBeads{issues: []*beads.Issue{blockedIssue}}
	w := NewWatcher(townRoot, bl, WatcherConfig{
		PollInterval:     10 * time.Millisecond,
		OverdueThreshold: 4 * time.Hour,
		Sender:           "bead-watcher",
	})

	// Run one tick synchronously
	if err := w.tick(); err != nil {
		t.Fatalf("tick() error: %v", err)
	}

	// Expect a nudge was sent to the mayor
	queued := readNudgesForSession(t, townRoot, mayorSession)
	if len(queued) == 0 {
		t.Fatalf("expected nudge for blocked bead hq-001, got none")
	}
	found := false
	for _, n := range queued {
		if containsSubstring(n.Message, "hq-001") && containsSubstring(n.Message, "blocked") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no nudge mentions hq-001 as blocked; got nudges: %+v", queued)
	}
}

// TestWatcher_DeduplicatesNotifications verifies that calling tick() twice with
// the same blocked bead does NOT send a second nudge (deduplication).
func TestWatcher_DeduplicatesNotifications(t *testing.T) {
	townRoot := setupTownRoot(t)
	mayorSession := session.MayorSessionName()

	now := time.Now()
	blockedIssue := &beads.Issue{
		ID:        "hq-002",
		Title:     "Another blocked task",
		Status:    string(beads.StatusBlocked),
		UpdatedAt: now.Format(time.RFC3339),
		CreatedAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
	}

	bl := &fakeBeads{issues: []*beads.Issue{blockedIssue}}
	w := NewWatcher(townRoot, bl, WatcherConfig{
		PollInterval:     10 * time.Millisecond,
		OverdueThreshold: 4 * time.Hour,
		Sender:           "bead-watcher",
	})

	// First tick — should enqueue one nudge
	if err := w.tick(); err != nil {
		t.Fatalf("tick() 1 error: %v", err)
	}
	// Second tick with same state — should NOT enqueue another
	if err := w.tick(); err != nil {
		t.Fatalf("tick() 2 error: %v", err)
	}

	queued := readNudgesForSession(t, townRoot, mayorSession)
	count := 0
	for _, n := range queued {
		if containsSubstring(n.Message, "hq-002") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 nudge for hq-002, got %d", count)
	}
}

// TestWatcher_ReNotifiesAfterStateChange verifies that after a bead clears
// its blocked state and then becomes blocked again, a new nudge IS sent.
func TestWatcher_ReNotifiesAfterStateChange(t *testing.T) {
	townRoot := setupTownRoot(t)
	mayorSession := session.MayorSessionName()

	now := time.Now()
	issue := &beads.Issue{
		ID:        "hq-003",
		Title:     "Cycling task",
		Status:    string(beads.StatusBlocked),
		UpdatedAt: now.Format(time.RFC3339),
		CreatedAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
	}

	bl := &fakeBeads{issues: []*beads.Issue{issue}}
	w := NewWatcher(townRoot, bl, WatcherConfig{
		PollInterval:     10 * time.Millisecond,
		OverdueThreshold: 4 * time.Hour,
		Sender:           "bead-watcher",
	})

	// First tick — becomes blocked, nudge sent
	if err := w.tick(); err != nil {
		t.Fatalf("tick() 1 error: %v", err)
	}

	// Bead resolves
	issue.Status = string(beads.StatusInProgress)
	if err := w.tick(); err != nil {
		t.Fatalf("tick() 2 (resolved) error: %v", err)
	}

	// Bead becomes blocked again
	issue.Status = string(beads.StatusBlocked)
	if err := w.tick(); err != nil {
		t.Fatalf("tick() 3 (blocked again) error: %v", err)
	}

	queued := readNudgesForSession(t, townRoot, mayorSession)
	count := 0
	for _, n := range queued {
		if containsSubstring(n.Message, "hq-003") {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 nudges for hq-003 (blocked, cleared, blocked again), got %d", count)
	}
}

// TestWatcher_OverdueBeadNudgesMayor verifies that an open bead older than the
// overdue threshold triggers a nudge.
func TestWatcher_OverdueBeadNudgesMayor(t *testing.T) {
	townRoot := setupTownRoot(t)
	mayorSession := session.MayorSessionName()

	// Created 5 hours ago — past the 4-hour default threshold
	old := time.Now().Add(-5 * time.Hour)
	overdueIssue := &beads.Issue{
		ID:        "hq-004",
		Title:     "Old open task",
		Status:    string(beads.StatusOpen),
		CreatedAt: old.Format(time.RFC3339),
		UpdatedAt: old.Format(time.RFC3339),
	}

	bl := &fakeBeads{issues: []*beads.Issue{overdueIssue}}
	w := NewWatcher(townRoot, bl, WatcherConfig{
		PollInterval:     10 * time.Millisecond,
		OverdueThreshold: 4 * time.Hour,
		Sender:           "bead-watcher",
	})

	if err := w.tick(); err != nil {
		t.Fatalf("tick() error: %v", err)
	}

	queued := readNudgesForSession(t, townRoot, mayorSession)
	found := false
	for _, n := range queued {
		if containsSubstring(n.Message, "hq-004") && containsSubstring(n.Message, "overdue") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected overdue nudge for hq-004; got nudges: %+v", queued)
	}
}

// TestWatcher_NoNudgeForRecentOpenBead verifies that an open bead within the
// overdue threshold does NOT trigger a nudge.
func TestWatcher_NoNudgeForRecentOpenBead(t *testing.T) {
	townRoot := setupTownRoot(t)
	mayorSession := session.MayorSessionName()

	// Created 1 hour ago — within the 4-hour threshold
	recent := time.Now().Add(-1 * time.Hour)
	freshIssue := &beads.Issue{
		ID:        "hq-005",
		Title:     "Recent open task",
		Status:    string(beads.StatusOpen),
		CreatedAt: recent.Format(time.RFC3339),
		UpdatedAt: recent.Format(time.RFC3339),
	}

	bl := &fakeBeads{issues: []*beads.Issue{freshIssue}}
	w := NewWatcher(townRoot, bl, WatcherConfig{
		PollInterval:     10 * time.Millisecond,
		OverdueThreshold: 4 * time.Hour,
		Sender:           "bead-watcher",
	})

	if err := w.tick(); err != nil {
		t.Fatalf("tick() error: %v", err)
	}

	queued := readNudgesForSession(t, townRoot, mayorSession)
	for _, n := range queued {
		if containsSubstring(n.Message, "hq-005") {
			t.Errorf("unexpected nudge for fresh bead hq-005: %s", n.Message)
		}
	}
}

// containsSubstring is a simple helper to check if a string contains a substring.
func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
