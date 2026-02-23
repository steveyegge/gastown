package beads

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// CleanStaleDoltServerPID removes the dolt-server.pid file inside a beads
// directory if the referenced process is no longer alive. A stale PID file
// causes bd to connect to port 3307 (configured in the co-located config.yaml),
// which may be occupied by a different Dolt server serving different databases.
// The resulting connection hangs until the bd read timeout (30s) kills it.
//
// This is a defensive measure — the Dolt server writes dolt-server.pid on
// startup but does not always clean it up on crash or unclean shutdown.
func CleanStaleDoltServerPID(beadsDir string) {
	pidPath := filepath.Join(beadsDir, "dolt", "dolt-server.pid")
	data, err := os.ReadFile(pidPath) //nolint:gosec // G304: path is constructed internally
	if err != nil {
		return // No PID file, nothing to clean
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		// Corrupt PID file — remove it
		_ = os.Remove(pidPath)
		return
	}

	// Check if the process is alive using signal 0 (no-op probe)
	proc, err := os.FindProcess(pid)
	if err != nil {
		_ = os.Remove(pidPath)
		return
	}

	if err := proc.Signal(syscall.Signal(0)); err != nil {
		// Process is dead — remove stale PID file
		_ = os.Remove(pidPath)
		fmt.Fprintf(os.Stderr, "Cleaned stale dolt-server.pid (PID %d) from %s\n", pid, beadsDir)
	}
}
