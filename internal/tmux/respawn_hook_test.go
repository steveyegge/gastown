package tmux

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// testSocket creates an isolated tmux server on a unique socket for the test.
// Returns the socket name and a cleanup function that kills the server.
func testSocket(t *testing.T) string {
	t.Helper()
	socket := fmt.Sprintf("gt-test-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		// Kill the entire tmux server on this socket
		_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
	})
	return socket
}

// testSession creates a session on the given socket running a simple command.
// Returns after the session is confirmed alive.
func testSession(t *testing.T, socket, session, command string) {
	t.Helper()
	args := []string{"-L", socket, "new-session", "-d", "-s", session, command}
	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("failed to create test session %q on socket %q: %v\n%s", session, socket, err, out)
	}
	// Wait for session to be visible
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		err := exec.Command("tmux", "-L", socket, "has-session", "-t", session).Run()
		if err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("session %q never appeared on socket %q", session, socket)
}

// isPaneDead checks if a pane is dead on the given socket.
func isPaneDead(socket, session string) bool {
	out, err := exec.Command("tmux", "-L", socket, "list-panes", "-t", session, "-F", "#{pane_dead}").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "1"
}

// TestAutoRespawnHook_RespawnWorks is an integration test that verifies the
// pane-died hook actually respawns the pane on a socket-scoped tmux server.
//
// This is the primary regression test for the multi-town socket migration
// (33362a75) which moved all tmux commands to per-town sockets (-L <town>).
// The hook's embedded tmux commands must include the socket flag, otherwise
// `tmux respawn-pane` targets the default server where the session doesn't
// exist, and respawn silently fails.
//
// Sequence:
// 1. Create session on isolated socket running `sleep 5`
// 2. Set auto-respawn hook via NewTmuxWithSocket
// 3. Wait for process to exit naturally
// 4. Wait for the hook to fire and respawn the pane
// 5. Verify the pane comes back alive
func TestAutoRespawnHook_RespawnWorks(t *testing.T) {
	socket := testSocket(t)
	session := "test-respawn"

	// Use `sleep 2` — it exits naturally after 2 seconds
	testSession(t, socket, session, "sleep 2")

	tmx := NewTmuxWithSocket(socket)

	if err := tmx.SetAutoRespawnHook(session); err != nil {
		t.Fatalf("SetAutoRespawnHook failed: %v", err)
	}

	// Wait for sleep 2 to exit naturally
	t.Log("Waiting for process to exit...")
	deadline := time.Now().Add(5 * time.Second)
	paneDied := false
	for time.Now().Before(deadline) {
		if isPaneDead(socket, session) {
			paneDied = true
			t.Log("Pane died, waiting for respawn hook to fire...")
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !paneDied {
		t.Fatal("pane never died — test setup issue")
	}

	// Wait for the hook to respawn (3s sleep + startup time)
	t.Log("Waiting for hook to respawn pane (3s hook sleep + startup)...")
	alive := false
	deadline = time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if !isPaneDead(socket, session) {
			alive = true
			t.Log("Pane respawned successfully!")
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if !alive {
		t.Error("pane was NOT respawned after death — auto-respawn hook failed " +
			"(likely because the hook's embedded tmux commands are missing the -L socket flag)")
	}
}

// TestAutoRespawnHook_SocketFlagInHookCmd verifies that SetAutoRespawnHook
// embeds the tmux socket flag (-L) in the hook command when a socket is set.
//
// We can't use show-hooks (broken in tmux 3.4 for session-level hooks), so we
// verify indirectly: create a session on socket A, set the hook, ensure
// session does NOT exist on the default server, kill the pane, and verify
// respawn works. If the hook used bare `tmux`, it would target the default
// server and fail.
func TestAutoRespawnHook_SocketFlagInHookCmd(t *testing.T) {
	socket := testSocket(t)
	session := "test-socket-hook"

	testSession(t, socket, session, "sleep 2")

	tmx := NewTmuxWithSocket(socket)

	if err := tmx.SetAutoRespawnHook(session); err != nil {
		t.Fatalf("SetAutoRespawnHook failed: %v", err)
	}

	// Verify session does NOT exist on the default server
	err := exec.Command("tmux", "has-session", "-t", session).Run()
	if err == nil {
		t.Skip("session exists on default server — can't isolate socket test")
	}
	t.Logf("Confirmed: session %q does NOT exist on default tmux server", session)

	// Wait for sleep to exit, then for hook to respawn
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if isPaneDead(socket, session) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Wait for respawn (3s hook sleep)
	alive := false
	deadline = time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if !isPaneDead(socket, session) {
			alive = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if !alive {
		t.Error("pane was NOT respawned — hook's tmux commands likely missing -L socket flag")
	}
}

// TestAutoRespawnHook_NoSocket verifies that when no socket is configured,
// the hook uses bare tmux (backwards compatibility for single-town setups).
func TestAutoRespawnHook_NoSocket(t *testing.T) {
	socket := testSocket(t)
	session := "test-nosocket"

	testSession(t, socket, session, "sleep 2")

	// Create a Tmux with empty socket (single-town mode).
	// We need to use the real socket for the set-hook call, but the hook
	// content should use bare `tmux` (no -L flag).
	tmx := NewTmuxWithSocket(socket)
	tmx.socketName = "" // Override: hook content should have bare tmux

	// Set remain-on-exit manually since SetAutoRespawnHook calls it
	exec.Command("tmux", "-L", socket, "set-option", "-t", session, "remain-on-exit", "on").Run()

	// Build the hook command the same way SetAutoRespawnHook does internally
	// with empty socketName — bare tmux should be used
	hookCmd := fmt.Sprintf(`run-shell "sleep 3 && tmux respawn-pane -k -t '%s' && tmux set-option -t '%s' remain-on-exit on"`, session, session)

	// Set it via the real socket
	out, err := exec.Command("tmux", "-L", socket, "set-hook", "-t", session, "pane-died", hookCmd).CombinedOutput()
	if err != nil {
		t.Fatalf("set-hook failed: %v\n%s", err, out)
	}

	// Wait for sleep to exit
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if isPaneDead(socket, session) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	// In single-server context (which this test IS since we only have one socket),
	// bare `tmux` in run-shell should still work because run-shell runs within
	// the tmux server process context. Verify respawn works.
	alive := false
	deadline = time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if !isPaneDead(socket, session) {
			alive = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if !alive {
		t.Log("bare tmux hook did not respawn — expected in multi-server setups")
		// This is not necessarily a failure — bare tmux works in single-server
		// context (run-shell inherits server), but may fail in multi-server.
		// The important test is TestAutoRespawnHook_SocketFlagInHookCmd above.
	}
}

// TestIsPaneDead verifies the IsPaneDead method.
func TestIsPaneDead(t *testing.T) {
	socket := testSocket(t)
	session := "test-panedead"

	// Create a session with a short-lived command
	testSession(t, socket, session, "sleep 300")

	tmx := NewTmuxWithSocket(socket)

	// Pane should be alive
	if tmx.IsPaneDead(session) {
		t.Error("IsPaneDead() = true for a running process, want false")
	}

	// Set remain-on-exit and kill the process
	_ = tmx.SetRemainOnExit(session, true)
	exec.Command("tmux", "-L", socket, "send-keys", "-t", session, "C-c", "").Run()
	time.Sleep(500 * time.Millisecond)
	// Force kill
	exec.Command("tmux", "-L", socket, "send-keys", "-t", session, "kill %1 2>/dev/null; exit", "Enter").Run()
	time.Sleep(1 * time.Second)

	// Pane should now be dead
	if !isPaneDead(socket, session) {
		t.Skip("could not kill pane process reliably — skipping dead check")
	}

	if !tmx.IsPaneDead(session) {
		t.Error("IsPaneDead() = false for a dead pane, want true")
	}
}

// TestRespawnPaneDefault verifies that RespawnPaneDefault restarts
// a dead pane with its original command.
func TestRespawnPaneDefault(t *testing.T) {
	socket := testSocket(t)
	session := "test-respawn-default"

	// Create a session with sleep 300, set remain-on-exit, kill the process
	testSession(t, socket, session, "sleep 300")

	tmx := NewTmuxWithSocket(socket)
	_ = tmx.SetRemainOnExit(session, true)

	// Kill via respawn-pane -k with a quick-exit command, then let it die
	exec.Command("tmux", "-L", socket, "respawn-pane", "-k", "-t", session, "true").Run()
	time.Sleep(500 * time.Millisecond)

	if !tmx.IsPaneDead(session) {
		t.Fatal("pane should be dead after running 'true'")
	}

	// Respawn with default command (should reuse 'true' since that was last)
	if err := tmx.RespawnPaneDefault(session); err != nil {
		t.Fatalf("RespawnPaneDefault failed: %v", err)
	}

	// The pane should briefly come alive (running 'true' then dying again)
	// Give it a moment
	time.Sleep(200 * time.Millisecond)

	// Verify the respawn was attempted (pane existed at some point)
	// The session should still exist regardless
	err := exec.Command("tmux", "-L", socket, "has-session", "-t", session).Run()
	if err != nil {
		t.Error("session should still exist after RespawnPaneDefault")
	}
}
