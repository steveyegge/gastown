package acp

import (
	"context"
	"testing"
	"time"
)

func TestNewPropeller(t *testing.T) {
	proxy := NewProxy()
	prop := NewPropeller(proxy, "/town", "hq-mayor")

	if prop.proxy != proxy {
		t.Error("proxy not set correctly")
	}
	if prop.townRoot != "/town" {
		t.Error("townRoot not set correctly")
	}
	if prop.session != "hq-mayor" {
		t.Error("session not set correctly")
	}
}

func TestPropeller_StartStop(t *testing.T) {
	prop := NewPropeller(nil, "", "hq-mayor")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	prop.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	prop.Stop()
}

func TestPropeller_DeliverNudges_NoProxy(t *testing.T) {
	// Test that deliverNudges handles nil proxy gracefully
	prop := NewPropeller(nil, "/town", "hq-mayor")
	prop.deliverNudges() // Should not panic
}

func TestPropeller_EventLoop_Cancellation(t *testing.T) {
	// Test that eventLoop exits on context cancellation
	prop := NewPropeller(nil, "/town", "hq-mayor")

	ctx, cancel := context.WithCancel(context.Background())
	prop.ctx = ctx
	prop.cancel = cancel

	// Start eventLoop in a goroutine
	done := make(chan struct{})
	go func() {
		prop.eventLoop()
		close(done)
	}()

	// Cancel context
	cancel()

	// Wait for eventLoop to exit
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("eventLoop did not exit after context cancellation")
	}
}
