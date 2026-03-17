//go:build !windows

package acp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestProxy_LargeMessageHandling(t *testing.T) {
	// 1MB message
	largeData := strings.Repeat("a", 1024*1024)
	msg := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "test/large",
		Params:  json.RawMessage(`{"data":"` + largeData + `"}`),
	}
	msgJSON, _ := json.Marshal(msg)
	msgJSON = append(msgJSON, '\n')

	t.Run("ForwardToAgent", func(t *testing.T) {
		p := NewProxy()
		p.done = make(chan struct{})

		stdinReader, stdinWriter, _ := os.Pipe()
		agentStdoutReader, agentStdinWriter, _ := os.Pipe()

		p.stdin = stdinReader
		p.agentStdin = agentStdinWriter

		// Start a dummy process so isProcessAlive() returns true
		ctx := context.Background()
		p.cmd = exec.CommandContext(ctx, "cat")
		if err := p.cmd.Start(); err != nil {
			t.Fatalf("failed to start dummy process: %v", err)
		}
		defer p.cmd.Process.Kill()

		p.wg.Add(1)
		go p.forwardToAgent()

		// Write large message to stdin
		go func() {
			stdinWriter.Write(msgJSON)
			stdinWriter.Close()
		}()

		// Read from agentStdoutReader (which is connected to p.agentStdin via our pipe)
		// Wait, agentStdoutReader should be the reader side of p.agentStdin
		// No, p.agentStdin is where we write. So we need to read from the other side of that pipe.

		received := make(chan []byte, 1)
		go func() {
			decoder := json.NewDecoder(agentStdoutReader)
			var receivedMsg JSONRPCMessage
			if err := decoder.Decode(&receivedMsg); err != nil {
				// Don't t.Errorf here, it might be called after test finishes
				return
			}
			data, _ := json.Marshal(receivedMsg)
			received <- data
		}()

		select {
		case data := <-received:
			var decodedMsg JSONRPCMessage
			if err := json.Unmarshal(data, &decodedMsg); err != nil {
				t.Fatalf("failed to unmarshal received message: %v", err)
			}
			var params map[string]string
			if err := json.Unmarshal(decodedMsg.Params, &params); err != nil {
				t.Fatalf("failed to unmarshal params: %v", err)
			}
			if params["data"] != largeData {
				t.Errorf("data mismatch: expected length %d, got %d", len(largeData), len(params["data"]))
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for large message forwardToAgent")
		}

		p.markDone()
		// We don't wait for wg because forwardToAgent might be blocked on ReadString
		// but since we closed stdinWriter, it should exit.
		p.wg.Wait()
	})

	t.Run("ForwardFromAgent", func(t *testing.T) {
		p := NewProxy()
		p.done = make(chan struct{})

		agentStdoutReader, agentStdoutWriter, _ := os.Pipe()
		stdoutReader, stdoutWriter, _ := os.Pipe()

		p.agentStdout = agentStdoutReader
		p.stdout = stdoutWriter
		p.uiEncoder = json.NewEncoder(stdoutWriter)

		p.wg.Add(1)
		go p.forwardFromAgent()

		// Write large message to agentStdout (via the writer side of the pipe)
		go func() {
			agentStdoutWriter.Write(msgJSON)
			agentStdoutWriter.Close()
		}()

		// Read from stdout (via the reader side of the pipe)
		received := make(chan []byte, 1)
		go func() {
			decoder := json.NewDecoder(stdoutReader)
			var receivedMsg JSONRPCMessage
			if err := decoder.Decode(&receivedMsg); err != nil {
				return
			}
			data, _ := json.Marshal(receivedMsg)
			received <- data
		}()

		select {
		case data := <-received:
			var decodedMsg JSONRPCMessage
			if err := json.Unmarshal(data, &decodedMsg); err != nil {
				t.Fatalf("failed to unmarshal received message: %v", err)
			}
			var params map[string]string
			if err := json.Unmarshal(decodedMsg.Params, &params); err != nil {
				t.Fatalf("failed to unmarshal params: %v", err)
			}
			if params["data"] != largeData {
				t.Errorf("data mismatch: expected length %d, got %d", len(largeData), len(params["data"]))
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for large message forwardFromAgent")
		}

		p.markDone()
		p.wg.Wait()
	})
}
