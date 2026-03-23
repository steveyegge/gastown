//go:build windows

package tmux

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// windowsFlockMu serializes acquireFlockLock calls on Windows where flock(2) is unavailable.
// In-process locking is sufficient for Windows since tmux is not available there anyway.
var windowsFlockMu sync.Mutex

// acquireFlockLock provides in-process locking on Windows (flock(2) is unavailable).
// Since tmux is not supported on Windows, this is only reached in tests; it uses
// a global mutex rather than per-path locking for simplicity.
func acquireFlockLock(lockPath string, timeout time.Duration) (func(), error) {
	dir := filepath.Dir(lockPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating lock dir: %w", err)
	}

	deadline := time.Now().Add(timeout)
	for {
		if windowsFlockMu.TryLock() {
			return func() { windowsFlockMu.Unlock() }, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout after %s waiting for lock", timeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
