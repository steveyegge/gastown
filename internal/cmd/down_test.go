package cmd

import (
	"os"
	"testing"

	"github.com/steveyegge/gastown/internal/tmux"
)

func TestIsProcessRunning_CurrentProcess(t *testing.T) {
	if !isProcessRunning(os.Getpid()) {
		t.Error("current process should be detected as running")
	}
}

func TestIsProcessRunning_InvalidPID(t *testing.T) {
	if isProcessRunning(99999999) {
		t.Error("invalid PID should not be detected as running")
	}
}

func TestIsProcessRunning_MaxPID(t *testing.T) {
	if isProcessRunning(2147483647) {
		t.Error("max PID should not be running")
	}
}

func TestLegacySocket_Constant(t *testing.T) {
	// The legacy socket must be "gt" to match sessions created before the migration.
	if legacySocket != "gt" {
		t.Errorf("legacySocket = %q, want %q", legacySocket, "gt")
	}
}

func TestSweepLegacySocketSessions_SkipsWhenOnLegacySocket(t *testing.T) {
	// When the current socket IS the legacy socket, sweep should be a no-op
	// (nothing to sweep — we're already on it).
	old := tmux.GetDefaultSocket()
	defer tmux.SetDefaultSocket(old)

	tmux.SetDefaultSocket(legacySocket)

	// Should not panic or error — just return immediately.
	sweepLegacySocketSessions(true, false)
}

func TestSweepLegacySocketSessions_SkipsWhenNoSocket(t *testing.T) {
	// When no socket is configured, sweep should be a no-op.
	old := tmux.GetDefaultSocket()
	defer tmux.SetDefaultSocket(old)

	tmux.SetDefaultSocket("")

	// Should not panic or error — just return immediately.
	sweepLegacySocketSessions(true, false)
}

func TestSweepLegacySocketSessions_DryRunNoServer(t *testing.T) {
	// When the legacy socket has no tmux server, sweep should silently succeed.
	old := tmux.GetDefaultSocket()
	defer tmux.SetDefaultSocket(old)

	tmux.SetDefaultSocket("default")

	// In dry-run mode, even if a server existed, it would only print.
	// With no server, it should return silently.
	sweepLegacySocketSessions(true, false)
}
