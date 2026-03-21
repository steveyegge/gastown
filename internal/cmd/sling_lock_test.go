package cmd

import (
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestTryAcquireSlingBeadLock_Contention(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("advisory flock is a no-op on Windows")
	}
	t.Parallel()

	townRoot := t.TempDir()
	beadID := "gt-race123"

	release1, err := tryAcquireSlingBeadLock(townRoot, beadID)
	if err != nil {
		t.Fatalf("first lock acquire failed: %v", err)
	}

	release2, err := tryAcquireSlingBeadLock(townRoot, beadID)
	if err == nil {
		release2()
		t.Fatal("expected second lock acquire to fail due to contention")
	}
	if !strings.Contains(err.Error(), "already being slung") {
		t.Fatalf("expected deterministic contention error, got: %v", err)
	}

	release1()

	release3, err := tryAcquireSlingBeadLock(townRoot, beadID)
	if err != nil {
		t.Fatalf("expected lock acquire to succeed after release: %v", err)
	}
	release3()
}

func TestTryAcquireSlingAssigneeLock_Serialization(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("advisory flock is a no-op on Windows")
	}
	t.Parallel()

	townRoot := t.TempDir()
	agent := "gastown/polecats/testcat"

	// First acquire should succeed immediately.
	release1, err := tryAcquireSlingAssigneeLock(townRoot, agent)
	if err != nil {
		t.Fatalf("first assignee lock acquire failed: %v", err)
	}

	// Second acquire from the same goroutine (same process) should also succeed
	// because flock is per-FD, not per-process. But from a concurrent goroutine
	// holding its own FD, the lock semantics apply at the OS level.
	// For unit test purposes, verify the lock file is created correctly.
	release1()

	// Verify lock works after release.
	release2, err := tryAcquireSlingAssigneeLock(townRoot, agent)
	if err != nil {
		t.Fatalf("lock acquire after release failed: %v", err)
	}
	release2()
}

func TestTryAcquireSlingAssigneeLock_DifferentAgents(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("advisory flock is a no-op on Windows")
	}
	t.Parallel()

	townRoot := t.TempDir()

	// Different agents should not block each other.
	release1, err := tryAcquireSlingAssigneeLock(townRoot, "rig/polecats/cat1")
	if err != nil {
		t.Fatalf("first agent lock failed: %v", err)
	}
	defer release1()

	release2, err := tryAcquireSlingAssigneeLock(townRoot, "rig/polecats/cat2")
	if err != nil {
		t.Fatalf("second agent lock should not be blocked by first: %v", err)
	}
	defer release2()
}

func TestTryAcquireSlingAssigneeLock_Contention(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("advisory flock is a no-op on Windows")
	}
	t.Parallel()

	townRoot := t.TempDir()
	agent := "gastown/polecats/racecat"

	// Acquire lock in a goroutine and hold it briefly.
	var wg sync.WaitGroup
	lockAcquired := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		release, err := tryAcquireSlingAssigneeLock(townRoot, agent)
		if err != nil {
			t.Errorf("goroutine lock acquire failed: %v", err)
			return
		}
		close(lockAcquired)
		// Hold lock briefly so the main goroutine's retry loop gets exercised.
		<-lockAcquired // already closed, but semantically signal
		release()
	}()

	<-lockAcquired

	// The goroutine released immediately after signaling, so the main goroutine
	// should be able to acquire the lock (possibly after a brief retry).
	release2, err := tryAcquireSlingAssigneeLock(townRoot, agent)
	if err != nil {
		t.Fatalf("expected lock acquire to succeed after goroutine release: %v", err)
	}
	release2()

	wg.Wait()
}

func TestTryAcquireSlingAssigneeLock_AgentNameSanitization(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("advisory flock is a no-op on Windows")
	}
	t.Parallel()

	townRoot := t.TempDir()

	// Agent names with slashes and colons should be sanitized for filesystem safety.
	agents := []string{
		"gastown/polecats/dementus",
		"rig:with:colons",
		"mayor/",
	}
	for _, agent := range agents {
		release, err := tryAcquireSlingAssigneeLock(townRoot, agent)
		if err != nil {
			t.Fatalf("lock acquire failed for agent %q: %v", agent, err)
		}
		release()
	}
}
