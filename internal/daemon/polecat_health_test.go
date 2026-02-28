package daemon

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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
	// Should NOT skip due to spawning guard — DB says "working"
	if strings.Contains(got, "Skipping restart") {
		t.Errorf("daemon should use DB agent_state (working), not stale description (spawning), got: %q", got)
	}
	// Should detect crash since DB says working + session is dead
	if !strings.Contains(got, "CRASH DETECTED") {
		t.Errorf("expected CRASH DETECTED when DB state is 'working' with dead session, got: %q", got)
	}
}

// TestCheckPolecatHealth_NotifiesWitnessOnCrash verifies that when a polecat
// crash is detected, the daemon sends a notification to the witness via
// `gt mail send` with a CRASHED_POLECAT subject. This ensures the Mayor has
// visibility into crashes even when auto-restart handles recovery.
func TestCheckPolecatHealth_NotifiesWitnessOnCrash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix shell script mocks for tmux and bd")
	}
	binDir := t.TempDir()
	writeFakeTestTmux(t, binDir)
	recentTime := time.Now().UTC().Format(time.RFC3339)
	bdPath := writeFakeTestBD(t, binDir, "working", "working", "gt-xyz", recentTime)

	// Create a fake gt script that logs invocations to a file
	gtLog := filepath.Join(t.TempDir(), "gt-invocations.log")
	fakeGt := filepath.Join(binDir, "gt")
	gtScript := fmt.Sprintf("#!/bin/sh\necho \"$@\" >> %s\n", gtLog)
	if err := os.WriteFile(fakeGt, []byte(gtScript), 0755); err != nil {
		t.Fatalf("writing fake gt: %v", err)
	}

	t.Setenv("PATH", binDir+":"+os.Getenv("PATH"))

	townRoot := t.TempDir()
	var logBuf strings.Builder
	d := &Daemon{
		config: &Config{TownRoot: townRoot},
		logger: log.New(&logBuf, "", 0),
		tmux:   tmux.NewTmux(),
		bdPath: bdPath,
		gtPath: fakeGt,
	}

	d.checkPolecatHealth("myr", "mycat")

	got := logBuf.String()
	if !strings.Contains(got, "CRASH DETECTED") {
		t.Fatalf("expected CRASH DETECTED, got: %q", got)
	}

	// Verify gt mail send was called with CRASHED_POLECAT subject
	logData, err := os.ReadFile(gtLog)
	if err != nil {
		t.Fatalf("reading gt invocation log: %v", err)
	}
	invocations := string(logData)
	if !strings.Contains(invocations, "mail send") {
		t.Errorf("expected gt mail send invocation, got: %q", invocations)
	}
	if !strings.Contains(invocations, "CRASHED_POLECAT") {
		t.Errorf("expected CRASHED_POLECAT in mail subject, got: %q", invocations)
	}
	if !strings.Contains(invocations, "myr/witness") {
		t.Errorf("expected witness address myr/witness, got: %q", invocations)
	}
}
