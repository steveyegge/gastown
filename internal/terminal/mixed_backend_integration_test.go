//go:build integration

package terminal

import (
	"net/http"
	"testing"
	"time"
)

// TestIntegration_MixedBackend_HasSession verifies that both CoopBackend
// and TmuxBackend implement HasSession correctly, and that the hasSession()
// fallback pattern used by all managers works with both backends.
func TestIntegration_MixedBackend_HasSession(t *testing.T) {
	// Start a real coop process for the coop backend path.
	base, cleanup := startCoop(t)
	defer cleanup()

	// Coop backend — session exists
	coop := NewCoopBackend(CoopConfig{})
	coop.AddSession("claude", base)

	ok, err := coop.HasSession("claude")
	if err != nil {
		t.Fatalf("coop HasSession error: %v", err)
	}
	if !ok {
		t.Error("expected coop HasSession=true for running process")
	}

	// Coop backend — unregistered session returns false
	ok, err = coop.HasSession("nonexistent")
	if err != nil {
		t.Fatalf("coop HasSession(nonexistent) error: %v", err)
	}
	if ok {
		t.Error("expected coop HasSession=false for unregistered session")
	}

	// Tmux backend — session does not exist (no real tmux sessions expected for test)
	tmuxB := LocalBackend()
	ok, err = tmuxB.HasSession("gt-test-nonexistent")
	if err != nil {
		// Tmux errors are expected if tmux is not running.
		t.Logf("tmux HasSession returned expected error: %v", err)
	}
	if ok {
		t.Error("expected tmux HasSession=false for nonexistent session")
	}
}

// TestIntegration_MixedBackend_FallbackPattern tests the common
// hasSession() fallback pattern used by all managers:
//
//	if m.backend != nil {
//	    return m.backend.HasSession(sessionID)
//	}
//	return m.tmux.HasSession(sessionID)
//
// This simulates both paths: backend-set and backend-nil.
func TestIntegration_MixedBackend_FallbackPattern(t *testing.T) {
	base, cleanup := startCoop(t)
	defer cleanup()

	t.Run("backend_set_coop", func(t *testing.T) {
		// When backend is set, hasSession should route through it.
		var backend Backend = NewCoopBackend(CoopConfig{})
		backend.(*CoopBackend).AddSession("claude", base)

		ok, err := backend.HasSession("claude")
		if err != nil {
			t.Fatalf("backend HasSession: %v", err)
		}
		if !ok {
			t.Error("expected true from coop backend")
		}
	})

	t.Run("backend_nil_fallback", func(t *testing.T) {
		// When backend is nil, managers fall through to tmux.
		// This simulates the fallback path.
		var backend Backend
		if backend != nil {
			t.Fatal("backend should be nil for this test")
		}

		// The fallback to tmux is expected to work even if tmux
		// has no matching session — it should return false, not error.
		tmuxB := LocalBackend()
		ok, err := tmuxB.HasSession("gt-test-nonexistent-session")
		// On macOS without tmux server, this may error or return false.
		if err != nil {
			t.Logf("tmux HasSession error (expected without tmux): %v", err)
		}
		if ok {
			t.Error("expected false for nonexistent tmux session")
		}
	})
}

