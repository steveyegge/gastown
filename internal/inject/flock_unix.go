//go:build !windows

package inject

import (
	"os"
	"syscall"
)

// lockFileExclusive acquires an exclusive lock on the file.
func lockFileExclusive(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

// lockFileShared acquires a shared lock on the file.
func lockFileShared(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_SH)
}

// unlockFile releases the lock on the file.
func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
