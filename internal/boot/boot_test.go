package boot

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/tmux"
)

func TestNew(t *testing.T) {
	b := New("/tmp/testtown")

	if b.townRoot != "/tmp/testtown" {
		t.Errorf("townRoot = %q, want %q", b.townRoot, "/tmp/testtown")
	}
	if b.bootDir != "/tmp/testtown/deacon/dogs/boot" {
		t.Errorf("bootDir = %q, want %q", b.bootDir, "/tmp/testtown/deacon/dogs/boot")
	}
	if b.deaconDir != "/tmp/testtown/deacon" {
		t.Errorf("deaconDir = %q, want %q", b.deaconDir, "/tmp/testtown/deacon")
	}
	if b.tmux == nil {
		t.Error("tmux should not be nil")
	}
	if b.degraded {
		t.Error("degraded should be false when GT_DEGRADED is not set")
	}
}

func TestNew_Degraded(t *testing.T) {
	t.Setenv("GT_DEGRADED", "true")
	b := New("/tmp/testtown")
	if !b.degraded {
		t.Error("degraded should be true when GT_DEGRADED=true")
	}
}

func TestNew_DegradedFalse(t *testing.T) {
	t.Setenv("GT_DEGRADED", "false")
	b := New("/tmp/testtown")
	if b.degraded {
		t.Error("degraded should be false when GT_DEGRADED=false")
	}
}

func TestEnsureDir(t *testing.T) {
	dir := t.TempDir()
	b := &Boot{
		bootDir: filepath.Join(dir, "a", "b", "c"),
	}

	if err := b.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir() error: %v", err)
	}

	info, err := os.Stat(b.bootDir)
	if err != nil {
		t.Fatalf("boot dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("boot dir is not a directory")
	}

	// Idempotent
	if err := b.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir() second call error: %v", err)
	}
}

func TestDir(t *testing.T) {
	b := &Boot{bootDir: "/test/boot"}
	if b.Dir() != "/test/boot" {
		t.Errorf("Dir() = %q, want %q", b.Dir(), "/test/boot")
	}
}

func TestDeaconDir(t *testing.T) {
	b := &Boot{deaconDir: "/test/deacon"}
	if b.DeaconDir() != "/test/deacon" {
		t.Errorf("DeaconDir() = %q, want %q", b.DeaconDir(), "/test/deacon")
	}
}

func TestTmux(t *testing.T) {
	b := New("/tmp/test")
	if b.Tmux() == nil {
		t.Error("Tmux() returned nil")
	}
}

func TestIsDegraded(t *testing.T) {
	b := &Boot{degraded: false}
	if b.IsDegraded() {
		t.Error("IsDegraded() = true, want false")
	}

	b.degraded = true
	if !b.IsDegraded() {
		t.Error("IsDegraded() = false, want true")
	}
}

func TestMarkerPath(t *testing.T) {
	b := &Boot{bootDir: "/test/boot"}
	want := "/test/boot/" + MarkerFileName
	if b.markerPath() != want {
		t.Errorf("markerPath() = %q, want %q", b.markerPath(), want)
	}
}

func TestStatusPath(t *testing.T) {
	b := &Boot{bootDir: "/test/boot"}
	want := "/test/boot/" + StatusFileName
	if b.statusPath() != want {
		t.Errorf("statusPath() = %q, want %q", b.statusPath(), want)
	}
}

func TestSaveStatus(t *testing.T) {
	dir := t.TempDir()
	b := &Boot{bootDir: filepath.Join(dir, "boot")}

	now := time.Now().Truncate(time.Second)
	status := &Status{
		Running:     true,
		StartedAt:   now,
		LastAction:  "wake",
		Target:      "deacon",
	}

	if err := b.SaveStatus(status); err != nil {
		t.Fatalf("SaveStatus() error: %v", err)
	}

	// Verify file written
	data, err := os.ReadFile(b.statusPath())
	if err != nil {
		t.Fatalf("reading status file: %v", err)
	}

	var loaded Status
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshaling status: %v", err)
	}

	if !loaded.Running {
		t.Error("Running = false, want true")
	}
	if loaded.LastAction != "wake" {
		t.Errorf("LastAction = %q, want %q", loaded.LastAction, "wake")
	}
	if loaded.Target != "deacon" {
		t.Errorf("Target = %q, want %q", loaded.Target, "deacon")
	}
}

