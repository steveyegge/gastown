//go:build windows

package daemon

import "syscall"

// doltSysProcAttr returns the SysProcAttr for starting dolt sql-server.
// On Windows, we don't need to set process group attributes.
func doltSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
