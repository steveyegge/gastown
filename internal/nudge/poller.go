// poller.go provides a background nudge-queue poller for agents that lack
// turn-boundary drain hooks (e.g., Gemini, Codex). Claude Code drains its
// queue via the UserPromptSubmit hook on every turn. Other runtimes have no
// equivalent hook, so queued nudges would sit undelivered forever.
//
// The poller runs as a background goroutine launched by crew/manager.Start().
// It polls the queue every PollInterval, waits for the agent to be idle, then
// drains and injects the formatted nudges via tmux NudgeSession.
//
// Lifecycle: StartPoller() → background loop → StopPoller() (or session death).
// A PID file at <townRoot>/.runtime/nudge_poller/<session>.pid allows Stop()
// to clean up even if the original manager has been replaced.
package nudge

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/steveyegge/gastown/internal/constants"
)

// Poller tuning defaults (overridable via flags or tests).
var (
	// DefaultPollInterval is how often the poller checks the queue.
	DefaultPollInterval = "10s"
	// DefaultIdleTimeout is how long to wait for the agent to become idle
	// before skipping this poll cycle and trying again next interval.
	DefaultIdleTimeout = "3s"
)

// pollerPidDir returns the directory for poller PID files.
func pollerPidDir(townRoot string) string {
	return filepath.Join(townRoot, constants.DirRuntime, "nudge_poller")
}

// pollerPidFile returns the PID file path for a session's poller.
func pollerPidFile(townRoot, session string) string {
	safe := strings.ReplaceAll(session, "/", "_")
	return filepath.Join(pollerPidDir(townRoot), safe+".pid")
}

// StartPoller launches a background `gt nudge-poller <session>` process.
// The process is detached (Setpgid) so it survives the caller's exit.
// Returns the PID of the launched process, or an error.
func StartPoller(townRoot, session string) (int, error) {
	pidDir := pollerPidDir(townRoot)
	if err := os.MkdirAll(pidDir, 0755); err != nil {
		return 0, fmt.Errorf("creating poller pid dir: %w", err)
	}

	// Check if a poller is already running for this session.
	if pid, alive := pollerAlive(townRoot, session); alive {
		return pid, nil // already running
	}

	// Find the gt binary.
	gtBin, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("finding gt binary: %w", err)
	}

	cmd := exec.Command(gtBin, "nudge-poller", session)
	cmd.Dir = townRoot
	cmd.Stdout = nil // discard
	cmd.Stderr = nil // discard
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // detach from parent process group
	}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("starting nudge-poller: %w", err)
	}

	pid := cmd.Process.Pid

	// Write PID file for later cleanup.
	pidPath := pollerPidFile(townRoot, session)
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0644); err != nil {
		// Non-fatal — the process is running, we just can't track it.
		fmt.Fprintf(os.Stderr, "Warning: failed to write poller PID file: %v\n", err)
	}

	// Release the process so it runs independently.
	_ = cmd.Process.Release()

	return pid, nil
}

// StopPoller terminates the nudge-poller for a session, if running.
func StopPoller(townRoot, session string) error {
	pidPath := pollerPidFile(townRoot, session)

	data, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no poller to stop
		}
		return fmt.Errorf("reading poller PID file: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		_ = os.Remove(pidPath)
		return nil // corrupt PID file, clean up
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		_ = os.Remove(pidPath)
		return nil
	}

	// Check if alive via signal 0.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		// Process already dead.
		_ = os.Remove(pidPath)
		return nil
	}

	// Send SIGTERM for graceful shutdown.
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		_ = os.Remove(pidPath)
		return fmt.Errorf("sending SIGTERM to poller (pid %d): %w", pid, err)
	}

	_ = os.Remove(pidPath)
	return nil
}

// pollerAlive checks if a poller is running for the given session.
// Returns the PID and whether the process is alive.
func pollerAlive(townRoot, session string) (int, bool) {
	pidPath := pollerPidFile(townRoot, session)

	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, false
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return 0, false
	}

	if err := proc.Signal(syscall.Signal(0)); err != nil {
		// Stale PID file — clean up.
		_ = os.Remove(pidPath)
		return 0, false
	}

	return pid, true
}
