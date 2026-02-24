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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gofrs/flock"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/util"
)

// EnsureDoltIdentity configures dolt global identity (user.name, user.email)
// if not already set. Copies values from git config as a sensible default.
// This must run before InitRig and Start, since dolt init requires identity.
func EnsureDoltIdentity() error {
	// Check each field independently to avoid creating duplicates with --add.
	// Distinguish "key not found" (exit code 1, empty output) from dolt crashes.
	needName, err := doltConfigMissing("user.name")
	if err != nil {
		return fmt.Errorf("probing dolt user.name: %w", err)
	}
	needEmail, err := doltConfigMissing("user.email")
	if err != nil {
		return fmt.Errorf("probing dolt user.email: %w", err)
	}

	if !needName && !needEmail {
		return nil // already configured
	}

	// Copy missing fields from git global config.
	// We read --global only (not repo-local) to avoid silently persisting
	// a repo-scoped override into dolt's permanent global config.
	if needName {
		gitName, err := exec.Command("git", "config", "--global", "user.name").Output()
		if err != nil || len(bytes.TrimSpace(gitName)) == 0 {
			return fmt.Errorf("dolt identity not configured and git user.name not available; run: dolt config --global --add user.name \"Your Name\"")
		}
		if err := setDoltGlobalConfig("user.name", strings.TrimSpace(string(gitName))); err != nil {
			return fmt.Errorf("failed to set dolt user.name: %w", err)
		}
	}

	if needEmail {
		gitEmail, err := exec.Command("git", "config", "--global", "user.email").Output()
		if err != nil || len(bytes.TrimSpace(gitEmail)) == 0 {
			return fmt.Errorf("dolt identity not configured and git user.email not available; run: dolt config --global --add user.email \"you@example.com\"")
		}
		if err := setDoltGlobalConfig("user.email", strings.TrimSpace(string(gitEmail))); err != nil {
			return fmt.Errorf("failed to set dolt user.email: %w", err)
		}
	}

	return nil
}

// doltConfigMissing checks whether a dolt global config key is unset.
// Returns (true, nil) for missing keys, (false, nil) for present keys,
// and (false, error) when dolt itself fails unexpectedly.
func doltConfigMissing(key string) (bool, error) {
	cmd := exec.Command("dolt", "config", "--global", "--get", key)
	out, err := cmd.Output()
	if err == nil {
		// Command succeeded — key exists if output is non-empty
		return len(bytes.TrimSpace(out)) == 0, nil
	}
	// dolt config --get exits 1 for missing keys with no stderr.
	// Any other failure (crash, permission error) is unexpected.
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return true, nil // key not found — expected
	}
	return false, fmt.Errorf("dolt config --global --get %s: %w", key, err)
}

// setDoltGlobalConfig idempotently sets a dolt global config key.
// Uses --unset then --add to avoid duplicate entries from repeated calls.
func setDoltGlobalConfig(key, value string) error {
	// Remove existing value (ignore error — key may not exist yet)
	_ = exec.Command("dolt", "config", "--global", "--unset", key).Run()
	return exec.Command("dolt", "config", "--global", "--add", key, value).Run()
}

