//go:build !windows

package acp

import (
	"os"
	"testing"
	"time"
)

func TestProxy_GracefulShutdownOnEOF(t *testing.T) {
	p := NewProxy()
	p.done = make(chan struct{})

	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer stdinReader.Close()

	p.stdin = stdinReader

	// Start forwardToAgent in a goroutine
	p.wg.Add(1)
	go p.forwardToAgent()

	// Close stdinWriter to trigger EOF
	stdinWriter.Close()

	// Verify that forwardToAgent exits and closes p.done
	select {
	case <-p.done:
		// Success: forwardToAgent triggered Shutdown which marked p.done
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for forwardToAgent to trigger shutdown on EOF")
	}

	if !p.isShuttingDown.Load() {
		t.Error("expected isShuttingDown to be true")
	}

	// Wait for goroutine to finish
	p.wg.Wait()
}

func TestProxy_GracefulShutdownAfterInput(t *testing.T) {
	p := NewProxy()
	p.done = make(chan struct{})

	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer stdinReader.Close()

	p.stdin = stdinReader

	// Start forwardToAgent in a goroutine
	p.wg.Add(1)
	go p.forwardToAgent()

	// Write some data
	stdinWriter.Write([]byte(`{"jsonrpc":"2.0","method":"test"}` + "\n"))
	time.Sleep(100 * time.Millisecond) // Give it time to process

	// Close stdinWriter to trigger EOF
	stdinWriter.Close()

	// Verify that forwardToAgent exits and closes p.done
	select {
	case <-p.done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for forwardToAgent to trigger shutdown after input")
	}

	if !p.isShuttingDown.Load() {
		t.Error("expected isShuttingDown to be true")
	}

	p.wg.Wait()
}
