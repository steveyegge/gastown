// Package testutil provides shared test infrastructure for integration tests.
package testutil

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// doltTestPort is the port for the shared test Dolt server. Set dynamically
// by startDoltServer: either from GT_DOLT_PORT (external/pre-started server)
// or via FindFreePort() to avoid colliding with production on 3307.
var doltTestPort string

// doltServer tracks the singleton dolt sql-server process for integration tests.
// Started once per test binary invocation via sync.Once; cleaned up at process exit.
var (
	doltServerOnce sync.Once
	doltServerErr  error
	// doltLockFile is held with LOCK_SH for the lifetime of the test binary.
	// This prevents other test processes from killing the server while we're
	// still using it. See CleanupDoltServer for the shutdown protocol.
	doltLockFile *os.File
	// doltWeStarted tracks whether this process started the server (vs reusing).
	doltWeStarted bool
	// doltPortSetByUs tracks whether we set GT_DOLT_PORT (vs it being set externally).
	doltPortSetByUs bool
)

// RequireDoltServer ensures a dolt sql-server is running on a dynamically
// chosen port for integration tests. The server is shared across all tests
// in the same test binary invocation.
//
// Port selection:
//   - If GT_DOLT_PORT is set externally, that port is used (allows reusing
//     a pre-started server).
//   - Otherwise, FindFreePort() picks an ephemeral port and sets GT_DOLT_PORT
//     so the gt/bd stack (via doltserver.DefaultConfig) connects to it.
//
// Port contention strategy:
//
//  1. In-process: sync.Once ensures only one goroutine attempts startup.
//
//  2. Cross-process: a file lock (/tmp/dolt-test-server-<port>.lock) serializes
//     startup across concurrent test binaries using the same port. The first
//     process to acquire LOCK_EX starts the server and writes its PID + data
//     dir to /tmp/dolt-test-server-<port>.pid. After startup, the lock is
//     downgraded to LOCK_SH (shared) and held for the lifetime of the test binary.
//
//  3. Safe shutdown: CleanupDoltServer tries to upgrade from LOCK_SH to
//     LOCK_EX (non-blocking). If it succeeds, no other test processes hold
//     the shared lock, so it's safe to kill the server.
//
//  4. External server: if the port is already listening before any test
//     process acquires the lock, we reuse it. No PID file is written, and
//     cleanup never kills an external server.
func RequireDoltServer(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("dolt"); err != nil {
		t.Skip("dolt not installed, skipping test")
	}

	doltServerOnce.Do(func() {
		doltServerErr = startDoltServer()
	})

	if doltServerErr != nil {
		t.Fatalf("dolt server setup failed: %v", doltServerErr)
	}
}

// DoltTestAddr returns the address (host:port) of the test Dolt server.
func DoltTestAddr() string {
	return "127.0.0.1:" + doltTestPort
}

// DoltTestPort returns the port of the test Dolt server.
func DoltTestPort() string {
	return doltTestPort
}

// LockFilePathForPort returns the lock file path for a given port.
// Port-specific paths prevent contention between test binaries using different ports.
func LockFilePathForPort(port string) string {
	return fmt.Sprintf("/tmp/dolt-test-server-%s.lock", port)
}

// PidFilePathForPort returns the PID file path for a given port.
func PidFilePathForPort(port string) string {
	return fmt.Sprintf("/tmp/dolt-test-server-%s.pid", port)
}

