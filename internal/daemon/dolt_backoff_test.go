package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAdvanceBackoff(t *testing.T) {
	m := &DoltServerManager{
		config: &DoltServerConfig{
			RestartDelay:    5 * time.Second,
			MaxRestartDelay: 5 * time.Minute,
		},
		logger: func(format string, v ...interface{}) {},
	}

	// First advance: 5s -> 10s
	m.advanceBackoff()
	if m.currentDelay != 10*time.Second {
		t.Errorf("expected 10s, got %v", m.currentDelay)
	}

	// Second advance: 10s -> 20s
	m.advanceBackoff()
	if m.currentDelay != 20*time.Second {
		t.Errorf("expected 20s, got %v", m.currentDelay)
	}

	// Third: 20s -> 40s
	m.advanceBackoff()
	if m.currentDelay != 40*time.Second {
		t.Errorf("expected 40s, got %v", m.currentDelay)
	}

	// Fourth: 40s -> 80s
	m.advanceBackoff()
	if m.currentDelay != 80*time.Second {
		t.Errorf("expected 80s, got %v", m.currentDelay)
	}

	// Fifth: 80s -> 160s
	m.advanceBackoff()
	if m.currentDelay != 160*time.Second {
		t.Errorf("expected 160s, got %v", m.currentDelay)
	}

	// Sixth: 160s -> 300s (capped at 5min)
	m.advanceBackoff()
	if m.currentDelay != 5*time.Minute {
		t.Errorf("expected 5m0s (cap), got %v", m.currentDelay)
	}

	// Stays capped
	m.advanceBackoff()
	if m.currentDelay != 5*time.Minute {
		t.Errorf("expected 5m0s (still capped), got %v", m.currentDelay)
	}
}

func TestGetBackoffDelay_InitialValue(t *testing.T) {
	m := &DoltServerManager{
		config: &DoltServerConfig{
			RestartDelay: 5 * time.Second,
		},
		logger: func(format string, v ...interface{}) {},
	}

	// Before any advances, should return base delay
	delay := m.getBackoffDelay()
	if delay != 5*time.Second {
		t.Errorf("expected initial delay 5s, got %v", delay)
	}
}

func TestPruneRestartTimes(t *testing.T) {
	now := time.Now()
	m := &DoltServerManager{
		config: &DoltServerConfig{
			RestartWindow: 10 * time.Minute,
		},
		logger: func(format string, v ...interface{}) {},
		restartTimes: []time.Time{
			now.Add(-15 * time.Minute), // Outside window
			now.Add(-11 * time.Minute), // Outside window
			now.Add(-5 * time.Minute),  // Inside window
			now.Add(-1 * time.Minute),  // Inside window
		},
	}

	m.pruneRestartTimes(now)

	if len(m.restartTimes) != 2 {
		t.Errorf("expected 2 times after pruning, got %d", len(m.restartTimes))
	}
}

func TestMaybeResetBackoff(t *testing.T) {
	m := &DoltServerManager{
		config: &DoltServerConfig{
			HealthyResetInterval: 5 * time.Minute,
		},
		logger:       func(format string, v ...interface{}) {},
		currentDelay: 40 * time.Second,
		restartTimes: []time.Time{time.Now()},
		escalated:    true,
	}

	// First call sets lastHealthyTime
	m.maybeResetBackoff()
	if m.currentDelay != 40*time.Second {
		t.Error("should not reset on first healthy check")
	}

	// Simulate time passing (set lastHealthyTime to 6 minutes ago)
	m.lastHealthyTime = time.Now().Add(-6 * time.Minute)
	m.maybeResetBackoff()

	if m.currentDelay != 0 {
		t.Errorf("expected delay reset to 0, got %v", m.currentDelay)
	}
	if m.restartTimes != nil {
		t.Error("expected restartTimes to be nil after reset")
	}
	if m.escalated {
		t.Error("expected escalated to be false after reset")
	}
}

func TestMaybeResetBackoff_NoResetIfNotLongEnough(t *testing.T) {
	m := &DoltServerManager{
		config: &DoltServerConfig{
			HealthyResetInterval: 5 * time.Minute,
		},
		logger:          func(format string, v ...interface{}) {},
		currentDelay:    40 * time.Second,
		lastHealthyTime: time.Now().Add(-2 * time.Minute), // Only 2 min healthy
		restartTimes:    []time.Time{time.Now()},
	}

	m.maybeResetBackoff()

	if m.currentDelay != 40*time.Second {
		t.Errorf("should not reset after only 2 minutes, got delay %v", m.currentDelay)
	}
}

