//go:build windows

package inject

import (
	"os"
)

// lockFileExclusive is a no-op on Windows.
// Gas Town is primarily designed for Unix systems where flock is available.
// On Windows, file locking is not implemented, but the queue will still work
// for single-process scenarios (which is the common case for Claude sessions).
func lockFileExclusive(f *os.File) error {
	return nil
}

// lockFileShared is a no-op on Windows.
func lockFileShared(f *os.File) error {
	return nil
}

// unlockFile is a no-op on Windows.
func unlockFile(f *os.File) error {
	return nil
}
