//go:build windows

package tmux

import (
	"os/exec"
	"syscall"
)

// hideConsoleWindow sets process creation flags so that tmux subprocesses
// don't flash a visible console window. Required because the daemon runs
// with CREATE_NO_WINDOW and Windows creates a new console for child
// console-apps whose parent has none.
func hideConsoleWindow(cmd *exec.Cmd) {
	const CREATE_NO_WINDOW = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW,
	}
}