func TestMaybeResetBackoff_AccumulatesAcrossHeartbeats(t *testing.T) {
	// Regression test: with the bug, lastHealthyTime was updated on every call,
	// so the delta never exceeded the heartbeat interval. With the fix,
	// lastHealthyTime is only updated on initial detection and after a successful
	// reset, allowing the delta to accumulate across multiple heartbeat calls.
	m := &DoltServerManager{
		config: &DoltServerConfig{
			HealthyResetInterval: 10 * time.Minute,
		},
		logger:       func(format string, v ...interface{}) {},
		currentDelay: 40 * time.Second,
		restartTimes: []time.Time{time.Now()},
		escalated:    true,
	}

	// First call: sets lastHealthyTime to now
	m.maybeResetBackoff()
	if m.currentDelay != 40*time.Second {
		t.Fatal("should not reset on first call")
	}

	// Simulate calling every 1 minute for 9 minutes (short heartbeats).
	// With the bug, each call reset lastHealthyTime so delta was always ~1min.
	// With the fix, lastHealthyTime stays at the initial value.
	for i := 1; i <= 9; i++ {
		m.maybeResetBackoff()
	}
	// After 9 calls at ~0 delta each (in test time), still should not reset
	// because no real time has passed. But importantly, lastHealthyTime should
	// NOT have been updated on these calls.
	if m.currentDelay != 40*time.Second {
		t.Fatal("should not have reset yet")
	}

	// Now set lastHealthyTime to 11 minutes ago (simulating accumulated healthy time)
	// This should trigger a reset because the initial healthy detection was >10min ago.
	m.lastHealthyTime = time.Now().Add(-11 * time.Minute)
	m.maybeResetBackoff()

	if m.currentDelay != 0 {
		t.Errorf("expected delay reset to 0 after 11 minutes healthy, got %v", m.currentDelay)
	}
	if m.escalated {
		t.Error("expected escalated to be false after reset")
	}
}

func TestDefaultConfig_BackoffFields(t *testing.T) {
	cfg := DefaultDoltServerConfig("/tmp/test")

	if cfg.MaxRestartDelay != 5*time.Minute {
		t.Errorf("expected MaxRestartDelay 5m, got %v", cfg.MaxRestartDelay)
	}
	if cfg.MaxRestartsInWindow != 5 {
		t.Errorf("expected MaxRestartsInWindow 5, got %d", cfg.MaxRestartsInWindow)
	}
	if cfg.RestartWindow != 10*time.Minute {
		t.Errorf("expected RestartWindow 10m, got %v", cfg.RestartWindow)
	}
	if cfg.HealthyResetInterval != 5*time.Minute {
		t.Errorf("expected HealthyResetInterval 5m, got %v", cfg.HealthyResetInterval)
	}
	if cfg.HealthCheckInterval != DefaultDoltHealthCheckInterval {
		t.Errorf("expected HealthCheckInterval %v, got %v", DefaultDoltHealthCheckInterval, cfg.HealthCheckInterval)
	}
}

func TestHealthCheckInterval_Default(t *testing.T) {
	m := &DoltServerManager{
		config: &DoltServerConfig{
			Enabled: true,
		},
		logger: func(format string, v ...interface{}) {},
	}

	// When HealthCheckInterval is not set (zero), should return default
	interval := m.HealthCheckInterval()
	if interval != DefaultDoltHealthCheckInterval {
		t.Errorf("expected default %v, got %v", DefaultDoltHealthCheckInterval, interval)
	}
}

func TestHealthCheckInterval_Configured(t *testing.T) {
	m := &DoltServerManager{
		config: &DoltServerConfig{
			Enabled:             true,
			HealthCheckInterval: 15 * time.Second,
		},
		logger: func(format string, v ...interface{}) {},
	}

	interval := m.HealthCheckInterval()
	if interval != 15*time.Second {
		t.Errorf("expected 15s, got %v", interval)
	}
}

func TestHealthCheckInterval_NilConfig(t *testing.T) {
	m := &DoltServerManager{
		config: nil,
		logger: func(format string, v ...interface{}) {},
	}

	interval := m.HealthCheckInterval()
	if interval != DefaultDoltHealthCheckInterval {
		t.Errorf("expected default %v with nil config, got %v", DefaultDoltHealthCheckInterval, interval)
	}
}

