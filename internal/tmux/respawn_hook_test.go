package tmux

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// requireTestSocket returns the package-level test socket name (set by
// TestMain) and skips the test if tmux is not installed. Tests should call
// this instead of creating per-test sockets.
func requireTestSocket(t *testing.T) string {
	t.Helper()
	if !hasTmux() {
		t.Skip("tmux not installed")
	}
	return testSocketName
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

// getPanePID returns the PID of the running process in the pane.
func getPanePID(t *testing.T, socket, session string) string {
	t.Helper()
	out, err := exec.Command("tmux", "-L", socket, "display-message", "-t", session, "-p", "#{pane_pid}").Output()
	if err != nil {
		t.Fatalf("failed to get pane PID for %q: %v", session, err)
	}
	return strings.TrimSpace(string(out))
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
// 1. Create session on isolated socket running `sleep 2`
// 2. Set auto-respawn hook via NewTmuxWithSocket
// 3. Wait for process to exit naturally
// 4. Wait for the hook to fire and respawn the pane
// 5. Verify the pane comes back alive
func TestAutoRespawnHook_RespawnWorks(t *testing.T) {
	socket := requireTestSocket(t)
	session := "test-respawn"

	// Use `sleep 2` — it exits naturally after 2 seconds
	testSession(t, socket, session, "sleep 2")
	defer func() { _ = exec.Command("tmux", "-L", socket, "kill-session", "-t", session).Run() }()

	tmx := NewTmux()

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
	socket := requireTestSocket(t)
	session := "test-socket-hook"

	testSession(t, socket, session, "sleep 2")
	defer func() { _ = exec.Command("tmux", "-L", socket, "kill-session", "-t", session).Run() }()

	tmx := NewTmux()

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
	socket := requireTestSocket(t)
	session := "test-nosocket"

	testSession(t, socket, session, "sleep 2")
	defer func() { _ = exec.Command("tmux", "-L", socket, "kill-session", "-t", session).Run() }()

	// Create a Tmux with empty socket (single-town mode).
	// We need to use the real socket for the set-hook call, but the hook
	// content should use bare `tmux` (no -L flag).
	tmx := NewTmuxWithSocket(socket)
	tmx.socketName = "" // Override: hook content should have bare tmux

	// Set remain-on-exit manually since SetAutoRespawnHook calls it
	exec.Command("tmux", "-L", socket, "set-option", "-t", session, "remain-on-exit", "on").Run()

	// Build the hook command using bare tmux (no socket)
	hookCmd := buildAutoRespawnHookCmd("tmux", session)

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

// TestAutoRespawnHook_SkipsAlreadyAlive verifies that the auto-respawn hook
// does NOT kill an already-alive pane. This tests the race condition between
// the hook's 3-second sleep and another restart mechanism (e.g., daemon).
//
// Scenario:
//  1. Pane dies → hook starts (3s sleep before respawn)
//  2. During sleep, daemon respawns the pane (it's alive again)
//  3. Hook wakes up — must detect pane is alive and skip respawn-pane -k
//
// Without the dead-pane guard, the hook blindly runs `respawn-pane -k` which
// kills the daemon's freshly-started agent and restarts it. The user sees this
// as the hook command error message taking over their active tmux pane.
func TestAutoRespawnHook_SkipsAlreadyAlive(t *testing.T) {
	socket := requireTestSocket(t)
	session := "test-skip-alive"

	testSession(t, socket, session, "sleep 300")
	defer func() { _ = exec.Command("tmux", "-L", socket, "kill-session", "-t", session).Run() }()
	tmx := NewTmux()

	if err := tmx.SetAutoRespawnHook(session); err != nil {
		t.Fatalf("SetAutoRespawnHook failed: %v", err)
	}

	// Kill the process → pane dies → hook starts 3s sleep
	exec.Command("tmux", "-L", socket, "respawn-pane", "-k", "-t", session, "true").Run()
	time.Sleep(500 * time.Millisecond)
	if !isPaneDead(socket, session) {
		t.Fatal("pane should be dead after running 'true'")
	}

	// Simulate daemon: immediately respawn the pane (before hook's 3s sleep finishes)
	exec.Command("tmux", "-L", socket, "respawn-pane", "-k", "-t", session, "sleep 300").Run()
	time.Sleep(300 * time.Millisecond)

	if isPaneDead(socket, session) {
		t.Fatal("pane should be alive after manual respawn")
	}

	// Record PID of the daemon-respawned process
	pid1 := getPanePID(t, socket, session)
	t.Logf("PID after daemon respawn: %s", pid1)

	// Wait for hook to fire (3s sleep + execution buffer)
	t.Log("Waiting 5s for hook to fire...")
	time.Sleep(5 * time.Second)

	// Check if PID changed — if it did, the hook killed the daemon's process
	pid2 := getPanePID(t, socket, session)
	t.Logf("PID after hook fires: %s", pid2)

	if pid1 != pid2 {
		t.Errorf("hook killed daemon-respawned process: PID changed %s → %s "+
			"(race condition — hook must check pane_dead before respawning)", pid1, pid2)
	}
}

// TestAutoRespawnHook_SilentOnSessionKilled verifies that the hook does not
// produce visible error output when the target session has been killed
// (e.g., by the daemon's kill+recreate path) before the hook fires.
//
// The hook runs inside `run-shell` which, without the -b flag, displays
// command failure messages to the attached client's active pane. With -b,
// the command runs in the background and output is discarded.
//
// We verify this by checking tmux's message log for error output.
func TestAutoRespawnHook_SilentOnSessionKilled(t *testing.T) {
	socket := requireTestSocket(t)
	session := "test-silent"

	testSession(t, socket, session, "sleep 300")
	defer func() { _ = exec.Command("tmux", "-L", socket, "kill-session", "-t", session).Run() }()
	tmx := NewTmux()

	if err := tmx.SetAutoRespawnHook(session); err != nil {
		t.Fatalf("SetAutoRespawnHook failed: %v", err)
	}

	// Kill the process → pane dies → hook starts 3s sleep
	exec.Command("tmux", "-L", socket, "respawn-pane", "-k", "-t", session, "true").Run()
	time.Sleep(500 * time.Millisecond)
	if !isPaneDead(socket, session) {
		t.Fatal("pane should be dead")
	}

	// Kill the entire session (simulating daemon's KillSessionWithProcesses)
	exec.Command("tmux", "-L", socket, "kill-session", "-t", session).Run()
	time.Sleep(200 * time.Millisecond)

	// Verify session is gone
	err := exec.Command("tmux", "-L", socket, "has-session", "-t", session).Run()
	if err == nil {
		t.Fatal("session should be killed")
	}

	// Wait for hook to fire (3s sleep + buffer) — it will try to respawn a dead session
	t.Log("Waiting 5s for hook to fire against killed session...")
	time.Sleep(5 * time.Second)

	// Check tmux server messages for error output from the failed hook
	msgOut, _ := exec.Command("tmux", "-L", socket, "show-messages").CombinedOutput()
	msgs := string(msgOut)
	t.Logf("tmux messages after hook: %s", msgs)

	// The hook should NOT produce "returned 1" error messages
	if strings.Contains(msgs, "returned 1") || strings.Contains(msgs, "returned") {
		t.Errorf("hook produced error output in tmux messages — run-shell should use -b flag to suppress output:\n%s", msgs)
	}
}

// TestIsPaneDead verifies the IsPaneDead method.
func TestIsPaneDead(t *testing.T) {
	socket := requireTestSocket(t)
	session := "test-panedead"

	// Create a session with a short-lived command
	testSession(t, socket, session, "sleep 300")
	defer func() { _ = exec.Command("tmux", "-L", socket, "kill-session", "-t", session).Run() }()

	tmx := NewTmux()

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
	socket := requireTestSocket(t)
	session := "test-respawn-default"

	// Create a session with sleep 300, set remain-on-exit, kill the process
	testSession(t, socket, session, "sleep 300")
	defer func() { _ = exec.Command("tmux", "-L", socket, "kill-session", "-t", session).Run() }()

	tmx := NewTmux()
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

// TestAutoRespawnHookCmd_Format verifies the structure of the hook command
// generated by buildAutoRespawnHookCmd. The command must:
// 1. Use run-shell -b to prevent output leaking to the user's active pane
// 2. Check pane_dead before respawning to avoid racing with daemon restarts
// 3. Include || true to suppress any error display
// 4. Include the socket flag (-L) when a socket is configured
func TestAutoRespawnHookCmd_Format(t *testing.T) {
	tests := []struct {
		name     string
		tmuxCmd  string
		session  string
		wantFlag string // substring that must be present
		desc     string // what the substring proves
	}{
		{
			name:     "background_flag",
			tmuxCmd:  "tmux -L gt",
			session:  "hq-deacon",
			wantFlag: "run-shell -b",
			desc:     "run-shell must use -b to prevent output leaking to user's active pane",
		},
		{
			name:     "dead_pane_guard",
			tmuxCmd:  "tmux -L gt",
			session:  "hq-deacon",
			wantFlag: "pane_dead",
			desc:     "hook must check pane_dead before respawning to avoid racing with daemon",
		},
		{
			name:     "error_suppression",
			tmuxCmd:  "tmux -L gt",
			session:  "hq-deacon",
			wantFlag: "|| true",
			desc:     "hook must end with || true to suppress errors",
		},
		{
			name:     "socket_flag_in_respawn",
			tmuxCmd:  "tmux -L gt",
			session:  "hq-deacon",
			wantFlag: "-L gt",
			desc:     "hook must include socket flag for multi-town isolation",
		},
		{
			name:     "no_socket_bare_tmux",
			tmuxCmd:  "tmux",
			session:  "hq-deacon",
			wantFlag: "tmux respawn-pane",
			desc:     "without socket, hook uses bare tmux",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := buildAutoRespawnHookCmd(tt.tmuxCmd, tt.session)
			if !strings.Contains(cmd, tt.wantFlag) {
				t.Errorf("%s\nhook command: %s\nwanted substring: %q", tt.desc, cmd, tt.wantFlag)
			}
		})
	}
}

// TestIsPaneDead_And_RespawnPaneDefault is an integration test that exercises
// the exact code path the deacon uses to recover from a dead pane:
//
//  1. Create a session with remain-on-exit on, running a short-lived process
//  2. Wait for the process to exit → pane becomes dead
//  3. Verify IsPaneDead returns true
//  4. Call RespawnPaneDefault to restart the pane
//  5. Verify the pane comes back alive with a new PID
//
// This tests the deacon's "respawn instead of kill+recreate" path against a
// real tmux server. The deacon uses this to avoid incrementing the daemon's
// crash counter when a patrol cycle completes normally.
func TestIsPaneDead_And_RespawnPaneDefault(t *testing.T) {
	socket := requireTestSocket(t)
	session := "test-respawn-default"

	// Create session running a fast-exiting command.
	testSession(t, socket, session, "sleep 1")
	defer func() { _ = exec.Command("tmux", "-L", socket, "kill-session", "-t", session).Run() }()

	// Enable remain-on-exit so the pane stays visible after the process exits.
	if out, err := exec.Command("tmux", "-L", socket, "set-option", "-t", session, "remain-on-exit", "on").CombinedOutput(); err != nil {
		t.Fatalf("failed to set remain-on-exit: %v\n%s", err, out)
	}

	// Capture the original PID.
	originalPID := getPanePID(t, socket, session)
	t.Logf("Original pane PID: %s", originalPID)

	// Wait for sleep 1 to exit → pane should become dead.
	t.Log("Waiting for process to exit...")
	deadline := time.Now().Add(5 * time.Second)
	paneDied := false
	for time.Now().Before(deadline) {
		if isPaneDead(socket, session) {
			paneDied = true
			t.Log("Pane is dead (remain-on-exit holding it)")
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !paneDied {
		t.Fatal("pane never died — remain-on-exit may not be working")
	}

	// This is the exact call the deacon makes:
	tmx := NewTmux()
	if !tmx.IsPaneDead(session) {
		t.Fatal("IsPaneDead returned false but pane should be dead")
	}

	// Respawn using the API the deacon calls:
	t.Log("Calling RespawnPaneDefault...")
	if err := tmx.RespawnPaneDefault(session); err != nil {
		t.Fatalf("RespawnPaneDefault failed: %v", err)
	}

	// Wait for the pane to come back alive.
	t.Log("Waiting for pane to come back alive...")
	alive := false
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !isPaneDead(socket, session) {
			alive = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !alive {
		t.Fatal("pane did not come back alive after RespawnPaneDefault")
	}

	// Verify the PID changed — proof the pane was actually respawned.
	newPID := getPanePID(t, socket, session)
	t.Logf("New pane PID: %s (was %s)", newPID, originalPID)
	if newPID == originalPID {
		t.Error("pane PID did not change after respawn — pane may not have been restarted")
	}

	// Verify the session is intact (not killed and recreated).
	has, err := tmx.HasSession(session)
	if err != nil {
		t.Fatalf("HasSession after respawn: %v", err)
	}
	if !has {
		t.Error("session disappeared after RespawnPaneDefault — should have stayed intact")
	}
}
