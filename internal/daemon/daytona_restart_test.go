package daemon

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

// TestBuildDaytonaExecCommand verifies the command string construction for
// restarting a polecat inside a daytona workspace via daytona exec.
func TestBuildDaytonaExecCommand(t *testing.T) {
	t.Parallel()

	d := &Daemon{
		logger: log.New(&strings.Builder{}, "", 0),
	}

	envVars := map[string]string{
		"GT_RIG":     "myrig",
		"GT_POLECAT": "amber",
		"GT_RUN":     "test-run-id",
	}

	rc := &config.RuntimeConfig{
		Provider: "claude",
		Command:  "claude",
		Args:     []string{"--dangerously-skip-permissions"},
	}

	cmd := d.buildDaytonaExecCommand("gt-abc12345-myrig-amber", envVars, rc)

	// Must start with exec daytona exec <workspace>
	if !strings.HasPrefix(cmd, "exec daytona exec gt-abc12345-myrig-amber") {
		t.Errorf("command should start with 'exec daytona exec <ws>', got: %q", cmd)
	}

	// Must contain --env flags for each env var
	if !strings.Contains(cmd, "--env GT_RIG=") {
		t.Errorf("command should contain --env GT_RIG=, got: %q", cmd)
	}
	if !strings.Contains(cmd, "--env GT_POLECAT=") {
		t.Errorf("command should contain --env GT_POLECAT=, got: %q", cmd)
	}
	if !strings.Contains(cmd, "--env GT_RUN=") {
		t.Errorf("command should contain --env GT_RUN=, got: %q", cmd)
	}

	// Must contain -- separator before the agent command
	if !strings.Contains(cmd, "-- claude") {
		t.Errorf("command should contain '-- claude', got: %q", cmd)
	}

	// Must contain the agent args
	if !strings.Contains(cmd, "--dangerously-skip-permissions") {
		t.Errorf("command should contain agent args, got: %q", cmd)
	}
}

// TestBuildDaytonaExecCommand_EnvKeysSorted verifies env keys are sorted
// for deterministic output.
func TestBuildDaytonaExecCommand_EnvKeysSorted(t *testing.T) {
	t.Parallel()

	d := &Daemon{
		logger: log.New(&strings.Builder{}, "", 0),
	}

	envVars := map[string]string{
		"ZZ_LAST":  "last",
		"AA_FIRST": "first",
		"MM_MID":   "mid",
	}

	rc := &config.RuntimeConfig{
		Command: "claude",
		Args:    []string{"--dangerously-skip-permissions"},
	}

	cmd := d.buildDaytonaExecCommand("ws", envVars, rc)

	// AA should appear before MM, which should appear before ZZ
	aaIdx := strings.Index(cmd, "AA_FIRST")
	mmIdx := strings.Index(cmd, "MM_MID")
	zzIdx := strings.Index(cmd, "ZZ_LAST")

	if aaIdx == -1 || mmIdx == -1 || zzIdx == -1 {
		t.Fatalf("expected all env vars in command, got: %q", cmd)
	}
	if aaIdx > mmIdx || mmIdx > zzIdx {
		t.Errorf("env vars should be sorted, got: %q", cmd)
	}
}

// TestBuildDaytonaExecCommand_CustomAgent verifies the command works with
// non-Claude agents (e.g., codex).
func TestBuildDaytonaExecCommand_CustomAgent(t *testing.T) {
	t.Parallel()

	d := &Daemon{
		logger: log.New(&strings.Builder{}, "", 0),
	}

	envVars := map[string]string{
		"GT_RIG": "myrig",
	}

	rc := &config.RuntimeConfig{
		Provider: "codex",
		Command:  "codex",
		Args:     []string{"--approval-mode", "full-auto"},
	}

	cmd := d.buildDaytonaExecCommand("gt-abc-myrig-garnet", envVars, rc)

	if !strings.Contains(cmd, "-- codex") {
		t.Errorf("command should use codex agent, got: %q", cmd)
	}
	if !strings.Contains(cmd, "--approval-mode") {
		t.Errorf("command should contain codex args, got: %q", cmd)
	}
}

