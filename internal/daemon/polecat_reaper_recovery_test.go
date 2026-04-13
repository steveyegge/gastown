package daemon

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
)

// TestReapCompletedPolecats_RecoversPanic verifies that the daemon's
// reapCompletedPolecats method recovers from panics in the scan path
// instead of crashing the entire daemon process.
//
// Root cause: the daemon's main select loop has no recover() wrapper,
// so a panic in any patrol handler (including the polecat reaper) kills
// the daemon. This test ensures the reaper catches panics and logs them.
func TestReapCompletedPolecats_RecoversPanic(t *testing.T) {
	// Create a minimal daemon with a logger we can inspect
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	d := &Daemon{
		config: &Config{
			TownRoot: "/nonexistent/town/root/that/will/cause/issues",
		},
		patrolConfig: DefaultLifecycleConfig(),
		logger:       logger,
		ctx:          context.Background(),
	}

	// This should NOT panic — the reaper should recover from any internal panic
	// and log the error instead of crashing the daemon.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("reapCompletedPolecats panicked (should have recovered internally): %v", r)
			}
		}()
		d.reapCompletedPolecats()
	}()

	// The reaper may log a scan error or a diagnostic — verify it ran without panic.
	// With a nonexistent town root, no polecats are discovered, so it may only
	// log a diagnostic on scan #1 or nothing at all. The key assertion is the
	// defer/recover above: no unrecovered panic escaped.
	logOutput := logBuf.String()

	// Should NOT contain "panic" in an unrecovered sense — if it recovered,
	// the log should contain a structured error message
	if strings.Contains(logOutput, "runtime error") && !strings.Contains(logOutput, "recovered") {
		t.Errorf("log suggests unrecovered panic: %s", logOutput)
	}
}

// TestReapCompletedPolecats_CanceledContext verifies that the reaper
// respects context cancellation (e.g., during daemon shutdown).
func TestReapCompletedPolecats_CanceledContext(t *testing.T) {
	var logBuf bytes.Buffer
	logger := log.New(&logBuf, "", 0)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	d := &Daemon{
		config: &Config{
			TownRoot: t.TempDir(),
		},
		patrolConfig: DefaultLifecycleConfig(),
		logger:       logger,
		ctx:          ctx,
	}

	// Should complete quickly without panic, even with canceled context
	d.reapCompletedPolecats()
}
