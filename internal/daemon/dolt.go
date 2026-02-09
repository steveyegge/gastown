package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const doltCmdTimeout = 15 * time.Second

// DefaultDoltHealthCheckInterval is how often the dedicated Dolt health check
// ticker fires, independent of the general daemon heartbeat (3 min).
// 30 seconds provides fast crash detection: a Dolt server crash is detected
// within 30s instead of up to 3 minutes.
const DefaultDoltHealthCheckInterval = 30 * time.Second

// DoltServerConfig holds configuration for the Dolt SQL server.
type DoltServerConfig struct {
	// Enabled controls whether the daemon manages a Dolt server.
	Enabled bool `json:"enabled"`

	// External indicates the server is externally managed (daemon monitors only).
	External bool `json:"external,omitempty"`

	// Port is the MySQL protocol port (default 3306).
	Port int `json:"port,omitempty"`

	// Host is the bind address (default 127.0.0.1).
	Host string `json:"host,omitempty"`

	// DataDir is the directory containing Dolt databases.
	// Each subdirectory becomes a database.
	DataDir string `json:"data_dir,omitempty"`

	// LogFile is the path to the Dolt server log file.
	LogFile string `json:"log_file,omitempty"`

	// AutoRestart controls whether to restart on crash.
	AutoRestart bool `json:"auto_restart,omitempty"`

	// RestartDelay is the initial delay before restarting after crash (default 5s).
	RestartDelay time.Duration `json:"restart_delay,omitempty"`

	// MaxRestartDelay is the maximum backoff delay (default 5min).
	MaxRestartDelay time.Duration `json:"max_restart_delay,omitempty"`

	// MaxRestartsInWindow is the maximum number of restarts allowed within
	// RestartWindow before escalating instead of retrying (default 5).
	MaxRestartsInWindow int `json:"max_restarts_in_window,omitempty"`

	// RestartWindow is the time window for counting restarts (default 10min).
	RestartWindow time.Duration `json:"restart_window,omitempty"`

	// HealthyResetInterval is how long the server must stay healthy before
	// the backoff counter resets (default 5min).
	HealthyResetInterval time.Duration `json:"healthy_reset_interval,omitempty"`

	// HealthCheckInterval is how often to run the Dolt health check,
	// independent of the general daemon heartbeat. This enables fast
	// detection of Dolt server crashes without changing the overall
	// heartbeat frequency. Default 30s.
	HealthCheckInterval time.Duration `json:"health_check_interval,omitempty"`
}

// DefaultDoltServerConfig returns sensible defaults for Dolt server config.
func DefaultDoltServerConfig(townRoot string) *DoltServerConfig {
	return &DoltServerConfig{
		Enabled:              false, // Opt-in
		Port:                 3306,
		Host:                 "127.0.0.1",
		DataDir:              filepath.Join(townRoot, "dolt"),
		LogFile:              filepath.Join(townRoot, "daemon", "dolt-server.log"),
		AutoRestart:          true,
		RestartDelay:         5 * time.Second,
		MaxRestartDelay:      5 * time.Minute,
		MaxRestartsInWindow:  5,
		RestartWindow:        10 * time.Minute,
		HealthyResetInterval: 5 * time.Minute,
		HealthCheckInterval:  DefaultDoltHealthCheckInterval,
	}
}

