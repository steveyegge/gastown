//go:build windows

package testutil

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

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
		os.Setenv("GT_DOLT_PORT", doltTestPort) //nolint:tenv // intentional process-wide env
		doltPortSetByUs = true
	}

	pidPath := PidFilePathForPort(doltTestPort)

	// On Windows, skip file locking (syscall.Flock is not available).
	// Check if a server is already running on the port.
	if portReady(2 * time.Second) {
		return nil
	}

	// No server running — start one.
	dataDir, err := os.MkdirTemp("", "dolt-test-server-*")
	if err != nil {
		return fmt.Errorf("creating dolt data dir: %w", err)
	}

	cmd := exec.Command("dolt", "sql-server",
		"--port", doltTestPort,
		"--data-dir", dataDir,
	)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(dataDir)
		return fmt.Errorf("starting dolt sql-server: %w", err)
	}

	// Write PID file so cleanup can find the server.
	pidContent := fmt.Sprintf("%d\n%s\n", cmd.Process.Pid, dataDir)
	if err := os.WriteFile(pidPath, []byte(pidContent), 0666); err != nil { //nolint:gosec // test infrastructure
		_ = cmd.Process.Kill()
		_ = os.RemoveAll(dataDir)
		return fmt.Errorf("writing PID file: %w", err)
	}

	// Reap the process in the background.
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()

	// Wait for server to accept connections (up to 30 seconds).
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if portReady(time.Second) {
			doltWeStarted = true
			return nil
		}
		select {
		case <-exited:
			_ = os.RemoveAll(dataDir)
			_ = os.Remove(pidPath)
			return fmt.Errorf("dolt sql-server exited prematurely")
		default:
		}
		time.Sleep(500 * time.Millisecond)
	}

	_ = cmd.Process.Kill()
	<-exited
	_ = os.RemoveAll(dataDir)
	_ = os.Remove(pidPath)
	return fmt.Errorf("dolt sql-server did not become ready within 30s")
}

// CleanupDoltServer kills the test dolt server on Windows. Called from TestMain.
// On Windows, file locking is not used, so cleanup simply reads the PID file
// and kills the server process.
func CleanupDoltServer() {
	defer func() {
		if doltPortSetByUs {
			_ = os.Unsetenv("GT_DOLT_PORT")
		}
	}()

	if doltTestPort == "" {
		return
	}

	pidPath := PidFilePathForPort(doltTestPort)
	data, err := os.ReadFile(pidPath)
	if err != nil {
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

	proc, err := os.FindProcess(pid)
	if err == nil {
		_ = proc.Kill()
		_, _ = proc.Wait()
	}

	if dataDir != "" {
		_ = os.RemoveAll(dataDir)
	}
	_ = os.Remove(pidPath)
}