// Default configuration
const (
	DefaultPort           = 3307
	DefaultUser           = "root" // Default Dolt user (no password for local access)
	DefaultMaxConnections = 200    // Support concurrent crews (10 conns/pool × ~15 bd processes + headroom)
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

	// Host is the Dolt server hostname or IP.
	// Empty means localhost (backward-compatible default).
	Host string

	// Port is the MySQL protocol port.
	Port int

	// User is the MySQL user name.
	User string

	// Password is the MySQL password.
	// Empty means no password (backward-compatible default for local access).
	Password string

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
// Environment variables override defaults when set:
//   - GT_DOLT_HOST → Host
//   - GT_DOLT_PORT → Port
//   - GT_DOLT_USER → User
//   - GT_DOLT_PASSWORD → Password
func DefaultConfig(townRoot string) *Config {
	daemonDir := filepath.Join(townRoot, "daemon")
	config := &Config{
		TownRoot:       townRoot,
		Port:           DefaultPort,
		User:           DefaultUser,
		DataDir:        filepath.Join(townRoot, ".dolt-data"),
		LogFile:        filepath.Join(daemonDir, "dolt.log"),
		PidFile:        filepath.Join(daemonDir, "dolt.pid"),
		MaxConnections: DefaultMaxConnections,
	}

	if h := os.Getenv("GT_DOLT_HOST"); h != "" {
		config.Host = h
	}
	if p := os.Getenv("GT_DOLT_PORT"); p != "" {
		if port, err := strconv.Atoi(p); err == nil {
			config.Port = port
		}
	}
	if u := os.Getenv("GT_DOLT_USER"); u != "" {
		config.User = u
	}
	if pw := os.Getenv("GT_DOLT_PASSWORD"); pw != "" {
		config.Password = pw
	}

	return config
}

// IsRemote returns true when the config points to a non-local Dolt server.
// Empty host, "127.0.0.1", "localhost", "::1", and "[::1]" are all considered local.
func (c *Config) IsRemote() bool {
	switch strings.ToLower(c.Host) {
	case "", "127.0.0.1", "localhost", "::1", "[::1]":
		return false
	}
	return true
}

// SQLArgs returns the dolt CLI flags needed to connect to a remote server.
// Returns nil for local servers (dolt auto-detects the running local server).
func (c *Config) SQLArgs() []string {
	if !c.IsRemote() {
		return nil
	}
	return []string{
		"--host", c.Host,
		"--port", strconv.Itoa(c.Port),
		"--user", c.User,
		"--no-tls",
	}
}

// userDSN returns the user[:password] portion of a MySQL DSN.
func (c *Config) userDSN() string {
	if c.Password != "" {
		return c.User + ":" + c.Password
	}
	return c.User
}

// HostPort returns "host:port", defaulting host to "127.0.0.1" when empty.
func (c *Config) HostPort() string {
	host := c.Host
	if host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("%s:%d", host, c.Port)
}

// buildDoltSQLCmd constructs a dolt sql command that works for both local and remote servers.
// For local: runs from config.DataDir so dolt auto-detects the running server.
// For remote: prepends connection flags and passes password via DOLT_CLI_PASSWORD env var.
func buildDoltSQLCmd(ctx context.Context, config *Config, args ...string) *exec.Cmd {
	sqlArgs := config.SQLArgs()
	fullArgs := make([]string, 0, len(sqlArgs)+1+len(args))
	fullArgs = append(fullArgs, "sql")
	fullArgs = append(fullArgs, sqlArgs...)
	fullArgs = append(fullArgs, args...)

	cmd := exec.CommandContext(ctx, "dolt", fullArgs...)

	if !config.IsRemote() {
		cmd.Dir = config.DataDir
	}

	if config.IsRemote() && config.Password != "" {
		cmd.Env = append(os.Environ(), "DOLT_CLI_PASSWORD="+config.Password)
	}

	return cmd
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
// For remote servers, skips PID/port scan and just does TCP reachability.
func IsRunning(townRoot string) (bool, int, error) {
	config := DefaultConfig(townRoot)

	// Remote server: no local PID/process to check — just TCP reachability.
	if config.IsRemote() {
		conn, err := net.DialTimeout("tcp", config.HostPort(), 2*time.Second)
		if err != nil {
			return false, 0, nil
		}
		_ = conn.Close()
		return true, 0, nil
	}

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
	addr := config.HostPort()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		hint := ""
		if !config.IsRemote() {
			hint = "\n\nStart with: gt dolt start"
		}
		return fmt.Errorf("Dolt server not reachable at %s: %w%s", addr, err, hint)
	}
	_ = conn.Close()
	return nil
}

// WaitForReady polls for the Dolt server to become reachable (TCP connection
// succeeds) within the given timeout. Returns nil if the server is reachable
// or if no server-mode metadata is configured (nothing to wait for).
// Returns an error if the timeout expires before the server is reachable.
//
// This is used by gt up to ensure the Dolt server is ready before starting
// agents (witnesses, refineries) that depend on beads database access.
// Without this, agents race the Dolt server startup and get "connection refused".
func WaitForReady(townRoot string, timeout time.Duration) error {
	// Check if any rig is configured for server mode.
	// If not, there's no Dolt server to wait for.
	if len(HasServerModeMetadata(townRoot)) == 0 {
		return nil
	}

	config := DefaultConfig(townRoot)
	addr := config.HostPort()
	deadline := time.Now().Add(timeout)
	interval := 100 * time.Millisecond

	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		dialTimeout := 1 * time.Second
		if remaining < dialTimeout {
			dialTimeout = remaining
		}
		conn, err := net.DialTimeout("tcp", addr, dialTimeout)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		remaining = time.Until(deadline)
		if remaining <= 0 {
			break
		}
		if interval > remaining {
			interval = remaining
		}
		time.Sleep(interval)
		// Exponential backoff capped at 500ms
		if interval < 500*time.Millisecond {
			interval = interval * 2
			if interval > 500*time.Millisecond {
				interval = 500 * time.Millisecond
			}
		}
	}

	return fmt.Errorf("Dolt server not ready at %s after %v", addr, timeout)
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

// getProcessDataDir extracts the --data-dir value from a dolt sql-server process's
// command line. Returns empty string if the process doesn't have --data-dir set
// (meaning it's using CWD-based launch) or if the command line can't be read.
func getProcessDataDir(pid int) string {
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	cmdline := strings.TrimSpace(string(output))

	// Look for --data-dir flag in command line
	parts := strings.Fields(cmdline)
	for i, part := range parts {
		if part == "--data-dir" && i+1 < len(parts) {
			return parts[i+1]
		}
		if strings.HasPrefix(part, "--data-dir=") {
			return strings.TrimPrefix(part, "--data-dir=")
		}
	}
	return ""
}

// VerifyServerDataDir checks whether the running Dolt server is serving the
// expected databases from the correct data directory. Returns true if the server
// is legitimate (serving databases from config.DataDir), false if it's an imposter
// (e.g., started from a different data directory with different/empty databases).
func VerifyServerDataDir(townRoot string) (bool, error) {
	config := DefaultConfig(townRoot)

	// First check: inspect the process's --data-dir flag
	running, pid, err := IsRunning(townRoot)
	if err != nil || !running {
		return false, fmt.Errorf("server not running")
	}

	processDataDir := getProcessDataDir(pid)
	if processDataDir != "" {
		// Normalize paths for comparison
		expectedDir, _ := filepath.Abs(config.DataDir)
		actualDir, _ := filepath.Abs(processDataDir)
		if expectedDir != actualDir {
			return false, fmt.Errorf("server data-dir mismatch: expected %s, got %s (PID %d)", expectedDir, actualDir, pid)
		}
		return true, nil
	}

	// No --data-dir flag means CWD-based launch — check served databases
	fsDatabases, fsErr := ListDatabases(townRoot)
	if fsErr != nil || len(fsDatabases) == 0 {
		// Can't verify if no databases expected
		return true, nil
	}

	served, _, verifyErr := VerifyDatabases(townRoot)
	if verifyErr != nil {
		return false, fmt.Errorf("could not query server databases: %w", verifyErr)
	}

	// If the server is serving none of our expected databases, it's an imposter
	servedSet := make(map[string]bool, len(served))
	for _, db := range served {
		servedSet[strings.ToLower(db)] = true
	}
	matchCount := 0
	for _, db := range fsDatabases {
		if servedSet[strings.ToLower(db)] {
			matchCount++
		}
	}
	if matchCount == 0 && len(fsDatabases) > 0 {
		return false, fmt.Errorf("server serves none of the expected %d databases — likely an imposter", len(fsDatabases))
	}

	return true, nil
}

// KillImposters finds and kills any dolt sql-server process on the configured
// port that is NOT serving from the expected data directory. This handles the
// case where another tool (e.g., bd) launched its own embedded Dolt server
// from a different directory, hijacking the port.
func KillImposters(townRoot string) error {
	config := DefaultConfig(townRoot)
	pid := findDoltServerOnPort(config.Port)
	if pid == 0 {
		return nil // No server on port
	}

	processDataDir := getProcessDataDir(pid)
	expectedDir, _ := filepath.Abs(config.DataDir)

	isImposter := false
	if processDataDir == "" {
		// No --data-dir flag — CWD-based launch, likely an imposter
		isImposter = true
	} else {
		actualDir, _ := filepath.Abs(processDataDir)
		if expectedDir != actualDir {
			isImposter = true
		}
	}

	if !isImposter {
		return nil
	}

	fmt.Fprintf(os.Stderr, "Killing imposter dolt sql-server (PID %d, data-dir: %q, expected: %s)\n",
		pid, processDataDir, expectedDir)

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("finding imposter process %d: %w", pid, err)
	}

	// SIGTERM first, then SIGKILL
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("sending SIGTERM to imposter PID %d: %w", pid, err)
	}

	// Wait for graceful shutdown
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		if err := process.Signal(syscall.Signal(0)); err != nil {
			// Clean up PID file if it pointed to the imposter
			_ = os.Remove(config.PidFile)
			return nil
		}
	}

	// Force kill
	_ = process.Signal(syscall.SIGKILL)
	time.Sleep(100 * time.Millisecond)
	_ = os.Remove(config.PidFile)

	return nil
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

	// Check if already running (checks both PID file AND port)
	running, pid, err := IsRunning(townRoot)
	if err != nil {
		return fmt.Errorf("checking server status: %w", err)
	}
	if running {
		// If data directory doesn't exist, this is an orphaned server (e.g., user
		// deleted ~/gt and re-ran gt install). Kill it so we can start fresh.
		if _, statErr := os.Stat(config.DataDir); os.IsNotExist(statErr) {
			fmt.Fprintf(os.Stderr, "Warning: Dolt server (PID %d) is running but data directory %s does not exist — stopping orphaned server\n", pid, config.DataDir)
			if stopErr := Stop(townRoot); stopErr != nil {
				if pid > 0 {
					if proc, findErr := os.FindProcess(pid); findErr == nil {
						_ = proc.Kill()
						time.Sleep(100 * time.Millisecond)
					}
				}
			}
			// Fall through to start a new server
		} else {
			// Server is running with valid data dir — check if it's an imposter
			// (e.g., bd launched its own dolt server from a different data directory).
			legitimate, verifyErr := VerifyServerDataDir(townRoot)
			if verifyErr == nil && !legitimate {
				fmt.Fprintf(os.Stderr, "Warning: running Dolt server (PID %d) is an imposter — killing and restarting\n", pid)
				if killErr := KillImposters(townRoot); killErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to kill imposter: %v\n", killErr)
				}
				// Wait for port to be released
				time.Sleep(500 * time.Millisecond)
				// Fall through to start a new server
			} else if verifyErr != nil && !legitimate {
				// Verification failed but server is suspicious — log and try to kill
				fmt.Fprintf(os.Stderr, "Warning: could not verify Dolt server identity: %v — killing and restarting\n", verifyErr)
				if killErr := KillImposters(townRoot); killErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to kill imposter: %v\n", killErr)
				}
				time.Sleep(500 * time.Millisecond)
			} else {
				// Server is legitimate — verify PID file is correct (gm-ouur fix)
				// If PID file is stale/missing but server is on port, update it
				pidFromFile := 0
				if data, err := os.ReadFile(config.PidFile); err == nil {
					pidFromFile, _ = strconv.Atoi(strings.TrimSpace(string(data)))
				}
				if pidFromFile != pid {
					// PID file is stale/wrong - update it
					fmt.Printf("Updating stale PID file (was %d, actual %d)\n", pidFromFile, pid)
					if err := os.WriteFile(config.PidFile, []byte(strconv.Itoa(pid)), 0644); err != nil {
						fmt.Fprintf(os.Stderr, "Warning: could not update PID file: %v\n", err)
					}
					// Update state too
					state, _ := LoadState(townRoot)
					if state != nil && state.PID != pid {
						state.PID = pid
						state.Running = true
						_ = SaveState(townRoot, state)
					}
				}
				return fmt.Errorf("Dolt server already running (PID %d)", pid)
			}
		}
	}

	// Ensure data directory exists
	if err := os.MkdirAll(config.DataDir, 0755); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	// Clean up stale Dolt LOCK files in all database directories
	databases, _ := ListDatabases(townRoot)
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

	// Wait for the server to be accepting connections, not just alive.
	// IsRunning only checks PID — we need CheckServerReachable to confirm
	// the port is listening. Retry with backoff since startup takes time.
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		time.Sleep(500 * time.Millisecond)

		running, _, err = IsRunning(townRoot)
		if err != nil {
			return fmt.Errorf("verifying server started: %w", err)
		}
		if !running {
			return fmt.Errorf("Dolt server failed to start (check logs with 'gt dolt logs')")
		}

		if err := CheckServerReachable(townRoot); err == nil {
			return nil // Server is up and accepting connections
		} else {
			lastErr = err
		}
	}

	return fmt.Errorf("Dolt server process started (PID %d) but not accepting connections after 5s: %w\nCheck logs with: gt dolt logs", cmd.Process.Pid, lastErr)
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
	return fmt.Sprintf("%s@tcp(%s)/", config.displayDSN(), config.HostPort())
}