// DoltServerStatus represents the current status of the Dolt server.
type DoltServerStatus struct {
	Running   bool      `json:"running"`
	PID       int       `json:"pid,omitempty"`
	Port      int       `json:"port,omitempty"`
	Host      string    `json:"host,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	Version   string    `json:"version,omitempty"`
	Databases []string  `json:"databases,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// DoltServerManager manages the Dolt SQL server lifecycle.
type DoltServerManager struct {
	config   *DoltServerConfig
	townRoot string
	logger   func(format string, v ...interface{})

	mu        sync.Mutex
	process   *os.Process
	startedAt time.Time
	lastCheck time.Time

	// Backoff state for restart logic
	currentDelay    time.Duration // Current backoff delay (grows exponentially)
	restartTimes    []time.Time   // Timestamps of recent restarts within window
	lastHealthyTime time.Time     // Last time the server was confirmed healthy
	escalated       bool          // Whether we've already escalated (avoid spamming)
	restarting      bool          // Whether a restart is in progress (guards against concurrent restarts)

	// Test hooks (nil = use real implementations; set only in tests)
	healthCheckFn      func() error
	writeProbeCheckFn  func() error
	startFn            func() error
	runningFn          func() (int, bool)
	stopFn             func()
	sleepFn            func(time.Duration)
	nowFn              func() time.Time
	escalateFn         func(int)
	unhealthyAlertFn   func(error)
	readOnlyAlertFn    func(error)
	crashAlertFn       func(int)
	listDatabasesFn    func() ([]string, error)
}

// NewDoltServerManager creates a new Dolt server manager.
func NewDoltServerManager(townRoot string, config *DoltServerConfig, logger func(format string, v ...interface{})) *DoltServerManager {
	if config == nil {
		config = DefaultDoltServerConfig(townRoot)
	}
	return &DoltServerManager{
		config:   config,
		townRoot: townRoot,
		logger:   logger,
	}
}

func (m *DoltServerManager) now() time.Time {
	if m.nowFn != nil {
		return m.nowFn()
	}
	return time.Now()
}

func (m *DoltServerManager) doSleep(d time.Duration) {
	if m.sleepFn != nil {
		m.sleepFn(d)
		return
	}
	time.Sleep(d)
}

// pidFile returns the path to the Dolt server PID file.
func (m *DoltServerManager) pidFile() string {
	return filepath.Join(m.townRoot, "daemon", "dolt.pid")
}

// IsEnabled returns whether Dolt server management is enabled.
func (m *DoltServerManager) IsEnabled() bool {
	return m.config != nil && m.config.Enabled
}

// IsExternal returns whether the Dolt server is externally managed.
func (m *DoltServerManager) IsExternal() bool {
	return m.config != nil && m.config.External
}

// HealthCheckInterval returns the configured health check interval,
// falling back to DefaultDoltHealthCheckInterval if not explicitly set.
func (m *DoltServerManager) HealthCheckInterval() time.Duration {
	if m.config != nil && m.config.HealthCheckInterval > 0 {
		return m.config.HealthCheckInterval
	}
	return DefaultDoltHealthCheckInterval
}

// Status returns the current status of the Dolt server.
func (m *DoltServerManager) Status() *DoltServerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	status := &DoltServerStatus{
		Port: m.config.Port,
		Host: m.config.Host,
	}

	// Check if process is running
	pid, running := m.isRunning()
	status.Running = running
	status.PID = pid

	if running {
		status.StartedAt = m.startedAt

		// Get version
		if version, err := m.getDoltVersion(); err == nil {
			status.Version = version
		}

		// List databases
		if databases, err := m.listDatabases(); err == nil {
			status.Databases = databases
		}
	}

	return status
}

// isRunning checks if the Dolt server process is running.
// Must be called with m.mu held.
func (m *DoltServerManager) isRunning() (int, bool) {
	if m.runningFn != nil {
		return m.runningFn()
	}
	// First check our tracked process
	if m.process != nil {
		if isProcessAlive(m.process) {
			return m.process.Pid, true
		}
		// Process died, clear it
		m.process = nil
	}

	// Check PID file
	data, err := os.ReadFile(m.pidFile())
	if err != nil {
		return 0, false
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}

	// Verify process is alive and is dolt
	process, err := os.FindProcess(pid)
	if err != nil {
		return 0, false
	}

	if !isProcessAlive(process) {
		// Process not running, clean up stale PID file
		_ = os.Remove(m.pidFile())
		return 0, false
	}

	// Verify it's actually dolt sql-server
	if !isDoltSqlServer(pid) {
		_ = os.Remove(m.pidFile())
		return 0, false
	}

	m.process = process
	return pid, true
}

// isDoltSqlServer checks if a PID is actually a dolt sql-server process.
func isDoltSqlServer(pid int) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(pid), "-o", "command=")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	cmdline := strings.TrimSpace(string(output))
	return strings.Contains(cmdline, "dolt") && strings.Contains(cmdline, "sql-server")
}

