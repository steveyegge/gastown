package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/polecat"
	"github.com/steveyegge/gastown/internal/tmux"
)

// writeFakeTestTmux creates a shell script in dir named "tmux" that simulates
// "session not found" for has-session calls and fails on anything else.
func writeFakeTestTmux(t *testing.T, dir string) {
	t.Helper()
	script := "#!/bin/sh\n" +
		"case \"$*\" in\n" +
		"  *has-session*) echo \"can't find session\" >&2; exit 1;;\n" +
		"  *) echo 'unexpected tmux command' >&2; exit 1;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0755); err != nil {
		t.Fatalf("writing fake tmux: %v", err)
	}
}

// writeFakeTestBD creates a shell script in dir named "bd" that outputs a
// polecat agent bead JSON. The descState parameter controls what appears in
// the description text (parsed by ParseAgentFields), while
// dbState controls the agent_state database column. updatedAt controls the
// bead's updated_at timestamp for time-bound testing.
func writeFakeTestBD(t *testing.T, dir, descState, dbState, hookBead, updatedAt string) string {
	t.Helper()
	desc := "agent_state: " + descState
	// JSON matches the structure that getAgentBeadInfo expects from bd show --json
	bdJSON := fmt.Sprintf(`[{"id":"gt-myr-polecat-mycat","issue_type":"agent","labels":["gt:agent"],"description":"%s","hook_bead":"%s","agent_state":"%s","updated_at":"%s"}]`,
		desc, hookBead, dbState, updatedAt)
	script := "#!/bin/sh\necho '" + bdJSON + "'\n"
	path := filepath.Join(dir, "bd")
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatalf("writing fake bd: %v", err)
	}
	return path
}

// writeFakeGt creates a shell script in dir named "gt" that logs invocations.
func writeFakeGt(t *testing.T, dir string) (gtPath, logPath string) {
	t.Helper()
	logPath = filepath.Join(t.TempDir(), "gt-invocations.log")
	gtPath = filepath.Join(dir, "gt")
	gtScript := fmt.Sprintf("#!/bin/sh\necho \"$@\" >> %s\n", logPath)
	if err := os.WriteFile(gtPath, []byte(gtScript), 0755); err != nil {
		t.Fatalf("writing fake gt: %v", err)
	}
	return gtPath, logPath
}

// newTestDaemon creates a Daemon with fake binaries for testing.
func newTestDaemon(t *testing.T, binDir string, bdPath, gtPath string) (*Daemon, *strings.Builder) {
	t.Helper()
	var logBuf strings.Builder
	townRoot := t.TempDir()
	d := &Daemon{
		config:         &Config{TownRoot: townRoot},
		logger:         log.New(&logBuf, "", 0),
		tmux:           tmux.NewTmux(),
		bdPath:         bdPath,
		gtPath:         gtPath,
		ctx:            context.Background(),
		restartTracker: NewRestartTracker(townRoot, RestartTrackerConfig{}),
	}
	return d, &logBuf
}

// TestCheckPolecatHealth_SkipsSpawning verifies that checkPolecatHealth does NOT
// attempt to restart a polecat in agent_state=spawning when recently updated.
// This is the regression test for the double-spawn bug (issue #1752): the daemon
// heartbeat fires during the window between bead creation (hook_bead set atomically
// by gt sling) and the actual tmux session launch, causing a second Claude process.
func TestCheckPolecatHealth_SkipsSpawning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell script mocks for tmux and bd")
	}
	binDir := t.TempDir()
	writeFakeTestTmux(t, binDir)
	// Use a recent timestamp so the spawning guard's time-bound is satisfied
	recentTime := time.Now().UTC().Format(time.RFC3339)
	bdPath := writeFakeTestBD(t, binDir, "spawning", "spawning", "gt-xyz", recentTime)

	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	var logBuf strings.Builder
	d := &Daemon{
		config: &Config{TownRoot: t.TempDir()},
		logger: log.New(&logBuf, "", 0),
		tmux:   tmux.NewTmux(),
		bdPath: bdPath,
	}

	d.checkPolecatHealth("myr", "mycat")

	got := logBuf.String()
	if !strings.Contains(got, "spawning") {
		t.Errorf("expected log to mention 'spawning', got: %q", got)
	}
	if strings.Contains(got, "CRASH DETECTED") {
		t.Errorf("spawning polecat must not trigger CRASH DETECTED, got: %q", got)
	}
}