func TestSaveStatus_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	b := &Boot{bootDir: filepath.Join(dir, "nested", "boot")}

	if err := b.SaveStatus(&Status{LastAction: "test"}); err != nil {
		t.Fatalf("SaveStatus() error: %v", err)
	}

	if _, err := os.Stat(b.statusPath()); err != nil {
		t.Errorf("status file not created: %v", err)
	}
}

func TestLoadStatus(t *testing.T) {
	dir := t.TempDir()
	b := &Boot{bootDir: dir}

	// Write a status file
	now := time.Now().Truncate(time.Second)
	original := &Status{
		Running:     false,
		StartedAt:   now,
		CompletedAt: now.Add(5 * time.Second),
		LastAction:  "nudge",
		Target:      "witness",
		Error:       "",
	}
	data, _ := json.MarshalIndent(original, "", "  ")
	if err := os.WriteFile(b.statusPath(), data, 0644); err != nil {
		t.Fatal(err)
	}

	loaded, err := b.LoadStatus()
	if err != nil {
		t.Fatalf("LoadStatus() error: %v", err)
	}

	if loaded.Running != original.Running {
		t.Errorf("Running = %v, want %v", loaded.Running, original.Running)
	}
	if loaded.LastAction != "nudge" {
		t.Errorf("LastAction = %q, want %q", loaded.LastAction, "nudge")
	}
	if loaded.Target != "witness" {
		t.Errorf("Target = %q, want %q", loaded.Target, "witness")
	}
	if !loaded.StartedAt.Equal(original.StartedAt) {
		t.Errorf("StartedAt mismatch")
	}
	if !loaded.CompletedAt.Equal(original.CompletedAt) {
		t.Errorf("CompletedAt mismatch")
	}
}

func TestLoadStatus_NoFile(t *testing.T) {
	dir := t.TempDir()
	b := &Boot{bootDir: dir}

	// No status file exists — should return empty status, no error
	status, err := b.LoadStatus()
	if err != nil {
		t.Fatalf("LoadStatus() error: %v", err)
	}
	if status.Running {
		t.Error("Running should be false for empty status")
	}
	if status.LastAction != "" {
		t.Errorf("LastAction = %q, want empty", status.LastAction)
	}
}

func TestLoadStatus_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	b := &Boot{bootDir: dir}

	if err := os.WriteFile(b.statusPath(), []byte("{bad json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := b.LoadStatus()
	if err == nil {
		t.Error("LoadStatus() expected error for invalid JSON")
	}
}

func TestSaveAndLoadStatus_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	b := &Boot{bootDir: dir}

	now := time.Now().Truncate(time.Second)
	original := &Status{
		Running:     true,
		StartedAt:   now,
		CompletedAt: now.Add(10 * time.Second),
		LastAction:  "start",
		Target:      "deacon",
		Error:       "something went wrong",
	}

	if err := b.SaveStatus(original); err != nil {
		t.Fatalf("SaveStatus() error: %v", err)
	}

	loaded, err := b.LoadStatus()
	if err != nil {
		t.Fatalf("LoadStatus() error: %v", err)
	}

	if loaded.Running != original.Running {
		t.Errorf("Running = %v, want %v", loaded.Running, original.Running)
	}
	if loaded.LastAction != original.LastAction {
		t.Errorf("LastAction = %q, want %q", loaded.LastAction, original.LastAction)
	}
	if loaded.Target != original.Target {
		t.Errorf("Target = %q, want %q", loaded.Target, original.Target)
	}
	if loaded.Error != original.Error {
		t.Errorf("Error = %q, want %q", loaded.Error, original.Error)
	}
	if !loaded.StartedAt.Equal(original.StartedAt) {
		t.Errorf("StartedAt mismatch")
	}
	if !loaded.CompletedAt.Equal(original.CompletedAt) {
		t.Errorf("CompletedAt mismatch")
	}
}

