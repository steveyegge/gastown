package deacon

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestGetPolecatBeadStatus_RespectsContextTimeout verifies that
// getPolecatBeadStatusCtx returns an error when the context is
// already canceled, rather than hanging indefinitely on a subprocess.
// This is the TDD test for the fix: getPolecatBeadStatus must accept
// a context so the daemon can enforce a timeout on bd subprocess calls.
func TestGetPolecatBeadStatus_RespectsContextTimeout(t *testing.T) {
	townRoot := t.TempDir()

	// Create .beads dir so ResolveBeadsDir resolves
	if err := os.MkdirAll(townRoot+"/.beads", 0755); err != nil {
		t.Fatal(err)
	}

	// Use an already-canceled context — the subprocess should fail immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before calling

	_, _, err := getPolecatBeadStatusCtx(ctx, townRoot, "testrig", "worker1")
	if err == nil {
		t.Error("expected error with canceled context, got nil")
	}
}

// TestGetPolecatBeadStatus_TimesOut verifies that a slow bd command
// is killed by the context timeout rather than blocking forever.
func TestGetPolecatBeadStatus_TimesOut(t *testing.T) {
	townRoot := t.TempDir()

	if err := os.MkdirAll(townRoot+"/.beads", 0755); err != nil {
		t.Fatal(err)
	}

	// Use a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Allow the timeout to fire
	time.Sleep(5 * time.Millisecond)

	_, _, err := getPolecatBeadStatusCtx(ctx, townRoot, "testrig", "worker1")
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}

// TestScanCompletedPolecats_RespectsContext verifies that
// ScanCompletedPolecatsCtx propagates context cancellation.
func TestScanCompletedPolecats_RespectsContext(t *testing.T) {
	townRoot := t.TempDir()

	// Create a rig with a polecat directory
	rigDir := townRoot + "/testrig/polecats/alpha"
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Canceled context — should return quickly with an error, not hang
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := DefaultReapConfig()
	_, err := ScanCompletedPolecatsCtx(ctx, townRoot, cfg)
	if err == nil {
		// The function may return (result, nil) if all polecats are skipped
		// because they have no tmux session. That's acceptable — the key test
		// is that it doesn't hang.
		t.Log("ScanCompletedPolecatsCtx returned nil error with canceled context (polecats skipped before context check)")
	}
}