// GetConnectionStringForRig returns the MySQL connection string for a specific rig database.
func GetConnectionStringForRig(townRoot, rigName string) string {
	config := DefaultConfig(townRoot)
	return fmt.Sprintf("%s@tcp(%s)/%s", config.displayDSN(), config.HostPort(), rigName)
}

// displayDSN returns the user[:password] portion for display, masking any password.
func (c *Config) displayDSN() string {
	if c.Password != "" {
		return c.User + ":****"
	}
	return c.User
}

// ListDatabases returns the list of available rig databases.
// For local servers, scans the data directory on disk.
// For remote servers, queries SHOW DATABASES via SQL.
func ListDatabases(townRoot string) ([]string, error) {
	config := DefaultConfig(townRoot)

	if config.IsRemote() {
		return listDatabasesRemote(config)
	}

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

// listDatabasesRemote queries SHOW DATABASES on a remote Dolt server.
func listDatabasesRemote(config *Config) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := buildDoltSQLCmd(ctx, config, "-r", "json", "-q", "SHOW DATABASES")

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("querying remote SHOW DATABASES: %w (stderr: %s)", err, strings.TrimSpace(stderrBuf.String()))
	}

	var result struct {
		Rows []struct {
			Database string `json:"Database"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parsing SHOW DATABASES JSON: %w", err)
	}

	var databases []string
	for _, row := range result.Rows {
		db := row.Database
		if db != "information_schema" && db != "mysql" {
			databases = append(databases, db)
		}
	}
	return databases, nil
}

// VerifyDatabases queries the running Dolt SQL server for SHOW DATABASES and
// compares the result against the filesystem-discovered databases from
// ListDatabases. Returns the list of databases the server is actually serving
// and any that exist on disk but are missing from the server.
//
// This catches the silent failure mode where Dolt skips databases with stale
// manifests after migration — the filesystem says they exist, but the server
// doesn't serve them.
func VerifyDatabases(townRoot string) (served, missing []string, err error) {
	return verifyDatabasesWithRetry(townRoot, 1)
}

// VerifyDatabasesWithRetry is like VerifyDatabases but retries the SHOW DATABASES
// query with exponential backoff to handle the case where the server has just started
// and is still loading databases.
func VerifyDatabasesWithRetry(townRoot string, maxAttempts int) (served, missing []string, err error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	return verifyDatabasesWithRetry(townRoot, maxAttempts)
}

func verifyDatabasesWithRetry(townRoot string, maxAttempts int) (served, missing []string, err error) {
	config := DefaultConfig(townRoot)

	// Retry with backoff since the server may still be loading databases
	// after a recent start (Start() only waits 500ms + process-alive check).
	// Both reachability and query are inside the loop so transient startup
	// failures are retried.
	const baseBackoff = 1 * time.Second
	const maxBackoff = 8 * time.Second
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Check if the server is reachable (TCP-level).
		if reachErr := CheckServerReachable(townRoot); reachErr != nil {
			lastErr = fmt.Errorf("server not reachable: %w", reachErr)
			if attempt < maxAttempts {
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

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		cmd := buildDoltSQLCmd(ctx, config,
			"-r", "json",
			"-q", "SHOW DATABASES",
		)

		// Capture stderr separately so it doesn't corrupt JSON parsing.
		// Dolt commonly writes deprecation/manifest warnings to stderr.
		// See also daemon/dolt.go:listDatabases() which uses cmd.Output()
		// for the same reason.
		var stderrBuf bytes.Buffer
		cmd.Stderr = &stderrBuf
		output, queryErr := cmd.Output()
		cancel()
		if queryErr != nil {
			stderrMsg := strings.TrimSpace(stderrBuf.String())
			errDetail := strings.TrimSpace(string(output))
			if stderrMsg != "" {
				errDetail = errDetail + " (stderr: " + stderrMsg + ")"
			}
			lastErr = fmt.Errorf("querying SHOW DATABASES: %w (output: %s)", queryErr, errDetail)
			if attempt < maxAttempts {
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

		var parseErr error
		served, parseErr = parseShowDatabases(output)
		if parseErr != nil {
			return nil, nil, fmt.Errorf("parsing SHOW DATABASES output: %w", parseErr)
		}

		// Compare against filesystem databases.
		fsDatabases, fsErr := ListDatabases(townRoot)
		if fsErr != nil {
			return served, nil, fmt.Errorf("listing filesystem databases: %w", fsErr)
		}

		missing = findMissingDatabases(served, fsDatabases)
		return served, missing, nil
	}
	return nil, nil, lastErr
}

// systemDatabases is the set of Dolt/MySQL internal databases that should be
// filtered from SHOW DATABASES results. These are not user rig databases:
//   - information_schema: MySQL standard metadata
//   - mysql: MySQL system database (privileges, users)
//   - dolt_cluster: Dolt clustering internal database (present when clustering is configured)
var systemDatabases = map[string]bool{
	"information_schema": true,
	"mysql":              true,
	"dolt_cluster":       true,
}

// IsSystemDatabase returns true if the given database name is a Dolt/MySQL
// internal database that should be excluded from user-facing database lists.
func IsSystemDatabase(name string) bool {
	return systemDatabases[strings.ToLower(name)]
}

// parseShowDatabases parses the output of SHOW DATABASES from dolt sql.
// It tries JSON parsing first, falling back to line-based parsing for
// plain-text output. Returns an error if the output format is unrecognized.
// Filters out system databases (information_schema, mysql, dolt_cluster).
func parseShowDatabases(output []byte) ([]string, error) {
	// Try JSON first. Use a raw map to detect schema presence.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(output, &raw); err != nil {
		// Check if the output looks like JSON that failed to parse —
		// don't fall through to line parsing with JSON-shaped text.
		trimmed := strings.TrimSpace(string(output))
		if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
			return nil, fmt.Errorf("output looks like JSON but failed to parse: %w", err)
		}

		// Fall back to line parsing for plain-text output.
		var databases []string
		for _, line := range strings.Split(string(output), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && line != "Database" && !strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "|") {
				if !IsSystemDatabase(line) {
					databases = append(databases, line)
				}
			}
		}
		if len(databases) == 0 && len(trimmed) > 0 {
			return nil, fmt.Errorf("fallback parser returned zero databases from non-empty output (%d bytes); format may be unrecognized", len(trimmed))
		}
		return databases, nil
	}

	// JSON parsed — require the expected "rows" key.
	rowsRaw, hasRows := raw["rows"]
	if !hasRows {
		return nil, fmt.Errorf("JSON output missing expected 'rows' key (keys: %v); Dolt output schema may have changed", jsonKeys(raw))
	}

	var rows []struct {
		Database string `json:"Database"`
	}
	if err := json.Unmarshal(rowsRaw, &rows); err != nil {
		return nil, fmt.Errorf("JSON 'rows' field has unexpected type: %w", err)
	}

	var databases []string
	for _, row := range rows {
		if row.Database != "" && !IsSystemDatabase(row.Database) {
			databases = append(databases, row.Database)
		}
	}
	return databases, nil
}

// findMissingDatabases returns filesystem databases not present in the served list.
// Comparison is case-insensitive since Dolt database names are case-insensitive
// in SQL but case-preserving on the filesystem.
func findMissingDatabases(served, fsDatabases []string) []string {
	servedSet := make(map[string]bool, len(served))
	for _, db := range served {
		servedSet[strings.ToLower(db)] = true
	}
	var missing []string
	for _, db := range fsDatabases {
		if !servedSet[strings.ToLower(db)] {
			missing = append(missing, db)
		}
	}
	return missing
}

// jsonKeys returns the top-level keys from a JSON object map, for diagnostics.
func jsonKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// InitRig initializes a new rig database in the data directory.
// If the Dolt server is running, it executes CREATE DATABASE to register the
// database with the live server (avoiding the need for a restart).
// Returns (serverWasRunning, created, err). created is false when the database
// already existed on disk (idempotent no-op).
func InitRig(townRoot, rigName string) (serverWasRunning bool, created bool, err error) {
	if rigName == "" {
		return false, false, fmt.Errorf("rig name cannot be empty")
	}

	config := DefaultConfig(townRoot)

	// Validate rig name (simple alphanumeric + underscore/dash)
	for _, r := range rigName {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false, false, fmt.Errorf("invalid rig name %q: must contain only alphanumeric, underscore, or dash", rigName)
		}
	}

	rigDir := filepath.Join(config.DataDir, rigName)

	// Check if already exists on disk — idempotent for callers like gt install.
	// Still run EnsureMetadata to repair missing/corrupt metadata.json.
	if _, err := os.Stat(filepath.Join(rigDir, ".dolt")); err == nil {
		running, _, _ := IsRunning(townRoot)
		if err := EnsureMetadata(townRoot, rigName); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: metadata.json update failed for existing database %q: %v\n", rigName, err)
		}
		return running, false, nil
	}

	// Check if server is running
	running, runningPID, _ := IsRunning(townRoot)

	if running {
		// If the data directory doesn't exist, the server is orphaned (e.g., user
		// deleted ~/gt and re-ran gt install while an old server was still running).
		// Stop the orphaned server and fall through to the offline init path.
		if _, err := os.Stat(config.DataDir); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Warning: Dolt server (PID %d) is running but data directory %s does not exist — stopping orphaned server\n", runningPID, config.DataDir)
			if stopErr := Stop(townRoot); stopErr != nil {
				// Force-kill if graceful stop fails (no PID file for orphaned server)
				if runningPID > 0 {
					if proc, err := os.FindProcess(runningPID); err == nil {
						_ = proc.Kill()
					}
				}
			}
			running = false
		}
	}

	if running {
		// Server is running: use CREATE DATABASE which both creates the
		// directory and registers the database with the live server.
		if err := serverExecSQL(townRoot, fmt.Sprintf("CREATE DATABASE `%s`", rigName)); err != nil {
			return true, false, fmt.Errorf("creating database on running server: %w", err)
		}
		// Wait for the new database to appear in the server's in-memory catalog.
		// CREATE DATABASE returns before the catalog is fully updated, so
		// subsequent USE/query operations can fail with "Unknown database".
		// Non-fatal: the database was created, so we log a warning and continue
		// to EnsureMetadata. The retry wrappers (doltSQLWithRetry) will handle
		// any residual catalog propagation delays in subsequent operations.
		if err := waitForCatalog(townRoot, rigName); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: catalog visibility wait timed out (will retry on use): %v\n", err)
		}
	} else {
		// Server not running: create directory and init manually.
		// The database will be picked up when the server starts.
		if err := os.MkdirAll(rigDir, 0755); err != nil {
			return false, false, fmt.Errorf("creating rig directory: %w", err)
		}

		cmd := exec.Command("dolt", "init")
		cmd.Dir = rigDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			return false, false, fmt.Errorf("initializing Dolt database: %w\n%s", err, output)
		}
	}

	// Update metadata.json to point to the server
	if err := EnsureMetadata(townRoot, rigName); err != nil {
		// Non-fatal: init succeeded, metadata update failed
		fmt.Fprintf(os.Stderr, "Warning: database initialized but metadata.json update failed: %v\n", err)
	}

	return running, true, nil
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

// OrphanedDatabase represents a database in .dolt-data/ that is not referenced
// by any rig's metadata.json. These are leftover from partial setups, renames,
// or failed migrations.
type OrphanedDatabase struct {
	// Name is the database directory name in .dolt-data/.
	Name string

	// Path is the full path to the database directory.
	Path string

	// SizeBytes is the total size of the database directory.
	SizeBytes int64
}

// FindOrphanedDatabases scans .dolt-data/ for databases that are not referenced
// by any rig's metadata.json dolt_database field. These orphans consume disk space
// and are served by the Dolt server unnecessarily.
func FindOrphanedDatabases(townRoot string) ([]OrphanedDatabase, error) {
	databases, err := ListDatabases(townRoot)
	if err != nil {
		return nil, fmt.Errorf("listing databases: %w", err)
	}
	if len(databases) == 0 {
		return nil, nil
	}

	// Collect all referenced database names from metadata.json files
	referenced := collectReferencedDatabases(townRoot)

	// Find databases that exist on disk but aren't referenced
	config := DefaultConfig(townRoot)
	var orphans []OrphanedDatabase
	for _, dbName := range databases {
		if referenced[dbName] {
			continue
		}
		dbPath := filepath.Join(config.DataDir, dbName)
		size := dirSize(dbPath)
		orphans = append(orphans, OrphanedDatabase{
			Name:      dbName,
			Path:      dbPath,
			SizeBytes: size,
		})
	}

	return orphans, nil
}

// readExistingDoltDatabase reads the dolt_database field from an existing metadata.json.
// Returns empty string if the file doesn't exist or can't be read.
func readExistingDoltDatabase(beadsDir string) string {
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return ""
	}
	var meta map[string]interface{}
	if err := json.Unmarshal(data, &meta); err != nil {
		return ""
	}
	if db, ok := meta["dolt_database"].(string); ok {
		return db
	}
	return ""
}

// collectReferencedDatabases returns a set of database names referenced by
// any rig's metadata.json dolt_database field.
func collectReferencedDatabases(townRoot string) map[string]bool {
	referenced := make(map[string]bool)

	// Check town-level beads (hq)
	townBeadsDir := filepath.Join(townRoot, ".beads")
	if db := readExistingDoltDatabase(townBeadsDir); db != "" {
		referenced[db] = true
	}

	// Check all rigs from rigs.json
	rigsPath := filepath.Join(townRoot, "mayor", "rigs.json")
	data, err := os.ReadFile(rigsPath)
	if err != nil {
		return referenced
	}
	var config struct {
		Rigs map[string]interface{} `json:"rigs"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return referenced
	}

	for rigName := range config.Rigs {
		beadsDir := FindRigBeadsDir(townRoot, rigName)
		if beadsDir == "" {
			continue
		}
		if db := readExistingDoltDatabase(beadsDir); db != "" {
			referenced[db] = true
		}
	}

	return referenced
}

