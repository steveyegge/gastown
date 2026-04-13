//go:build !windows

package util

import (
	"os/exec"
	"testing"
)

// TestSetDetachedProcessGroupSetsSetsid verifies that daemon processes are
// fully detached into their own session (Setsid=true), not just a new process
// group (Setpgid=true). Without Setsid, the daemon shares a session with its
// parent and can receive SIGHUP when the parent terminal exits.
// See sbx-gastown-6d7d bug #1: daemon stops when sling exits.
func TestSetDetachedProcessGroupSetsSetsid(t *testing.T) {
	cmd := exec.Command("true")
	SetDetachedProcessGroup(cmd)

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr should be set")
	}
	if !cmd.SysProcAttr.Setsid {
		t.Error("SetDetachedProcessGroup must set Setsid=true for full session detach")
	}
}
