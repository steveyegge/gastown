package daemon

import (
	"bytes"
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

	// RestartDelay is the delay before restarting after crash.
	RestartDelay time.Duration `json:"restart_delay,omitempty"`
}

// DefaultDoltServerConfig returns sensible defaults for Dolt server config.
func DefaultDoltServerConfig(townRoot string) *DoltServerConfig {
	return &DoltServerConfig{
		Enabled:      false, // Opt-in
		Port:         3306,
		Host:         "127.0.0.1",
		DataDir:      filepath.Join(townRoot, "dolt"),
		LogFile:      filepath.Join(townRoot, "daemon", "dolt-server.log"),
		AutoRestart:  true,
		RestartDelay: 5 * time.Second,
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

// pidFile returns the path to the Dolt server PID file.
func (m *DoltServerManager) pidFile() string {
	return filepath.Join(m.townRoot, "daemon", "dolt-server.pid")
}

// IsEnabled returns whether Dolt server management is enabled.
func (m *DoltServerManager) IsEnabled() bool {
	return m.config != nil && m.config.Enabled
}

// IsExternal returns whether the Dolt server is externally managed.
func (m *DoltServerManager) IsExternal() bool {
	return m.config != nil && m.config.External
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
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	cmdline := strings.TrimSpace(string(output))
	return strings.Contains(cmdline, "dolt") && strings.Contains(cmdline, "sql-server")
}

// EnsureRunning ensures the Dolt server is running.
// If not running, starts it. If running but unhealthy, restarts it.
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

	pid, running := m.isRunning()
	if running {
		// Already running, check health
		m.lastCheck = time.Now()
		if err := m.checkHealthLocked(); err != nil {
			m.logger("Dolt server unhealthy: %v, restarting...", err)
			_ = m.stopLocked()
			time.Sleep(m.config.RestartDelay)
			return m.startLocked()
		}
		return nil
	}

	// Not running, start it
	if pid > 0 {
		m.logger("Dolt server PID %d is dead, cleaning up and restarting...", pid)
	}
	return m.startLocked()
}

// Start starts the Dolt SQL server.
func (m *DoltServerManager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startLocked()
}

// startLocked starts the Dolt server. Must be called with m.mu held.
func (m *DoltServerManager) startLocked() error {
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
		_ = logFile.Close()
		return fmt.Errorf("starting dolt sql-server: %w", err)
	}

	// Don't wait for it - it's a long-running server
	go func() {
		_ = cmd.Wait()
		_ = logFile.Close()
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
	return m.stopLocked()
}

// stopLocked stops the Dolt server. Must be called with m.mu held.
// Note: This function intentionally logs errors and returns nil to allow best-effort cleanup.
func (m *DoltServerManager) stopLocked() error { //nolint:unparam // intentionally returns nil for best-effort cleanup
	pid, running := m.isRunning()
	if !running {
		return nil
	}

	m.logger("Stopping Dolt SQL server (PID %d)...", pid)

	process, err := os.FindProcess(pid)
	if err != nil {
		return nil // Already gone
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

	return nil
}

// checkHealth checks if the Dolt server is healthy (can accept connections).
func (m *DoltServerManager) checkHealth() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.checkHealthLocked()
}

// checkHealthLocked checks health. Must be called with m.mu held.
func (m *DoltServerManager) checkHealthLocked() error {
	// Try to connect via MySQL protocol
	// Use dolt sql -q to test connectivity
	cmd := exec.Command("dolt", "sql",
		"--host", m.config.Host,
		"--port", strconv.Itoa(m.config.Port),
		"--no-auto-commit",
		"-q", "SELECT 1",
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("health check failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	return nil
}

// getDoltVersion returns the Dolt server version.
func (m *DoltServerManager) getDoltVersion() (string, error) {
	cmd := exec.Command("dolt", "version")
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
	cmd := exec.Command("dolt", "sql",
		"--host", m.config.Host,
		"--port", strconv.Itoa(m.config.Port),
		"--no-auto-commit",
		"-q", "SHOW DATABASES",
		"--result-format", "json",
	)

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
