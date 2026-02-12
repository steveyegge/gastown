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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gofrs/flock"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/util"
)

// Default configuration
const (
	DefaultPort           = 3307
	DefaultUser           = "root" // Default Dolt user (no password for local access)
	DefaultMaxConnections = 50     // Conservative default to prevent connection storms
)

// metadataMu provides per-path mutexes for EnsureMetadata goroutine synchronization.
// flock is inter-process only and cannot reliably synchronize goroutines within the
// same process (the same process may acquire the same flock twice without blocking).
var metadataMu sync.Map // map[string]*sync.Mutex

// getMetadataMu returns a mutex for the given metadata file path, creating one if needed.
func getMetadataMu(path string) *sync.Mutex {
	mu, _ := metadataMu.LoadOrStore(path, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

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

	// MaxConnections is the maximum number of simultaneous connections the server will accept.
	// Set to 0 to use the Dolt default (1000). Gas Town defaults to 50 to prevent
	// connection storms during mass polecat slings.
	MaxConnections int
}

// DefaultConfig returns the default Dolt server configuration.
func DefaultConfig(townRoot string) *Config {
	daemonDir := filepath.Join(townRoot, "daemon")
	return &Config{
		TownRoot:       townRoot,
		Port:           DefaultPort,
		User:           DefaultUser,
		DataDir:        filepath.Join(townRoot, ".dolt-data"),
		LogFile:        filepath.Join(daemonDir, "dolt.log"),
		PidFile:        filepath.Join(daemonDir, "dolt.pid"),
		MaxConnections: DefaultMaxConnections,
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

// CheckServerReachable verifies the Dolt server is actually accepting TCP connections.
// This catches the case where a process exists but the server hasn't finished starting,
// or the PID file is stale and the port is not actually listening.
// Returns nil if reachable, error describing the problem otherwise.
func CheckServerReachable(townRoot string) error {
	config := DefaultConfig(townRoot)
	addr := fmt.Sprintf("127.0.0.1:%d", config.Port)
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return fmt.Errorf("Dolt server not reachable at %s: %w\n\nStart with: gt dolt start", addr, err)
	}
	_ = conn.Close()
	return nil
}

// HasServerModeMetadata checks whether any rig has metadata.json configured for
// Dolt server mode. Returns the list of rig names configured for server mode.
// This is used to detect the split-brain risk: if metadata says "server" but
// the server isn't running, bd commands may silently create isolated databases.
func HasServerModeMetadata(townRoot string) []string {
	var serverRigs []string

	// Check town-level beads (hq)
	townBeadsDir := filepath.Join(townRoot, ".beads")
	if hasServerMode(townBeadsDir) {
		serverRigs = append(serverRigs, "hq")
	}

	// Check rig-level beads
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	data, err := os.ReadFile(rigsPath)
	if err != nil {
		return serverRigs
	}
	var config struct {
		Rigs map[string]interface{} `json:"rigs"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return serverRigs
	}

	for rigName := range config.Rigs {
		beadsDir := FindRigBeadsDir(townRoot, rigName)
		if beadsDir != "" && hasServerMode(beadsDir) {
			serverRigs = append(serverRigs, rigName)
		}
	}

	return serverRigs
}

// hasServerMode reads metadata.json and returns true if dolt_mode is "server".
func hasServerMode(beadsDir string) bool {
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return false
	}
	var metadata struct {
		DoltMode string `json:"dolt_mode"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return false
	}
	return metadata.DoltMode == "server"
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
	args := []string{"sql-server",
		"--port", strconv.Itoa(config.Port),
		"--data-dir", config.DataDir,
	}
	if config.MaxConnections > 0 {
		args = append(args, "--max-connections", strconv.Itoa(config.MaxConnections))
	}
	cmd := exec.Command("dolt", args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Detach from terminal
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		if closeErr := logFile.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close dolt log file: %v\n", closeErr)
		}
		return fmt.Errorf("starting Dolt server: %w", err)
	}

	// Close log file in parent (child has its own handle)
	if closeErr := logFile.Close(); closeErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to close dolt log file: %v\n", closeErr)
	}

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
// If the Dolt server is running, it executes CREATE DATABASE to register the
// database with the live server (avoiding the need for a restart).
// Returns true if the database was registered with a running server.
func InitRig(townRoot, rigName string) (serverWasRunning bool, err error) {
	if rigName == "" {
		return false, fmt.Errorf("rig name cannot be empty")
	}

	config := DefaultConfig(townRoot)

	// Validate rig name (simple alphanumeric + underscore/dash)
	for _, r := range rigName {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false, fmt.Errorf("invalid rig name %q: must contain only alphanumeric, underscore, or dash", rigName)
		}
	}

	rigDir := filepath.Join(config.DataDir, rigName)

	// Check if already exists on disk
	if _, err := os.Stat(filepath.Join(rigDir, ".dolt")); err == nil {
		return false, fmt.Errorf("rig database %q already exists at %s", rigName, rigDir)
	}

	// Check if server is running
	running, _, _ := IsRunning(townRoot)

	if running {
		// Server is running: use CREATE DATABASE which both creates the
		// directory and registers the database with the live server.
		if err := serverExecSQL(townRoot, fmt.Sprintf("CREATE DATABASE `%s`", rigName)); err != nil {
			return true, fmt.Errorf("creating database on running server: %w", err)
		}
	} else {
		// Server not running: create directory and init manually.
		// The database will be picked up when the server starts.
		if err := os.MkdirAll(rigDir, 0755); err != nil {
			return false, fmt.Errorf("creating rig directory: %w", err)
		}

		cmd := exec.Command("dolt", "init")
		cmd.Dir = rigDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return false, fmt.Errorf("initializing Dolt database: %w\n%s", err, output)
		}
	}

	// Update metadata.json to point to the server
	if err := EnsureMetadata(townRoot, rigName); err != nil {
		// Non-fatal: init succeeded, metadata update failed
		fmt.Fprintf(os.Stderr, "Warning: database initialized but metadata.json update failed: %v\n", err)
	}

	return running, nil
}