// EnsureRunning ensures the Dolt server is running.
// If not running, starts it. If running but unhealthy, restarts it.
// Uses exponential backoff and a max-restart cap to avoid crash-looping.
func (m *DoltServerManager) EnsureRunning() error {
	if !m.IsEnabled() {
		return nil
	}

	if m.IsExternal() {
		// External mode: just check health, don't manage lifecycle
		return m.checkHealth()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Another goroutine is already restarting — skip to avoid double-starts
	if m.restarting {
		m.logger("Dolt server restart already in progress, skipping")
		return nil
	}

	pid, running := m.isRunning()
	if running {
		// Already running, check health
		m.lastCheck = m.now()
		if err := m.checkHealthLocked(); err != nil {
			m.logger("Dolt server unhealthy: %v, restarting...", err)
			m.sendUnhealthyAlert(err)
			m.writeUnhealthySignal("health_check_failed", err.Error())
			m.stopLocked()
			return m.restartWithBackoff()
		}
		// Check write capability (read-only detection).
		// The SELECT 1 health check above only verifies read connectivity.
		// Under concurrent write load, Dolt can enter a persistent read-only
		// state that requires a server restart to clear.
		if err := m.checkWriteHealthLocked(); err != nil {
			m.logger("Dolt server read-only: %v, restarting...", err)
			m.sendReadOnlyAlert(err)
			m.writeUnhealthySignal("read_only", err.Error())
			m.stopLocked()
			return m.restartWithBackoff()
		}
		// Server is healthy — clear any stale unhealthy signal and reset backoff
		m.clearUnhealthySignal()
		m.maybeResetBackoff()
		return nil
	}

	// Not running, start it
	if pid > 0 {
		m.logger("Dolt server PID %d is dead, cleaning up and restarting...", pid)
		m.sendCrashAlert(pid)
		m.writeUnhealthySignal("server_dead", fmt.Sprintf("PID %d is dead", pid))
	}
	return m.restartWithBackoff()
}

// restartWithBackoff attempts to restart the Dolt server with exponential backoff
// and a max-restart cap. If the cap is exceeded, it escalates instead of retrying.
// Must be called with m.mu held.
func (m *DoltServerManager) restartWithBackoff() error {
	now := m.now()

	// Prune restart times outside the window
	m.pruneRestartTimes(now)

	// Check if we've exceeded the restart cap
	maxRestarts := m.config.MaxRestartsInWindow
	if maxRestarts <= 0 {
		maxRestarts = 5
	}
	if len(m.restartTimes) >= maxRestarts {
		if !m.escalated {
			m.escalated = true
			m.logger("Dolt server restart cap reached (%d restarts in %v), escalating to mayor",
				len(m.restartTimes), m.config.RestartWindow)
			m.sendEscalationMail(len(m.restartTimes))
		}
		return fmt.Errorf("dolt server restart cap exceeded (%d restarts in %v); escalated to mayor",
			len(m.restartTimes), m.config.RestartWindow)
	}

	// Mark restart in progress to prevent concurrent restarts during backoff sleep
	m.restarting = true
	defer func() { m.restarting = false }()

	// Apply exponential backoff delay
	delay := m.getBackoffDelay()
	if delay > 0 {
		m.logger("Backing off %v before Dolt server restart (attempt %d in window)",
			delay, len(m.restartTimes)+1)
		// Unlock during sleep so we don't hold the mutex during backoff
		m.mu.Unlock()
		m.doSleep(delay)
		m.mu.Lock()

		// Re-check after re-acquiring the lock: another goroutine may have
		// started the server while we were sleeping (TOCTOU guard).
		if _, running := m.isRunning(); running {
			m.logger("Dolt server started by another goroutine during backoff, skipping")
			return nil
		}
	}

	// Record this restart attempt
	m.restartTimes = append(m.restartTimes, m.now())

	// Advance the backoff for next time
	m.advanceBackoff()

	return m.startLocked()
}

// getBackoffDelay returns the current backoff delay.
func (m *DoltServerManager) getBackoffDelay() time.Duration {
	if m.currentDelay <= 0 {
		return m.config.RestartDelay
	}
	return m.currentDelay
}

// advanceBackoff doubles the current delay up to MaxRestartDelay.
func (m *DoltServerManager) advanceBackoff() {
	baseDelay := m.config.RestartDelay
	if baseDelay <= 0 {
		baseDelay = 5 * time.Second
	}
	maxDelay := m.config.MaxRestartDelay
	if maxDelay <= 0 {
		maxDelay = 5 * time.Minute
	}

	if m.currentDelay <= 0 {
		m.currentDelay = baseDelay
	}
	m.currentDelay *= 2
	if m.currentDelay > maxDelay {
		m.currentDelay = maxDelay
	}
}

// pruneRestartTimes removes restart timestamps outside the configured window.
func (m *DoltServerManager) pruneRestartTimes(now time.Time) {
	window := m.config.RestartWindow
	if window <= 0 {
		window = 10 * time.Minute
	}
	cutoff := now.Add(-window)
	pruned := m.restartTimes[:0]
	for _, t := range m.restartTimes {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	m.restartTimes = pruned
}

// maybeResetBackoff resets backoff state if the server has been healthy
// for the configured HealthyResetInterval.
// Must be called with m.mu held.
func (m *DoltServerManager) maybeResetBackoff() {
	now := m.now()
	resetInterval := m.config.HealthyResetInterval
	if resetInterval <= 0 {
		resetInterval = 5 * time.Minute
	}

	if m.lastHealthyTime.IsZero() {
		m.lastHealthyTime = now
		return
	}

	if now.Sub(m.lastHealthyTime) >= resetInterval {
		if m.currentDelay > 0 || len(m.restartTimes) > 0 || m.escalated {
			m.logger("Dolt server healthy for %v, resetting backoff state", resetInterval)
			m.currentDelay = 0
			m.restartTimes = nil
			m.escalated = false
		}
		// Reset the healthy timestamp after a successful reset so the next
		// reset interval is measured from now, not from the original detection.
		m.lastHealthyTime = now
	}
}

// sendEscalationMail sends a mail to the mayor when the Dolt server has
// exceeded its restart cap, indicating a systemic issue.
// Runs the mail command asynchronously to avoid blocking the mutex.
func (m *DoltServerManager) sendEscalationMail(restartCount int) {
	if m.escalateFn != nil {
		m.escalateFn(restartCount)
		return
	}
	subject := fmt.Sprintf("ESCALATION: Dolt server crash-looping (%d restarts)", restartCount)
	body := fmt.Sprintf(`The Dolt server has restarted %d times within %v and has been capped.

The daemon will NOT restart it again until the backoff window expires or the issue is resolved.

Possible causes:
- Bad configuration
- Corrupt data directory
- Disk full
- Port conflict

Data dir: %s
Log file: %s
Host: %s:%d

Action needed: Investigate and fix the root cause, then restart the daemon or the Dolt server manually.`,
		restartCount, m.config.RestartWindow,
		m.config.DataDir, m.config.LogFile,
		m.config.Host, m.config.Port)

	townRoot := m.townRoot
	logger := m.logger

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "gt", "mail", "send", "mayor/", "-s", subject, "-m", body) //nolint:gosec // G204: args are constructed internally
		cmd.Dir = townRoot
		cmd.Env = os.Environ()

		if err := cmd.Run(); err != nil {
			logger("Warning: failed to send escalation mail to mayor: %v", err)
		} else {
			logger("Sent escalation mail to mayor about Dolt server crash-loop")
		}

		// Also notify all witnesses so they can react to degraded Dolt state
		sendDoltAlertToWitnesses(townRoot, subject, body, logger)
	}()
}

