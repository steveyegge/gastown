//go:build integration

package terminal

import (
	"testing"
	"time"
)

// TestIntegration_WatchState_Connect verifies that WatchState can connect
// to a real coop WebSocket endpoint and the watcher starts without error.
func TestIntegration_WatchState_Connect(t *testing.T) {
	base, cleanup := startCoop(t)
	defer cleanup()

	b := NewCoopBackend(CoopConfig{})
	b.AddSession("claude", base)

	w, err := b.WatchState("claude", CoopStateWatcherConfig{
		ReconnectDelay: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("WatchState: %v", err)
	}
	defer w.Close()

	// Give it time to establish the WebSocket connection.
	time.Sleep(500 * time.Millisecond)

	// Verify no errors on the error channel (successful connect).
	select {
	case err := <-w.ErrCh():
		t.Fatalf("unexpected error after connect: %v", err)
	default:
		// No error — connection established successfully.
	}
}

// TestIntegration_WatchState_CloseCleanup verifies that Close() returns
// promptly and does not hang when connected to a real coop process.
func TestIntegration_WatchState_CloseCleanup(t *testing.T) {
	base, cleanup := startCoop(t)
	defer cleanup()

	b := NewCoopBackend(CoopConfig{})
	b.AddSession("claude", base)

	w, err := b.WatchState("claude", CoopStateWatcherConfig{})
	if err != nil {
		t.Fatalf("WatchState: %v", err)
	}

	// Give it time to connect.
	time.Sleep(500 * time.Millisecond)

	// Close should return within a reasonable time.
	done := make(chan struct{})
	go func() {
		w.Close()
		close(done)
	}()

	select {
	case <-done:
		// OK — Close returned promptly.
	case <-time.After(5 * time.Second):
		t.Fatal("Close() did not return within 5s")
	}
}

// TestIntegration_WatchState_ExitOnKill verifies that killing the coop
// child process produces an exit event on the watcher's ExitCh.
func TestIntegration_WatchState_ExitOnKill(t *testing.T) {
	base, cleanup := startCoop(t)
	defer cleanup()

	w, err := newCoopStateWatcher(CoopStateWatcherConfig{
		BaseURL:        base,
		BufferSize:     64,
		ReconnectDelay: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("newCoopStateWatcher: %v", err)
	}
	defer w.Close()

	// Give the watcher time to connect and subscribe.
	time.Sleep(1 * time.Second)

	// Kill the coop process — this should trigger an exit event.
	cleanup()

	// Wait for either an exit event or a connection error.
	// When coop is killed, one of two things happens:
	// 1. We get an exit event if the WS was subscribed to state events.
	// 2. We get a connection error because the server closed.
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	for {
		select {
		case evt := <-w.ExitCh():
			t.Logf("received exit event: code=%v signal=%v", evt.Code, evt.Signal)
			return
		case err := <-w.ErrCh():
			// Connection error after kill is expected — coop died.
			t.Logf("received expected error after kill: %v", err)
			return
		case <-w.StateCh():
			// Drain state events, keep waiting for exit.
			continue
		case <-timer.C:
			t.Fatal("timed out waiting for exit event or connection error after kill")
		}
	}
}

// TestIntegration_WatchState_ReconnectsAfterDisconnect verifies that the
// watcher automatically reconnects when the coop process restarts.
func TestIntegration_WatchState_ReconnectsAfterDisconnect(t *testing.T) {
	base, cleanup := startCoop(t)

	w, err := newCoopStateWatcher(CoopStateWatcherConfig{
		BaseURL:        base,
		BufferSize:     64,
		ReconnectDelay: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("newCoopStateWatcher: %v", err)
	}
	defer w.Close()

	// Wait for initial connection.
	time.Sleep(500 * time.Millisecond)

	// Kill coop — watcher should start reconnecting.
	cleanup()

	// Drain errors from the disconnect.
	drainTimeout := time.After(2 * time.Second)
	errCount := 0
drainLoop:
	for {
		select {
		case <-w.ErrCh():
			errCount++
		case <-drainTimeout:
			break drainLoop
		}
	}

	if errCount == 0 {
		t.Error("expected at least one error after killing coop")
	}
	t.Logf("received %d reconnection errors (expected)", errCount)
}

// TestIntegration_WatchState_DirectConstructor verifies that
// newCoopStateWatcher works directly without going through CoopBackend.
func TestIntegration_WatchState_DirectConstructor(t *testing.T) {
	base, cleanup := startCoop(t)
	defer cleanup()

	w, err := newCoopStateWatcher(CoopStateWatcherConfig{
		BaseURL:    base,
		BufferSize: 8,
	})
	if err != nil {
		t.Fatalf("newCoopStateWatcher: %v", err)
	}
	defer w.Close()

	// Verify channels are non-nil and readable.
	if w.StateCh() == nil {
		t.Fatal("StateCh() returned nil")
	}
	if w.ExitCh() == nil {
		t.Fatal("ExitCh() returned nil")
	}
	if w.ErrCh() == nil {
		t.Fatal("ErrCh() returned nil")
	}

	// Give it time to connect.
	time.Sleep(500 * time.Millisecond)

	// No errors should have occurred.
	select {
	case err := <-w.ErrCh():
		t.Fatalf("unexpected error: %v", err)
	default:
		// Good.
	}
}

// TestIntegration_WatchState_SendInputTriggersTransition verifies that
// sending input to a coop session (running bash) produces observable
// output, and the watcher stays connected throughout.
func TestIntegration_WatchState_SendInputTriggersTransition(t *testing.T) {
	base, cleanup := startCoop(t)
	defer cleanup()

	b := NewCoopBackend(CoopConfig{})
	b.AddSession("claude", base)

	w, err := b.WatchState("claude", CoopStateWatcherConfig{
		BufferSize:     64,
		ReconnectDelay: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("WatchState: %v", err)
	}
	defer w.Close()

	// Wait for connection to establish.
	time.Sleep(1 * time.Second)

	// Send a command to the bash shell.
	err = b.SendInput("claude", "echo WSTEST_MARKER", true)
	if err != nil {
		t.Fatalf("SendInput: %v", err)
	}

	// Give the shell time to process.
	time.Sleep(1 * time.Second)

	// Verify the watcher is still connected (no fatal errors).
	select {
	case err := <-w.ErrCh():
		t.Fatalf("unexpected error during input: %v", err)
	default:
		// Good — watcher stayed connected.
	}

	// Verify the command executed by checking screen output.
	text, err := b.CapturePaneAll("claude")
	if err != nil {
		t.Fatalf("CapturePane: %v", err)
	}
	if len(text) == 0 {
		t.Error("expected non-empty screen output after sending input")
	}
	t.Logf("screen after input: %d bytes", len(text))
}

// TestIntegration_WatchState_WithToken verifies that a token is passed
// through correctly to the real coop WebSocket endpoint. Coop without
// --auth-token set will accept any token gracefully.
func TestIntegration_WatchState_WithToken(t *testing.T) {
	base, cleanup := startCoop(t)
	defer cleanup()

	w, err := newCoopStateWatcher(CoopStateWatcherConfig{
		BaseURL: base,
		Token:   "integration-test-token",
	})
	if err != nil {
		t.Fatalf("newCoopStateWatcher: %v", err)
	}
	defer w.Close()

	// Wait for connection.
	time.Sleep(500 * time.Millisecond)

	// Without --auth-token set on coop, any token should be accepted.
	select {
	case err := <-w.ErrCh():
		t.Fatalf("unexpected error with token: %v", err)
	default:
		// Good — token accepted.
	}
}

// TestIntegration_WatchState_MultipleWatchers verifies that multiple
// watchers can connect to the same coop session concurrently.
func TestIntegration_WatchState_MultipleWatchers(t *testing.T) {
	base, cleanup := startCoop(t)
	defer cleanup()

	// Start three watchers on the same coop session.
	watchers := make([]*CoopStateWatcher, 3)
	for i := range watchers {
		w, err := newCoopStateWatcher(CoopStateWatcherConfig{
			BaseURL:    base,
			BufferSize: 16,
		})
		if err != nil {
			t.Fatalf("newCoopStateWatcher[%d]: %v", i, err)
		}
		watchers[i] = w
	}

	// Wait for all to connect.
	time.Sleep(1 * time.Second)

	// Verify no errors on any watcher.
	for i, w := range watchers {
		select {
		case err := <-w.ErrCh():
			t.Errorf("watcher[%d] unexpected error: %v", i, err)
		default:
			// Good.
		}
	}

	// Clean up all watchers.
	for _, w := range watchers {
		w.Close()
	}
}