func TestRestartingFlag_PreventsConcurrentRestarts(t *testing.T) {
	// Verify the restarting flag prevents concurrent calls to EnsureRunning
	// from both entering restartWithBackoff.
	var callCount atomic.Int32
	m := &DoltServerManager{
		config: &DoltServerConfig{
			Enabled:             true,
			Port:                13306, // Non-standard port to avoid conflicts
			Host:                "127.0.0.1",
			RestartDelay:        50 * time.Millisecond,
			MaxRestartDelay:     100 * time.Millisecond,
			MaxRestartsInWindow: 10,
			RestartWindow:       10 * time.Minute,
		},
		logger: func(format string, v ...interface{}) {},
	}

	// Simulate: set restarting=true as if restartWithBackoff is sleeping
	m.mu.Lock()
	m.restarting = true
	m.mu.Unlock()

	// Multiple concurrent EnsureRunning calls should all return immediately
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := m.EnsureRunning()
			if err == nil {
				callCount.Add(1)
			}
		}()
	}
	wg.Wait()

	// All 5 should have returned nil (skipped because restarting=true)
	if got := callCount.Load(); got != 5 {
		t.Errorf("expected all 5 goroutines to return nil (skipped), got %d", got)
	}
}

func TestStartLocked_SkipsIfAlreadyRunning(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("isProcessAlive uses Signal(nil) on Windows which doesn't reliably detect live processes")
	}
	// Verify that startLocked() re-checks isRunning() to close the TOCTOU window.
	// If the server is already running (m.process is alive), startLocked() should
	// return nil without attempting to start a second instance.
	tmpDir := t.TempDir()
	daemonDir := filepath.Join(tmpDir, "daemon")
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		t.Fatal(err)
	}

	var logMessages []string
	m := &DoltServerManager{
		config: &DoltServerConfig{
			Enabled:  true,
			Port:     13307,
			Host:     "127.0.0.1",
			DataDir:  filepath.Join(tmpDir, "dolt"),
			LogFile:  filepath.Join(daemonDir, "dolt-server.log"),
		},
		townRoot: tmpDir,
		logger: func(format string, v ...interface{}) {
			logMessages = append(logMessages, fmt.Sprintf(format, v...))
		},
	}

	// Set m.process to our own process so isRunning() returns true.
	// Our own process is always alive.
	self, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	m.process = self

	// Call startLocked() with the mutex held (as the contract requires).
	m.mu.Lock()
	err = m.startLocked()
	m.mu.Unlock()

	if err != nil {
		t.Fatalf("expected nil error when server already running, got: %v", err)
	}

	// Verify the skip was logged
	found := false
	for _, msg := range logMessages {
		if msg == "Dolt server already running, skipping start" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'already running' log message, got: %v", logMessages)
	}
}

func TestRestartWithBackoff_SkipsIfStartedDuringSleep(t *testing.T) {
	// Verify that restartWithBackoff() re-checks isRunning() after the backoff
	// sleep to detect if another goroutine started the server during the window.
	//
	// Previous version used time.Sleep races which caused deadlocks when OS
	// scheduling varied. This version uses channel-based synchronization via
	// test hooks for deterministic behavior.
	sleepStarted := make(chan struct{})
	sleepDone := make(chan struct{})

	var running atomic.Bool
	var logMessages []string
	var logMu sync.Mutex

	m := newTestManager(t)
	m.logger = func(format string, v ...interface{}) {
		logMu.Lock()
		logMessages = append(logMessages, fmt.Sprintf(format, v...))
		logMu.Unlock()
	}
	m.runningFn = func() (int, bool) {
		if running.Load() {
			return os.Getpid(), true
		}
		return 0, false
	}
	m.sleepFn = func(d time.Duration) {
		close(sleepStarted)
		<-sleepDone
	}
	m.startFn = func() error {
		t.Error("startLocked should not be called when server started during sleep")
		return nil
	}

	// restartWithBackoff expects to be called with m.mu held
	m.mu.Lock()

	done := make(chan error, 1)
	go func() {
		done <- m.restartWithBackoff()
	}()

	// Wait for the goroutine to enter backoff sleep (lock is released during sleep)
	<-sleepStarted

	// Simulate another goroutine starting the server during the backoff window
	running.Store(true)

	// Let the sleep finish — goroutine will re-acquire lock and check isRunning()
	close(sleepDone)

	// Wait for restartWithBackoff to complete
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil error when server started during backoff, got: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("restartWithBackoff never completed — possible deadlock")
	}

	// Verify the skip was logged
	logMu.Lock()
	defer logMu.Unlock()
	found := false
	for _, msg := range logMessages {
		if strings.Contains(msg, "started by another goroutine during backoff") ||
			strings.Contains(msg, "already running, skipping start") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected skip log message, got: %v", logMessages)
	}
}

