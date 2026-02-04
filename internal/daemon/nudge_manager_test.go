package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGenerateNudgeID(t *testing.T) {
	ts := time.Unix(1706789012, 0)

	// Same inputs produce same ID
	id1 := GenerateNudgeID("gt-test", "hello", ts)
	id2 := GenerateNudgeID("gt-test", "hello", ts)
	if id1 != id2 {
		t.Errorf("expected same ID for same inputs, got %s and %s", id1, id2)
	}

	// Different target produces different ID
	id3 := GenerateNudgeID("gt-other", "hello", ts)
	if id1 == id3 {
		t.Errorf("expected different ID for different target")
	}

	// Different message produces different ID
	id4 := GenerateNudgeID("gt-test", "world", ts)
	if id1 == id4 {
		t.Errorf("expected different ID for different message")
	}

	// Different timestamp produces different ID
	ts2 := time.Unix(1706789013, 0) // 1 second later
	id5 := GenerateNudgeID("gt-test", "hello", ts2)
	if id1 == id5 {
		t.Errorf("expected different ID for different timestamp")
	}

	// ID should be base36 (6-7 chars)
	if len(id1) < 1 || len(id1) > 7 {
		t.Errorf("expected ID length 1-7, got %d", len(id1))
	}
}

func TestNudgeQueueOperations(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "nudges.jsonl")

	// Create a NudgeManager without tmux (for queue tests only)
	nm := &NudgeManager{
		queuePath: queuePath,
		lockPath:  queuePath + ".lock",
		sessions:  make(map[string]*SessionState),
		logger:    func(format string, args ...interface{}) {},
	}

	// Load empty queue
	nudges, err := nm.loadQueue()
	if err != nil {
		t.Fatalf("loadQueue failed: %v", err)
	}
	if len(nudges) != 0 {
		t.Errorf("expected empty queue, got %d", len(nudges))
	}

	// Write some nudges manually
	f, _ := os.Create(queuePath)
	f.WriteString(`{"id":"abc123","t":"gt-test","m":"hello","ts":"2024-02-01T12:00:00Z"}` + "\n")
	f.WriteString(`{"id":"def456","t":"gt-test","m":"world","ts":"2024-02-01T12:00:01Z"}` + "\n")
	f.Close()

	// Load queue with content
	nudges, err = nm.loadQueue()
	if err != nil {
		t.Fatalf("loadQueue failed: %v", err)
	}
	if len(nudges) != 2 {
		t.Errorf("expected 2 nudges, got %d", len(nudges))
	}
	if nudges[0].ID != "abc123" {
		t.Errorf("expected ID abc123, got %s", nudges[0].ID)
	}

	// Rewrite queue
	remaining := []NudgeRequest{nudges[1]}
	if err := nm.rewriteQueue(remaining); err != nil {
		t.Fatalf("rewriteQueue failed: %v", err)
	}

	// Verify rewrite
	nudges, _ = nm.loadQueue()
	if len(nudges) != 1 {
		t.Errorf("expected 1 nudge after rewrite, got %d", len(nudges))
	}
	if nudges[0].ID != "def456" {
		t.Errorf("expected ID def456, got %s", nudges[0].ID)
	}
}

func TestNudgeDedup(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "nudges.jsonl")

	nm := &NudgeManager{
		queuePath: queuePath,
		lockPath:  queuePath + ".lock",
		sessions:  make(map[string]*SessionState),
		logger:    func(format string, args ...interface{}) {},
	}

	// Queue first nudge
	if err := nm.QueueNudge("gt-test", "hello", "sender"); err != nil {
		t.Fatalf("first QueueNudge failed: %v", err)
	}

	nudges, _ := nm.loadQueue()
	if len(nudges) != 1 {
		t.Fatalf("expected 1 nudge, got %d", len(nudges))
	}

	// Queue duplicate (same target, message, within 1 second) - should be deduplicated
	if err := nm.QueueNudge("gt-test", "hello", "sender"); err != nil {
		t.Fatalf("second QueueNudge failed: %v", err)
	}

	nudges, _ = nm.loadQueue()
	if len(nudges) != 1 {
		t.Errorf("expected still 1 nudge after dedup, got %d", len(nudges))
	}

	// Queue different message - should be added
	if err := nm.QueueNudge("gt-test", "world", "sender"); err != nil {
		t.Fatalf("third QueueNudge failed: %v", err)
	}

	nudges, _ = nm.loadQueue()
	if len(nudges) != 2 {
		t.Errorf("expected 2 nudges after different message, got %d", len(nudges))
	}
}

