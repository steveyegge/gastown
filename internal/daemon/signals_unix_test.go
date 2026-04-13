//go:build !windows

package daemon

import (
	"syscall"
	"testing"
)

// TestDaemonSignalsIncludesSIGHUP verifies that the daemon handles SIGHUP.
// Without SIGHUP in the signal list, Go's default behavior terminates the
// process when the parent session exits — this caused the daemon to stop
// when sling completed (sbx-gastown-6d7d bug #1).
func TestDaemonSignalsIncludesSIGHUP(t *testing.T) {
	signals := daemonSignals()
	found := false
	for _, s := range signals {
		if s == syscall.SIGHUP {
			found = true
			break
		}
	}
	if !found {
		t.Error("daemonSignals() must include SIGHUP to survive parent session exit")
	}
}

// TestSIGHUPIsNoopSignal verifies that SIGHUP is treated as a no-op (ignored)
// rather than triggering shutdown or lifecycle processing.
func TestSIGHUPIsNoopSignal(t *testing.T) {
	if isLifecycleSignal(syscall.SIGHUP) {
		t.Error("SIGHUP should not be a lifecycle signal")
	}
	if isReloadRestartSignal(syscall.SIGHUP) {
		t.Error("SIGHUP should not be a reload-restart signal")
	}
	if !isNoopSignal(syscall.SIGHUP) {
		t.Error("SIGHUP must be a noop signal (ignored by daemon)")
	}
}