// FindFreePort binds to port 0 to let the OS assign an ephemeral port,
// then closes the listener and returns the port number.
func FindFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("finding free port: %w", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

func startDoltServer() error {
	// Determine port: use GT_DOLT_PORT if set externally, otherwise find a free one.
	if p := os.Getenv("GT_DOLT_PORT"); p != "" {
		doltTestPort = p
	} else {
		port, err := FindFreePort()
		if err != nil {
			return err
		}
		doltTestPort = strconv.Itoa(port)
		os.Setenv("GT_DOLT_PORT", doltTestPort)
		doltPortSetByUs = true
	}

	lockPath := LockFilePathForPort(doltTestPort)
	pidPath := PidFilePathForPort(doltTestPort)

	// Open the lock file (kept open for the lifetime of the test binary).
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return fmt.Errorf("opening lock file %s: %w", lockPath, err)
	}

	// Acquire exclusive lock for the startup phase.
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		lockFile.Close()
		return fmt.Errorf("acquiring startup lock: %w", err)
	}

	// Under the exclusive lock: check if a server is already running
	// (started by another process that held the lock before us, or external).
	if portReady(2 * time.Second) {
		// Downgrade to shared lock — signals "I'm using the server".
		if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_SH); err != nil {
			lockFile.Close()
			return fmt.Errorf("downgrading to shared lock: %w", err)
		}
		doltLockFile = lockFile
		return nil
	}

	// No server running — start one.
	dataDir, err := os.MkdirTemp("", "dolt-test-server-*")
	if err != nil {
		syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		lockFile.Close()
		return fmt.Errorf("creating dolt data dir: %w", err)
	}

	cmd := exec.Command("dolt", "sql-server",
		"--port", doltTestPort,
		"--data-dir", dataDir,
	)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		os.RemoveAll(dataDir)
		syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		lockFile.Close()
		return fmt.Errorf("starting dolt sql-server: %w", err)
	}

	// Write PID file so any last-exiting process can clean up.
	// Format: "PID\nDATA_DIR\n"
	pidContent := fmt.Sprintf("%d\n%s\n", cmd.Process.Pid, dataDir)
	if err := os.WriteFile(pidPath, []byte(pidContent), 0666); err != nil {
		cmd.Process.Kill()
		os.RemoveAll(dataDir)
		syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		lockFile.Close()
		return fmt.Errorf("writing PID file: %w", err)
	}

	// Reap the process in the background so ProcessState is populated on exit.
	exited := make(chan struct{})
	go func() {
		cmd.Wait()
		close(exited)
	}()

	// Wait for server to accept connections (up to 30 seconds).
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if portReady(time.Second) {
			// Server is ready. Downgrade to shared lock.
			if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_SH); err != nil {
				lockFile.Close()
				return fmt.Errorf("downgrading to shared lock: %w", err)
			}
			doltLockFile = lockFile
			doltWeStarted = true
			return nil
		}
		// Check if process exited (port bind failure, etc).
		select {
		case <-exited:
			os.RemoveAll(dataDir)
			os.Remove(pidPath)
			syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
			lockFile.Close()
			return fmt.Errorf("dolt sql-server exited prematurely")
		default:
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Timed out — kill and clean up.
	cmd.Process.Kill()
	<-exited
	os.RemoveAll(dataDir)
	os.Remove(pidPath)
	syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
	lockFile.Close()
	return fmt.Errorf("dolt sql-server did not become ready within 30s")
}

// portReady returns true if the dolt test port is accepting TCP connections.
func portReady(timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", DoltTestAddr(), timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// CleanupDoltServer conditionally kills the test dolt server. Called from TestMain.
//
// Shutdown protocol: try to upgrade from LOCK_SH to LOCK_EX (non-blocking).
//   - If we get LOCK_EX: no other test processes hold the shared lock, so we're
//     the last user. Read the PID file to find and kill the server.
//   - If LOCK_EX fails (EWOULDBLOCK): another process still holds LOCK_SH,
//     meaning it's actively using the server. Skip cleanup — the last process
//     to exit will handle it.
//
// The PID file enables any last-exiting process to clean up, not just the
// process that originally started the server. This prevents leaked servers
// when the starter exits before other consumers.
func CleanupDoltServer() {
	// Release our shared lock regardless.
	defer func() {
		if doltLockFile != nil {
			syscall.Flock(int(doltLockFile.Fd()), syscall.LOCK_UN)
			doltLockFile.Close()
			doltLockFile = nil
		}
		// Clear GT_DOLT_PORT if we set it, so subsequent processes
		// don't inherit a stale port.
		if doltPortSetByUs {
			os.Unsetenv("GT_DOLT_PORT")
		}
	}()

	if doltLockFile == nil || doltTestPort == "" {
		return
	}

	pidPath := PidFilePathForPort(doltTestPort)

	// Try to acquire exclusive lock (non-blocking). If another process
	// holds LOCK_SH, this fails with EWOULDBLOCK — the server is still in use.
	err := syscall.Flock(int(doltLockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		// Another process is using the server. Don't kill it.
		return
	}
	// We got LOCK_EX — we're the last process. Kill from PID file.

	data, err := os.ReadFile(pidPath)
	if err != nil {
		// No PID file — either external server or already cleaned up.
		return
	}

	lines := strings.SplitN(string(data), "\n", 3)
	if len(lines) < 2 {
		return
	}

	pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil || pid <= 0 {
		return
	}
	dataDir := strings.TrimSpace(lines[1])

	// Kill the server process.
	proc, err := os.FindProcess(pid)
	if err == nil {
		proc.Kill()
		proc.Wait()
	}

	// Clean up data dir, PID file, and lock file.
	if dataDir != "" {
		os.RemoveAll(dataDir)
	}
	os.Remove(pidPath)
	os.Remove(LockFilePathForPort(doltTestPort))
}