// TestCheckPolecatHealth_DetectsCrashedPolecat verifies that checkPolecatHealth
// does detect a crash for a polecat in agent_state=working with a dead session.
// This ensures the spawning guard in issue #1752 does not accidentally suppress
// legitimate crash detection for polecats that were running normally.
func TestCheckPolecatHealth_DetectsCrashedPolecat(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell script mocks for tmux and bd")
	}
	binDir := t.TempDir()
	writeFakeTestTmux(t, binDir)
	recentTime := time.Now().UTC().Format(time.RFC3339)
	bdPath := writeFakeTestBD(t, binDir, "working", "working", "gt-xyz", recentTime)
	gtPath, _ := writeFakeGt(t, binDir)

	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	d, logBuf := newTestDaemon(t, binDir, bdPath, gtPath)

	d.checkPolecatHealth("myr", "mycat")

	got := logBuf.String()
	if !strings.Contains(got, "CRASH DETECTED") {
		t.Errorf("expected CRASH DETECTED for working polecat with dead session, got: %q", got)
	}
}

// TestCheckPolecatHealth_SpawningGuardExpires verifies that the spawning guard
// has a time-bound: polecats stuck in agent_state=spawning for more than 5 minutes
// are treated as crashed (gt sling may have failed during spawn).
func TestCheckPolecatHealth_SpawningGuardExpires(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell script mocks for tmux and bd")
	}
	binDir := t.TempDir()
	writeFakeTestTmux(t, binDir)
	// Use a timestamp >5 minutes ago to expire the spawning guard
	oldTime := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	bdPath := writeFakeTestBD(t, binDir, "spawning", "spawning", "gt-xyz", oldTime)
	gtPath, _ := writeFakeGt(t, binDir)

	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	d, logBuf := newTestDaemon(t, binDir, bdPath, gtPath)

	d.checkPolecatHealth("myr", "mycat")

	got := logBuf.String()
	if !strings.Contains(got, "Spawning guard expired") {
		t.Errorf("expected spawning guard to expire for old timestamp, got: %q", got)
	}
	if !strings.Contains(got, "CRASH DETECTED") {
		t.Errorf("expected CRASH DETECTED after spawning guard expires, got: %q", got)
	}
}

// TestCheckPolecatHealth_DBStateOverridesDescription verifies that the daemon
// reads agent_state from the DB column (source of truth), not the description
// text. UpdateAgentState updates the DB column but not the description, so a
// polecat that transitioned from "spawning" to "working" will have stale
// description text. The DB column must be authoritative.
func TestCheckPolecatHealth_DBStateOverridesDescription(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell script mocks for tmux and bd")
	}
	binDir := t.TempDir()
	writeFakeTestTmux(t, binDir)
	recentTime := time.Now().UTC().Format(time.RFC3339)
	// Description says "spawning" (stale) but DB column says "working" (truth)
	bdPath := writeFakeTestBD(t, binDir, "spawning", "working", "gt-xyz", recentTime)
	gtPath, _ := writeFakeGt(t, binDir)

	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	d, logBuf := newTestDaemon(t, binDir, bdPath, gtPath)

	d.checkPolecatHealth("myr", "mycat")

	got := logBuf.String()
	// Should NOT skip due to spawning guard — DB says "working"
	if strings.Contains(got, "Skipping restart") {
		t.Errorf("daemon should use DB agent_state (working), not stale description (spawning), got: %q", got)
	}
	// Should detect crash since DB says working + session is dead
	if !strings.Contains(got, "CRASH DETECTED") {
		t.Errorf("expected CRASH DETECTED when DB state is 'working' with dead session, got: %q", got)
	}
}

