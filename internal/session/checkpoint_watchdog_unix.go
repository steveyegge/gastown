//go:build !windows

package session

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ActivateCheckpointWatchdog spawns a detached `gt checkpoint watchdog` process
// that periodically auto-commits uncommitted work as WIP checkpoints.
//
// The process is started with Setsid so it survives the parent's exit.
// A PID file at /tmp/gt-checkpoint-<session>.pid ensures only one watchdog
// runs per session: any previous watchdog is killed before spawning a new one.
//
// interval is the checkpoint interval (e.g., "10m"). Pass "" for the default (10m).
func ActivateCheckpointWatchdog(sessionID, workDir, interval string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable: %w", err)
	}

	pidFile := checkpointWatchdogPIDFile(sessionID)

	// Kill any previous watchdog for this session.
	killPreviousCheckpointWatchdog(pidFile)

	args := []string{"checkpoint", "watchdog",
		"--work-dir", workDir,
		"--session", sessionID,
	}
	if interval != "" {
		args = append(args, "--interval", interval)
	}

	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Env = os.Environ()
	// Suppress stdio — this is a background daemon process.
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting checkpoint watchdog: %w", err)
	}

	// Write PID for later cleanup.
	pidStr := strconv.Itoa(cmd.Process.Pid)
	_ = os.WriteFile(pidFile, []byte(pidStr), 0600)

	return nil
}

// DeactivateCheckpointWatchdog kills the detached checkpoint watchdog for sessionID,
// if one is running. Safe to call even when no watchdog is running (no-op).
func DeactivateCheckpointWatchdog(sessionID string) {
	killPreviousCheckpointWatchdog(checkpointWatchdogPIDFile(sessionID))
}

// checkpointWatchdogPIDFile returns the PID file path for a session's checkpoint watchdog.
func checkpointWatchdogPIDFile(sessionID string) string {
	safe := strings.ReplaceAll(sessionID, "/", "-")
	return "/tmp/gt-checkpoint-" + safe + ".pid"
}

// killPreviousCheckpointWatchdog kills any previously running checkpoint watchdog
// by reading and signaling the stored PID file.
func killPreviousCheckpointWatchdog(pidFile string) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = proc.Signal(syscall.SIGTERM)
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = os.Remove(pidFile)
}
