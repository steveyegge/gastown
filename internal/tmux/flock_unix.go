//go:build !windows

package tmux

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// acquireFlockLock acquires a file-based lock using flock(2) for cross-process
// serialization. Returns an unlock function that must be called to release the lock.
// Uses non-blocking flock in a polling loop to respect the timeout.
func acquireFlockLock(lockPath string, timeout time.Duration) (func(), error) {
	dir := filepath.Dir(lockPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating lock dir: %w", err)
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}

	deadline := time.Now().Add(timeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				f.Close()
			}, nil
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, fmt.Errorf("timeout after %s waiting for flock", timeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
