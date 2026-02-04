package boot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gofrs/flock"
)

func TestAcquireLock(t *testing.T) {
	// Create temp directory for test
	tmpDir := t.TempDir()

	b := &Boot{
		townRoot: tmpDir,
		bootDir:  filepath.Join(tmpDir, "deacon", "dogs", "boot"),
	}

	// First acquire should succeed
	if err := b.AcquireLock(); err != nil {
		t.Fatalf("First AcquireLock failed: %v", err)
	}

	// Verify marker file exists
	markerPath := filepath.Join(b.bootDir, MarkerFileName)
	if _, err := os.Stat(markerPath); os.IsNotExist(err) {
		t.Error("Marker file was not created")
	}

	// Verify lock is held by trying to acquire from another flock instance
	otherLock := flock.New(markerPath)
	locked, err := otherLock.TryLock()
	if err != nil {
		t.Fatalf("TryLock failed: %v", err)
	}
	if locked {
		t.Error("Should not be able to acquire lock while first lock is held")
		_ = otherLock.Unlock()
	}

	// Release should succeed
	if err := b.ReleaseLock(); err != nil {
		t.Fatalf("ReleaseLock failed: %v", err)
	}

	// After release, another instance should be able to acquire
	locked, err = otherLock.TryLock()
	if err != nil {
		t.Fatalf("TryLock after release failed: %v", err)
	}
	if !locked {
		t.Error("Should be able to acquire lock after release")
	}
	_ = otherLock.Unlock()
}

func TestAcquireLockConcurrent(t *testing.T) {
	tmpDir := t.TempDir()

	b1 := &Boot{
		townRoot: tmpDir,
		bootDir:  filepath.Join(tmpDir, "deacon", "dogs", "boot"),
	}
	b2 := &Boot{
		townRoot: tmpDir,
		bootDir:  filepath.Join(tmpDir, "deacon", "dogs", "boot"),
	}

	// First boot acquires lock
	if err := b1.AcquireLock(); err != nil {
		t.Fatalf("First boot AcquireLock failed: %v", err)
	}

	// Second boot should fail to acquire
	err := b2.AcquireLock()
	if err == nil {
		t.Error("Second boot should have failed to acquire lock")
		_ = b2.ReleaseLock()
	}

	// Release first lock
	if err := b1.ReleaseLock(); err != nil {
		t.Fatalf("First boot ReleaseLock failed: %v", err)
	}

	// Now second boot should succeed
	if err := b2.AcquireLock(); err != nil {
		t.Fatalf("Second boot AcquireLock after release failed: %v", err)
	}

	_ = b2.ReleaseLock()
}

func TestReleaseLockIdempotent(t *testing.T) {
	tmpDir := t.TempDir()

	b := &Boot{
		townRoot: tmpDir,
		bootDir:  filepath.Join(tmpDir, "deacon", "dogs", "boot"),
	}

	// Release without acquire should not error (lockHandle is nil)
	if err := b.ReleaseLock(); err != nil {
		t.Errorf("ReleaseLock without acquire should not error: %v", err)
	}

	// Acquire then release twice should not error
	if err := b.AcquireLock(); err != nil {
		t.Fatalf("AcquireLock failed: %v", err)
	}
	if err := b.ReleaseLock(); err != nil {
		t.Fatalf("First ReleaseLock failed: %v", err)
	}
	if err := b.ReleaseLock(); err != nil {
		t.Errorf("Second ReleaseLock should not error: %v", err)
	}
}