// Migration represents a database migration from old to new location.
type Migration struct {
	RigName    string
	SourcePath string
	TargetPath string
}

// findLocalDoltDB scans beadsDir/dolt/ for a subdirectory containing a .dolt
// directory (an embedded Dolt database). Returns the full path to the database
// directory, or "" if none found.
//
// bd names the subdirectory based on internal conventions (e.g., beads_hq,
// beads_gt) that have changed across versions. Scanning avoids hardcoding
// assumptions about the naming scheme.
//
// If multiple databases are found, returns "" and logs a warning to stderr.
// Callers should not silently pick one — ambiguity requires manual resolution.
func findLocalDoltDB(beadsDir string) string {
	doltParent := filepath.Join(beadsDir, "dolt")
	entries, err := os.ReadDir(doltParent)
	if err != nil {
		return ""
	}
	var candidates []string
	for _, e := range entries {
		// Resolve symlinks: DirEntry.IsDir() returns false for symlinks-to-directories
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(filepath.Join(doltParent, e.Name()))
			if err != nil {
				continue
			}
			fi, err := os.Stat(resolved)
			if err != nil || !fi.IsDir() {
				continue
			}
		} else if !e.IsDir() {
			continue
		}
		candidate := filepath.Join(doltParent, e.Name())
		if _, err := os.Stat(filepath.Join(candidate, ".dolt")); err == nil {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 {
		if len(entries) > 0 {
			fmt.Fprintf(os.Stderr, "[doltserver] Warning: %s exists but contains no valid dolt database\n", doltParent)
		}
		return ""
	}
	if len(candidates) > 1 {
		fmt.Fprintf(os.Stderr, "[doltserver] Warning: multiple dolt databases found in %s: %v — manual resolution required\n", doltParent, candidates)
		return ""
	}
	return candidates[0]
}

// FindMigratableDatabases finds existing dolt databases that can be migrated.
func FindMigratableDatabases(townRoot string) []Migration {
	var migrations []Migration
	config := DefaultConfig(townRoot)

	// Check town-level beads database -> .dolt-data/hq
	townBeadsDir := beads.ResolveBeadsDir(townRoot)
	townSource := findLocalDoltDB(townBeadsDir)
	if townSource != "" {
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
	// Look for directories in townRoot, following .beads/redirect if present
	entries, err := os.ReadDir(townRoot)
	if err != nil {
		return migrations
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		rigName := entry.Name()
		resolvedBeadsDir := beads.ResolveBeadsDir(filepath.Join(townRoot, rigName))
		rigSource := findLocalDoltDB(resolvedBeadsDir)

		if rigSource != "" {
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
// This is used to migrate from the old per-rig .beads/dolt/<db_name> layout to the new
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

	// Move the database directory (with cross-filesystem fallback)
	if err := moveDir(sourcePath, targetDir); err != nil {
		return fmt.Errorf("moving database: %w", err)
	}

	// Update metadata.json to point to the server
	if err := EnsureMetadata(townRoot, rigName); err != nil {
		// Non-fatal: migration succeeded, metadata update failed
		fmt.Fprintf(os.Stderr, "Warning: database migrated but metadata.json update failed: %v\n", err)
	}

	return nil
}

// DatabaseExists checks whether a rig database exists in the centralized .dolt-data/ directory.
func DatabaseExists(townRoot, rigName string) bool {
	config := DefaultConfig(townRoot)
	doltDir := filepath.Join(config.DataDir, rigName, ".dolt")
	_, err := os.Stat(doltDir)
	return err == nil
}

// BrokenWorkspace represents a workspace whose metadata.json points to a
// nonexistent database on the Dolt server.
type BrokenWorkspace struct {
	// RigName is the rig whose database is missing.
	RigName string

	// BeadsDir is the path to the .beads directory with the broken metadata.
	BeadsDir string

	// ConfiguredDB is the dolt_database value from metadata.json.
	ConfiguredDB string

	// HasLocalData is true if .beads/dolt/<dbname> exists locally and can be migrated.
	HasLocalData bool

	// LocalDataPath is the path to local Dolt data, if present.
	LocalDataPath string
}

// FindBrokenWorkspaces scans all rig metadata.json files for Dolt server
// configuration where the referenced database doesn't exist in .dolt-data/.
// These workspaces are broken: bd commands will fail or silently create
// isolated local databases instead of connecting to the centralized server.
func FindBrokenWorkspaces(townRoot string) []BrokenWorkspace {
	var broken []BrokenWorkspace

	// Check town-level beads (hq)
	townBeadsDir := filepath.Join(townRoot, ".beads")
	if ws := checkWorkspace(townRoot, "hq", townBeadsDir); ws != nil {
		broken = append(broken, *ws)
	}

	// Check rig-level beads via rigs.json
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	data, err := os.ReadFile(rigsPath)
	if err != nil {
		return broken
	}
	var config struct {
		Rigs map[string]interface{} `json:"rigs"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return broken
	}

	for rigName := range config.Rigs {
		beadsDir := FindRigBeadsDir(townRoot, rigName)
		if beadsDir == "" {
			continue
		}
		if ws := checkWorkspace(townRoot, rigName, beadsDir); ws != nil {
			broken = append(broken, *ws)
		}
	}

	return broken
}

// checkWorkspace checks a single rig's metadata.json for broken Dolt configuration.
// Returns nil if the workspace is healthy or not configured for Dolt server mode.
func checkWorkspace(townRoot, rigName, beadsDir string) *BrokenWorkspace {
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil
	}

	var metadata struct {
		DoltMode     string `json:"dolt_mode"`
		DoltDatabase string `json:"dolt_database"`
		Backend      string `json:"backend"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil
	}

	// Only check workspaces configured for Dolt server mode
	if metadata.DoltMode != "server" || metadata.Backend != "dolt" {
		return nil
	}

	dbName := metadata.DoltDatabase
	if dbName == "" {
		dbName = rigName
	}

	// Check if the database actually exists
	if DatabaseExists(townRoot, dbName) {
		return nil // healthy
	}

	ws := &BrokenWorkspace{
		RigName:      rigName,
		BeadsDir:     beadsDir,
		ConfiguredDB: dbName,
	}

	// Check for local data that could be migrated
	localDoltPath := findLocalDoltDB(beadsDir)
	if localDoltPath != "" {
		ws.HasLocalData = true
		ws.LocalDataPath = localDoltPath
	}

	return ws
}

// RepairWorkspace fixes a broken workspace by creating the missing database
// or migrating local data if present. Returns a description of what was done.
func RepairWorkspace(townRoot string, ws BrokenWorkspace) (string, error) {
	if ws.HasLocalData {
		// Migrate local data to centralized location
		if err := MigrateRigFromBeads(townRoot, ws.ConfiguredDB, ws.LocalDataPath); err != nil {
			return "", fmt.Errorf("migrating local data for %s: %w", ws.RigName, err)
		}
		return fmt.Sprintf("migrated local data from %s", ws.LocalDataPath), nil
	}

	// No local data — create a fresh database
	if _, err := InitRig(townRoot, ws.ConfiguredDB); err != nil {
		return "", fmt.Errorf("creating database for %s: %w", ws.RigName, err)
	}
	return "created new database", nil
}

// EnsureMetadata writes or updates the metadata.json for a rig's beads directory
// to include proper Dolt server configuration. This prevents the split-brain problem
// where bd falls back to local embedded databases instead of connecting to the
// centralized Dolt server.
//
// For the "hq" rig, it writes to <townRoot>/.beads/metadata.json.
// For other rigs, it writes to <townRoot>/<rigName>/mayor/rig/.beads/metadata.json.
func EnsureMetadata(townRoot, rigName string) error {
	// Use FindOrCreateRigBeadsDir to atomically resolve and create the directory,
	// avoiding the TOCTOU race where the directory state changes between
	// FindRigBeadsDir's Stat check and our subsequent file operations.
	beadsDir, err := FindOrCreateRigBeadsDir(townRoot, rigName)
	if err != nil {
		return fmt.Errorf("resolving beads directory for rig %q: %w", rigName, err)
	}

	metadataPath := filepath.Join(beadsDir, "metadata.json")

	// Acquire per-path mutex for goroutine synchronization.
	// EnsureAllMetadata calls EnsureMetadata concurrently; flock (inter-process)
	// cannot reliably synchronize goroutines within the same process.
	mu := getMetadataMu(metadataPath)
	mu.Lock()
	defer mu.Unlock()

	// Load existing metadata if present (preserve any extra fields)
	existing := make(map[string]interface{})
	if data, err := os.ReadFile(metadataPath); err == nil {
		_ = json.Unmarshal(data, &existing) // best effort
	}

	// Set/update the dolt server fields
	existing["database"] = "dolt"
	existing["backend"] = "dolt"
	existing["dolt_mode"] = "server"
	existing["dolt_database"] = rigName

	// Always set jsonl_export to the canonical filename.
	// Historical migrations may have left stale values (e.g., "beads.jsonl").
	existing["jsonl_export"] = "issues.jsonl"

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	if err := util.AtomicWriteFile(metadataPath, append(data, '\n'), 0600); err != nil {
		return fmt.Errorf("writing metadata.json: %w", err)
	}

	return nil
}

// EnsureAllMetadata updates metadata.json for all rig databases known to the
// Dolt server. This is the fix for the split-brain problem where worktrees
// each have their own isolated database.
func EnsureAllMetadata(townRoot string) (updated []string, errs []error) {
	databases, err := ListDatabases(townRoot)
	if err != nil {
		return nil, []error{fmt.Errorf("listing databases: %w", err)}
	}

	for _, dbName := range databases {
		if err := EnsureMetadata(townRoot, dbName); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", dbName, err))
		} else {
			updated = append(updated, dbName)
		}
	}

	return updated, errs
}

// FindRigBeadsDir returns the .beads directory path for a rig (read-only lookup).
// For "hq", returns <townRoot>/.beads.
// For other rigs, returns <townRoot>/<rigName>/mayor/rig/.beads if it exists,
// otherwise <townRoot>/<rigName>/.beads if it exists,
// otherwise <townRoot>/<rigName>/mayor/rig/.beads (for creation by caller).
//
// WARNING: This function has a TOCTOU race — the returned directory may change
// state between the Stat check and the caller's operation. For write operations
// that need the directory to exist, use FindOrCreateRigBeadsDir instead.
// For read-only operations, handle errors on the returned path gracefully.
func FindRigBeadsDir(townRoot, rigName string) string {
	if rigName == "hq" {
		return filepath.Join(townRoot, ".beads")
	}

	// Prefer mayor/rig/.beads (canonical location for tracked beads)
	mayorBeads := filepath.Join(townRoot, rigName, "mayor", "rig", ".beads")
	if _, err := os.Stat(mayorBeads); err == nil {
		return mayorBeads
	}

	// Fall back to rig-root .beads
	rigBeads := filepath.Join(townRoot, rigName, ".beads")
	if _, err := os.Stat(rigBeads); err == nil {
		return rigBeads
	}

	// Neither exists; return mayor path (caller will create it)
	return mayorBeads
}

// FindOrCreateRigBeadsDir atomically resolves and ensures the .beads directory
// exists for a rig. Unlike FindRigBeadsDir, this combines directory resolution
// with creation to avoid TOCTOU races where the directory state changes between
// the existence check and the caller's write operation.
//
// Use this for write operations (EnsureMetadata, etc.) where the directory must
// exist. Use FindRigBeadsDir for read-only lookups where graceful failure on
// missing directories is acceptable.
func FindOrCreateRigBeadsDir(townRoot, rigName string) (string, error) {
	if rigName == "hq" {
		dir := filepath.Join(townRoot, ".beads")
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("creating HQ beads dir: %w", err)
		}
		return dir, nil
	}

	// Check mayor/rig/.beads first (canonical location)
	mayorBeads := filepath.Join(townRoot, rigName, "mayor", "rig", ".beads")
	if _, err := os.Stat(mayorBeads); err == nil {
		return mayorBeads, nil
	}

	// Check rig-root .beads
	rigBeads := filepath.Join(townRoot, rigName, ".beads")
	if _, err := os.Stat(rigBeads); err == nil {
		return rigBeads, nil
	}

	// Neither exists — atomically create the canonical mayor path.
	// MkdirAll uses mkdir(2) which is atomic per POSIX, so concurrent
	// callers creating the same path won't race.
	if err := os.MkdirAll(mayorBeads, 0755); err != nil {
		return "", fmt.Errorf("creating beads dir: %w", err)
	}

	return mayorBeads, nil
}

// GetActiveConnectionCount queries the Dolt server to get the number of active connections.
// Uses `dolt sql` to query information_schema.PROCESSLIST, which avoids needing
// a MySQL driver dependency. Returns 0 if the server is unreachable or the query fails.
func GetActiveConnectionCount(townRoot string) (int, error) {
	config := DefaultConfig(townRoot)

	// Use dolt sql-client to query the server with a timeout to prevent
	// hanging indefinitely if the Dolt server is unresponsive.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx,
		"dolt", "sql",
		"-r", "csv",
		"-q", "SELECT COUNT(*) AS cnt FROM information_schema.PROCESSLIST",
	)
	cmd.Dir = config.DataDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("querying connection count: %w (output: %s)", err, strings.TrimSpace(string(output)))
	}

	// Parse CSV output: "cnt\n5\n"
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("unexpected output from connection count query: %s", string(output))
	}
	count, err := strconv.Atoi(strings.TrimSpace(lines[len(lines)-1]))
	if err != nil {
		return 0, fmt.Errorf("parsing connection count %q: %w", lines[len(lines)-1], err)
	}

	return count, nil
}

