package session

import (
	"strings"
	"testing"
	"time"
)

// ConformanceConfig configures the conformance test suite.
type ConformanceConfig struct {
	// StartupDelay is the time to wait after Start for session to be ready.
	// Use 0 for test doubles, ~200ms for real tmux.
	StartupDelay time.Duration
}

// RunConformanceTests runs the Sessions interface conformance test suite.
// Both the real tmux implementation and the test double must pass these tests.
// This verifies the test double accurately models real tmux behavior.
func RunConformanceTests(t *testing.T, factory func() Sessions, cleanup func()) {
	RunConformanceTestsWithConfig(t, factory, cleanup, ConformanceConfig{})
}

// RunConformanceTestsWithConfig runs conformance tests with custom configuration.
func RunConformanceTestsWithConfig(t *testing.T, factory func() Sessions, cleanup func(), cfg ConformanceConfig) {
	if cleanup != nil {
		t.Cleanup(cleanup)
	}

	t.Run("Start", func(t *testing.T) {
		runStartTests(t, factory, cfg)
	})

	t.Run("Stop", func(t *testing.T) {
		runStopTests(t, factory, cfg)
	})

	t.Run("Exists", func(t *testing.T) {
		runExistsTests(t, factory, cfg)
	})

	t.Run("List", func(t *testing.T) {
		runListTests(t, factory, cfg)
	})

	t.Run("Send", func(t *testing.T) {
		runSendTests(t, factory, cfg)
	})

	t.Run("SendControl", func(t *testing.T) {
		runSendControlTests(t, factory, cfg)
	})

	t.Run("Capture", func(t *testing.T) {
		runCaptureTests(t, factory, cfg)
	})

	t.Run("GetInfo", func(t *testing.T) {
		runGetInfoTests(t, factory, cfg)
	})

	// Note: IsRunning and WaitFor tests moved to processes package
}

func conformanceUniqueName(t *testing.T) string {
	// Use underscore for milliseconds separator - tmux interprets "." as pane specifier
	return "test-" + strings.ReplaceAll(t.Name(), "/", "-") + "-" + time.Now().Format("150405_000")
}

// waitForStartup waits for the configured delay after session start.
func waitForStartup(cfg ConformanceConfig) {
	if cfg.StartupDelay > 0 {
		time.Sleep(cfg.StartupDelay)
	}
}

// --- Start tests ---

func runStartTests(t *testing.T, factory func() Sessions, cfg ConformanceConfig) {
	t.Run("creates session and returns ID", func(t *testing.T) {
		sess := factory()
		name := conformanceUniqueName(t)
		t.Cleanup(func() { _ = sess.Stop(SessionID(name)) })

		id, err := sess.Start(name, "", "sleep 60")
		if err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		if string(id) != name {
			t.Errorf("expected ID %q, got %q", name, id)
		}

		waitForStartup(cfg)

		exists, _ := sess.Exists(id)
		if !exists {
			t.Error("session should exist after Start")
		}
	})

	t.Run("fails on duplicate session name", func(t *testing.T) {
		sess := factory()
		name := conformanceUniqueName(t)
		t.Cleanup(func() { _ = sess.Stop(SessionID(name)) })

		_, err := sess.Start(name, "", "sleep 60")
		if err != nil {
			t.Fatalf("first Start failed: %v", err)
		}

		waitForStartup(cfg)

		_, err = sess.Start(name, "", "sleep 60")
		if err == nil {
			t.Error("second Start should fail for duplicate name")
		}
	})

	t.Run("fails on empty name", func(t *testing.T) {
		sess := factory()

		_, err := sess.Start("", "", "sleep 60")
		if err == nil {
			t.Error("Start with empty name should fail")
		}
	})
}

// --- Stop tests ---

func runStopTests(t *testing.T, factory func() Sessions, cfg ConformanceConfig) {
	t.Run("removes existing session", func(t *testing.T) {
		sess := factory()
		name := conformanceUniqueName(t)

		id, _ := sess.Start(name, "", "sleep 60")
		waitForStartup(cfg)

		err := sess.Stop(id)
		if err != nil {
			t.Fatalf("Stop failed: %v", err)
		}

		exists, _ := sess.Exists(id)
		if exists {
			t.Error("session should not exist after Stop")
		}
	})

	t.Run("is idempotent for non-existent session", func(t *testing.T) {
		sess := factory()
		// Need at least one session to keep tmux server alive
		keepalive := conformanceUniqueName(t) + "-keepalive"
		_, _ = sess.Start(keepalive, "", "sleep 60")
		waitForStartup(cfg)
		t.Cleanup(func() { _ = sess.Stop(SessionID(keepalive)) })

		err := sess.Stop(SessionID("nonexistent-session-12345"))
		if err != nil {
			t.Errorf("Stop on non-existent session should succeed: %v", err)
		}
	})
}

// --- Exists tests ---

