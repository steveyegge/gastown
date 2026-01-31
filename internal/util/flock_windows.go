//go:build windows

// flock_windows.go provides stub implementations for Windows where flock(2) is not available.
// Gas Town is primarily a Unix-based system, but this allows the code to compile on Windows.

package util

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrWindowsNotSupported is returned when file locking is attempted on Windows.
var ErrWindowsNotSupported = errors.New("file locking not supported on Windows")

// FileLock provides cross-process file locking using flock(2).
// On Windows, this is a stub that returns errors since flock is not available.
type FileLock struct {
	path string
	file *os.File
}

// NewFileLock creates a new file lock for the given path.
func NewFileLock(path string) *FileLock {
	return &FileLock{path: path}
}

// Lock acquires an exclusive lock on the file.
// On Windows, this creates the lock file but does not provide actual locking.
func (l *FileLock) Lock() error {
	dir := filepath.Dir(l.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	l.file = file
	return nil
}

// TryLock attempts to acquire the lock without blocking.
// On Windows, this always succeeds but provides no actual locking.
func (l *FileLock) TryLock() (bool, error) {
	if err := l.Lock(); err != nil {
		return false, err
	}
	return true, nil
}

// Unlock releases the lock.
func (l *FileLock) Unlock() error {
	if l.file == nil {
		return nil
	}
	if err := l.file.Close(); err != nil {
		l.file = nil
		return err
	}
	l.file = nil
	return nil
}

// WithLock executes a function while holding the lock.
func (l *FileLock) WithLock(fn func() error) error {
	if err := l.Lock(); err != nil {
		return err
	}
	defer func() { _ = l.Unlock() }() // Unlock error ignored; fn() error takes precedence
	return fn()
}

// FlockExclusive is a no-op on Windows where flock(2) is not available.
// This is a low-level helper for use with already-opened files.
func FlockExclusive(fd uintptr) error {
	return nil
}

// FlockShared is a no-op on Windows where flock(2) is not available.
// This is a low-level helper for use with already-opened files.
func FlockShared(fd uintptr) error {
	return nil
}

// FlockUnlock is a no-op on Windows where flock(2) is not available.
// This is a low-level helper for use with already-opened files.
func FlockUnlock(fd uintptr) error {
	return nil
}
