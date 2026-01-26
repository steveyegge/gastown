// Package doltserver manages the Dolt SQL server for Gas Town.
//
// The Dolt server provides multi-client access to beads databases,
// avoiding the single-writer limitation of embedded Dolt mode.
//
// Server configuration:
//   - Port: 3307 (avoids conflict with MySQL on 3306)
//   - User: root (default Dolt user, no password for localhost)
//   - Data directory: ~/gt/.dolt-data/ (contains all rig databases)
//
// Each rig (hq, gastown, beads) has its own database subdirectory:
//
//	~/gt/.dolt-data/
//	├── hq/        # Town beads (hq-*)
//	├── gastown/   # Gastown rig (gt-*)
//	├── beads/     # Beads rig (bd-*)
//	└── ...        # Other rigs
//
// Usage:
//
//	gt dolt start           # Start the server
//	gt dolt stop            # Stop the server
//	gt dolt status          # Check server status
//	gt dolt logs            # View server logs
//	gt dolt sql             # Open SQL shell
//	gt dolt init-rig <name> # Initialize a new rig database
package doltserver

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gofrs/flock"
	"github.com/steveyegge/gastown/internal/util"
)

// Default configuration
const (
	DefaultPort = 3307
	DefaultUser = "root" // Default Dolt user (no password for local access)
)

// Config holds Dolt server configuration.
type Config struct {
	// TownRoot is the Gas Town workspace root.
	TownRoot string

	// Port is the MySQL protocol port.
	Port int

	// User is the MySQL user name.
	User string

	// DataDir is the root directory containing all rig databases.
	// Each subdirectory is a separate database that will be served.
	DataDir string

	// LogFile is the path to the server log file.
	LogFile string

	// PidFile is the path to the PID file.
	PidFile string
}

// DefaultConfig returns the default Dolt server configuration.
func DefaultConfig(townRoot string) *Config {
	daemonDir := filepath.Join(townRoot, "daemon")
	return &Config{
		TownRoot: townRoot,
		Port:     DefaultPort,
		User:     DefaultUser,
		DataDir:  filepath.Join(townRoot, ".dolt-data"),
		LogFile:  filepath.Join(daemonDir, "dolt.log"),
		PidFile:  filepath.Join(daemonDir, "dolt.pid"),
	}
}

// RigDatabaseDir returns the database directory for a specific rig.
func RigDatabaseDir(townRoot, rigName string) string {
	config := DefaultConfig(townRoot)
	return filepath.Join(config.DataDir, rigName)
}

// State represents the Dolt server's runtime state.
type State struct {
	// Running indicates if the server is running.
	Running bool `json:"running"`

	// PID is the process ID of the server.
	PID int `json:"pid"`

	// Port is the port the server is listening on.
	Port int `json:"port"`

	// StartedAt is when the server started.
	StartedAt time.Time `json:"started_at"`

	// DataDir is the data directory containing all rig databases.
	DataDir string `json:"data_dir"`

	// Databases is the list of available databases (rig names).
	Databases []string `json:"databases,omitempty"`
}

// StateFile returns the path to the state file.
func StateFile(townRoot string) string {
	return filepath.Join(townRoot, "daemon", "dolt-state.json")
}

// LoadState loads Dolt server state from disk.
func LoadState(townRoot string) (*State, error) {
	stateFile := StateFile(townRoot)
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{}, nil
		}
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// SaveState saves Dolt server state to disk using atomic write.
func SaveState(townRoot string, state *State) error {
	stateFile := StateFile(townRoot)

	// Ensure daemon directory exists
	if err := os.MkdirAll(filepath.Dir(stateFile), 0755); err != nil {
		return err
	}

	return util.AtomicWriteJSON(stateFile, state)
}