// sendCrashAlert sends a mail to the mayor when the Dolt server is found dead.
// This is for single crash detection — distinct from crash-loop escalation.
// Runs asynchronously to avoid blocking.
func (m *DoltServerManager) sendCrashAlert(deadPID int) {
	if m.crashAlertFn != nil {
		m.crashAlertFn(deadPID)
		return
	}
	subject := "ALERT: Dolt server crashed"
	body := fmt.Sprintf(`The Dolt server (PID %d) was found dead. The daemon is restarting it.

Data dir: %s
Log file: %s
Host: %s:%d

Check the log file for crash details. If crashes recur, the daemon will escalate after %d restarts in %v.`,
		deadPID,
		m.config.DataDir, m.config.LogFile,
		m.config.Host, m.config.Port,
		m.config.MaxRestartsInWindow, m.config.RestartWindow)

	townRoot := m.townRoot
	logger := m.logger

	go func() {
		sendDoltAlertMail(townRoot, "mayor/", subject, body, logger)
		sendDoltAlertToWitnesses(townRoot, subject, body, logger)
	}()
}

// sendUnhealthyAlert sends a mail to the mayor when the Dolt server fails health checks.
// The server is running but not responding to queries. Runs asynchronously.
func (m *DoltServerManager) sendUnhealthyAlert(healthErr error) {
	if m.unhealthyAlertFn != nil {
		m.unhealthyAlertFn(healthErr)
		return
	}
	subject := "ALERT: Dolt server unhealthy"
	body := fmt.Sprintf(`The Dolt server is running but failing health checks. The daemon is restarting it.

Health check error: %v

Data dir: %s
Log file: %s
Host: %s:%d

This may indicate high load, connection exhaustion, or internal server errors.`,
		healthErr,
		m.config.DataDir, m.config.LogFile,
		m.config.Host, m.config.Port)

	townRoot := m.townRoot
	logger := m.logger

	go func() {
		sendDoltAlertMail(townRoot, "mayor/", subject, body, logger)
		sendDoltAlertToWitnesses(townRoot, subject, body, logger)
	}()
}