// TestRestartPolecatSession_DelegatesToDaytona verifies that when a rig has
// RemoteBackend configured (and agent bead is unreadable), restartPolecatSession
// falls back to config and delegates to the daytona restart path.
func TestRestartPolecatSession_DelegatesToDaytona(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()

	// Create rig settings with RemoteBackend
	rigDir := filepath.Join(townRoot, "myrig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	settingsPath := config.RigSettingsPath(rigDir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath,
		[]byte(`{"remote_backend":{"provider":"daytona"}}`), 0644); err != nil {
		t.Fatal(err)
	}

	var logBuf strings.Builder
	d := &Daemon{
		config:  &Config{TownRoot: townRoot},
		logger:  log.New(&logBuf, "", 0),
		metrics: &daemonMetrics{},
	}

	// restartDaytonaPolecatSession will fail (no town config), but we can
	// verify it was called by checking the error message.
	err := d.restartPolecatSession("myrig", "amber", "gt-myrig-amber")

	if err == nil {
		t.Fatal("expected error (no town config), got nil")
	}
	// The error should come from the daytona path, not the local path.
	// Local path would say "worktree does not exist".
	if strings.Contains(err.Error(), "worktree does not exist") {
		t.Errorf("should delegate to daytona path, not local path; error: %v", err)
	}
	if !strings.Contains(err.Error(), "daytona") && !strings.Contains(err.Error(), "town config") {
		t.Errorf("expected daytona-related error, got: %v", err)
	}
}

// TestIsPolecatDaytona_FailsWithoutBd verifies that isPolecatDaytona returns
// an error when the bd CLI is not available, triggering the config fallback.
func TestIsPolecatDaytona_FailsWithoutBd(t *testing.T) {
	t.Parallel()

	d := &Daemon{
		config: &Config{TownRoot: t.TempDir()},
		bdPath: "/nonexistent/bd",
	}

	isDaytona, err := d.isPolecatDaytona("myrig", "amber")
	if err == nil {
		t.Fatal("expected error when bd is not available, got nil")
	}
	if isDaytona {
		t.Error("expected isDaytona=false when bd fails")
	}
}