func TestLoadStatus_WithErrorField(t *testing.T) {
	dir := t.TempDir()
	b := &Boot{bootDir: dir}

	status := &Status{
		LastAction: "wake",
		Error:      "tmux session not responding",
	}
	b.SaveStatus(status)

	loaded, err := b.LoadStatus()
	if err != nil {
		t.Fatalf("LoadStatus() error: %v", err)
	}
	if loaded.Error != "tmux session not responding" {
		t.Errorf("Error = %q, want %q", loaded.Error, "tmux session not responding")
	}
}

// --- Lock tests (existing) ---

func TestAcquireLock(t *testing.T) {
	tmpDir := t.TempDir()

	b := &Boot{
		townRoot: tmpDir,
		bootDir:  filepath.Join(tmpDir, "deacon", "dogs", "boot"),
	}

	// First acquire should succeed
	if err := b.AcquireLock(); err != nil {
		t.Fatalf("First AcquireLock failed: %v", err)
	}

	// Verify marker file exists
	markerPath := filepath.Join(b.bootDir, MarkerFileName)
	if _, err := os.Stat(markerPath); os.IsNotExist(err) {
		t.Error("Marker file was not created")
	}

	// Verify lock is held by trying to acquire from another flock instance
	otherLock := flock.New(markerPath)
	locked, err := otherLock.TryLock()
	if err != nil {
		t.Fatalf("TryLock failed: %v", err)
	}
	if locked {
		t.Error("Should not be able to acquire lock while first lock is held")
		_ = otherLock.Unlock()
	}

	// Release should succeed
	if err := b.ReleaseLock(); err != nil {
		t.Fatalf("ReleaseLock failed: %v", err)
	}

	// After release, another instance should be able to acquire
	locked, err = otherLock.TryLock()
	if err != nil {
		t.Fatalf("TryLock after release failed: %v", err)
	}
	if !locked {
		t.Error("Should be able to acquire lock after release")
	}
	_ = otherLock.Unlock()
}

func TestAcquireLockConcurrent(t *testing.T) {
	tmpDir := t.TempDir()

	b1 := &Boot{
		townRoot: tmpDir,
		bootDir:  filepath.Join(tmpDir, "deacon", "dogs", "boot"),
	}
	b2 := &Boot{
		townRoot: tmpDir,
		bootDir:  filepath.Join(tmpDir, "deacon", "dogs", "boot"),
	}

	// First boot acquires lock
	if err := b1.AcquireLock(); err != nil {
		t.Fatalf("First boot AcquireLock failed: %v", err)
	}

	// Second boot should fail to acquire
	err := b2.AcquireLock()
	if err == nil {
		t.Error("Second boot should have failed to acquire lock")
		_ = b2.ReleaseLock()
	}

	// Release first lock
	if err := b1.ReleaseLock(); err != nil {
		t.Fatalf("First boot ReleaseLock failed: %v", err)
	}

	// Now second boot should succeed
	if err := b2.AcquireLock(); err != nil {
		t.Fatalf("Second boot AcquireLock after release failed: %v", err)
	}

	_ = b2.ReleaseLock()
}