// RemoveDatabase removes an orphaned database directory from .dolt-data/.
// The caller should verify the database is actually orphaned before calling this.
// If the Dolt server is running, it will DROP the database first.
func RemoveDatabase(townRoot, dbName string) error {
	config := DefaultConfig(townRoot)
	dbPath := filepath.Join(config.DataDir, dbName)

	// Verify the directory exists
	if _, err := os.Stat(filepath.Join(dbPath, ".dolt")); err != nil {
		return fmt.Errorf("database %q not found at %s", dbName, dbPath)
	}

	// If server is running, DROP the database first and clean up branch control entries.
	// In Dolt 1.81.x, DROP DATABASE does not automatically remove dolt_branch_control
	// entries for the dropped database. These stale entries cause the database directory
	// to be recreated when connections reference the database name (gt-zlv7l).
	running, _, _ := IsRunning(townRoot)
	if running {
		// Try to DROP — ignore errors (database might not be loaded)
		_ = serverExecSQL(townRoot, fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dbName))
		// Explicitly clean up branch control entries to prevent the database from being
		// recreated on subsequent connections. `database` is a reserved word, so backtick-quote it.
		_ = serverExecSQL(townRoot, fmt.Sprintf("DELETE FROM dolt_branch_control WHERE `database` = '%s'", dbName))
	}

	// Remove the directory
	if err := os.RemoveAll(dbPath); err != nil {
		return fmt.Errorf("removing database directory: %w", err)
	}

	return nil
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
	_, created, err := InitRig(townRoot, ws.ConfiguredDB)
	if err != nil {
		return "", fmt.Errorf("creating database for %s: %w", ws.RigName, err)
	}
	if !created {
		return "database already exists (no-op)", nil
	}
	return "created new database", nil
}

