//go:build !windows

// flock.go provides cross-process file locking using flock(2).
// This is used for synchronizing resource allocation across multiple processes.

package util

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// FileLock provides cross-process file locking using flock(2).
// Unlike sync.Mutex which only works within a process, FileLock ensures
// mutual exclusion across multiple processes on the same machine.
type FileLock struct {
	path string
	file *os.File
}

// NewFileLock creates a new file lock for the given path.
// The lock file will be created if it doesn't exist.
func NewFileLock(path string) *FileLock {
	return &FileLock{path: path}
}

// Lock acquires an exclusive lock on the file.
// This blocks until the lock is acquired.
// The caller must call Unlock when done.
func (l *FileLock) Lock() error {
	// Ensure parent directory exists
	dir := filepath.Dir(l.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating lock directory: %w", err)
	}

	// Open or create the lock file
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("opening lock file: %w", err)
	}
	l.file = file

	// Acquire exclusive lock (blocks until available)
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close() // Best effort cleanup on error path
		l.file = nil
		return fmt.Errorf("acquiring lock: %w", err)
	}

	return nil
}

// TryLock attempts to acquire the lock without blocking.
// Returns true if the lock was acquired, false if it's already held.
func (l *FileLock) TryLock() (bool, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(l.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return false, fmt.Errorf("creating lock directory: %w", err)
	}

	// Open or create the lock file
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return false, fmt.Errorf("opening lock file: %w", err)
	}
	l.file = file

	// Try to acquire exclusive lock (non-blocking)
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		_ = file.Close() // Best effort cleanup on error path
		l.file = nil
		if err == syscall.EWOULDBLOCK {
			return false, nil // Lock is held by another process
		}
		return false, fmt.Errorf("acquiring lock: %w", err)
	}

	return true, nil
}

// Unlock releases the lock.
// Safe to call even if not locked.
func (l *FileLock) Unlock() error {
	if l.file == nil {
		return nil
	}

	// Release the lock
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil {
		// Still try to close the file
		_ = l.file.Close() // Best effort cleanup on error path
		l.file = nil
		return fmt.Errorf("releasing lock: %w", err)
	}

	// Close the file
	if err := l.file.Close(); err != nil {
		l.file = nil
		return fmt.Errorf("closing lock file: %w", err)
	}

	l.file = nil
	return nil
}

// WithLock executes a function while holding the lock.
// This is a convenience wrapper that handles Lock/Unlock automatically.
func (l *FileLock) WithLock(fn func() error) error {
	if err := l.Lock(); err != nil {
		return err
	}
	defer func() { _ = l.Unlock() }() // Unlock error ignored; fn() error takes precedence
	return fn()
}

// FlockExclusive acquires an exclusive (write) lock on an open file descriptor.
// This is a low-level helper for use with already-opened files.
func FlockExclusive(fd uintptr) error {
	return syscall.Flock(int(fd), syscall.LOCK_EX)
}

// FlockShared acquires a shared (read) lock on an open file descriptor.
// This is a low-level helper for use with already-opened files.
func FlockShared(fd uintptr) error {
	return syscall.Flock(int(fd), syscall.LOCK_SH)
}

// FlockUnlock releases a lock on an open file descriptor.
// This is a low-level helper for use with already-opened files.
func FlockUnlock(fd uintptr) error {
	return syscall.Flock(int(fd), syscall.LOCK_UN)
}