func TestWriteAndClearUnhealthySignal(t *testing.T) {
	tmpDir := t.TempDir()
	daemonDir := filepath.Join(tmpDir, "daemon")
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		t.Fatal(err)
	}

	m := &DoltServerManager{
		config:   DefaultDoltServerConfig(tmpDir),
		townRoot: tmpDir,
		logger:   func(format string, v ...interface{}) {},
	}

	// Initially no signal
	if IsDoltUnhealthy(tmpDir) {
		t.Error("expected no unhealthy signal initially")
	}

	// Write signal
	m.writeUnhealthySignal("server_dead", "PID 12345 is dead")

	if !IsDoltUnhealthy(tmpDir) {
		t.Error("expected unhealthy signal after write")
	}

	// Verify signal file contains JSON
	data, err := os.ReadFile(m.unhealthySignalFile())
	if err != nil {
		t.Fatalf("failed to read signal file: %v", err)
	}
	content := string(data)
	if content == "" {
		t.Error("signal file should not be empty")
	}

	// Clear signal
	m.clearUnhealthySignal()

	if IsDoltUnhealthy(tmpDir) {
		t.Error("expected no unhealthy signal after clear")
	}
}

func TestUnhealthySignalFile_Path(t *testing.T) {
	m := &DoltServerManager{
		config:   DefaultDoltServerConfig("/tmp/test-town"),
		townRoot: "/tmp/test-town",
		logger:   func(format string, v ...interface{}) {},
	}

	expected := filepath.Join("/tmp", "test-town", "daemon", "DOLT_UNHEALTHY")
	if got := m.unhealthySignalFile(); got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestIsDoltUnhealthy_NoDir(t *testing.T) {
	// Non-existent directory should return false
	if IsDoltUnhealthy("/nonexistent/path") {
		t.Error("expected false for non-existent directory")
	}
}

// ============================================================================
// Concurrent EnsureRunning and integrated backoff tests
// ============================================================================

// newTestManager creates a DoltServerManager with test hooks for concurrency testing.
// Default hooks: server not running, health check passes, start succeeds.
func newTestManager(t *testing.T) *DoltServerManager {
	t.Helper()
	tmpDir := t.TempDir()
	daemonDir := filepath.Join(tmpDir, "daemon")
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		t.Fatal(err)
	}
	return &DoltServerManager{
		config: &DoltServerConfig{
			Enabled:              true,
			Port:                 13306,
			Host:                 "127.0.0.1",
			RestartDelay:         10 * time.Millisecond,
			MaxRestartDelay:      100 * time.Millisecond,
			MaxRestartsInWindow:  5,
			RestartWindow:        10 * time.Minute,
			HealthyResetInterval: 50 * time.Millisecond,
		},
		townRoot:      tmpDir,
		logger:           func(format string, v ...interface{}) { t.Logf(format, v...) },
		runningFn:        func() (int, bool) { return 0, false },
		healthCheckFn:    func() error { return nil },
		startFn:          func() error { return nil },
		stopFn:           func() {},
		unhealthyAlertFn: func(error) {},
		crashAlertFn:     func(int) {},
	}
}

// TestConcurrentEnsureRunning_OnlyOneRestart verifies that when multiple
// goroutines call EnsureRunning concurrently and the server is not running,
// only one goroutine enters restartWithBackoff and starts the server.
func TestConcurrentEnsureRunning_OnlyOneRestart(t *testing.T) {
	var startCount atomic.Int32

	m := newTestManager(t)
	m.startFn = func() error {
		startCount.Add(1)
		return nil
	}
	m.sleepFn = func(d time.Duration) {
		time.Sleep(200 * time.Millisecond) // Hold to let other goroutines try
	}

	const n = 10
	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_ = m.EnsureRunning()
		}()
	}

	close(start)
	wg.Wait()

	if got := startCount.Load(); got != 1 {
		t.Errorf("expected exactly 1 start call, got %d", got)
	}
}

