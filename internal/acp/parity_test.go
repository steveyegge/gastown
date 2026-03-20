package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"runtime"
	"testing"
)

type mockWriteCloser struct {
	io.Writer
}

func (m *mockWriteCloser) Close() error { return nil }

func TestWriteToAgent_Parity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ACP parity test requires Unix process groups and sleep command")
	}

	tests := []struct {
		name string
		msg  *JSONRPCMessage
	}{
		{
			name: "simple request",
			msg: &JSONRPCMessage{
				JSONRPC: "2.0",
				ID:      1,
				Method:  "test",
				Params:  json.RawMessage(`{"foo":"bar"}`),
			},
		},
		{
			name: "unicode",
			msg: &JSONRPCMessage{
				JSONRPC: "2.0",
				ID:      4,
				Method:  "unicode",
				Params:  json.RawMessage(`{"emoji":"🚀","japanese":"こんにちは"}`),
			},
		},
		{
			name: "special characters",
			msg: &JSONRPCMessage{
				JSONRPC: "2.0",
				ID:      3,
				Method:  "chars",
				Params:  json.RawMessage(`{"text":"hello\nworld\t\"quoted\""}`),
			},
		},
		{
			name: "large complex message",
			msg: &JSONRPCMessage{
				JSONRPC: "2.0",
				ID:      100,
				Method:  "complex",
				Params:  json.RawMessage(`{"array":[1,2,3,{"nested":"value"}],"bool":true,"null":null,"string":"lots of characters: !@#$%^&*()_+=-{}[]|\\:;\"'<>,.?/~` + "`" + " 🚀🍱🍙🍮🍜🔥💯" + `"}`),
			},
		},
		{
			name: "string ID",
			msg: &JSONRPCMessage{
				JSONRPC: "2.0",
				ID:      "req-123",
				Method:  "test",
				Params:  json.RawMessage(`{}`),
			},
		},
		{
			name: "no ID (notification)",
			msg: &JSONRPCMessage{
				JSONRPC: "2.0",
				Method:  "notify",
				Params:  json.RawMessage(`{}`),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewProxy()

			// Mock process as alive
			cmd := exec.CommandContext(context.Background(), "sleep", "1")
			p.cmd = cmd
			p.setupProcessGroup()
			if err := cmd.Start(); err != nil {
				t.Fatalf("failed to start process: %v", err)
			}
			defer cmd.Process.Kill()

			var buf bytes.Buffer
			p.agentStdin = &mockWriteCloser{&buf}

			if err := p.writeToAgent(tt.msg); err != nil {
				t.Fatalf("writeToAgent failed: %v", err)
			}

			var gotMsg JSONRPCMessage
			if err := json.Unmarshal(buf.Bytes(), &gotMsg); err != nil {
				t.Fatalf("failed to unmarshal output: %v", err)
			}

			// Compare fields
			if gotMsg.JSONRPC != tt.msg.JSONRPC {
				t.Errorf("jsonrpc mismatch: got %q, want %q", gotMsg.JSONRPC, tt.msg.JSONRPC)
			}
			if gotMsg.Method != tt.msg.Method {
				t.Errorf("method mismatch: got %q, want %q", gotMsg.Method, tt.msg.Method)
			}

			wP, _ := json.Marshal(tt.msg.Params)
			gP, _ := json.Marshal(gotMsg.Params)
			// Unmarshal to any to normalize
			var w, g any
			json.Unmarshal(wP, &w)
			json.Unmarshal(gP, &g)
			wN, _ := json.Marshal(w)
			gN, _ := json.Marshal(g)
			if string(wN) != string(gN) {
				t.Errorf("params mismatch: got %s, want %s", string(gN), string(wN))
			}
		})
	}
}
