package util

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFileLock_BasicLockUnlock(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	lock := NewFileLock(lockPath)

	// Should be able to acquire lock
	if err := lock.Lock(); err != nil {
		t.Fatalf("Lock() failed: %v", err)
	}

	// Lock file should exist
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Error("Lock file was not created")
	}

	// Should be able to release lock
	if err := lock.Unlock(); err != nil {
		t.Fatalf("Unlock() failed: %v", err)
	}
}

func TestFileLock_TryLock(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	lock1 := NewFileLock(lockPath)
	lock2 := NewFileLock(lockPath)

	// First lock should succeed
	acquired, err := lock1.TryLock()
	if err != nil {
		t.Fatalf("TryLock() failed: %v", err)
	}
	if !acquired {
		t.Error("Expected to acquire lock")
	}

	// Second lock should fail (non-blocking)
	acquired, err = lock2.TryLock()
	if err != nil {
		t.Fatalf("TryLock() returned error: %v", err)
	}
	if acquired {
		t.Error("Expected TryLock to fail when lock is held")
	}

	// Release first lock
	if err := lock1.Unlock(); err != nil {
		t.Fatalf("Unlock() failed: %v", err)
	}

	// Now second lock should succeed
	acquired, err = lock2.TryLock()
	if err != nil {
		t.Fatalf("TryLock() failed: %v", err)
	}
	if !acquired {
		t.Error("Expected to acquire lock after first was released")
	}

	lock2.Unlock()
}

func TestFileLock_WithLock(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	lock := NewFileLock(lockPath)
	executed := false

	err := lock.WithLock(func() error {
		executed = true
		return nil
	})

	if err != nil {
		t.Fatalf("WithLock() failed: %v", err)
	}
	if !executed {
		t.Error("Function was not executed")
	}
}

func TestFileLock_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	// Test that concurrent goroutines are serialized
	var counter int64
	var maxConcurrent int64
	var currentConcurrent int64
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lock := NewFileLock(lockPath)
			lock.WithLock(func() error {
				// Track concurrent access
				curr := atomic.AddInt64(&currentConcurrent, 1)
				if curr > atomic.LoadInt64(&maxConcurrent) {
					atomic.StoreInt64(&maxConcurrent, curr)
				}

				// Simulate some work
				time.Sleep(10 * time.Millisecond)
				atomic.AddInt64(&counter, 1)

				atomic.AddInt64(&currentConcurrent, -1)
				return nil
			})
		}()
	}

	wg.Wait()

	// All goroutines should have executed
	if counter != 10 {
		t.Errorf("Expected counter=10, got %d", counter)
	}

	// Max concurrent should be 1 (lock enforces serialization)
	if maxConcurrent != 1 {
		t.Errorf("Expected max concurrent=1, got %d (race condition detected)", maxConcurrent)
	}
}

func TestFileLock_UnlockWithoutLock(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "test.lock")

	lock := NewFileLock(lockPath)

	// Unlock without lock should be safe (no-op)
	if err := lock.Unlock(); err != nil {
		t.Errorf("Unlock() without lock should not error, got: %v", err)
	}
}

func TestFileLock_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	lockPath := filepath.Join(tmpDir, "subdir", "deep", "test.lock")

	lock := NewFileLock(lockPath)

	if err := lock.Lock(); err != nil {
		t.Fatalf("Lock() failed: %v", err)
	}
	defer lock.Unlock()

	// Directory should have been created
	dir := filepath.Dir(lockPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("Lock directory was not created")
	}
}