// IsRunning checks if a Dolt server is running for the given town.
// Returns (running, pid, error).
// Checks both PID file AND port to detect externally-started servers.
func IsRunning(townRoot string) (bool, int, error) {
	config := DefaultConfig(townRoot)

	// First check PID file
	data, err := os.ReadFile(config.PidFile)
	if err == nil {
		pidStr := strings.TrimSpace(string(data))
		pid, err := strconv.Atoi(pidStr)
		if err == nil {
			// Check if process is alive
			process, err := os.FindProcess(pid)
			if err == nil {
				// On Unix, FindProcess always succeeds. Send signal 0 to check if alive.
				if err := process.Signal(syscall.Signal(0)); err == nil {
					// Verify it's actually a dolt process
					if isDoltProcess(pid) {
						return true, pid, nil
					}
				}
			}
		}
		// PID file is stale, clean it up
		_ = os.Remove(config.PidFile)
	}

	// No valid PID file - check if port is in use by dolt anyway
	// This catches externally-started dolt servers
	pid := findDoltServerOnPort(config.Port)
	if pid > 0 {
		return true, pid, nil
	}

	return false, 0, nil
}

// findDoltServerOnPort finds a dolt sql-server process listening on the given port.
// Returns the PID or 0 if not found.
func findDoltServerOnPort(port int) int {
	// Use lsof to find process on port
	cmd := exec.Command("lsof", "-i", fmt.Sprintf(":%d", port), "-t")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	// Parse first PID from output
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return 0
	}

	pid, err := strconv.Atoi(lines[0])
	if err != nil {
		return 0
	}

	// Verify it's a dolt process
	if isDoltProcess(pid) {
		return pid
	}

	return 0
}

// isDoltProcess checks if a PID is actually a dolt sql-server process.
func isDoltProcess(pid int) bool {
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	cmdline := strings.TrimSpace(string(output))
	return strings.Contains(cmdline, "dolt") && strings.Contains(cmdline, "sql-server")
}

// Start starts the Dolt SQL server.
func Start(townRoot string) error {
	config := DefaultConfig(townRoot)

	// Ensure daemon directory exists
	daemonDir := filepath.Dir(config.LogFile)
	if err := os.MkdirAll(daemonDir, 0755); err != nil {
		return fmt.Errorf("creating daemon directory: %w", err)
	}

	// Acquire exclusive lock to prevent concurrent starts (same pattern as gt daemon)
	lockFile := filepath.Join(daemonDir, "dolt.lock")
	fileLock := flock.New(lockFile)
	locked, err := fileLock.TryLock()
	if err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("another gt dolt start is in progress")
	}
	defer func() { _ = fileLock.Unlock() }()

	// Check if already running
	running, pid, err := IsRunning(townRoot)
	if err != nil {
		return fmt.Errorf("checking server status: %w", err)
	}
	if running {
		return fmt.Errorf("Dolt server already running (PID %d)", pid)
	}

	// Ensure data directory exists
	if err := os.MkdirAll(config.DataDir, 0755); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	// List available databases
	databases, err := ListDatabases(townRoot)
	if err != nil {
		return fmt.Errorf("listing databases: %w", err)
	}

	if len(databases) == 0 {
		return fmt.Errorf("no databases found in %s\nInitialize with: gt dolt init-rig <name>", config.DataDir)
	}

	// Clean up stale Dolt LOCK files in all database directories
	for _, db := range databases {
		dbDir := filepath.Join(config.DataDir, db)
		if err := cleanupStaleDoltLock(dbDir); err != nil {
			// Non-fatal warning
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
	}

	// Open log file
	logFile, err := os.OpenFile(config.LogFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}

	// Start dolt sql-server with --data-dir to serve all databases
	// Note: --user flag is deprecated in newer Dolt; authentication is handled
	// via privilege system. Default is root user with no password for localhost.
	cmd := exec.Command("dolt", "sql-server",
		"--port", strconv.Itoa(config.Port),
		"--data-dir", config.DataDir,
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Detach from terminal
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("starting Dolt server: %w", err)
	}

	// Close log file in parent (child has its own handle)
	_ = logFile.Close()

	// Write PID file
	if err := os.WriteFile(config.PidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0644); err != nil {
		// Try to kill the process we just started
		_ = cmd.Process.Kill()
		return fmt.Errorf("writing PID file: %w", err)
	}

	// Save state
	state := &State{
		Running:   true,
		PID:       cmd.Process.Pid,
		Port:      config.Port,
		StartedAt: time.Now(),
		DataDir:   config.DataDir,
		Databases: databases,
	}
	if err := SaveState(townRoot, state); err != nil {
		// Non-fatal - server is still running
		fmt.Fprintf(os.Stderr, "Warning: failed to save state: %v\n", err)
	}

	// Wait briefly and verify it started
	time.Sleep(500 * time.Millisecond)

	running, _, err = IsRunning(townRoot)
	if err != nil {
		return fmt.Errorf("verifying server started: %w", err)
	}
	if !running {
		return fmt.Errorf("Dolt server failed to start (check logs with 'gt dolt logs')")
	}

	return nil
}