// HasConnectionCapacity checks whether the Dolt server has capacity for new connections.
// Returns true if the active connection count is below the threshold (80% of max_connections).
// Returns false with error if the connection count cannot be determined — fail closed
// to prevent connection storms that cause read-only mode (gt-lfc0d).
func HasConnectionCapacity(townRoot string) (bool, int, error) {
	config := DefaultConfig(townRoot)
	maxConn := config.MaxConnections
	if maxConn <= 0 {
		maxConn = 1000 // Dolt default
	}

	active, err := GetActiveConnectionCount(townRoot)
	if err != nil {
		// Fail closed: if we can't check, the server may be overloaded
		return false, 0, err
	}

	// Use 80% threshold to leave headroom for existing operations
	threshold := (maxConn * 80) / 100
	if threshold < 1 {
		threshold = 1
	}

	return active < threshold, active, nil
}

// HealthMetrics holds resource monitoring data for the Dolt server.
type HealthMetrics struct {
	// Connections is the number of active connections (from information_schema.PROCESSLIST).
	Connections int `json:"connections"`

	// MaxConnections is the configured maximum connections.
	MaxConnections int `json:"max_connections"`

	// ConnectionPct is the percentage of max connections in use.
	ConnectionPct float64 `json:"connection_pct"`

	// DiskUsageBytes is the total size of the .dolt-data/ directory.
	DiskUsageBytes int64 `json:"disk_usage_bytes"`

	// DiskUsageHuman is a human-readable disk usage string.
	DiskUsageHuman string `json:"disk_usage_human"`

	// QueryLatency is the time taken for a SELECT 1 round-trip.
	QueryLatency time.Duration `json:"query_latency_ms"`

	// ReadOnly indicates whether the server is in read-only mode.
	// When true, the server accepts reads but rejects all writes.
	ReadOnly bool `json:"read_only"`

	// Healthy indicates whether the server is within acceptable resource limits.
	Healthy bool `json:"healthy"`

	// Warnings contains any degradation warnings (non-fatal).
	Warnings []string `json:"warnings,omitempty"`
}

