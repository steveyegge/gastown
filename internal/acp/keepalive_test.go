package acp

import (
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestProxy_RunKeepAlive_Logic(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ACP keepalive requires Unix process groups and sleep command")
	}
	p := NewProxy()
	p.sessionID = "test-session"
	p.heartbeatSupported.Store(true)
	p.heartbeatMethod = "set_mode"
	p.currentModeID = "test-mode"

	// Mock agent stdin to capture heartbeats
	agentInR, agentInW := io.Pipe()
	p.agentStdin = agentInW

	// Start a dummy process so isProcessAlive() returns true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.cmd = exec.CommandContext(ctx, "sleep", "100")
	p.setupProcessGroup()
	if err := p.cmd.Start(); err != nil {
		t.Fatalf("failed to start dummy process: %v", err)
	}
	defer p.cmd.Process.Kill()

	// Capture heartbeats in a goroutine
	heartbeats := make(chan *JSONRPCMessage, 10)
	go func() {
		dec := json.NewDecoder(agentInR)
		for {
			var msg JSONRPCMessage
			if err := dec.Decode(&msg); err != nil {
				return
			}
			heartbeats <- &msg
		}
	}()

	tickerChan := make(chan time.Time)
	p.wg.Add(1)
	go p.runKeepAlive(tickerChan)

	// Case 1: Idle for 50s -> should send heartbeat
	p.lastActivity.Store(time.Now().Add(-50 * time.Second).UnixNano())
	tickerChan <- time.Now()

	select {
	case msg := <-heartbeats:
		if msg.Method != "session/set_mode" {
			t.Errorf("expected session/set_mode, got %q", msg.Method)
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for heartbeat (Case 1)")
	}

	// Case 2: Idle for 10s -> should NOT send heartbeat (threshold 45s)
	p.lastActivity.Store(time.Now().Add(-10 * time.Second).UnixNano())
	tickerChan <- time.Now()

	select {
	case msg := <-heartbeats:
		t.Errorf("unexpected heartbeat sent: %v", msg)
	case <-time.After(100 * time.Millisecond):
		// OK
	}

	// Case 3: Busy agent, idle for 50s -> should NOT send heartbeat
	p.promptMux.Lock()
	p.activePromptID = "prompt-123"
	p.promptMux.Unlock()
	p.lastActivity.Store(time.Now().Add(-50 * time.Second).UnixNano())
	tickerChan <- time.Now()

	select {
	case msg := <-heartbeats:
		t.Errorf("unexpected heartbeat sent while busy: %v", msg)
	case <-time.After(100 * time.Millisecond):
		// OK
	}

	// Case 4: Busy agent, idle for 70s -> FORCE RECOVERY -> should send heartbeat
	p.lastActivity.Store(time.Now().Add(-70 * time.Second).UnixNano())
	tickerChan <- time.Now()

	select {
	case msg := <-heartbeats:
		if msg.Method != "session/set_mode" {
			t.Errorf("expected session/set_mode after recovery, got %q", msg.Method)
		}
		if p.activePromptID != "" {
			t.Error("expected activePromptID to be cleared after recovery")
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for heartbeat after recovery (Case 4)")
	}

	// Case 5: Propelled mode -> should NOT send heartbeat
	p.SetPropelled(true)
	p.lastActivity.Store(time.Now().Add(-50 * time.Second).UnixNano())
	tickerChan <- time.Now()

	select {
	case msg := <-heartbeats:
		t.Errorf("unexpected heartbeat sent during propulsion: %v", msg)
	case <-time.After(100 * time.Millisecond):
		// OK
	}

	// Cleanup
	close(p.done)
	p.wg.Wait()
}
