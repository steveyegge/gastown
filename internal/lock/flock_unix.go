//go:build !windows

package lock

import (
	"fmt"
	"os"
	"syscall"
)

// FlockAcquire opens a flock file and acquires an exclusive advisory lock.
// Returns a cleanup function that releases the lock and closes the file.
// This is a general-purpose cross-process lock suitable for any read-modify-write
// operation that needs serialization across separate CLI invocations.
func FlockAcquire(path string) (func(), error) {
	return flockAcquire(path)
}

// flockAcquire opens a flock file and acquires an exclusive advisory lock.
// Returns a cleanup function that releases the lock and closes the file.
// The flock prevents concurrent Acquire() calls from racing on the same lock path.
func flockAcquire(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644) //nolint:gosec // G304,G306: lock files are internal operational data
	if err != nil {
		return nil, fmt.Errorf("opening flock file: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("acquiring flock: %w", err)
	}

	cleanup := func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
		f.Close()
	}
	return cleanup, nil
}

// FlockTryAcquire attempts a non-blocking exclusive advisory lock on the given path.
// Returns (cleanup, true, nil) if the lock was acquired, or (nil, false, nil) if
// another process already holds it. The cleanup function releases the lock and
// closes the file descriptor.
func FlockTryAcquire(path string) (func(), bool, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644) //nolint:gosec // G304,G306: lock files are internal operational data
	if err != nil {
		return nil, false, fmt.Errorf("opening flock file: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if err == syscall.EWOULDBLOCK {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("acquiring flock: %w", err)
	}

	cleanup := func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
		f.Close()
	}
	return cleanup, true, nil
}