// GetHealthMetrics collects resource monitoring metrics from the Dolt server.
// Returns partial metrics if some checks fail — always returns what it can.
func GetHealthMetrics(townRoot string) *HealthMetrics {
	config := DefaultConfig(townRoot)
	metrics := &HealthMetrics{
		Healthy:        true,
		MaxConnections: config.MaxConnections,
	}
	if metrics.MaxConnections <= 0 {
		metrics.MaxConnections = 1000 // Dolt default
	}

	// 1. Query latency: time a SELECT 1
	latency, err := MeasureQueryLatency(townRoot)
	if err == nil {
		metrics.QueryLatency = latency
		if latency > 1*time.Second {
			metrics.Warnings = append(metrics.Warnings,
				fmt.Sprintf("query latency %v exceeds 1s threshold — server may be under stress", latency.Round(time.Millisecond)))
		}
	}

	// 2. Connection count
	connCount, err := GetActiveConnectionCount(townRoot)
	if err == nil {
		metrics.Connections = connCount
		metrics.ConnectionPct = float64(connCount) / float64(metrics.MaxConnections) * 100
		if metrics.ConnectionPct >= 80 {
			metrics.Healthy = false
			metrics.Warnings = append(metrics.Warnings,
				fmt.Sprintf("connection count %d is %.0f%% of max %d — approaching limit",
					connCount, metrics.ConnectionPct, metrics.MaxConnections))
		}
	}

	// 3. Disk usage
	diskBytes := dirSize(config.DataDir)
	metrics.DiskUsageBytes = diskBytes
	metrics.DiskUsageHuman = formatBytes(diskBytes)

	// 4. Read-only probe: attempt a test write
	readOnly, _ := CheckReadOnly(townRoot)
	metrics.ReadOnly = readOnly
	if readOnly {
		metrics.Healthy = false
		metrics.Warnings = append(metrics.Warnings,
			"server is in READ-ONLY mode — requires restart to recover")
	}

	return metrics
}