// TestCheckPolecatHealth_AutoRestartsOnCrash verifies that when a polecat
// crash is detected, the daemon auto-restarts it via `gt session restart`
// and records the restart in the tracker.
func TestCheckPolecatHealth_AutoRestartsOnCrash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell script mocks for tmux and bd")
	}
	binDir := t.TempDir()
	writeFakeTestTmux(t, binDir)
	recentTime := time.Now().UTC().Format(time.RFC3339)
	bdPath := writeFakeTestBD(t, binDir, "working", "working", "gt-xyz", recentTime)
	gtPath, gtLog := writeFakeGt(t, binDir)

	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	d, logBuf := newTestDaemon(t, binDir, bdPath, gtPath)

	d.checkPolecatHealth("myr", "mycat")

	got := logBuf.String()
	if !strings.Contains(got, "CRASH DETECTED") {
		t.Fatalf("expected CRASH DETECTED, got: %q", got)
	}
	if !strings.Contains(got, "Auto-restarting polecat myr/mycat") {
		t.Errorf("expected auto-restart log, got: %q", got)
	}

	// Verify gt session restart was called
	logData, err := os.ReadFile(gtLog)
	if err != nil {
		t.Fatalf("reading gt invocation log: %v", err)
	}
	invocations := string(logData)
	if !strings.Contains(invocations, "session restart myr/mycat --force") {
		t.Errorf("expected 'gt session restart myr/mycat --force', got: %q", invocations)
	}

	// Verify restart was recorded in tracker
	agentID := "myr/polecats/mycat"
	if d.restartTracker.CanRestart(agentID) {
		// After first restart, should be in backoff period
		remaining := d.restartTracker.GetBackoffRemaining(agentID)
		if remaining <= 0 {
			t.Errorf("expected backoff after restart, but backoff remaining is %v", remaining)
		}
	}
}

// TestCheckPolecatHealth_RespectsBackoff verifies that a polecat in backoff
// is not restarted until the backoff period expires.
func TestCheckPolecatHealth_RespectsBackoff(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell script mocks for tmux and bd")
	}
	binDir := t.TempDir()
	writeFakeTestTmux(t, binDir)
	recentTime := time.Now().UTC().Format(time.RFC3339)
	bdPath := writeFakeTestBD(t, binDir, "working", "working", "gt-xyz", recentTime)
	gtPath, gtLog := writeFakeGt(t, binDir)

	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	d, logBuf := newTestDaemon(t, binDir, bdPath, gtPath)

	// Pre-record a restart to put the agent in backoff
	agentID := "myr/polecats/mycat"
	d.restartTracker.RecordRestart(agentID)

	d.checkPolecatHealth("myr", "mycat")

	got := logBuf.String()
	if !strings.Contains(got, "CRASH DETECTED") {
		t.Fatalf("expected CRASH DETECTED, got: %q", got)
	}
	if !strings.Contains(got, "restart in backoff") {
		t.Errorf("expected backoff message, got: %q", got)
	}

	// Verify gt session restart was NOT called
	logData, _ := os.ReadFile(gtLog)
	if strings.Contains(string(logData), "session restart") {
		t.Errorf("should NOT have called gt session restart during backoff, got: %q", string(logData))
	}
}

// TestCheckPolecatHealth_CrashLoopNotifiesWitness verifies that when a polecat
// is in a crash loop, the daemon skips auto-restart but still notifies the witness.
func TestCheckPolecatHealth_CrashLoopNotifiesWitness(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell script mocks for tmux and bd")
	}
	binDir := t.TempDir()
	writeFakeTestTmux(t, binDir)
	recentTime := time.Now().UTC().Format(time.RFC3339)
	bdPath := writeFakeTestBD(t, binDir, "working", "working", "gt-xyz", recentTime)
	gtPath, gtLog := writeFakeGt(t, binDir)

	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	d, logBuf := newTestDaemon(t, binDir, bdPath, gtPath)

	// Simulate crash loop by recording many restarts
	agentID := "myr/polecats/mycat"
	for i := 0; i < 6; i++ {
		d.restartTracker.RecordRestart(agentID)
	}

	d.checkPolecatHealth("myr", "mycat")

	got := logBuf.String()
	if !strings.Contains(got, "crash loop") {
		t.Errorf("expected crash loop message, got: %q", got)
	}

	// Verify witness was notified (fallback when auto-restart is blocked)
	logData, err := os.ReadFile(gtLog)
	if err != nil {
		t.Fatalf("reading gt invocation log: %v", err)
	}
	invocations := string(logData)
	if !strings.Contains(invocations, "mail send") {
		t.Errorf("expected witness notification during crash loop, got: %q", invocations)
	}
	if !strings.Contains(invocations, "CRASHED_POLECAT") {
		t.Errorf("expected CRASHED_POLECAT in mail subject, got: %q", invocations)
	}
}