// cleanupStaleDoltLock removes a stale Dolt LOCK file if no process holds it.
// Dolt's embedded mode uses a file lock at .dolt/noms/LOCK that can become stale
// after crashes. This checks if any process holds the lock before removing.
// Returns nil if lock is held by active processes (this is expected if bd is running).
func cleanupStaleDoltLock(databaseDir string) error {
	lockPath := filepath.Join(databaseDir, ".dolt", "noms", "LOCK")

	// Check if lock file exists
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		return nil // No lock file, nothing to clean
	}

	// Check if any process holds this file open using lsof
	cmd := exec.Command("lsof", lockPath)
	_, err := cmd.Output()
	if err != nil {
		// lsof returns exit code 1 when no process has the file open
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// No process holds the lock - safe to remove stale lock
			if err := os.Remove(lockPath); err != nil {
				return fmt.Errorf("failed to remove stale LOCK file: %w", err)
			}
			return nil
		}
		// Other error - ignore, let dolt handle it
		return nil
	}

	// lsof found processes - lock is legitimately held (likely by bd)
	// This is not an error condition; dolt server will handle the conflict
	return nil
}

// Stop stops the Dolt SQL server.
// Works for both servers started via gt dolt start AND externally-started servers.
func Stop(townRoot string) error {
	config := DefaultConfig(townRoot)

	running, pid, err := IsRunning(townRoot)
	if err != nil {
		return err
	}
	if !running {
		return fmt.Errorf("Dolt server is not running")
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("finding process: %w", err)
	}

	// Send SIGTERM for graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("sending SIGTERM: %w", err)
	}

	// Wait for graceful shutdown (dolt needs more time)
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		if err := process.Signal(syscall.Signal(0)); err != nil {
			// Process has exited
			break
		}
	}

	// Check if still running
	if err := process.Signal(syscall.Signal(0)); err == nil {
		// Still running, force kill
		_ = process.Signal(syscall.SIGKILL)
		time.Sleep(100 * time.Millisecond)
	}

	// Clean up PID file
	_ = os.Remove(config.PidFile)

	// Update state - preserve historical info
	state, _ := LoadState(townRoot)
	if state == nil {
		state = &State{}
	}
	state.Running = false
	state.PID = 0
	_ = SaveState(townRoot, state)

	return nil
}

// GetConnectionString returns the MySQL connection string for the server.
// Use GetConnectionStringForRig for a specific database.
func GetConnectionString(townRoot string) string {
	config := DefaultConfig(townRoot)
	return fmt.Sprintf("%s@tcp(127.0.0.1:%d)/", config.User, config.Port)
}

// GetConnectionStringForRig returns the MySQL connection string for a specific rig database.
func GetConnectionStringForRig(townRoot, rigName string) string {
	config := DefaultConfig(townRoot)
	return fmt.Sprintf("%s@tcp(127.0.0.1:%d)/%s", config.User, config.Port, rigName)
}

// ListDatabases returns the list of available rig databases in the data directory.
func ListDatabases(townRoot string) ([]string, error) {
	config := DefaultConfig(townRoot)

	entries, err := os.ReadDir(config.DataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var databases []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Check if this directory is a valid Dolt database
		doltDir := filepath.Join(config.DataDir, entry.Name(), ".dolt")
		if _, err := os.Stat(doltDir); err == nil {
			databases = append(databases, entry.Name())
		}
	}

	return databases, nil
}

