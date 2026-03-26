//go:build windows

package cmd

import (
	"os/exec"
	"syscall"
)

func configureBackgroundDaemonStart(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000200, // CREATE_NEW_PROCESS_GROUP
	}
}
