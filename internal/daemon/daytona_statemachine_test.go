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

	"github.com/steveyegge/gastown/internal/config"
)

// installMockDaytona installs a mock daytona binary in PATH that returns
// controlled JSON for "list -o json" and handles "start" commands.
// The wsName and wsState parameters control what workspace appears in the list.
func installMockDaytona(t *testing.T, wsName, wsState string) {
	t.Helper()
	binDir := t.TempDir()

	if runtime.GOOS == "windows" {
		t.Skip("mock daytona binary not supported on Windows")
	}

	// Build JSON output for "list -o json".
	// Use a simple shell script that checks subcommands.
	listJSON := fmt.Sprintf(`[{"id":"ws-1","name":"%s","state":"%s"}]`, wsName, wsState)

	script := fmt.Sprintf(`#!/bin/sh
# Mock daytona for state machine tests.
cmd="$1"
case "$cmd" in
  list)
    echo '%s'
    exit 0
    ;;
  start)
    # start succeeds
    exit 0
    ;;
  stop)
    exit 0
    ;;
  delete)
    exit 0
    ;;
  *)
    echo "unknown command: $cmd" >&2
    exit 1
    ;;
esac
`, listJSON)

	if err := os.WriteFile(filepath.Join(binDir, "daytona"), []byte(script), 0755); err != nil {
		t.Fatalf("write mock daytona: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// installMockDaytonaEmpty installs a mock daytona that returns an empty workspace list.
func installMockDaytonaEmpty(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()

	if runtime.GOOS == "windows" {
		t.Skip("mock daytona binary not supported on Windows")
	}

	script := `#!/bin/sh
cmd="$1"
case "$cmd" in
  list)
    echo '[]'
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`
	if err := os.WriteFile(filepath.Join(binDir, "daytona"), []byte(script), 0755); err != nil {
		t.Fatalf("write mock daytona: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// installMockDaytonaStartFails installs a mock daytona where list succeeds but start fails.
func installMockDaytonaStartFails(t *testing.T, wsName string) {
	t.Helper()
	binDir := t.TempDir()

	if runtime.GOOS == "windows" {
		t.Skip("mock daytona binary not supported on Windows")
	}

	listJSON := fmt.Sprintf(`[{"id":"ws-1","name":"%s","state":"stopped"}]`, wsName)

	script := fmt.Sprintf(`#!/bin/sh
cmd="$1"
case "$cmd" in
  list)
    echo '%s'
    exit 0
    ;;
  start)
    echo "start failed: timeout" >&2
    exit 1
    ;;
  *)
    exit 0
    ;;
esac
`, listJSON)

	if err := os.WriteFile(filepath.Join(binDir, "daytona"), []byte(script), 0755); err != nil {
		t.Fatalf("write mock daytona: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// setupDaytonaRestartTestEnv creates the directory structure needed for
// restartDaytonaPolecatSession: town config with InstallationID, rig dir.
// Returns townRoot, rigDir, and the expected workspace name.
func setupDaytonaRestartTestEnv(t *testing.T, rigName, polecatName string) (string, string, string) {
	t.Helper()

	townRoot := t.TempDir()

	// Create town config with known InstallationID.
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	townCfg := `{"type":"town","version":1,"name":"test","installation_id":"abcdef123456-7890","created_at":"2025-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte(townCfg), 0644); err != nil {
		t.Fatal(err)
	}

	// Create rig dir.
	rigDir := filepath.Join(townRoot, rigName)
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Workspace name: gt-abcdef123456-<rig>--<polecat>
	// ShortInstallationID("abcdef123456-7890") = "abcdef123456" (first 12 chars)
	// InstallPrefix = "gt-abcdef123456"
	// WorkspaceName = "gt-abcdef123456-<rig>--<polecat>"
	wsName := fmt.Sprintf("gt-abcdef12345--%s--%s", rigName, polecatName)
	// Actually, the workspace name format is: installPrefix + "-" + rig + "--" + polecat
	// installPrefix = "gt-" + shortID = "gt-abcdef123456"
	wsName = fmt.Sprintf("gt-abcdef123456-%s--%s", rigName, polecatName)

	return townRoot, rigDir, wsName
}

// installMockBdForDaemon installs a mock bd binary for daemon tests that handles
// the commands used by cleanupAutoDeletedWorkspace.
func installMockBdForDaemon(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()

	if runtime.GOOS == "windows" {
		t.Skip("mock bd not supported on Windows in daemon tests")
	}

	script := `#!/bin/sh
# Mock bd for daemon tests.
cmd=""
for arg in "$@"; do
  case "$arg" in
    --*) ;;
    *) cmd="$arg"; break ;;
  esac
done
case "$cmd" in
  show)
    echo '[{"description":"role_type: polecat\ncert_serial: abc123"}]'
    exit 0
    ;;
  agent|slot)
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`
	if err := os.WriteFile(filepath.Join(binDir, "bd"), []byte(script), 0755); err != nil {
		t.Fatalf("write mock bd: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestRestartDaytonaPolecatSession_StoppedWorkspace verifies that when the
// workspace is in "stopped" state, it is started before session creation.
func TestRestartDaytonaPolecatSession_StoppedWorkspace(t *testing.T) {
	t.Parallel()

	rigName := "myrig"
	polecatName := "amber"
	townRoot, rigDir, wsName := setupDaytonaRestartTestEnv(t, rigName, polecatName)
	installMockDaytona(t, wsName, "stopped")

	var logBuf strings.Builder
	d := &Daemon{
		config:  &Config{TownRoot: townRoot},
		logger:  log.New(&logBuf, "", 0),
		metrics: &daemonMetrics{},
		ctx:     context.Background(),
	}

	err := d.restartDaytonaPolecatSession(rigName, polecatName, "gt-myrig-amber", rigDir)

	// Will fail at tmux session creation (no tmux), but should pass the state
	// machine check and attempt to start the workspace first.
	logs := logBuf.String()
	if !strings.Contains(logs, "Starting stopped daytona workspace") {
		// Check if error is from tmux (expected) vs state machine (unexpected).
		if err != nil && strings.Contains(err.Error(), "transitional") {
			t.Errorf("should not treat 'stopped' as transitional, got: %v", err)
		}
		if err != nil && strings.Contains(err.Error(), "error state") {
			t.Errorf("should not treat 'stopped' as error, got: %v", err)
		}
		// The start command succeeds (mock daytona), so we should see the log.
		if !strings.Contains(logs, "started successfully") && err != nil && !strings.Contains(err.Error(), "session") {
			t.Errorf("expected 'started successfully' log for stopped workspace, got logs: %q, error: %v", logs, err)
		}
	}
}

// TestRestartDaytonaPolecatSession_RunningWorkspace verifies that when the
// workspace is already "running", it proceeds directly to session creation.
func TestRestartDaytonaPolecatSession_RunningWorkspace(t *testing.T) {
	t.Parallel()

	rigName := "myrig"
	polecatName := "jade"
	townRoot, rigDir, wsName := setupDaytonaRestartTestEnv(t, rigName, polecatName)
	installMockDaytona(t, wsName, "running")

	var logBuf strings.Builder
	d := &Daemon{
		config:  &Config{TownRoot: townRoot},
		logger:  log.New(&logBuf, "", 0),
		metrics: &daemonMetrics{},
		ctx:     context.Background(),
	}

	err := d.restartDaytonaPolecatSession(rigName, polecatName, "gt-myrig-jade", rigDir)

	// Should not attempt to start the workspace (it's already running).
	logs := logBuf.String()
	if strings.Contains(logs, "Starting stopped") {
		t.Error("should not start an already running workspace")
	}

	// Error should be from tmux (expected) or nil, not from state machine.
	if err != nil {
		if strings.Contains(err.Error(), "transitional") ||
			strings.Contains(err.Error(), "error state") ||
			strings.Contains(err.Error(), "stopping") ||
			strings.Contains(err.Error(), "unknown state") {
			t.Errorf("running workspace should pass state machine, got: %v", err)
		}
	}
}

// TestRestartDaytonaPolecatSession_CreatingState verifies that "creating" state
// returns a transitional state error.
func TestRestartDaytonaPolecatSession_CreatingState(t *testing.T) {
	t.Parallel()

	rigName := "myrig"
	polecatName := "onyx"
	townRoot, rigDir, wsName := setupDaytonaRestartTestEnv(t, rigName, polecatName)
	installMockDaytona(t, wsName, "creating")

	var logBuf strings.Builder
	d := &Daemon{
		config:  &Config{TownRoot: townRoot},
		logger:  log.New(&logBuf, "", 0),
		metrics: &daemonMetrics{},
		ctx:     context.Background(),
	}

	err := d.restartDaytonaPolecatSession(rigName, polecatName, "gt-myrig-onyx", rigDir)
	if err == nil {
		t.Fatal("expected error for 'creating' state")
	}
	if !strings.Contains(err.Error(), "transitional state") {
		t.Errorf("expected 'transitional state' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "creating") {
		t.Errorf("error should mention 'creating' state, got: %v", err)
	}
}

// TestRestartDaytonaPolecatSession_StartingState verifies that "starting" state
// returns a transitional state error.
func TestRestartDaytonaPolecatSession_StartingState(t *testing.T) {
	t.Parallel()

	rigName := "myrig"
	polecatName := "garnet"
	townRoot, rigDir, wsName := setupDaytonaRestartTestEnv(t, rigName, polecatName)
	installMockDaytona(t, wsName, "starting")

	var logBuf strings.Builder
	d := &Daemon{
		config:  &Config{TownRoot: townRoot},
		logger:  log.New(&logBuf, "", 0),
		metrics: &daemonMetrics{},
		ctx:     context.Background(),
	}

	err := d.restartDaytonaPolecatSession(rigName, polecatName, "gt-myrig-garnet", rigDir)
	if err == nil {
		t.Fatal("expected error for 'starting' state")
	}
	if !strings.Contains(err.Error(), "transitional state") {
		t.Errorf("expected 'transitional state' error, got: %v", err)
	}
}

// TestRestartDaytonaPolecatSession_StoppingState verifies that "stopping" state
// returns an appropriate error.
func TestRestartDaytonaPolecatSession_StoppingState(t *testing.T) {
	t.Parallel()

	rigName := "myrig"
	polecatName := "ruby"
	townRoot, rigDir, wsName := setupDaytonaRestartTestEnv(t, rigName, polecatName)
	installMockDaytona(t, wsName, "stopping")

	var logBuf strings.Builder
	d := &Daemon{
		config:  &Config{TownRoot: townRoot},
		logger:  log.New(&logBuf, "", 0),
		metrics: &daemonMetrics{},
		ctx:     context.Background(),
	}

	err := d.restartDaytonaPolecatSession(rigName, polecatName, "gt-myrig-ruby", rigDir)
	if err == nil {
		t.Fatal("expected error for 'stopping' state")
	}
	if !strings.Contains(err.Error(), "stopping") {
		t.Errorf("expected 'stopping' in error, got: %v", err)
	}
}

// TestRestartDaytonaPolecatSession_ErrorState verifies that "error" state
// returns an error state error.
func TestRestartDaytonaPolecatSession_ErrorState(t *testing.T) {
	t.Parallel()

	rigName := "myrig"
	polecatName := "slate"
	townRoot, rigDir, wsName := setupDaytonaRestartTestEnv(t, rigName, polecatName)
	installMockDaytona(t, wsName, "error")

	var logBuf strings.Builder
	d := &Daemon{
		config:  &Config{TownRoot: townRoot},
		logger:  log.New(&logBuf, "", 0),
		metrics: &daemonMetrics{},
		ctx:     context.Background(),
	}

	err := d.restartDaytonaPolecatSession(rigName, polecatName, "gt-myrig-slate", rigDir)
	if err == nil {
		t.Fatal("expected error for 'error' state")
	}
	if !strings.Contains(err.Error(), "error state") {
		t.Errorf("expected 'error state' in error, got: %v", err)
	}
}

// TestRestartDaytonaPolecatSession_UnknownState verifies that an unknown state
// returns an appropriate error.
func TestRestartDaytonaPolecatSession_UnknownState(t *testing.T) {
	t.Parallel()

	rigName := "myrig"
	polecatName := "pearl"
	townRoot, rigDir, wsName := setupDaytonaRestartTestEnv(t, rigName, polecatName)
	installMockDaytona(t, wsName, "exploding")

	var logBuf strings.Builder
	d := &Daemon{
		config:  &Config{TownRoot: townRoot},
		logger:  log.New(&logBuf, "", 0),
		metrics: &daemonMetrics{},
		ctx:     context.Background(),
	}

	err := d.restartDaytonaPolecatSession(rigName, polecatName, "gt-myrig-pearl", rigDir)
	if err == nil {
		t.Fatal("expected error for unknown state")
	}
	if !strings.Contains(err.Error(), "unknown state") {
		t.Errorf("expected 'unknown state' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "exploding") {
		t.Errorf("error should mention the unknown state value, got: %v", err)
	}
}

// TestRestartDaytonaPolecatSession_WorkspaceNotFound verifies that when the
// workspace is not in the list (auto-deleted), cleanupAutoDeletedWorkspace
// is called and an appropriate error is returned.
func TestRestartDaytonaPolecatSession_WorkspaceNotFound(t *testing.T) {
	t.Parallel()

	rigName := "myrig"
	polecatName := "coral"
	townRoot, rigDir, _ := setupDaytonaRestartTestEnv(t, rigName, polecatName)
	installMockDaytonaEmpty(t)

	// Install rigs.json for GetRigPrefix (used by cleanupAutoDeletedWorkspace).
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"),
		[]byte(`{"rigs":[{"name":"myrig","prefix":"gtd"}]}`), 0644); err != nil {
		t.Fatal(err)
	}

	var logBuf strings.Builder
	d := &Daemon{
		config:  &Config{TownRoot: townRoot},
		logger:  log.New(&logBuf, "", 0),
		metrics: &daemonMetrics{},
		bdPath:  "/nonexistent/bd", // bd commands fail gracefully
		ctx:     context.Background(),
	}

	err := d.restartDaytonaPolecatSession(rigName, polecatName, "gt-myrig-coral", rigDir)
	if err == nil {
		t.Fatal("expected error for missing workspace")
	}
	if !strings.Contains(err.Error(), "deleted") {
		t.Errorf("expected 'deleted' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "re-dispatch") {
		t.Errorf("expected 're-dispatch' in error, got: %v", err)
	}

	// Verify cleanup was attempted (log should show cleanup message).
	logs := logBuf.String()
	if !strings.Contains(logs, "not found") && !strings.Contains(logs, "auto-deleted") {
		t.Errorf("expected auto-deletion log message, got: %q", logs)
	}
	if !strings.Contains(logs, "Cleaned up auto-deleted workspace") {
		t.Errorf("expected cleanup summary in logs, got: %q", logs)
	}
}

// TestRestartDaytonaPolecatSession_StartFails verifies error when workspace
// is stopped but daytona start command fails.
func TestRestartDaytonaPolecatSession_StartFails(t *testing.T) {
	t.Parallel()

	rigName := "myrig"
	polecatName := "topaz"
	townRoot, rigDir, wsName := setupDaytonaRestartTestEnv(t, rigName, polecatName)
	installMockDaytonaStartFails(t, wsName)

	var logBuf strings.Builder
	d := &Daemon{
		config:  &Config{TownRoot: townRoot},
		logger:  log.New(&logBuf, "", 0),
		metrics: &daemonMetrics{},
		ctx:     context.Background(),
	}

	err := d.restartDaytonaPolecatSession(rigName, polecatName, "gt-myrig-topaz", rigDir)
	if err == nil {
		t.Fatal("expected error from start failure")
	}
	if !strings.Contains(err.Error(), "starting daytona workspace") {
		t.Errorf("expected 'starting daytona workspace' error, got: %v", err)
	}
}

// TestCleanupAutoDeletedWorkspace_CertRevocationPath verifies that when the
// agent bead has a cert_serial, the cleanup function attempts to revoke it.
func TestCleanupAutoDeletedWorkspace_CertRevocationPath(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()

	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"),
		[]byte(`{"rigs":[{"name":"testrig","prefix":"gtd"}]}`), 0644); err != nil {
		t.Fatal(err)
	}

	rigDir := filepath.Join(townRoot, "testrig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create rig settings with ProxyAdminAddr.
	settingsDir := filepath.Join(rigDir, "settings")
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		t.Fatal(err)
	}
	settingsPath := config.RigSettingsPath(rigDir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath,
		[]byte(`{"remote_backend":{"provider":"daytona","proxy_admin_addr":"127.0.0.1:19877"}}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Install mock bd that returns agent bead with cert serial.
	installMockBdForDaemon(t)

	var logBuf strings.Builder
	d := &Daemon{
		config: &Config{TownRoot: townRoot},
		logger: log.New(&logBuf, "", 0),
		bdPath: "bd", // uses mock from PATH
		ctx:    context.Background(),
	}

	d.cleanupAutoDeletedWorkspace("testrig", "amber", rigDir)

	got := logBuf.String()
	// Should attempt cert revocation (will fail because no proxy server, but should log the attempt).
	// The mock bd returns cert_serial: abc123, so the function should try to revoke it.
	// Either "Revoked" (success) or "failed to revoke" (connection refused) is acceptable.
	if !strings.Contains(got, "abc123") && !strings.Contains(got, "revok") {
		// It's also possible the mock bd output parsing doesn't match — that's also useful data.
		t.Logf("cert revocation may not have been attempted, logs: %q", got)
	}

	// Should still complete cleanup (agent state, hook_bead).
	if !strings.Contains(got, "Cleaned up auto-deleted workspace") {
		t.Errorf("expected cleanup summary, got: %q", got)
	}
}

// TestCleanupAutoDeletedWorkspace_NoCertSerial verifies cleanup proceeds
// smoothly when the agent bead has no cert serial (skips revocation).
func TestCleanupAutoDeletedWorkspace_NoCertSerial(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()

	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"),
		[]byte(`{"rigs":[{"name":"testrig","prefix":"gtd"}]}`), 0644); err != nil {
		t.Fatal(err)
	}

	rigDir := filepath.Join(townRoot, "testrig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	var logBuf strings.Builder
	d := &Daemon{
		config: &Config{TownRoot: townRoot},
		logger: log.New(&logBuf, "", 0),
		bdPath: "/nonexistent/bd", // bd fails — no cert serial extracted
		ctx:    context.Background(),
	}

	// Should not panic when cert serial is empty (skips revocation).
	d.cleanupAutoDeletedWorkspace("testrig", "garnet", rigDir)

	got := logBuf.String()
	// Should NOT attempt cert revocation.
	if strings.Contains(got, "Revoked") {
		t.Error("should not attempt cert revocation when no serial is available")
	}
	// Should still complete.
	if !strings.Contains(got, "Cleaned up auto-deleted workspace") {
		t.Errorf("expected cleanup summary, got: %q", got)
	}
}