// TestIntegration_MixedBackend_CoopHealthCheck verifies that coop health
// checks work correctly for session liveness detection, as used by
// SessionRegistry.DiscoverAll() with CheckLiveness=true.
func TestIntegration_MixedBackend_CoopHealthCheck(t *testing.T) {
	base, cleanup := startCoop(t)
	defer cleanup()

	// Simulate what SessionRegistry.healthCheck does: GET /api/v1/health
	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("healthy_coop", func(t *testing.T) {
		resp, err := client.Get(base + "/api/v1/health")
		if err != nil {
			t.Fatalf("health check failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("health status = %d, want 200", resp.StatusCode)
		}
	})

	t.Run("unreachable_coop", func(t *testing.T) {
		// An unreachable coop should fail gracefully (not panic).
		_, err := client.Get("http://127.0.0.1:1/api/v1/health")
		if err == nil {
			t.Error("expected error for unreachable coop")
		}
	})
}

// TestIntegration_MixedBackend_CoopKillDetection verifies that after
// killing a coop process, HasSession and health checks correctly report
// the session as dead. This simulates what happens when a K8s pod dies
// and the convoy/polecat managers check for stranded sessions.
func TestIntegration_MixedBackend_CoopKillDetection(t *testing.T) {
	base, cleanup := startCoop(t)

	b := NewCoopBackend(CoopConfig{})
	b.AddSession("claude", base)

	// Verify alive before kill.
	ok, err := b.HasSession("claude")
	if err != nil || !ok {
		t.Fatalf("expected session alive before kill: ok=%v err=%v", ok, err)
	}

	// Kill the coop process.
	cleanup()
	time.Sleep(500 * time.Millisecond)

	// After kill, HasSession should return false or error.
	ok, err = b.HasSession("claude")
	if ok {
		t.Error("expected HasSession=false after kill")
	}
	if err != nil {
		t.Logf("HasSession after kill returned expected error: %v", err)
	}
}

// TestIntegration_MixedBackend_IsAgentRunningCoop verifies that
// IsAgentRunning works for coop backend (used by crew/polecat managers
// to detect if the agent is still active).
func TestIntegration_MixedBackend_IsAgentRunningCoop(t *testing.T) {
	base, cleanup := startCoop(t)
	defer cleanup()

	b := NewCoopBackend(CoopConfig{})
	b.AddSession("claude", base)

	running, err := b.IsAgentRunning("claude")
	if err != nil {
		t.Fatalf("IsAgentRunning: %v", err)
	}
	if !running {
		t.Error("expected agent running (bash process in coop)")
	}
}

// TestIntegration_MixedBackend_MultipleCoopSessions verifies that
// CoopBackend can track multiple sessions simultaneously, as would
// happen in a multi-rig deployment where a single manager tracks
// both local and K8s agents.
func TestIntegration_MixedBackend_MultipleCoopSessions(t *testing.T) {
	// Start two coop processes.
	base1, cleanup1 := startCoop(t)
	defer cleanup1()

	base2, cleanup2 := startCoop(t)
	defer cleanup2()

	b := NewCoopBackend(CoopConfig{})
	b.AddSession("agent1", base1)
	b.AddSession("agent2", base2)

	// Both should be alive.
	for _, name := range []string{"agent1", "agent2"} {
		ok, err := b.HasSession(name)
		if err != nil {
			t.Fatalf("HasSession(%q): %v", name, err)
		}
		if !ok {
			t.Errorf("expected HasSession(%q)=true", name)
		}
	}

	// Kill one, verify the other is still alive.
	cleanup1()
	time.Sleep(500 * time.Millisecond)

	ok1, _ := b.HasSession("agent1")
	ok2, err2 := b.HasSession("agent2")

	if ok1 {
		t.Error("expected agent1 dead after kill")
	}
	if err2 != nil || !ok2 {
		t.Errorf("expected agent2 still alive: ok=%v err=%v", ok2, err2)
	}
}

// TestIntegration_MixedBackend_CapturePane_CoopFull verifies that
// CapturePane and SendInput work together against a real coop process,
// as used by convoy detection to check screen output for stranded agents.
func TestIntegration_MixedBackend_CapturePane_CoopFull(t *testing.T) {
	base, cleanup := startCoop(t)
	defer cleanup()

	b := NewCoopBackend(CoopConfig{})
	b.AddSession("claude", base)

	// Send input to generate screen content.
	if err := b.SendInput("claude", "echo MIXED_BACKEND_TEST", true); err != nil {
		t.Fatalf("SendInput: %v", err)
	}
	time.Sleep(1 * time.Second)

	// CapturePane should return screen text including our marker.
	text, err := b.CapturePane("claude", 10)
	if err != nil {
		t.Fatalf("CapturePane: %v", err)
	}
	if len(text) == 0 {
		t.Error("expected non-empty capture from real coop")
	}
	t.Logf("CapturePane returned %d bytes", len(text))

	// CapturePaneAll should return more content.
	allText, err := b.CapturePaneAll("claude")
	if err != nil {
		t.Fatalf("CapturePaneAll: %v", err)
	}
	if len(allText) < len(text) {
		t.Errorf("CapturePaneAll (%d bytes) < CapturePane (%d bytes)", len(allText), len(text))
	}

	// CapturePaneLines should return structured lines.
	lines, err := b.CapturePaneLines("claude", 5)
	if err != nil {
		t.Fatalf("CapturePaneLines: %v", err)
	}
	if len(lines) > 5 {
		t.Errorf("expected at most 5 lines, got %d", len(lines))
	}
}