// CheckReadOnly probes the Dolt server to detect read-only state by attempting
// a test write. The server can enter read-only mode under concurrent write load
// ("cannot update manifest: database is read only") and will NOT self-recover.
// Returns (true, nil) if read-only, (false, nil) if writable, (false, err) on probe failure.
func CheckReadOnly(townRoot string) (bool, error) {
	config := DefaultConfig(townRoot)

	// Need a database to test writes against
	databases, err := ListDatabases(townRoot)
	if err != nil || len(databases) == 0 {
		return false, nil // Can't probe without a database
	}

	db := databases[0]
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Attempt a write operation: create a temp table, write a row, drop it.
	// If the server is in read-only mode, this will fail with a characteristic error.
	query := fmt.Sprintf(
		"USE `%s`; CREATE TABLE IF NOT EXISTS `__gt_health_probe` (v INT PRIMARY KEY); REPLACE INTO `__gt_health_probe` VALUES (1); DROP TABLE IF EXISTS `__gt_health_probe`",
		db,
	)
	cmd := exec.CommandContext(ctx, "dolt", "sql", "-q", query)
	cmd.Dir = config.DataDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if IsReadOnlyError(msg) {
			return true, nil
		}
		return false, fmt.Errorf("write probe failed: %w (%s)", err, msg)
	}

	return false, nil
}

// IsReadOnlyError checks if an error message indicates a Dolt read-only state.
// The characteristic error is "cannot update manifest: database is read only".
func IsReadOnlyError(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "read only") ||
		strings.Contains(lower, "read-only") ||
		strings.Contains(lower, "readonly")
}