// sendDoltAlertMail sends a Dolt alert mail to a specific recipient.
func sendDoltAlertMail(townRoot, recipient, subject, body string, logger func(format string, v ...interface{})) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gt", "mail", "send", recipient, "-s", subject, "-m", body) //nolint:gosec // G204: args are constructed internally
	cmd.Dir = townRoot
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		logger("Warning: failed to send Dolt alert to %s: %v", recipient, err)
	}
}

// sendDoltAlertToWitnesses sends a Dolt alert to all rig witnesses.
// Discovers rigs from mayor/rigs.json and sends to each <rig>/witness.
func sendDoltAlertToWitnesses(townRoot, subject, body string, logger func(format string, v ...interface{})) {
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	data, err := os.ReadFile(rigsPath)
	if err != nil {
		return // No rigs.json, nothing to notify
	}

	var parsed struct {
		Rigs map[string]interface{} `json:"rigs"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return
	}

	for rigName := range parsed.Rigs {
		recipient := rigName + "/witness"
		sendDoltAlertMail(townRoot, recipient, subject, body, logger)
	}
}

// unhealthySignalFile returns the path to the DOLT_UNHEALTHY signal file.
// Witness patrols can check for this file to detect degraded Dolt state.
func (m *DoltServerManager) unhealthySignalFile() string {
	return filepath.Join(m.townRoot, "daemon", "DOLT_UNHEALTHY")
}

// writeUnhealthySignal writes the DOLT_UNHEALTHY signal file.
// This file signals to witness patrols that the Dolt server is degraded.
func (m *DoltServerManager) writeUnhealthySignal(reason, detail string) {
	payload := fmt.Sprintf(`{"reason":%q,"detail":%q,"timestamp":%q}`,
		reason, detail, time.Now().UTC().Format(time.RFC3339))
	if err := os.WriteFile(m.unhealthySignalFile(), []byte(payload), 0644); err != nil {
		m.logger("Warning: failed to write DOLT_UNHEALTHY signal: %v", err)
	}
}

// clearUnhealthySignal removes the DOLT_UNHEALTHY signal file when the server is healthy.
func (m *DoltServerManager) clearUnhealthySignal() {
	_ = os.Remove(m.unhealthySignalFile())
}

// IsDoltUnhealthy checks if the DOLT_UNHEALTHY signal file exists.
// This is a package-level function for use by witness patrols and other consumers.
func IsDoltUnhealthy(townRoot string) bool {
	_, err := os.Stat(filepath.Join(townRoot, "daemon", "DOLT_UNHEALTHY"))
	return err == nil
}

// Start starts the Dolt SQL server.
func (m *DoltServerManager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startLocked()
}

// startLocked starts the Dolt server. Must be called with m.mu held.
func (m *DoltServerManager) startLocked() error {
	if m.startFn != nil {
		return m.startFn()
	}

	// Re-check if the server is already running to close the TOCTOU window.
	// Another goroutine may have started the server while we were waiting
	// for the mutex (via Start()) or during backoff sleep (via restartWithBackoff()).
	if _, running := m.isRunning(); running {
		m.logger("Dolt server already running, skipping start")
		return nil
	}

	// Ensure data directory exists
	if err := os.MkdirAll(m.config.DataDir, 0755); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	// Check if dolt is installed
	doltPath, err := exec.LookPath("dolt")
	if err != nil {
		return fmt.Errorf("dolt not found in PATH: %w", err)
	}

	// Build command arguments
	args := []string{
		"sql-server",
		"--host", m.config.Host,
		"--port", strconv.Itoa(m.config.Port),
		"--data-dir", m.config.DataDir,
	}

	// Open log file
	logFile, err := os.OpenFile(m.config.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}

	// Start dolt sql-server as background process
	cmd := exec.Command(doltPath, args...)
	cmd.Dir = m.config.DataDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Detach from this process group so it survives daemon restart
	setSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		if closeErr := logFile.Close(); closeErr != nil {
			m.logger("Warning: failed to close dolt log file: %v", closeErr)
		}
		return fmt.Errorf("starting dolt sql-server: %w", err)
	}

	// Don't wait for it - it's a long-running server
	go func() {
		_ = cmd.Wait()
		if closeErr := logFile.Close(); closeErr != nil {
			m.logger("Warning: failed to close dolt log file: %v", closeErr)
		}
	}()

	m.process = cmd.Process
	m.startedAt = time.Now()

	// Write PID file
	if err := os.WriteFile(m.pidFile(), []byte(strconv.Itoa(cmd.Process.Pid)), 0644); err != nil {
		m.logger("Warning: failed to write PID file: %v", err)
	}

	m.logger("Started Dolt SQL server (PID %d) on %s:%d", cmd.Process.Pid, m.config.Host, m.config.Port)

	// Wait a moment for server to initialize
	time.Sleep(500 * time.Millisecond)

	// Verify it started successfully
	if err := m.checkHealthLocked(); err != nil {
		m.logger("Warning: Dolt server may not be healthy: %v", err)
	}

	return nil
}

// Stop stops the Dolt SQL server.
func (m *DoltServerManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
	return nil
}

// stopLocked stops the Dolt server. Must be called with m.mu held.
func (m *DoltServerManager) stopLocked() {
	if m.stopFn != nil {
		m.stopFn()
		return
	}
	pid, running := m.isRunning()
	if !running {
		return
	}

	m.logger("Stopping Dolt SQL server (PID %d)...", pid)

	process, err := os.FindProcess(pid)
	if err != nil {
		return // Already gone
	}

	// Send termination signal for graceful shutdown
	if err := sendTermSignal(process); err != nil {
		m.logger("Warning: failed to send termination signal: %v", err)
	}

	// Wait for graceful shutdown (up to 5 seconds)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			if !isProcessAlive(process) {
				close(done)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	select {
	case <-done:
		m.logger("Dolt SQL server stopped gracefully")
	case <-time.After(5 * time.Second):
		// Force kill
		m.logger("Dolt SQL server did not stop gracefully, forcing termination")
		_ = sendKillSignal(process)
	}

	// Clean up
	_ = os.Remove(m.pidFile())
	m.process = nil
}

// checkHealth checks if the Dolt server is healthy (can accept connections).
func (m *DoltServerManager) checkHealth() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.checkHealthLocked()
}

// checkHealthLocked checks health. Must be called with m.mu held.
// Performs a connectivity check (SELECT 1) with latency measurement, and logs
// warnings for degraded resource conditions (high latency, high connection count,
// disk usage). Returns an error only if the server is unreachable.
func (m *DoltServerManager) checkHealthLocked() error {
	if m.healthCheckFn != nil {
		return m.healthCheckFn()
	}
	// 1. Connectivity + latency: time a SELECT 1
	ctx, cancel := context.WithTimeout(context.Background(), doltCmdTimeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(ctx, "dolt", "sql", "-q", "SELECT 1")
	cmd.Dir = m.config.DataDir

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("health check failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	latency := time.Since(start)
	if latency > 1*time.Second {
		m.logger("Warning: Dolt health check latency %v exceeds 1s threshold — server may be under stress", latency.Round(time.Millisecond))
	}

	// 2. Connection count (best-effort, non-fatal)
	m.checkConnectionCount()

	// 3. Disk space (best-effort, non-fatal)
	m.checkDiskUsage()

	return nil
}

// checkConnectionCount queries the connection count and logs a warning if approaching the limit.
// Non-fatal: failures are silently ignored.
func (m *DoltServerManager) checkConnectionCount() {
	ctx, cancel := context.WithTimeout(context.Background(), doltCmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "dolt", "sql",
		"-r", "csv",
		"-q", "SELECT COUNT(*) AS cnt FROM information_schema.PROCESSLIST",
	)
	cmd.Dir = m.config.DataDir

	output, err := cmd.Output()
	if err != nil {
		return // non-fatal
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return
	}
	count, err := strconv.Atoi(strings.TrimSpace(lines[len(lines)-1]))
	if err != nil {
		return
	}

	// Use the doltserver package default (50) as a reasonable cap reference
	maxConn := 50
	threshold := (maxConn * 80) / 100
	if count >= threshold {
		m.logger("Warning: Dolt connection count %d is at %d%% of max %d — approaching limit",
			count, (count*100)/maxConn, maxConn)
	}
}

// checkDiskUsage checks disk usage of the data directory and logs a warning
// if it exceeds 1 GB. Non-fatal: failures are silently ignored.
func (m *DoltServerManager) checkDiskUsage() {
	dataDir := m.config.DataDir
	if dataDir == "" {
		return
	}

	var total int64
	_ = filepath.Walk(dataDir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})

	const gb = 1024 * 1024 * 1024
	if total > gb {
		m.logger("Warning: Dolt data directory %s is %.1f GB", dataDir, float64(total)/float64(gb))
	}
}

// checkWriteHealthLocked probes the Dolt server's write capability by attempting
// a test write operation. If the server is in read-only mode (e.g., from concurrent
// write contention on the manifest), the write probe will fail with a characteristic
// error. Returns an error only if the server is confirmed read-only.
// Must be called with m.mu held.
func (m *DoltServerManager) checkWriteHealthLocked() error {
	if m.writeProbeCheckFn != nil {
		return m.writeProbeCheckFn()
	}

	// Get a database to test writes against
	databases, err := m.getDatabases()
	if err != nil || len(databases) == 0 {
		return nil // Skip write probe if no databases available
	}

	db := databases[0]

	ctx, cancel := context.WithTimeout(context.Background(), doltCmdTimeout)
	defer cancel()

	// Attempt a write operation to detect read-only mode.
	// CREATE TABLE IF NOT EXISTS is idempotent (safe if table lingers from previous probe).
	// REPLACE INTO always writes a row, testing the storage layer even if the table existed.
	// DROP TABLE IF EXISTS cleans up.
	// If ANY statement triggers "database is read only", the command fails and we detect it.
	query := fmt.Sprintf(
		"USE `%s`; CREATE TABLE IF NOT EXISTS `__gt_health_probe` (v INT PRIMARY KEY); REPLACE INTO `__gt_health_probe` VALUES (1); DROP TABLE IF EXISTS `__gt_health_probe`",
		db,
	)
	cmd := exec.CommandContext(ctx, "dolt", "sql", "-q", query)
	cmd.Dir = m.config.DataDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := strings.TrimSpace(string(output))
		if isReadOnlyError(errMsg) {
			return fmt.Errorf("dolt server is in read-only mode: %s", errMsg)
		}
		// Non-read-only failures: log warning but don't fail health check.
		// These could be transient issues (timeout, lock contention) that
		// don't indicate a persistent read-only state.
		m.logger("Warning: Dolt write probe failed (non-read-only): %v (%s)", err, errMsg)
	}

	return nil
}

// getDatabases returns the list of databases. Uses the test hook if set.
func (m *DoltServerManager) getDatabases() ([]string, error) {
	if m.listDatabasesFn != nil {
		return m.listDatabasesFn()
	}
	return m.listDatabases()
}

// isReadOnlyError checks if an error message indicates a Dolt read-only state.
func isReadOnlyError(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "read only") ||
		strings.Contains(lower, "read-only") ||
		strings.Contains(lower, "readonly")
}

// sendReadOnlyAlert sends an alert when the Dolt server enters read-only mode.
// This is distinct from unhealthy alerts — the server is running and responding
// to reads, but cannot accept writes. Runs asynchronously.
func (m *DoltServerManager) sendReadOnlyAlert(readOnlyErr error) {
	if m.readOnlyAlertFn != nil {
		m.readOnlyAlertFn(readOnlyErr)
		return
	}
	subject := "ALERT: Dolt server entered READ-ONLY mode"
	body := fmt.Sprintf(`The Dolt server is running but has entered read-only mode.
All write operations (beads create, update, close) will fail until the server is restarted.

The daemon is restarting the server automatically.

Error: %v

Data dir: %s
Log file: %s
Host: %s:%d

This typically occurs under heavy concurrent write load when multiple agents
contend for the storage manifest. If it recurs frequently, consider reducing
concurrent polecat count or staggering write-heavy operations.`,
		readOnlyErr,
		m.config.DataDir, m.config.LogFile,
		m.config.Host, m.config.Port)

	townRoot := m.townRoot
	logger := m.logger

	go func() {
		sendDoltAlertMail(townRoot, "mayor/", subject, body, logger)
		sendDoltAlertToWitnesses(townRoot, subject, body, logger)
	}()
}

// getDoltVersion returns the Dolt server version.
func (m *DoltServerManager) getDoltVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), doltCmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "dolt", "version")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// Parse "dolt version X.Y.Z"
	line := strings.TrimSpace(string(output))
	parts := strings.Fields(line)
	if len(parts) >= 3 {
		return parts[2], nil
	}
	return line, nil
}

// listDatabases returns the list of databases in the Dolt server.
func (m *DoltServerManager) listDatabases() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), doltCmdTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "dolt", "sql",
		"-r", "json",
		"-q", "SHOW DATABASES",
	)
	cmd.Dir = m.config.DataDir

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Parse JSON output
	var result struct {
		Rows []struct {
			Database string `json:"Database"`
		} `json:"rows"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		// Fall back to line parsing
		var databases []string
		for _, line := range strings.Split(string(output), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && line != "Database" && !strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "|") {
				databases = append(databases, line)
			}
		}
		return databases, nil
	}

	var databases []string
	for _, row := range result.Rows {
		if row.Database != "" && row.Database != "information_schema" {
			databases = append(databases, row.Database)
		}
	}
	return databases, nil
}

// CountDoltServers returns the count of running dolt sql-server processes.
func CountDoltServers() int {
	cmd := exec.Command("sh", "-c", "pgrep -f 'dolt sql-server' 2>/dev/null | wc -l")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}
	count, _ := strconv.Atoi(strings.TrimSpace(string(output)))
	return count
}

// StopAllDoltServers stops all dolt sql-server processes.
// Returns (killed, remaining).
func StopAllDoltServers(force bool) (int, int) {
	before := CountDoltServers()
	if before == 0 {
		return 0, 0
	}

	if force {
		_ = exec.Command("pkill", "-9", "-f", "dolt sql-server").Run()
	} else {
		_ = exec.Command("pkill", "-TERM", "-f", "dolt sql-server").Run()
		time.Sleep(2 * time.Second)
		if remaining := CountDoltServers(); remaining > 0 {
			_ = exec.Command("pkill", "-9", "-f", "dolt sql-server").Run()
		}
	}

	time.Sleep(100 * time.Millisecond)

	after := CountDoltServers()
	killed := before - after
	if killed < 0 {
		killed = 0
	}
	return killed, after
}