func TestNudgeExpiration(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "nudges.jsonl")

	nm := &NudgeManager{
		queuePath: queuePath,
		lockPath:  queuePath + ".lock",
		sessions:  make(map[string]*SessionState),
		logger:    func(format string, args ...interface{}) {},
	}

	// Write nudges with different ages
	now := time.Now()
	fresh := NudgeRequest{ID: "fresh", Target: "gt-test", Message: "new", Timestamp: now}
	expired := NudgeRequest{ID: "expired", Target: "gt-test", Message: "old", Timestamp: now.Add(-3 * time.Minute)}

	if err := nm.rewriteQueue([]NudgeRequest{fresh, expired}); err != nil {
		t.Fatalf("rewriteQueue failed: %v", err)
	}

	// Load and filter like processQueue does
	nudges, _ := nm.loadQueue()
	var remaining []NudgeRequest
	for _, n := range nudges {
		if now.Sub(n.Timestamp) <= NudgeMaxAge {
			remaining = append(remaining, n)
		}
	}

	if len(remaining) != 1 {
		t.Errorf("expected 1 non-expired nudge, got %d", len(remaining))
	}
	if remaining[0].ID != "fresh" {
		t.Errorf("expected fresh nudge to remain, got %s", remaining[0].ID)
	}
}

func TestNudgeMaxAttempts(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "nudges.jsonl")

	nm := &NudgeManager{
		queuePath: queuePath,
		lockPath:  queuePath + ".lock",
		sessions:  make(map[string]*SessionState),
		logger:    func(format string, args ...interface{}) {},
	}

	// Write nudges with different attempt counts
	now := time.Now()
	retryable := NudgeRequest{ID: "retry", Target: "gt-test", Message: "can retry", Timestamp: now, Attempts: 2}
	exhausted := NudgeRequest{ID: "done", Target: "gt-test", Message: "max attempts", Timestamp: now, Attempts: NudgeMaxAttempts}

	if err := nm.rewriteQueue([]NudgeRequest{retryable, exhausted}); err != nil {
		t.Fatalf("rewriteQueue failed: %v", err)
	}

	// Load and filter like processQueue does
	nudges, _ := nm.loadQueue()
	var remaining []NudgeRequest
	for _, n := range nudges {
		if n.Attempts < NudgeMaxAttempts {
			remaining = append(remaining, n)
		}
	}

	if len(remaining) != 1 {
		t.Errorf("expected 1 retryable nudge, got %d", len(remaining))
	}
	if remaining[0].ID != "retry" {
		t.Errorf("expected retryable nudge to remain, got %s", remaining[0].ID)
	}
}

func TestNudgeRateLimits(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "nudges.jsonl")

	nm := &NudgeManager{
		queuePath: queuePath,
		lockPath:  queuePath + ".lock",
		sessions:  make(map[string]*SessionState),
		logger:    func(format string, args ...interface{}) {},
	}

	// Queue up to the per-agent limit
	for i := 0; i < NudgeMaxPerAgent; i++ {
		msg := "message" + string(rune('0'+i))
		if err := nm.QueueNudge("gt-test", msg, "sender"); err != nil {
			t.Fatalf("QueueNudge %d failed: %v", i, err)
		}
		time.Sleep(time.Second) // ensure different IDs
	}

	// Next one should fail
	err := nm.QueueNudge("gt-test", "overflow", "sender")
	if err == nil {
		t.Error("expected error when exceeding per-agent limit")
	}

	// But a different target should work
	if err := nm.QueueNudge("gt-other", "hello", "sender"); err != nil {
		t.Errorf("different target should succeed: %v", err)
	}
}
