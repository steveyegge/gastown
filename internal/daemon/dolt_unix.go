//go:build !windows

package daemon

import "syscall"

// doltSysProcAttr returns the SysProcAttr for starting dolt sql-server.
// On Unix, we detach from the process group so it survives daemon restart.
func doltSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true,
	}
}