// RecoverReadOnly detects a read-only Dolt server, restarts it, and verifies
// recovery. This is the gt-level counterpart to the daemon's auto-recovery:
// when a gt command (spawn, done, etc.) encounters persistent read-only errors,
// it can call this to attempt recovery without waiting for the daemon's 30s loop.
// Returns nil if recovery succeeded, an error if recovery failed or wasn't needed.
func RecoverReadOnly(townRoot string) error {
	readOnly, err := CheckReadOnly(townRoot)
	if err != nil {
		return fmt.Errorf("read-only probe failed: %w", err)
	}
	if !readOnly {
		return nil // Server is writable, no recovery needed
	}

	fmt.Printf("Dolt server is in read-only mode, attempting recovery...\n")

	// Stop the server
	if err := Stop(townRoot); err != nil {
		// Server might already be stopped or unreachable
		fmt.Printf("Warning: stop returned error (proceeding with restart): %v\n", err)
	}

	// Brief pause for cleanup
	time.Sleep(1 * time.Second)

	// Restart the server
	if err := Start(townRoot); err != nil {
		return fmt.Errorf("failed to restart Dolt server: %w", err)
	}

	// Verify recovery with exponential backoff (server may need time to become writable)
	const maxAttempts = 5
	const baseBackoff = 500 * time.Millisecond
	const maxBackoff = 8 * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		backoff := baseBackoff
		for i := 1; i < attempt; i++ {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
				break
			}
		}
		time.Sleep(backoff)

		readOnly, err = CheckReadOnly(townRoot)
		if err != nil {
			if attempt == maxAttempts {
				return fmt.Errorf("post-restart probe failed after %d attempts: %w", maxAttempts, err)
			}
			continue
		}
		if !readOnly {
			fmt.Printf("Dolt server recovered from read-only state\n")
			return nil
		}
	}

	return fmt.Errorf("Dolt server still read-only after restart (%d verification attempts)", maxAttempts)
}

// doltSQLWithRecovery executes a SQL statement with retry logic and, if retries
// are exhausted due to read-only errors, attempts server restart before a final retry.
// This is the gt-level recovery path for polecat management operations (spawn, done).
func doltSQLWithRecovery(townRoot, rigDB, query string) error {
	err := doltSQLWithRetry(townRoot, rigDB, query)
	if err == nil {
		return nil
	}

	// If the final error is a read-only error, attempt recovery
	if !IsReadOnlyError(err.Error()) {
		return err
	}

	// Attempt server recovery
	if recoverErr := RecoverReadOnly(townRoot); recoverErr != nil {
		return fmt.Errorf("read-only recovery failed: %w (original: %v)", recoverErr, err)
	}

	// Retry the operation after recovery
	if retryErr := doltSQL(townRoot, rigDB, query); retryErr != nil {
		return fmt.Errorf("operation failed after read-only recovery: %w", retryErr)
	}

	return nil
}

// MeasureQueryLatency times a SELECT 1 query against the Dolt server.
func MeasureQueryLatency(townRoot string) (time.Duration, error) {
	config := DefaultConfig(townRoot)

	start := time.Now()
	cmd := exec.Command("dolt", "sql", "-q", "SELECT 1")
	cmd.Dir = config.DataDir
	output, err := cmd.CombinedOutput()
	elapsed := time.Since(start)

	if err != nil {
		return 0, fmt.Errorf("SELECT 1 failed: %w (output: %s)", err, strings.TrimSpace(string(output)))
	}

	return elapsed, nil
}

// dirSize returns the total size of a directory tree in bytes.
func dirSize(path string) int64 {
	var total int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

// formatBytes returns a human-readable size string.
func formatBytes(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// moveDir moves a directory from src to dest. It first tries os.Rename for
// efficiency, but falls back to copy+delete if src and dest are on different
// filesystems (which causes EXDEV error on rename).
func moveDir(src, dest string) error {
	if err := os.Rename(src, dest); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}

	// Cross-filesystem: copy then delete source
	if runtime.GOOS == "windows" {
		cmd := exec.Command("robocopy", src, dest, "/E", "/MOVE", "/R:1", "/W:1")
		if err := cmd.Run(); err != nil {
			// robocopy returns 1 for success with copies
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() <= 7 {
				return nil
			}
			return fmt.Errorf("robocopy: %w", err)
		}
		return nil
	}
	cmd := exec.Command("cp", "-a", src, dest)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("copying directory: %w", err)
	}
	if err := os.RemoveAll(src); err != nil {
		return fmt.Errorf("removing source after copy: %w", err)
	}
	return nil
}

