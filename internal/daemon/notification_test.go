package daemon

import (
	"sync"
	"testing"
	"time"
)

func TestNotificationManager_BasicFlow(t *testing.T) {
	dir := t.TempDir()
	mgr := NewNotificationManager(dir, 5*time.Minute)

	// Initially should allow sending
	ok, err := mgr.ShouldSend("sess1", "heartbeat")
	if err != nil {
		t.Fatalf("ShouldSend: %v", err)
	}
	if !ok {
		t.Fatal("expected ShouldSend=true for empty slot")
	}

	// Record a send
	if err := mgr.RecordSend("sess1", "heartbeat", "hello"); err != nil {
		t.Fatalf("RecordSend: %v", err)
	}

	// Should now suppress
	ok, err = mgr.ShouldSend("sess1", "heartbeat")
	if err != nil {
		t.Fatalf("ShouldSend: %v", err)
	}
	if ok {
		t.Fatal("expected ShouldSend=false after RecordSend")
	}

	// Mark consumed -> should allow again
	if err := mgr.MarkConsumed("sess1", "heartbeat"); err != nil {
		t.Fatalf("MarkConsumed: %v", err)
	}

	ok, err = mgr.ShouldSend("sess1", "heartbeat")
	if err != nil {
		t.Fatalf("ShouldSend: %v", err)
	}
	if !ok {
		t.Fatal("expected ShouldSend=true after MarkConsumed")
	}
}

func TestNotificationManager_SendIfReady(t *testing.T) {
	dir := t.TempDir()
	mgr := NewNotificationManager(dir, 5*time.Minute)

	// First call should succeed
	ok, err := mgr.SendIfReady("sess1", "heartbeat", "msg1")
	if err != nil {
		t.Fatalf("SendIfReady: %v", err)
	}
	if !ok {
		t.Fatal("expected SendIfReady=true for empty slot")
	}

	// Second call should be suppressed
	ok, err = mgr.SendIfReady("sess1", "heartbeat", "msg2")
	if err != nil {
		t.Fatalf("SendIfReady: %v", err)
	}
	if ok {
		t.Fatal("expected SendIfReady=false for pending slot")
	}

	// Verify the recorded message is from the first call
	ns, err := mgr.GetSlot("sess1", "heartbeat")
	if err != nil {
		t.Fatalf("GetSlot: %v", err)
	}
	if ns.Message != "msg1" {
		t.Fatalf("expected message 'msg1', got %q", ns.Message)
	}
}

func TestNotificationManager_StaleSlot(t *testing.T) {
	dir := t.TempDir()
	mgr := NewNotificationManager(dir, 1*time.Millisecond) // Very short maxAge

	if err := mgr.RecordSend("sess1", "heartbeat", "hello"); err != nil {
		t.Fatalf("RecordSend: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	ok, err := mgr.ShouldSend("sess1", "heartbeat")
	if err != nil {
		t.Fatalf("ShouldSend: %v", err)
	}
	if !ok {
		t.Fatal("expected ShouldSend=true for stale slot")
	}
}

func TestNotificationManager_ClearSlot(t *testing.T) {
	dir := t.TempDir()
	mgr := NewNotificationManager(dir, 5*time.Minute)

	if err := mgr.RecordSend("sess1", "heartbeat", "hello"); err != nil {
		t.Fatalf("RecordSend: %v", err)
	}

	if err := mgr.ClearSlot("sess1", "heartbeat"); err != nil {
		t.Fatalf("ClearSlot: %v", err)
	}

	ns, err := mgr.GetSlot("sess1", "heartbeat")
	if err != nil {
		t.Fatalf("GetSlot: %v", err)
	}
	if ns != nil {
		t.Fatal("expected nil slot after clear")
	}
}

func TestNotificationManager_ClearSlot_Nonexistent(t *testing.T) {
	dir := t.TempDir()
	mgr := NewNotificationManager(dir, 5*time.Minute)

	// Should not error on nonexistent slot
	if err := mgr.ClearSlot("sess1", "heartbeat"); err != nil {
		t.Fatalf("ClearSlot on nonexistent: %v", err)
	}
}

func TestNotificationManager_MarkSessionActive(t *testing.T) {
	dir := t.TempDir()
	mgr := NewNotificationManager(dir, 5*time.Minute)

	// Create two slots for the same session
	if err := mgr.RecordSend("sess1", "heartbeat", "hb"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.RecordSend("sess1", "status", "st"); err != nil {
		t.Fatal(err)
	}

	// Both should suppress
	ok, _ := mgr.ShouldSend("sess1", "heartbeat")
	if ok {
		t.Fatal("expected heartbeat suppressed")
	}
	ok, _ = mgr.ShouldSend("sess1", "status")
	if ok {
		t.Fatal("expected status suppressed")
	}

	// Mark session active -> both consumed
	if err := mgr.MarkSessionActive("sess1"); err != nil {
		t.Fatal(err)
	}

	ok, _ = mgr.ShouldSend("sess1", "heartbeat")
	if !ok {
		t.Fatal("expected heartbeat allowed after MarkSessionActive")
	}
	ok, _ = mgr.ShouldSend("sess1", "status")
	if !ok {
		t.Fatal("expected status allowed after MarkSessionActive")
	}
}

func TestNotificationManager_ConcurrentSendIfReady(t *testing.T) {
	dir := t.TempDir()
	mgr := NewNotificationManager(dir, 5*time.Minute)

	const goroutines = 50
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		winners int
	)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			ok, err := mgr.SendIfReady("sess1", "heartbeat", "concurrent")
			if err != nil {
				t.Errorf("SendIfReady: %v", err)
				return
			}
			if ok {
				mu.Lock()
				winners++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if winners != 1 {
		t.Fatalf("expected exactly 1 winner, got %d", winners)
	}
}

func TestNotificationManager_ConcurrentMarkConsumed(t *testing.T) {
	dir := t.TempDir()
	mgr := NewNotificationManager(dir, 5*time.Minute)

	if err := mgr.RecordSend("sess1", "heartbeat", "hello"); err != nil {
		t.Fatal(err)
	}

	// Concurrent MarkConsumed should not panic or corrupt state
	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if err := mgr.MarkConsumed("sess1", "heartbeat"); err != nil {
				t.Errorf("MarkConsumed: %v", err)
			}
		}()
	}
	wg.Wait()

	// Verify slot is consumed
	ns, err := mgr.GetSlot("sess1", "heartbeat")
	if err != nil {
		t.Fatal(err)
	}
	if !ns.Consumed {
		t.Fatal("expected slot to be consumed")
	}
}

func TestNotificationManager_ConcurrentMixedOps(t *testing.T) {
	dir := t.TempDir()
	mgr := NewNotificationManager(dir, 5*time.Minute)

	// Run mixed operations concurrently - should not panic with -race
	const goroutines = 30
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			switch i % 6 {
			case 0:
				_, _ = mgr.ShouldSend("sess1", "heartbeat")
			case 1:
				_ = mgr.RecordSend("sess1", "heartbeat", "msg")
			case 2:
				_ = mgr.MarkConsumed("sess1", "heartbeat")
			case 3:
				_, _ = mgr.GetSlot("sess1", "heartbeat")
			case 4:
				_ = mgr.MarkSessionActive("sess1")
			case 5:
				_, _ = mgr.SendIfReady("sess1", "heartbeat", "msg")
			}
		}()
	}
	wg.Wait()
}