// TestRestartPolecatSession_LocalWhenNoRemoteBackend verifies that when a rig
// has no RemoteBackend, the local restart path is used.
func TestRestartPolecatSession_LocalWhenNoRemoteBackend(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()

	// Create rig settings without RemoteBackend
	rigDir := filepath.Join(townRoot, "myrig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	settingsPath := config.RigSettingsPath(rigDir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath,
		[]byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	var logBuf strings.Builder
	d := &Daemon{
		config:  &Config{TownRoot: townRoot},
		logger:  log.New(&logBuf, "", 0),
		metrics: &daemonMetrics{},
	}

	err := d.restartPolecatSession("myrig", "amber", "gt-myrig-amber")

	if err == nil {
		t.Fatal("expected error (no worktree), got nil")
	}
	// Local path error: worktree doesn't exist
	if !strings.Contains(err.Error(), "worktree does not exist") {
		t.Errorf("expected local worktree error, got: %v", err)
	}
}

// TestRestartDaytonaPolecatSession_MissingInstallationID verifies error when
// town config exists but has no InstallationID.
func TestRestartDaytonaPolecatSession_MissingInstallationID(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()

	// Create town config without InstallationID.
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	townCfg := `{"type":"town","version":1,"name":"test","created_at":"2025-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte(townCfg), 0644); err != nil {
		t.Fatal(err)
	}

	rigDir := filepath.Join(townRoot, "myrig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	var logBuf strings.Builder
	d := &Daemon{
		config:  &Config{TownRoot: townRoot},
		logger:  log.New(&logBuf, "", 0),
		metrics: &daemonMetrics{},
	}

	err := d.restartDaytonaPolecatSession("myrig", "amber", "gt-myrig-amber", rigDir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "InstallationID") {
		t.Errorf("expected InstallationID error, got: %v", err)
	}
}

// TestRestartDaytonaPolecatSession_WorkspaceNotFound verifies error when the
// workspace doesn't exist in daytona's workspace list.
func TestRestartDaytonaPolecatSession_ShortInstallationID(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()

	// Create town config with a short installation ID (< 12 chars).
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	townCfg := `{"type":"town","version":1,"name":"test","installation_id":"abc","created_at":"2025-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte(townCfg), 0644); err != nil {
		t.Fatal(err)
	}

	rigDir := filepath.Join(townRoot, "rig1")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	var logBuf strings.Builder
	d := &Daemon{
		config:  &Config{TownRoot: townRoot},
		logger:  log.New(&logBuf, "", 0),
		metrics: &daemonMetrics{},
	}

	// Will fail at ListOwned (daytona not installed), but verifies the short ID
	// path doesn't truncate beyond the string length.
	err := d.restartDaytonaPolecatSession("rig1", "onyx", "gt-rig1-onyx", rigDir)
	if err == nil {
		t.Fatal("expected error (no daytona CLI), got nil")
	}
	// Should contain "daytona" in error (from ListOwned failure),
	// not "InstallationID" (which would mean the short ID check failed).
	if strings.Contains(err.Error(), "InstallationID") {
		t.Errorf("short installation ID should be accepted, got: %v", err)
	}
}

// TestRestartDaytonaPolecatSession_DelegatesToDaytonaPath verifies that when
// RemoteBackend is configured and town config has an InstallationID, the
// restart function attempts to interact with daytona (fails in test env but
// confirms the correct code path is executed).
func TestRestartDaytonaPolecatSession_DelegatesToDaytonaPath(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()

	// Full town config with InstallationID.
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	townCfg := `{"type":"town","version":1,"name":"test","installation_id":"abcdef12-3456-7890","created_at":"2025-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(mayorDir, "town.json"), []byte(townCfg), 0644); err != nil {
		t.Fatal(err)
	}

	// Create rig settings with RemoteBackend.
	rigDir := filepath.Join(townRoot, "myrig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}
	settingsPath := config.RigSettingsPath(rigDir)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath,
		[]byte(`{"remote_backend":{"provider":"daytona"}}`), 0644); err != nil {
		t.Fatal(err)
	}

	var logBuf strings.Builder
	d := &Daemon{
		config:  &Config{TownRoot: townRoot},
		logger:  log.New(&logBuf, "", 0),
		metrics: &daemonMetrics{},
	}

	err := d.restartDaytonaPolecatSession("myrig", "amber", "gt-myrig-amber", rigDir)
	if err == nil {
		t.Fatal("expected error (no daytona CLI), got nil")
	}

	// Error should be from the daytona ListOwned call, not from town config issues.
	errMsg := err.Error()
	if strings.Contains(errMsg, "town config") || strings.Contains(errMsg, "InstallationID") {
		t.Errorf("should pass config checks and fail at daytona interaction, got: %v", err)
	}
	if !strings.Contains(errMsg, "daytona") && !strings.Contains(errMsg, "workspace") {
		t.Errorf("error should relate to daytona workspace operations, got: %v", err)
	}
}

// TestCleanupAutoDeletedWorkspace_NoFatalOnBdFailure verifies that
// cleanupAutoDeletedWorkspace doesn't panic when bd CLI commands fail
// (e.g., non-existent bd path). All operations are best-effort.
func TestCleanupAutoDeletedWorkspace_NoFatalOnBdFailure(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()

	// Create rigs.json so GetRigPrefix resolves (avoids fallback noise).
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"),
		[]byte(`{"version":1,"rigs":{"myrig":{"beads":{"prefix":"gtd"}}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	rigDir := filepath.Join(townRoot, "myrig")
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		t.Fatal(err)
	}

	var logBuf strings.Builder
	d := &Daemon{
		config: &Config{TownRoot: townRoot},
		logger: log.New(&logBuf, "", 0),
		bdPath: "/nonexistent/bd", // bd not available — all commands should fail gracefully
		ctx:    context.Background(),
	}

	// Should not panic or fatal — all bd operations are best-effort.
	d.cleanupAutoDeletedWorkspace("myrig", "garnet", rigDir)

	got := logBuf.String()
	// Should log warnings about failed commands (since bd doesn't exist).
	if !strings.Contains(got, "Warning") {
		t.Errorf("expected warning logs for failed bd commands, got: %q", got)
	}
	// Should log the final cleanup summary.
	if !strings.Contains(got, "Cleaned up auto-deleted workspace") {
		t.Errorf("expected cleanup summary log, got: %q", got)
	}
}

// TestCleanupAutoDeletedWorkspace_LogsAgentBeadID verifies that the cleanup
// function correctly computes and logs the agent bead ID.
func TestCleanupAutoDeletedWorkspace_LogsAgentBeadID(t *testing.T) {
	t.Parallel()

	townRoot := t.TempDir()

	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Configure prefix "gtd" for the rig.
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"),
		[]byte(`{"version":1,"rigs":{"testrig":{"beads":{"prefix":"gtd"}}}}`), 0644); err != nil {
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
		bdPath: "/nonexistent/bd",
		ctx:    context.Background(),
	}

	d.cleanupAutoDeletedWorkspace("testrig", "amber", rigDir)

	got := logBuf.String()
	// The bead ID should be logged in the cleanup summary.
	// Expected format: gtd-testrig-polecat-amber
	if !strings.Contains(got, "gtd-testrig-polecat-amber") {
		t.Errorf("expected agent bead ID in log output, got: %q", got)
	}
}