// EnsureMetadata writes or updates the metadata.json for a rig's beads directory
// to include proper Dolt server configuration. This prevents the split-brain problem
// where bd falls back to local embedded databases instead of connecting to the
// centralized Dolt server.
//
// For the "hq" rig, it writes to <townRoot>/.beads/metadata.json.
// For other rigs, it writes to mayor/rig/.beads/metadata.json if that path exists,
// otherwise to <townRoot>/<rigName>/.beads/metadata.json.
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

	// Patch dolt server fields. Only set fields that are gastown's responsibility
	// (ensuring server mode). dolt_database is owned by bd init — only set it as
	// a fallback when bd init hasn't run yet (no existing value).
	existing["database"] = "dolt"
	existing["backend"] = "dolt"
	existing["dolt_mode"] = "server"
	if existing["dolt_database"] == nil || existing["dolt_database"] == "" {
		existing["dolt_database"] = rigName
	}

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
	if townRoot == "" || rigName == "" {
		return ""
	}
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

	// Neither exists; return rig-root path (consistent with FindOrCreateRigBeadsDir)
	return rigBeads
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
	if townRoot == "" {
		return "", fmt.Errorf("townRoot cannot be empty")
	}
	if rigName == "" {
		return "", fmt.Errorf("rigName cannot be empty")
	}
	if rigName == "hq" {
		dir := filepath.Join(townRoot, ".beads")
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("creating HQ beads dir: %w", err)
		}
		return dir, nil
	}

	// Check mayor/rig/.beads first (canonical location).
	// Use MkdirAll as an idempotent existence check+create to close the
	// TOCTOU window between os.Stat and the caller's file operations.
	mayorBeads := filepath.Join(townRoot, rigName, "mayor", "rig", ".beads")
	if _, err := os.Stat(mayorBeads); err == nil {
		// Ensure it still exists (no-op if present, recreates if deleted)
		if err := os.MkdirAll(mayorBeads, 0755); err != nil {
			return "", fmt.Errorf("ensuring mayor beads dir: %w", err)
		}
		return mayorBeads, nil
	}

	// Check rig-root .beads
	rigBeads := filepath.Join(townRoot, rigName, ".beads")
	if _, err := os.Stat(rigBeads); err == nil {
		if err := os.MkdirAll(rigBeads, 0755); err != nil {
			return "", fmt.Errorf("ensuring rig beads dir: %w", err)
		}
		return rigBeads, nil
	}

	// Neither exists — create rig-root .beads (NOT mayor path).
	// The mayor/rig/.beads path should only be used when the source repo
	// has tracked beads (checked out via git clone). Creating it here would
	// cause InitBeads to misdetect an untracked repo as having tracked beads,
	// taking the redirect early-return and skipping config.yaml/issues.jsonl
	// creation (see rig/manager.go InitBeads).
	if err := os.MkdirAll(rigBeads, 0755); err != nil {
		return "", fmt.Errorf("creating beads dir: %w", err)
	}

	return rigBeads, nil
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

	// Always connect as a TCP client to the running server, even for local servers.
	// Without explicit --host/--port, dolt sql runs in embedded mode which loads all
	// databases into memory — causing OOM kills on large data dirs.
	// Note: --host, --port, --user, --no-tls are dolt GLOBAL args and must come
	// BEFORE the "sql" subcommand.
	fullArgs := []string{
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(config.Port),
		"--user", config.User,
		"--no-tls",
		"sql",
		"-r", "csv",
		"-q", "SELECT COUNT(*) AS cnt FROM information_schema.PROCESSLIST",
	}
	cmd := exec.CommandContext(ctx, "dolt", fullArgs...)
	// Always set DOLT_CLI_PASSWORD to prevent interactive password prompt.
	// When empty, dolt connects without a password (which is the default for local servers).
	cmd.Env = append(os.Environ(), "DOLT_CLI_PASSWORD="+config.Password)
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
	cmd := buildDoltSQLCmd(ctx, config, "-q", query)

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
		style.PrintWarning("stop returned error (proceeding with restart): %v", err)
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
	ctx := context.Background()
	cmd := buildDoltSQLCmd(ctx, config, "-q", "SELECT 1")
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

	cmd := buildDoltSQLCmd(ctx, config, "-q", query)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w (output: %s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// waitForCatalog polls the Dolt server until the named database is visible in the
// in-memory catalog. This bridges the race between CREATE DATABASE returning and the
// catalog being updated — without this, immediate USE/query operations can fail with
// "Unknown database". Uses exponential backoff: 100ms, 200ms, 400ms, 800ms, 1.6s.
// Only retries on catalog-race errors ("Unknown database"); returns immediately for
// other failures (e.g., server crash, binary missing).
func waitForCatalog(townRoot, dbName string) error {
	const maxAttempts = 5
	const baseBackoff = 100 * time.Millisecond
	const maxBackoff = 2 * time.Second

	query := fmt.Sprintf("USE %s", dbName)
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := serverExecSQL(townRoot, query); err != nil {
			lastErr = err
			// Only retry catalog-race errors; fail fast on other errors
			// (connection refused, binary missing, etc.)
			if !strings.Contains(err.Error(), "Unknown database") {
				return fmt.Errorf("database %q probe failed (non-retryable): %w", dbName, err)
			}
			if attempt < maxAttempts {
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
	return fmt.Errorf("database %q not visible after %d attempts: %w", dbName, maxAttempts, lastErr)
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
	cmd := buildDoltSQLCmd(ctx, config, "-q", fullQuery)
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
// Covers manifest lock contention, read-only mode, optimistic lock failures, timeouts,
// and catalog propagation delays after CREATE DATABASE.
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
		strings.Contains(msg, "try restarting transaction") ||
		strings.Contains(msg, "Unknown database")
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

	cmd := buildDoltSQLCmd(ctx, config, "--file", tmpFile.Name())
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

