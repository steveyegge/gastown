// Package testutil provides shared test infrastructure for integration tests.
package testutil

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"sync"
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
	doltWeStarted bool //nolint:unused // reserved for future cleanup logic
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
	_ = l.Close()
	return port, nil
}

// portReady returns true if the dolt test port is accepting TCP connections.
func portReady(timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", DoltTestAddr(), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
