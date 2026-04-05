//go:build windows

package util

import (
	"os/exec"
	"syscall"
)

// SetProcessGroup configures a command to run in its own process group
// without a visible console window. On Windows, CREATE_NEW_PROCESS_GROUP
// detaches from the parent's console and CREATE_NO_WINDOW suppresses the
// transient console window that Windows otherwise creates for console apps
// spawned from a no-console parent (e.g. the daemon).
func SetProcessGroup(cmd *exec.Cmd) {
	const CREATE_NO_WINDOW = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW,
	}
}

// SetDetachedProcessGroup is the same as SetProcessGroup on Windows.
func SetDetachedProcessGroup(cmd *exec.Cmd) {
	SetProcessGroup(cmd)
}