// serverExecSQL executes a SQL statement against the Dolt server without targeting
// a specific database. Used for server-level commands like CREATE DATABASE.
func serverExecSQL(townRoot, query string) error {
	config := DefaultConfig(townRoot)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "dolt", "sql", "-q", query)
	cmd.Dir = config.DataDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (output: %s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// doltSQL executes a SQL statement against a specific rig database on the Dolt server.
// Uses the dolt CLI from the data directory (auto-detects running server).
// The USE prefix selects the database since --use-db is not available on all dolt versions.
func doltSQL(townRoot, rigDB, query string) error {
	config := DefaultConfig(townRoot)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Prepend USE <db> to select the target database.
	fullQuery := fmt.Sprintf("USE %s; %s", rigDB, query)
	cmd := exec.CommandContext(ctx, "dolt", "sql", "-q", fullQuery)
	cmd.Dir = config.DataDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (output: %s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// doltSQLWithRetry executes a SQL statement with exponential backoff on transient errors.
func doltSQLWithRetry(townRoot, rigDB, query string) error {
	const maxRetries = 5
	const baseBackoff = 500 * time.Millisecond
	const maxBackoff = 15 * time.Second

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := doltSQL(townRoot, rigDB, query); err != nil {
			lastErr = err
			if !isDoltRetryableError(err) {
				return err
			}
			if attempt < maxRetries {
				backoff := baseBackoff
				for i := 1; i < attempt; i++ {
					backoff *= 2
					if backoff > maxBackoff {
						backoff = maxBackoff
						break
					}
				}
				time.Sleep(backoff)
			}
			continue
		}
		return nil
	}
	return fmt.Errorf("after %d retries: %w", maxRetries, lastErr)
}

// isDoltRetryableError returns true if the error is a transient Dolt failure worth retrying.
// Covers manifest lock contention, read-only mode, optimistic lock failures, and timeouts.
func isDoltRetryableError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "database is read only") ||
		strings.Contains(msg, "cannot update manifest") ||
		strings.Contains(msg, "optimistic lock") ||
		strings.Contains(msg, "serialization failure") ||
		strings.Contains(msg, "lock wait timeout") ||
		strings.Contains(msg, "try restarting transaction")
}

// validBranchNameRe matches only safe branch name characters: alphanumeric, hyphen,
// underscore, dot, and forward slash. This prevents SQL injection via branch names
// interpolated into Dolt stored procedure calls.
var validBranchNameRe = regexp.MustCompile(`^[a-zA-Z0-9._/-]+$`)

// validateBranchName returns an error if branchName contains characters that could
// break out of SQL string literals. Defense-in-depth: PolecatBranchName generates
// safe names, but callers accept arbitrary strings.
func validateBranchName(branchName string) error {
	if branchName == "" {
		return fmt.Errorf("branch name must not be empty")
	}
	if !validBranchNameRe.MatchString(branchName) {
		return fmt.Errorf("branch name %q contains invalid characters", branchName)
	}
	return nil
}

// PolecatBranchName returns the Dolt branch name for a polecat.
// Format: polecat-<name>-<unix-timestamp>
func PolecatBranchName(polecatName string) string {
	return fmt.Sprintf("polecat-%s-%d", strings.ToLower(polecatName), time.Now().Unix())
}

// CreatePolecatBranch creates a Dolt branch for a polecat's isolated writes.
// Each polecat gets its own branch to eliminate optimistic lock contention.
// Retries with exponential backoff on transient errors (read-only, manifest lock, etc).
// If read-only errors persist after retries, attempts server recovery (gt-chx92).
func CreatePolecatBranch(townRoot, rigDB, branchName string) error {
	if err := validateBranchName(branchName); err != nil {
		return fmt.Errorf("creating Dolt branch in %s: %w", rigDB, err)
	}
	query := fmt.Sprintf("CALL DOLT_BRANCH('%s')", branchName)

	if err := doltSQLWithRecovery(townRoot, rigDB, query); err != nil {
		return fmt.Errorf("creating Dolt branch %s in %s: %w", branchName, rigDB, err)
	}
	return nil
}

// CommitServerWorkingSet stages all pending changes and commits them on the current branch via SQL.
// This flushes the Dolt working set to HEAD so that DOLT_BRANCH (which forks from
// HEAD, not the working set) will include all recent writes. Critical for the sling
// flow where BD_DOLT_AUTO_COMMIT=off leaves writes in working set only.
//
// NOTE: This flushes ALL pending working set changes on the target branch, not just
// those from a specific polecat. In batch sling, polecat B's flush may capture
// polecat A's writes. This is benign because beads are keyed by unique ID, so
// duplicate data across branches merges cleanly.
func CommitServerWorkingSet(townRoot, rigDB, message string) error {
	if err := doltSQLWithRecovery(townRoot, rigDB, "CALL DOLT_ADD('-A')"); err != nil {
		return fmt.Errorf("staging working set in %s: %w", rigDB, err)
	}
	escaped := strings.ReplaceAll(message, "'", "''")
	query := fmt.Sprintf("CALL DOLT_COMMIT('--allow-empty', '-m', '%s')", escaped)
	if err := doltSQLWithRecovery(townRoot, rigDB, query); err != nil {
		return fmt.Errorf("committing working set in %s: %w", rigDB, err)
	}
	return nil
}