func runExistsTests(t *testing.T, factory func() Sessions, cfg ConformanceConfig) {
	t.Run("returns false for non-existent session", func(t *testing.T) {
		sess := factory()

		exists, err := sess.Exists(SessionID("nonexistent-12345"))
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if exists {
			t.Error("should return false for non-existent session")
		}
	})

	t.Run("returns true for existing session", func(t *testing.T) {
		sess := factory()
		name := conformanceUniqueName(t)
		t.Cleanup(func() { _ = sess.Stop(SessionID(name)) })

		id, _ := sess.Start(name, "", "sleep 60")
		waitForStartup(cfg)

		exists, err := sess.Exists(id)
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if !exists {
			t.Error("should return true for existing session")
		}
	})

	t.Run("uses exact match", func(t *testing.T) {
		sess := factory()
		name := conformanceUniqueName(t)
		t.Cleanup(func() { _ = sess.Stop(SessionID(name)) })

		_, _ = sess.Start(name, "", "sleep 60")
		waitForStartup(cfg)

		// Should not match prefix
		exists, _ := sess.Exists(SessionID(name[:len(name)-3]))
		if exists {
			t.Error("Exists should use exact match, not prefix")
		}
	})
}

// --- List tests ---

func runListTests(t *testing.T, factory func() Sessions, cfg ConformanceConfig) {
	t.Run("returns created sessions", func(t *testing.T) {
		sess := factory()
		name1 := conformanceUniqueName(t) + "-1"
		name2 := conformanceUniqueName(t) + "-2"
		t.Cleanup(func() {
			_ = sess.Stop(SessionID(name1))
			_ = sess.Stop(SessionID(name2))
		})

		_, _ = sess.Start(name1, "", "sleep 60")
		_, _ = sess.Start(name2, "", "sleep 60")
		waitForStartup(cfg)

		sessions, err := sess.List()
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}

		found1, found2 := false, false
		for _, id := range sessions {
			if string(id) == name1 {
				found1 = true
			}
			if string(id) == name2 {
				found2 = true
			}
		}

		if !found1 || !found2 {
			t.Errorf("List should include both sessions, found1=%v, found2=%v", found1, found2)
		}
	})

	t.Run("excludes stopped sessions", func(t *testing.T) {
		sess := factory()
		name := conformanceUniqueName(t)

		id, _ := sess.Start(name, "", "sleep 60")
		waitForStartup(cfg)
		_ = sess.Stop(id)

		sessions, _ := sess.List()
		for _, sid := range sessions {
			if string(sid) == name {
				t.Error("List should not include stopped session")
			}
		}
	})
}

// --- Send tests ---

func runSendTests(t *testing.T, factory func() Sessions, cfg ConformanceConfig) {
	t.Run("succeeds on existing session", func(t *testing.T) {
		sess := factory()
		name := conformanceUniqueName(t)
		t.Cleanup(func() { _ = sess.Stop(SessionID(name)) })

		id, _ := sess.Start(name, "", "sleep 60")
		waitForStartup(cfg)

		err := sess.Send(id, "hello")
		if err != nil {
			t.Errorf("Send failed: %v", err)
		}
	})

	t.Run("fails on non-existent session", func(t *testing.T) {
		sess := factory()

		err := sess.Send(SessionID("nonexistent-12345"), "hello")
		if err == nil {
			t.Error("Send on non-existent session should fail")
		}
	})
}

// --- SendControl tests ---

func runSendControlTests(t *testing.T, factory func() Sessions, cfg ConformanceConfig) {
	t.Run("succeeds on existing session", func(t *testing.T) {
		sess := factory()
		name := conformanceUniqueName(t)
		t.Cleanup(func() { _ = sess.Stop(SessionID(name)) })

		id, _ := sess.Start(name, "", "sleep 60")
		waitForStartup(cfg)

		err := sess.SendControl(id, "C-c")
		if err != nil {
			t.Errorf("SendControl failed: %v", err)
		}
	})

	t.Run("fails on non-existent session", func(t *testing.T) {
		sess := factory()

		err := sess.SendControl(SessionID("nonexistent-12345"), "C-c")
		if err == nil {
			t.Error("SendControl on non-existent session should fail")
		}
	})
}

// --- Capture tests ---

func runCaptureTests(t *testing.T, factory func() Sessions, cfg ConformanceConfig) {
	t.Run("succeeds on existing session", func(t *testing.T) {
		sess := factory()
		name := conformanceUniqueName(t)
		t.Cleanup(func() { _ = sess.Stop(SessionID(name)) })

		id, _ := sess.Start(name, "", "sleep 60")
		waitForStartup(cfg)

		_, err := sess.Capture(id, 10)
		if err != nil {
			t.Errorf("Capture failed: %v", err)
		}
	})

	t.Run("fails on non-existent session", func(t *testing.T) {
		sess := factory()

		_, err := sess.Capture(SessionID("nonexistent-12345"), 10)
		if err == nil {
			t.Error("Capture on non-existent session should fail")
		}
	})
}

// --- GetInfo tests ---

func runGetInfoTests(t *testing.T, factory func() Sessions, cfg ConformanceConfig) {
	t.Run("returns info for existing session", func(t *testing.T) {
		sess := factory()
		name := conformanceUniqueName(t)
		t.Cleanup(func() { _ = sess.Stop(SessionID(name)) })

		id, _ := sess.Start(name, "", "sleep 60")
		waitForStartup(cfg)

		info, err := sess.GetInfo(id)
		if err != nil {
			t.Fatalf("GetInfo failed: %v", err)
		}
		if info.Name != name {
			t.Errorf("expected name %q, got %q", name, info.Name)
		}
	})

	t.Run("fails on non-existent session", func(t *testing.T) {
		sess := factory()

		_, err := sess.GetInfo(SessionID("nonexistent-12345"))
		if err == nil {
			t.Error("GetInfo on non-existent session should fail")
		}
	})
}

// Note: IsRunning and WaitFor tests have been moved to the processes package
// since those methods are now part of processes.Processes, not session.Sessions.