func TestReleaseLockIdempotent(t *testing.T) {
	tmpDir := t.TempDir()

	b := &Boot{
		townRoot: tmpDir,
		bootDir:  filepath.Join(tmpDir, "deacon", "dogs", "boot"),
	}

	// Release without acquire should not error (lockHandle is nil)
	if err := b.ReleaseLock(); err != nil {
		t.Errorf("ReleaseLock without acquire should not error: %v", err)
	}

	// Acquire then release twice should not error
	if err := b.AcquireLock(); err != nil {
		t.Fatalf("AcquireLock failed: %v", err)
	}
	if err := b.ReleaseLock(); err != nil {
		t.Fatalf("First ReleaseLock failed: %v", err)
	}
	if err := b.ReleaseLock(); err != nil {
		t.Errorf("Second ReleaseLock should not error: %v", err)
	}
}

// --- Session and spawn tests using mock overrides ---

// withMockHasSession overrides hasSessionFn for the duration of a test.
func withMockHasSession(t *testing.T, fn func(*tmux.Tmux, string) (bool, error)) {
	t.Helper()
	orig := hasSessionFn
	t.Cleanup(func() { hasSessionFn = orig })
	hasSessionFn = fn
}

// withMockKillSession overrides killSessionFn for the duration of a test.
func withMockKillSession(t *testing.T, fn func(*tmux.Tmux, string) error) {
	t.Helper()
	orig := killSessionFn
	t.Cleanup(func() { killSessionFn = orig })
	killSessionFn = fn
}

// withMockStartSession overrides startSessionFn for the duration of a test.
func withMockStartSession(t *testing.T, fn func(*tmux.Tmux, session.SessionConfig) (*session.StartResult, error)) {
	t.Helper()
	orig := startSessionFn
	t.Cleanup(func() { startSessionFn = orig })
	startSessionFn = fn
}

// withMockStartCmd overrides startCmdFn for the duration of a test.
func withMockStartCmd(t *testing.T, fn func(*exec.Cmd) error) {
	t.Helper()
	orig := startCmdFn
	t.Cleanup(func() { startCmdFn = orig })
	startCmdFn = fn
}

func TestIsRunning_SessionAlive(t *testing.T) {
	withMockHasSession(t, func(_ *tmux.Tmux, _ string) (bool, error) {
		return true, nil
	})

	b := New(t.TempDir())
	if !b.IsRunning() {
		t.Error("IsRunning() = false, want true when session is alive")
	}
}

func TestIsRunning_SessionDead(t *testing.T) {
	withMockHasSession(t, func(_ *tmux.Tmux, _ string) (bool, error) {
		return false, nil
	})

	b := New(t.TempDir())
	if b.IsRunning() {
		t.Error("IsRunning() = true, want false when session is dead")
	}
}

func TestIsSessionAlive_Error(t *testing.T) {
	withMockHasSession(t, func(_ *tmux.Tmux, _ string) (bool, error) {
		return false, errors.New("tmux not running")
	})

	b := New(t.TempDir())
	if b.IsSessionAlive() {
		t.Error("IsSessionAlive() = true, want false on error")
	}
}

func TestSpawn_Degraded(t *testing.T) {
	withMockStartCmd(t, func(cmd *exec.Cmd) error {
		// Verify command is configured correctly
		if len(cmd.Args) < 4 || cmd.Args[3] != "--degraded" {
			t.Errorf("unexpected cmd args: %v", cmd.Args)
		}
		return nil
	})

	dir := t.TempDir()
	b := &Boot{
		townRoot:  dir,
		bootDir:   filepath.Join(dir, "boot"),
		deaconDir: filepath.Join(dir, "deacon"),
		degraded:  true,
	}

	if err := b.Spawn(""); err != nil {
		t.Fatalf("Spawn() degraded error: %v", err)
	}
}

func TestSpawn_DegradedError(t *testing.T) {
	withMockStartCmd(t, func(cmd *exec.Cmd) error {
		return errors.New("process failed to start")
	})

	dir := t.TempDir()
	b := &Boot{
		townRoot:  dir,
		bootDir:   filepath.Join(dir, "boot"),
		deaconDir: filepath.Join(dir, "deacon"),
		degraded:  true,
	}

	if err := b.Spawn(""); err == nil {
		t.Error("Spawn() degraded expected error")
	}
}