// TestCheckPolecatHealth_HeartbeatStaleWarning verifies that when a polecat session
// is alive but its heartbeat is stale, a warning is logged.
func TestCheckPolecatHealth_HeartbeatStaleWarning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell script mocks for tmux and bd")
	}

	// Create a fake tmux that reports session as alive
	binDir := t.TempDir()
	tmuxScript := "#!/bin/sh\n" +
		"case \"$*\" in\n" +
		"  *has-session*) exit 0;;\n" +
		"  *) echo 'unexpected tmux command' >&2; exit 1;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(binDir, "tmux"), []byte(tmuxScript), 0755); err != nil {
		t.Fatalf("writing fake tmux: %v", err)
	}

	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	townRoot := t.TempDir()
	var logBuf strings.Builder
	d := &Daemon{
		config:         &Config{TownRoot: townRoot},
		logger:         log.New(&logBuf, "", 0),
		tmux:           tmux.NewTmux(),
		restartTracker: NewRestartTracker(townRoot, RestartTrackerConfig{}),
		ctx:            context.Background(),
	}

	// Write a stale heartbeat with an old timestamp in the JSON
	sessionName := "gt-mycat"
	hbDir := filepath.Join(townRoot, ".runtime", "heartbeats")
	if err := os.MkdirAll(hbDir, 0755); err != nil {
		t.Fatalf("creating heartbeats dir: %v", err)
	}
	hbPath := filepath.Join(hbDir, sessionName+".json")
	staleJSON := fmt.Sprintf(`{"timestamp":"%s","state":"working","context":"test","bead":"gt-xyz"}`,
		time.Now().Add(-10*time.Minute).UTC().Format(time.RFC3339Nano))
	if err := os.WriteFile(hbPath, []byte(staleJSON), 0644); err != nil {
		t.Fatalf("writing stale heartbeat: %v", err)
	}

	d.checkPolecatHealth("myr", "mycat")

	got := logBuf.String()
	if !strings.Contains(got, "STALE HEARTBEAT") {
		t.Errorf("expected STALE HEARTBEAT warning, got: %q", got)
	}
}

// TestCheckPolecatHealth_FreshHeartbeatResetsBackoff verifies that a fresh
// heartbeat from an alive polecat resets the restart tracker backoff.
func TestCheckPolecatHealth_FreshHeartbeatResetsBackoff(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell script mocks for tmux and bd")
	}

	// Create a fake tmux that reports session as alive
	binDir := t.TempDir()
	tmuxScript := "#!/bin/sh\n" +
		"case \"$*\" in\n" +
		"  *has-session*) exit 0;;\n" +
		"  *) echo 'unexpected tmux command' >&2; exit 1;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(binDir, "tmux"), []byte(tmuxScript), 0755); err != nil {
		t.Fatalf("writing fake tmux: %v", err)
	}

	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	townRoot := t.TempDir()
	var logBuf strings.Builder
	rt := NewRestartTracker(townRoot, RestartTrackerConfig{
		StabilityPeriod: 0, // Reset immediately on success
	})
	d := &Daemon{
		config:         &Config{TownRoot: townRoot},
		logger:         log.New(&logBuf, "", 0),
		tmux:           tmux.NewTmux(),
		restartTracker: rt,
		ctx:            context.Background(),
	}

	// Pre-record some restarts
	agentID := "myr/polecats/mycat"
	rt.RecordRestart(agentID)
	rt.RecordRestart(agentID)

	// Write a fresh heartbeat
	sessionName := "gt-mycat"
	polecat.TouchSessionHeartbeatWithState(townRoot, sessionName, polecat.HeartbeatWorking, "test", "gt-xyz")

	d.checkPolecatHealth("myr", "mycat")

	// After fresh heartbeat, backoff should be cleared (stability period = 0)
	if !rt.CanRestart(agentID) {
		t.Errorf("expected backoff to be reset after fresh heartbeat, but agent is still in backoff")
	}
}