// InitRig initializes a new rig database in the data directory.
func InitRig(townRoot, rigName string) error {
	if rigName == "" {
		return fmt.Errorf("rig name cannot be empty")
	}

	config := DefaultConfig(townRoot)

	// Validate rig name (simple alphanumeric + underscore/dash)
	for _, r := range rigName {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return fmt.Errorf("invalid rig name %q: must contain only alphanumeric, underscore, or dash", rigName)
		}
	}

	rigDir := filepath.Join(config.DataDir, rigName)

	// Check if already exists
	if _, err := os.Stat(filepath.Join(rigDir, ".dolt")); err == nil {
		return fmt.Errorf("rig database %q already exists at %s", rigName, rigDir)
	}

	// Create the rig directory
	if err := os.MkdirAll(rigDir, 0755); err != nil {
		return fmt.Errorf("creating rig directory: %w", err)
	}

	// Initialize Dolt database
	cmd := exec.Command("dolt", "init")
	cmd.Dir = rigDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("initializing Dolt database: %w\n%s", err, output)
	}

	return nil
}

// Migration represents a database migration from old to new location.
type Migration struct {
	RigName    string
	SourcePath string
	TargetPath string
}

// FindMigratableDatabases finds existing dolt databases that can be migrated.
func FindMigratableDatabases(townRoot string) []Migration {
	var migrations []Migration
	config := DefaultConfig(townRoot)

	// Check town-level beads database: .beads/dolt/beads -> .dolt-data/hq
	townSource := filepath.Join(townRoot, ".beads", "dolt", "beads")
	if _, err := os.Stat(filepath.Join(townSource, ".dolt")); err == nil {
		// Check target doesn't already have data
		targetDir := filepath.Join(config.DataDir, "hq")
		if _, err := os.Stat(filepath.Join(targetDir, ".dolt")); os.IsNotExist(err) {
			migrations = append(migrations, Migration{
				RigName:    "hq",
				SourcePath: townSource,
				TargetPath: targetDir,
			})
		}
	}

	// Check rig-level beads databases
	// Look for directories in townRoot that have .beads/dolt/beads
	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return migrations
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		rigName := entry.Name()
		rigSource := filepath.Join(townRoot, rigName, ".beads", "dolt", "beads")

		if _, err := os.Stat(filepath.Join(rigSource, ".dolt")); err == nil {
			// Check target doesn't already have data
			targetDir := filepath.Join(config.DataDir, rigName)
			if _, err := os.Stat(filepath.Join(targetDir, ".dolt")); os.IsNotExist(err) {
				migrations = append(migrations, Migration{
					RigName:    rigName,
					SourcePath: rigSource,
					TargetPath: targetDir,
				})
			}
		}
	}

	return migrations
}

// MigrateRigFromBeads migrates an existing beads Dolt database to the data directory.
// This is used to migrate from the old per-rig .beads/dolt/beads layout to the new
// centralized .dolt-data/<rigname> layout.
func MigrateRigFromBeads(townRoot, rigName, sourcePath string) error {
	config := DefaultConfig(townRoot)

	targetDir := filepath.Join(config.DataDir, rigName)

	// Check if target already exists
	if _, err := os.Stat(filepath.Join(targetDir, ".dolt")); err == nil {
		return fmt.Errorf("rig database %q already exists at %s", rigName, targetDir)
	}

	// Check if source exists
	if _, err := os.Stat(filepath.Join(sourcePath, ".dolt")); os.IsNotExist(err) {
		return fmt.Errorf("source database not found at %s", sourcePath)
	}

	// Ensure data directory exists
	if err := os.MkdirAll(config.DataDir, 0755); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	// Move the database directory
	if err := os.Rename(sourcePath, targetDir); err != nil {
		return fmt.Errorf("moving database: %w", err)
	}

	return nil
}