// MergePolecatBranch merges a polecat's Dolt branch into main and deletes it.
// Called at gt done time to make the polecat's beads changes visible.
//
// CRITICAL: The entire operation runs as a single SQL script (one connection).
// In Dolt server mode, each `dolt sql -q` call opens a new connection, and
// DOLT_CHECKOUT only affects the current connection. Separate calls would
// checkout the polecat branch on connection 1, then ADD/COMMIT on connection 2
// (which defaults back to main), silently losing all polecat working set data.
//
// The script handles two scenarios:
//  1. Fast-forward merge (no conflict): commit polecat working set, merge to main
//  2. Conflict: disable autocommit, merge, resolve with --theirs (polecat wins), commit
//
// On conflict, a second script runs with autocommit disabled so conflicts can
// be resolved rather than triggering an automatic rollback.
func MergePolecatBranch(townRoot, rigDB, branchName string) error {
	if err := validateBranchName(branchName); err != nil {
		return fmt.Errorf("merging Dolt branch in %s: %w", rigDB, err)
	}

	// Phase 1: Commit polecat working set and attempt merge.
	// All in one connection so DOLT_CHECKOUT persists across statements.
	// NOTE: DOLT_BRANCH('-D') is deliberately NOT in the merge scripts.
	// If the merge fails (conflict), the branch must still exist for Phase 2.
	// Branch deletion happens separately after successful merge.
	escaped := strings.ReplaceAll(branchName, "'", "''")
	script := fmt.Sprintf(`USE %s;
CALL DOLT_CHECKOUT('%s');
CALL DOLT_ADD('-A');
CALL DOLT_COMMIT('--allow-empty', '-m', 'polecat %s final state');
CALL DOLT_CHECKOUT('main');
CALL DOLT_MERGE('%s');
`, rigDB, escaped, escaped, escaped)

	if err := doltSQLScriptWithRetry(townRoot, script); err != nil {
		if !strings.Contains(err.Error(), "Merge conflict") {
			return fmt.Errorf("merging %s to main in %s: %w", branchName, rigDB, err)
		}

		// Phase 2: Conflict detected. Re-run merge with autocommit disabled
		// so conflicts are staged (not rolled back) and can be resolved.
		// --theirs: polecat state wins (latest mutations, always authoritative).
		fmt.Printf("Dolt merge conflict on %s, auto-resolving (--theirs)...\n", branchName)
		conflictScript := fmt.Sprintf(`USE %s;
SET @@autocommit = 0;
CALL DOLT_CHECKOUT('main');
CALL DOLT_MERGE('%s');
CALL DOLT_CONFLICTS_RESOLVE('--theirs', '.');
CALL DOLT_COMMIT('-m', 'merge %s (conflicts auto-resolved)');
SET @@autocommit = 1;
`, rigDB, escaped, escaped)

		if err := doltSQLScriptWithRetry(townRoot, conflictScript); err != nil {
			return fmt.Errorf("conflict-resolving merge of %s in %s: %w", branchName, rigDB, err)
		}
	}

	// Delete branch only after successful merge (either phase).
	// This prevents branch loss if the merge script fails partway through.
	DeletePolecatBranch(townRoot, rigDB, branchName)
	return nil
}

// doltSQLScript executes a multi-statement SQL script via a temp file.
// Uses `dolt sql --file` for reliable multi-statement execution within a
// single connection, preserving DOLT_CHECKOUT state across statements.
func doltSQLScript(townRoot, script string) error {
	config := DefaultConfig(townRoot)

	tmpFile, err := os.CreateTemp("", "dolt-script-*.sql")
	if err != nil {
		return fmt.Errorf("creating temp SQL file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(script); err != nil {
		tmpFile.Close()
		return fmt.Errorf("writing SQL script: %w", err)
	}
	tmpFile.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "dolt", "sql", "--file", tmpFile.Name())
	cmd.Dir = config.DataDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (output: %s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// doltSQLScriptWithRetry executes a SQL script with exponential backoff on transient errors.
// Callers must ensure scripts are idempotent, as partial execution may have occurred
// before the retry. Uses the same retry classification as doltSQLWithRetry but with
// fewer retries and shorter backoff since multi-statement scripts are more expensive.
func doltSQLScriptWithRetry(townRoot, script string) error {
	const maxRetries = 3
	const baseBackoff = 500 * time.Millisecond
	const maxBackoff = 8 * time.Second

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := doltSQLScript(townRoot, script); err != nil {
			lastErr = err
			if !isDoltRetryableError(err) {
				return err
			}
			if attempt < maxRetries {
				backoff := baseBackoff
				for i := 1; i < attempt; i++ {
					backoff *= 2
					if backoff > maxBackoff {
						backoff = maxBackoff
						break
					}
				}
				time.Sleep(backoff)
			}
			continue
		}
		return nil
	}
	return fmt.Errorf("after %d retries: %w", maxRetries, lastErr)
}

// DeletePolecatBranch deletes a polecat's Dolt branch (cleanup/nuke).
// Best-effort: logs warning if branch doesn't exist or deletion fails.
func DeletePolecatBranch(townRoot, rigDB, branchName string) {
	if err := validateBranchName(branchName); err != nil {
		fmt.Printf("Warning: invalid Dolt branch name %q: %v\n", branchName, err)
		return
	}
	query := fmt.Sprintf("CALL DOLT_BRANCH('-d', '%s')", branchName)
	if err := doltSQL(townRoot, rigDB, query); err != nil {
		// Non-fatal: branch may not exist (already merged/deleted)
		fmt.Printf("Warning: could not delete Dolt branch %s: %v\n", branchName, err)
	}
}
