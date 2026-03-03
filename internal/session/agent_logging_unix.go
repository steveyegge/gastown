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

// ActivateAgentLogging spawns a detached `gt agent-log` process to stream the
// session's Claude Code JSONL conversation log to VictoriaLogs.
//
// The process is started with Setsid so it survives the parent's exit.
// A PID file at /tmp/gt-agentlog-<session>.pid ensures only one watcher
// runs per session: any previous watcher is killed before spawning a new one.
//
// --since is set to ~60s before now so only JSONL files from this GT session's
// Claude instance are watched, excluding pre-existing user sessions or other
// Gas Town rigs running in the same work directory.
//
// runID is the GASTA run identifier (GT_RUN) generated at session spawn time.
// It is passed to the agent-log subprocess so every agent.event it emits
// carries the same run.id for waterfall correlation. Pass "" to omit.
//
// Opt-in: caller must check GT_LOG_AGENT_OUTPUT=true before calling.
func ActivateAgentLogging(sessionID, workDir, runID string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable: %w", err)
	}

	pidFile := agentLogPIDFile(sessionID)

	// Kill any previous watcher for this session (e.g. on daemon restart).
	killPreviousAgentLogger(pidFile)

	logsURL := os.Getenv("GT_OTEL_LOGS_URL")
	metricsURL := os.Getenv("GT_OTEL_METRICS_URL")

	// --since: exclude JSONL files that predate this session start.
	// We use now-60s to give a buffer for Claude's startup time while still
	// filtering out older sessions from unrelated Claude instances.
	since := time.Now().Add(-60 * time.Second).UTC().Format(time.RFC3339)

	args := []string{"agent-log",
		"--session", sessionID,
		"--work-dir", workDir,
		"--since", since,
	}
	if runID != "" {
		args = append(args, "--run-id", runID)
	}
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	env := append(os.Environ(),
		"GT_OTEL_LOGS_URL="+logsURL,
		"GT_OTEL_METRICS_URL="+metricsURL,
	)
	if runID != "" {
		env = append(env, "GT_RUN="+runID)
	}
	cmd.Env = env
	// Suppress stdio — this is a background daemon process.
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting agent-log process: %w", err)
	}

	// Write PID for later cleanup.
	pidStr := strconv.Itoa(cmd.Process.Pid)
	_ = os.WriteFile(pidFile, []byte(pidStr), 0600)

	return nil
}

// DeactivateAgentLogging kills the detached agent-log watcher for sessionID,
// if one is running. It is the counterpart to ActivateAgentLogging and must be
// called from every session teardown path to avoid orphan processes.
// Safe to call even when no watcher is running (no-op in that case).
func DeactivateAgentLogging(sessionID string) {
	killPreviousAgentLogger(agentLogPIDFile(sessionID))
}

// agentLogPIDFile returns the PID file path for a session's agent-log watcher.
func agentLogPIDFile(sessionID string) string {
	// Sanitize sessionID for use in a filename (replace / with -).
	safe := strings.ReplaceAll(sessionID, "/", "-")
	return "/tmp/gt-agentlog-" + safe + ".pid"
}

// killPreviousAgentLogger kills any previously running agent-log watcher for
// the session by reading and signaling the stored PID file.
func killPreviousAgentLogger(pidFile string) {
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
	// Wait briefly for the process to exit to avoid overlapping watchers
	// emitting duplicate events. Signal(0) returns an error once the process
	// has exited (ESRCH on Linux/macOS).
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			break // process has exited
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = os.Remove(pidFile)
}