// TestConcurrentEnsureRunning_BackoffSleepReleasesLock verifies that during
// the backoff sleep in restartWithBackoff, the mutex is released, allowing
// concurrent callers to check the restarting flag and return immediately.
func TestConcurrentEnsureRunning_BackoffSleepReleasesLock(t *testing.T) {
	sleepStarted := make(chan struct{})
	sleepDone := make(chan struct{})

	m := newTestManager(t)
	m.startFn = func() error { return nil }
	m.sleepFn = func(d time.Duration) {
		close(sleepStarted)
		<-sleepDone
	}

	// First goroutine enters restartWithBackoff
	done1 := make(chan error, 1)
	go func() {
		done1 <- m.EnsureRunning()
	}()

	// Wait for first goroutine to be sleeping (mutex released)
	<-sleepStarted

	// Second goroutine should see restarting=true and return immediately
	done2 := make(chan error, 1)
	go func() {
		done2 <- m.EnsureRunning()
	}()

	select {
	case err := <-done2:
		if err != nil {
			t.Errorf("concurrent caller got error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent caller blocked while restart was sleeping")
	}

	// Let first goroutine finish
	close(sleepDone)

	select {
	case err := <-done1:
		if err != nil {
			t.Errorf("first caller got error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first caller never completed")
	}
}

// TestEnsureRunning_UnhealthyTriggersRestart verifies the full flow:
// server running but unhealthy -> stop -> restart with backoff.
func TestEnsureRunning_UnhealthyTriggersRestart(t *testing.T) {
	var stopCount, startCount atomic.Int32
	var running atomic.Bool
	running.Store(true) // Server starts as running

	m := newTestManager(t)
	m.runningFn = func() (int, bool) {
		if running.Load() {
			return 1234, true
		}
		return 0, false
	}
	m.healthCheckFn = func() error { return fmt.Errorf("connection refused") }
	m.stopFn = func() {
		stopCount.Add(1)
		running.Store(false) // Stop marks server as not running
	}
	m.startFn = func() error {
		startCount.Add(1)
		running.Store(true) // Start marks server as running
		return nil
	}
	m.sleepFn = func(d time.Duration) {} // instant

	err := m.EnsureRunning()
	if err != nil {
		t.Fatalf("EnsureRunning returned error: %v", err)
	}

	if got := stopCount.Load(); got != 1 {
		t.Errorf("expected 1 stop, got %d", got)
	}
	if got := startCount.Load(); got != 1 {
		t.Errorf("expected 1 start, got %d", got)
	}
}

// TestEnsureRunning_HealthyResetsBackoff verifies that a healthy server
// resets backoff state after HealthyResetInterval elapses.
func TestEnsureRunning_HealthyResetsBackoff(t *testing.T) {
	var clockOffset atomic.Int64
	baseTime := time.Now()

	m := newTestManager(t)
	m.config.HealthyResetInterval = 100 * time.Millisecond
	m.runningFn = func() (int, bool) { return 1234, true }
	m.healthCheckFn = func() error { return nil }
	m.nowFn = func() time.Time {
		return baseTime.Add(time.Duration(clockOffset.Load()))
	}

	// Set up accumulated backoff state
	m.currentDelay = 40 * time.Second
	m.restartTimes = []time.Time{baseTime}
	m.escalated = true

	// First heartbeat: sets lastHealthyTime
	_ = m.EnsureRunning()
	if m.currentDelay != 40*time.Second {
		t.Errorf("should not reset on first heartbeat, got delay %v", m.currentDelay)
	}

	// Advance clock past HealthyResetInterval
	clockOffset.Store(int64(200 * time.Millisecond))

	// Second heartbeat: should reset backoff
	_ = m.EnsureRunning()
	if m.currentDelay != 0 {
		t.Errorf("expected backoff reset to 0, got %v", m.currentDelay)
	}
	if m.restartTimes != nil {
		t.Error("expected restartTimes nil after healthy reset")
	}
	if m.escalated {
		t.Error("expected escalated false after healthy reset")
	}
}

// TestEscalation_RestartCapExceeded verifies that when the restart cap is
// exceeded, sendEscalationMail is called exactly once with the correct count,
// and subsequent calls do not double-escalate.
func TestEscalation_RestartCapExceeded(t *testing.T) {
	var escalateCount atomic.Int32
	var escalateArg atomic.Int32

	m := newTestManager(t)
	m.config.MaxRestartsInWindow = 3
	m.sleepFn = func(d time.Duration) {}
	m.escalateFn = func(count int) {
		escalateCount.Add(1)
		escalateArg.Store(int32(count))
	}

	// Pre-fill restart times to reach the cap
	now := time.Now()
	m.restartTimes = []time.Time{
		now.Add(-1 * time.Minute),
		now.Add(-30 * time.Second),
		now.Add(-10 * time.Second),
	}

	// First call: cap exceeded -> escalate
	err := m.EnsureRunning()
	if err == nil {
		t.Fatal("expected error when restart cap exceeded")
	}
	if got := escalateCount.Load(); got != 1 {
		t.Errorf("expected 1 escalation, got %d", got)
	}
	if got := escalateArg.Load(); got != 3 {
		t.Errorf("expected escalation count 3, got %d", got)
	}

	// Second call: still exceeded but no double escalation
	err = m.EnsureRunning()
	if err == nil {
		t.Fatal("expected error on second call")
	}
	if got := escalateCount.Load(); got != 1 {
		t.Errorf("expected still 1 escalation (no double), got %d", got)
	}
}

// TestEnsureRunning_BackoffDelayIncreases verifies that each restart
// through the full EnsureRunning flow increases the backoff delay exponentially.
func TestEnsureRunning_BackoffDelayIncreases(t *testing.T) {
	var mu sync.Mutex
	var delays []time.Duration

	m := newTestManager(t)
	m.startFn = func() error { return nil }
	m.sleepFn = func(d time.Duration) {
		mu.Lock()
		delays = append(delays, d)
		mu.Unlock()
	}

	for i := 0; i < 4; i++ {
		if err := m.EnsureRunning(); err != nil {
			t.Fatalf("restart %d: %v", i, err)
		}
	}

	// Verify delays increase: 10ms, 20ms, 40ms, 80ms
	expected := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		40 * time.Millisecond,
		80 * time.Millisecond,
	}

	if len(delays) != len(expected) {
		t.Fatalf("expected %d sleeps, got %d: %v", len(expected), len(delays), delays)
	}
	for i, exp := range expected {
		if delays[i] != exp {
			t.Errorf("delay[%d]: expected %v, got %v", i, exp, delays[i])
		}
	}
}

// TestEnsureRunning_MultipleRestartCycles verifies that after a restart
// completes, the restarting flag is properly cleared so subsequent calls
// can initiate new restarts.
func TestEnsureRunning_MultipleRestartCycles(t *testing.T) {
	var startCount atomic.Int32

	m := newTestManager(t)
	m.startFn = func() error {
		startCount.Add(1)
		return nil
	}
	m.sleepFn = func(d time.Duration) {}

	// First restart
	if err := m.EnsureRunning(); err != nil {
		t.Fatalf("first: %v", err)
	}

	// Restarting flag must be cleared
	m.mu.Lock()
	if m.restarting {
		t.Error("restarting should be false after completion")
	}
	m.mu.Unlock()

	// Second restart (not blocked by stale flag)
	if err := m.EnsureRunning(); err != nil {
		t.Fatalf("second: %v", err)
	}

	if got := startCount.Load(); got != 2 {
		t.Errorf("expected 2 starts, got %d", got)
	}
}

// TestEnsureRunning_HeartbeatCycle simulates a daemon heartbeat loop:
// server not running -> start -> healthy -> unhealthy -> stop+restart.
func TestEnsureRunning_HeartbeatCycle(t *testing.T) {
	var (
		healthy atomic.Bool
		running atomic.Bool
		started atomic.Int32
	)

	m := newTestManager(t)
	m.runningFn = func() (int, bool) {
		if running.Load() {
			return 1234, true
		}
		return 0, false
	}
	m.healthCheckFn = func() error {
		if healthy.Load() {
			return nil
		}
		return fmt.Errorf("unhealthy")
	}
	m.startFn = func() error {
		started.Add(1)
		running.Store(true)
		healthy.Store(true)
		return nil
	}
	m.stopFn = func() {
		running.Store(false)
	}
	m.sleepFn = func(d time.Duration) {}

	// Phase 1: not running -> start
	if err := m.EnsureRunning(); err != nil {
		t.Fatalf("phase 1: %v", err)
	}
	if started.Load() != 1 {
		t.Fatalf("expected 1 start, got %d", started.Load())
	}

	// Phase 2: running and healthy -> no restart
	if err := m.EnsureRunning(); err != nil {
		t.Fatalf("phase 2: %v", err)
	}
	if started.Load() != 1 {
		t.Error("should not restart when healthy")
	}

	// Phase 3: unhealthy -> stop and restart
	healthy.Store(false)
	if err := m.EnsureRunning(); err != nil {
		t.Fatalf("phase 3: %v", err)
	}
	if started.Load() != 2 {
		t.Errorf("expected 2 starts, got %d", started.Load())
	}
}

// TestEnsureRunning_StartFailurePropagates verifies that an error from
// startLocked propagates through restartWithBackoff to EnsureRunning.
func TestEnsureRunning_StartFailurePropagates(t *testing.T) {
	m := newTestManager(t)
	m.startFn = func() error { return fmt.Errorf("dolt not found in PATH") }
	m.sleepFn = func(d time.Duration) {}

	err := m.EnsureRunning()
	if err == nil {
		t.Fatal("expected error when start fails")
	}
	if err.Error() != "dolt not found in PATH" {
		t.Errorf("unexpected error: %v", err)
	}
}

// ============================================================================
// Read-only detection and write probe tests
// ============================================================================

func TestIsReadOnlyError(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"cannot update manifest: database is read only", true},
		{"database is read only", true},
		{"Database Is Read Only", true},
		{"error: read-only mode", true},
		{"server is readonly", true},
		{"connection refused", false},
		{"timeout", false},
		{"", false},
		{"table not found", false},
	}

	for _, tt := range tests {
		if got := isReadOnlyError(tt.msg); got != tt.want {
			t.Errorf("isReadOnlyError(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

// TestCheckWriteHealth_ReadOnlyDetected verifies that when the write probe
// returns a read-only error, checkWriteHealthLocked returns an error.
func TestCheckWriteHealth_ReadOnlyDetected(t *testing.T) {
	m := newTestManager(t)
	m.writeProbeCheckFn = func() error {
		return fmt.Errorf("dolt server is in read-only mode: cannot update manifest: database is read only")
	}

	m.mu.Lock()
	err := m.checkWriteHealthLocked()
	m.mu.Unlock()

	if err == nil {
		t.Fatal("expected error from write probe")
	}
	if !isReadOnlyError(err.Error()) {
		t.Errorf("expected read-only error, got: %v", err)
	}
}

// TestCheckWriteHealth_WritableServer verifies that when the write probe
// succeeds, checkWriteHealthLocked returns nil.
func TestCheckWriteHealth_WritableServer(t *testing.T) {
	m := newTestManager(t)
	m.writeProbeCheckFn = func() error {
		return nil
	}

	m.mu.Lock()
	err := m.checkWriteHealthLocked()
	m.mu.Unlock()

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

// TestCheckWriteHealth_NonReadOnlyError verifies that non-read-only write
// failures do not cause the health check to fail (logged as warnings instead).
func TestCheckWriteHealth_NonReadOnlyError(t *testing.T) {
	m := newTestManager(t)
	m.writeProbeCheckFn = func() error {
		return nil // Non-read-only errors are logged, not returned
	}

	m.mu.Lock()
	err := m.checkWriteHealthLocked()
	m.mu.Unlock()

	if err != nil {
		t.Fatalf("expected nil error for non-read-only failure, got: %v", err)
	}
}

// TestCheckWriteHealth_NoDatabases verifies that the write probe is skipped
// when no databases are available.
func TestCheckWriteHealth_NoDatabases(t *testing.T) {
	m := newTestManager(t)
	m.writeProbeCheckFn = nil // Use real implementation
	m.listDatabasesFn = func() ([]string, error) {
		return nil, nil // No databases
	}

	m.mu.Lock()
	err := m.checkWriteHealthLocked()
	m.mu.Unlock()

	if err != nil {
		t.Fatalf("expected nil error when no databases, got: %v", err)
	}
}

// TestEnsureRunning_ReadOnlyTriggersRestart verifies the full flow:
// server running, healthy (SELECT 1 passes), but read-only -> stop -> restart.
func TestEnsureRunning_ReadOnlyTriggersRestart(t *testing.T) {
	var stopCount, startCount atomic.Int32
	var readOnlyAlerted atomic.Bool
	var running atomic.Bool
	running.Store(true)

	m := newTestManager(t)
	m.runningFn = func() (int, bool) {
		if running.Load() {
			return 1234, true
		}
		return 0, false
	}
	m.healthCheckFn = func() error { return nil } // SELECT 1 passes
	m.writeProbeCheckFn = func() error {
		return fmt.Errorf("dolt server is in read-only mode: cannot update manifest: database is read only")
	}
	m.stopFn = func() {
		stopCount.Add(1)
		running.Store(false)
	}
	m.startFn = func() error {
		startCount.Add(1)
		running.Store(true)
		return nil
	}
	m.readOnlyAlertFn = func(err error) {
		readOnlyAlerted.Store(true)
	}
	m.sleepFn = func(d time.Duration) {} // instant

	err := m.EnsureRunning()
	if err != nil {
		t.Fatalf("EnsureRunning returned error: %v", err)
	}

	if got := stopCount.Load(); got != 1 {
		t.Errorf("expected 1 stop, got %d", got)
	}
	if got := startCount.Load(); got != 1 {
		t.Errorf("expected 1 start (restart), got %d", got)
	}
	if !readOnlyAlerted.Load() {
		t.Error("expected read-only alert to be sent")
	}
}

// TestEnsureRunning_ReadOnlyWritesUnhealthySignal verifies that read-only
// detection writes the DOLT_UNHEALTHY signal file with "read_only" reason.
func TestEnsureRunning_ReadOnlyWritesUnhealthySignal(t *testing.T) {
	var running atomic.Bool
	running.Store(true)

	m := newTestManager(t)
	m.runningFn = func() (int, bool) {
		if running.Load() {
			return 1234, true
		}
		return 0, false
	}
	m.healthCheckFn = func() error { return nil }
	m.writeProbeCheckFn = func() error {
		return fmt.Errorf("dolt server is in read-only mode: database is read only")
	}
	m.stopFn = func() { running.Store(false) }
	m.startFn = func() error {
		running.Store(true)
		return nil
	}
	m.readOnlyAlertFn = func(err error) {}
	m.sleepFn = func(d time.Duration) {}

	_ = m.EnsureRunning()

	// Check the DOLT_UNHEALTHY signal file was written
	signalFile := filepath.Join(m.townRoot, "daemon", "DOLT_UNHEALTHY")
	data, err := os.ReadFile(signalFile)
	if err != nil {
		t.Fatalf("expected DOLT_UNHEALTHY signal file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "read_only") {
		t.Errorf("expected 'read_only' reason in signal file, got: %s", content)
	}
}

// TestEnsureRunning_HealthyAfterReadOnlyRecovery verifies that after a
// read-only restart, subsequent healthy checks clear the unhealthy signal.
func TestEnsureRunning_HealthyAfterReadOnlyRecovery(t *testing.T) {
	var running atomic.Bool
	var readOnly atomic.Bool
	running.Store(true)
	readOnly.Store(true) // Initially read-only

	m := newTestManager(t)
	m.runningFn = func() (int, bool) {
		if running.Load() {
			return 1234, true
		}
		return 0, false
	}
	m.healthCheckFn = func() error { return nil }
	m.writeProbeCheckFn = func() error {
		if readOnly.Load() {
			return fmt.Errorf("dolt server is in read-only mode")
		}
		return nil
	}
	m.stopFn = func() { running.Store(false) }
	m.startFn = func() error {
		running.Store(true)
		readOnly.Store(false) // Server recovers after restart
		return nil
	}
	m.readOnlyAlertFn = func(err error) {}
	m.sleepFn = func(d time.Duration) {}

	// Phase 1: read-only -> restart
	if err := m.EnsureRunning(); err != nil {
		t.Fatalf("phase 1: %v", err)
	}

	// Phase 2: healthy after restart
	if err := m.EnsureRunning(); err != nil {
		t.Fatalf("phase 2: %v", err)
	}

	// Unhealthy signal should be cleared
	if IsDoltUnhealthy(m.townRoot) {
		t.Error("expected DOLT_UNHEALTHY signal to be cleared after recovery")
	}
}