func TestSpawn_Tmux(t *testing.T) {
	withMockHasSession(t, func(_ *tmux.Tmux, _ string) (bool, error) {
		return false, nil
	})
	withMockStartSession(t, func(_ *tmux.Tmux, cfg session.SessionConfig) (*session.StartResult, error) {
		if cfg.Role != "boot" {
			t.Errorf("SessionConfig.Role = %q, want %q", cfg.Role, "boot")
		}
		return &session.StartResult{}, nil
	})

	dir := t.TempDir()
	b := &Boot{
		townRoot:  dir,
		bootDir:   filepath.Join(dir, "boot"),
		deaconDir: filepath.Join(dir, "deacon"),
		tmux:      tmux.NewTmux(),
		degraded:  false,
	}

	if err := b.Spawn(""); err != nil {
		t.Fatalf("Spawn() tmux error: %v", err)
	}
}

func TestSpawn_TmuxKillsStaleSession(t *testing.T) {
	killCalled := false
	withMockHasSession(t, func(_ *tmux.Tmux, _ string) (bool, error) {
		return true, nil // session exists
	})
	withMockKillSession(t, func(_ *tmux.Tmux, _ string) error {
		killCalled = true
		return nil
	})
	withMockStartSession(t, func(_ *tmux.Tmux, _ session.SessionConfig) (*session.StartResult, error) {
		return &session.StartResult{}, nil
	})

	dir := t.TempDir()
	b := &Boot{
		townRoot:  dir,
		bootDir:   filepath.Join(dir, "boot"),
		deaconDir: filepath.Join(dir, "deacon"),
		tmux:      tmux.NewTmux(),
		degraded:  false,
	}

	if err := b.Spawn(""); err != nil {
		t.Fatalf("Spawn() error: %v", err)
	}
	if !killCalled {
		t.Error("expected KillSessionWithProcesses to be called for stale session")
	}
}

func TestSpawn_TmuxStartSessionError(t *testing.T) {
	withMockHasSession(t, func(_ *tmux.Tmux, _ string) (bool, error) {
		return false, nil
	})
	withMockStartSession(t, func(_ *tmux.Tmux, _ session.SessionConfig) (*session.StartResult, error) {
		return nil, errors.New("tmux session create failed")
	})

	dir := t.TempDir()
	b := &Boot{
		townRoot:  dir,
		bootDir:   filepath.Join(dir, "boot"),
		deaconDir: filepath.Join(dir, "deacon"),
		tmux:      tmux.NewTmux(),
		degraded:  false,
	}

	err := b.Spawn("")
	if err == nil {
		t.Error("Spawn() expected error from StartSession failure")
	}
}

func TestSpawn_TmuxWithAgentOverride(t *testing.T) {
	withMockHasSession(t, func(_ *tmux.Tmux, _ string) (bool, error) {
		return false, nil
	})
	var capturedCfg session.SessionConfig
	withMockStartSession(t, func(_ *tmux.Tmux, cfg session.SessionConfig) (*session.StartResult, error) {
		capturedCfg = cfg
		return &session.StartResult{}, nil
	})

	dir := t.TempDir()
	b := &Boot{
		townRoot:  dir,
		bootDir:   filepath.Join(dir, "boot"),
		deaconDir: filepath.Join(dir, "deacon"),
		tmux:      tmux.NewTmux(),
		degraded:  false,
	}

	if err := b.Spawn("custom-agent"); err != nil {
		t.Fatalf("Spawn() error: %v", err)
	}
	if capturedCfg.AgentOverride != "custom-agent" {
		t.Errorf("AgentOverride = %q, want %q", capturedCfg.AgentOverride, "custom-agent")
	}
}
