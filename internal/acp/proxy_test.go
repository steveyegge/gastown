//go:build !windows

package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestNewProxy(t *testing.T) {
	p := NewProxy()
	if p == nil {
		t.Fatal("NewProxy returned nil")
	}
	if p.done == nil {
		t.Error("done channel not initialized")
	}
}

func TestProxy_SessionID(t *testing.T) {
	p := NewProxy()

	if sid := p.SessionID(); sid != "" {
		t.Errorf("expected empty session ID, got %q", sid)
	}

	p.sessionMux.Lock()
	p.sessionID = "test-session-123"
	p.sessionMux.Unlock()

	if sid := p.SessionID(); sid != "test-session-123" {
		t.Errorf("expected session ID %q, got %q", "test-session-123", sid)
	}
}

func TestProxy_ExtractSessionID(t *testing.T) {
	tests := []struct {
		name    string
		msg     *JSONRPCMessage
		wantSID string
	}{
		{
			name: "session/new response with sessionId",
			msg: &JSONRPCMessage{
				ID:     1,
				Result: json.RawMessage(`{"sessionId":"sess_abc123"}`),
			},
			wantSID: "sess_abc123",
		},
		{
			name: "response without sessionId",
			msg: &JSONRPCMessage{
				ID:     2,
				Result: json.RawMessage(`{"other":"field"}`),
			},
			wantSID: "",
		},
		{
			name: "notification without ID",
			msg: &JSONRPCMessage{
				Method: "session/update",
				Params: json.RawMessage(`{"sessionId":"sess_xyz"}`),
			},
			wantSID: "",
		},
		{
			name: "request without result",
			msg: &JSONRPCMessage{
				ID:     3,
				Method: "session/prompt",
			},
			wantSID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewProxy()
			p.extractSessionID(tt.msg)

			if sid := p.SessionID(); sid != tt.wantSID {
				t.Errorf("expected session ID %q, got %q", tt.wantSID, sid)
			}
		})
	}
}

func TestProxy_InjectNotification(t *testing.T) {
	p := NewProxy()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer r.Close()

	// Use setStreams to inject our pipe
	p.setStreams(nil, w)

	p.sessionMux.Lock()
	p.sessionID = "test-session"
	p.sessionMux.Unlock()

	go func() {
		_ = p.InjectNotificationToUI("session/update", map[string]any{"status": "working"})
		w.Close()
	}()

	var buf bytes.Buffer
	io.Copy(&buf, r)

	var msg JSONRPCMessage
	if err := json.Unmarshal(buf.Bytes(), &msg); err != nil {
		t.Fatalf("failed to parse message: %v", err)
	}

	if msg.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %q", msg.JSONRPC)
	}
	if msg.Method != "session/update" {
		t.Errorf("expected method session/update, got %q", msg.Method)
	}

	var params map[string]any
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Fatalf("failed to parse params: %v", err)
	}
	if params["sessionId"] != "test-session" {
		t.Errorf("expected sessionId test-session, got %v", params["sessionId"])
	}
	if params["status"] != "working" {
		t.Errorf("expected status working, got %v", params["status"])
	}
}

func TestProxy_Shutdown(t *testing.T) {
	p := NewProxy()

	_, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	p.done = make(chan struct{})

	p.Shutdown()

	select {
	case <-p.done:
	case <-time.After(100 * time.Millisecond):
		t.Error("done channel not closed after shutdown")
	}
}

func TestProxy_MarkDone(t *testing.T) {
	p := NewProxy()
	p.done = make(chan struct{})

	p.markDone()
	p.markDone()

	select {
	case <-p.done:
	default:
		t.Error("done channel not closed after markDone")
	}
}

func TestJSONRPCMessage_MarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name string
		msg  JSONRPCMessage
	}{
		{
			name: "request",
			msg: JSONRPCMessage{
				JSONRPC: "2.0",
				ID:      1,
				Method:  "initialize",
				Params:  json.RawMessage(`{"protocolVersion":1}`),
			},
		},
		{
			name: "response with result",
			msg: JSONRPCMessage{
				JSONRPC: "2.0",
				ID:      1,
				Result:  json.RawMessage(`{"sessionId":"sess_123"}`),
			},
		},
		{
			name: "response with error",
			msg: JSONRPCMessage{
				JSONRPC: "2.0",
				ID:      1,
				Error: &JSONRPCError{
					Code:    -32600,
					Message: "Invalid Request",
				},
			},
		},
		{
			name: "notification",
			msg: JSONRPCMessage{
				JSONRPC: "2.0",
				Method:  "session/update",
				Params:  json.RawMessage(`{"sessionId":"sess_123"}`),
			},
		},
		{
			name: "string id",
			msg: JSONRPCMessage{
				JSONRPC: "2.0",
				ID:      "abc-123",
				Method:  "session/prompt",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(&tt.msg)
			if err != nil {
				t.Fatalf("failed to marshal: %v", err)
			}

			var got JSONRPCMessage
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			if got.JSONRPC != tt.msg.JSONRPC {
				t.Errorf("jsonrpc mismatch: got %q, want %q", got.JSONRPC, tt.msg.JSONRPC)
			}

			if tt.msg.Method != "" && got.Method != tt.msg.Method {
				t.Errorf("method mismatch: got %q, want %q", got.Method, tt.msg.Method)
			}
		})
	}
}

func TestProxy_StartProcessGroup(t *testing.T) {
	p := NewProxy()

	ctx := context.Background()
	p.cmd = p.createTestCmd(ctx)

	if p.cmd.SysProcAttr == nil {
		t.Error("SysProcAttr should be set for process group control")
	}
}

func (p *Proxy) createTestCmd(ctx context.Context) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "echo", "test")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	return cmd
}

func TestProxy_ConcurrentSessionID(t *testing.T) {
	p := NewProxy()

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(2)

		go func(id int) {
			defer wg.Done()
			p.sessionMux.Lock()
			p.sessionID = fmt.Sprintf("session-%d", id)
			p.sessionMux.Unlock()
		}(i)

		go func() {
			defer wg.Done()
			_ = p.SessionID()
		}()
	}

	wg.Wait()
}

func TestProxy_InjectNotification_NoSessionID(t *testing.T) {
	p := NewProxy()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer r.Close()

	// Use setStreams to inject our pipe
	p.setStreams(nil, w)

	go func() {
		_ = p.InjectNotificationToUI("test/notification", nil)
		w.Close()
	}()

	var buf bytes.Buffer
	io.Copy(&buf, r)

	if buf.Len() == 0 {
		t.Fatal("expected message to be written")
	}

	var msg JSONRPCMessage
	if err := json.Unmarshal(buf.Bytes(), &msg); err != nil {
		t.Fatalf("failed to parse message: %v", err)
	}

	if msg.Method != "test/notification" {
		t.Errorf("expected method test/notification, got %q", msg.Method)
	}
}

func TestProxy_InjectNotification_WithParams(t *testing.T) {
	p := NewProxy()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer r.Close()

	// Use setStreams to inject our pipe
	p.setStreams(nil, w)

	p.sessionMux.Lock()
	p.sessionID = "sess-test"
	p.sessionMux.Unlock()

	go func() {
		_ = p.InjectNotificationToUI("custom/method", map[string]any{
			"key1": "value1",
			"key2": 42,
		})
		w.Close()
	}()

	var buf bytes.Buffer
	io.Copy(&buf, r)

	var msg JSONRPCMessage
	if err := json.Unmarshal(buf.Bytes(), &msg); err != nil {
		t.Fatalf("failed to parse message: %v", err)
	}

	var params map[string]any
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Fatalf("failed to parse params: %v", err)
	}

	if params["sessionId"] != "sess-test" {
		t.Errorf("expected sessionId sess-test, got %v", params["sessionId"])
	}
	if params["key1"] != "value1" {
		t.Errorf("expected key1 value1, got %v", params["key1"])
	}
}

func TestProxy_AgentDone(t *testing.T) {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "true")

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start command: %v", err)
	}

	p := &Proxy{cmd: cmd}

	done := p.agentDone()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("command failed: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for command to complete")
	}
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

func TestJSONRPCError(t *testing.T) {
	rpcErr := &JSONRPCError{
		Code:    -32600,
		Message: "Invalid Request",
	}

	msg := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      1,
		Error:   rpcErr,
	}

	data, err := json.Marshal(&msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	if !strings.Contains(string(data), `"code":-32600`) {
		t.Error("error code not in marshaled output")
	}
	if !strings.Contains(string(data), `"message":"Invalid Request"`) {
		t.Error("error message not in marshaled output")
	}
}

func TestIntegration_HandshakeSequence(t *testing.T) {
	p := NewProxy()

	initResp := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      1,
		Result:  json.RawMessage(`{"protocolVersion":1,"capabilities":{}}`),
	}
	p.extractSessionID(initResp)

	if sid := p.SessionID(); sid != "" {
		t.Errorf("expected empty session ID after init, got %q", sid)
	}

	sessionResp := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      2,
		Result:  json.RawMessage(`{"sessionId":"test-session-12345"}`),
	}
	p.extractSessionID(sessionResp)

	if sid := p.SessionID(); sid != "test-session-12345" {
		t.Errorf("expected session ID test-session-12345, got %q", sid)
	}
}

func TestIntegration_StartupPromptInjection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	mockAgent := createMockACPAgent(t, true)
	defer os.Remove(mockAgent)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	p := NewProxy()
	testPrompt := "GAS TOWN INTEGRATION TEST PROMPT"
	p.SetStartupPrompt(testPrompt)

	tmpDir := t.TempDir()
	if err := p.Start(ctx, mockAgent, nil, tmpDir); err != nil {
		t.Fatalf("failed to start proxy: %v", err)
	}

	go func() {
		time.Sleep(200 * time.Millisecond)
		p.Shutdown()
	}()

	_ = p.Forward()

	if p.getStartupPrompt() != testPrompt {
		t.Errorf("startup prompt not set correctly")
	}
}

func TestIntegration_PropulsionNotificationFormat(t *testing.T) {
	p := NewProxy()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	defer r.Close()

	// Use setStreams to inject our pipe
	p.setStreams(nil, w)

	p.sessionMux.Lock()
	p.sessionID = "test-session-propulsion"
	p.sessionMux.Unlock()

	propulsionParams := map[string]any{
		"role":    "polecat",
		"rig":     "gastown",
		"message": "Polecat nux checking in",
	}

	go func() {
		_ = p.InjectNotificationToUI("session/update", propulsionParams)
		w.Close()
	}()

	var buf bytes.Buffer
	io.Copy(&buf, r)

	var msg JSONRPCMessage
	if err := json.Unmarshal(buf.Bytes(), &msg); err != nil {
		t.Fatalf("failed to parse message: %v", err)
	}

	if msg.Method != "session/update" {
		t.Errorf("expected method session/update, got %q", msg.Method)
	}

	var params map[string]any
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Fatalf("failed to parse params: %v", err)
	}

	if params["sessionId"] != "test-session-propulsion" {
		t.Errorf("expected sessionId test-session-propulsion, got %v", params["sessionId"])
	}
	if params["role"] != "polecat" {
		t.Errorf("expected role polecat, got %v", params["role"])
	}
}

func TestIntegration_CleanExitOnAgentTermination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	exitScript := createTempScript(t, "#!/bin/sh\nexit 0\n")
	defer os.Remove(exitScript)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	p := NewProxy()

	tmpDir := t.TempDir()
	if err := p.Start(ctx, exitScript, nil, tmpDir); err != nil {
		t.Fatalf("failed to start proxy: %v", err)
	}

	// Drain stdout to prevent blocking, since we aren't calling Forward()
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		_, _ = io.Copy(io.Discard, p.agentStdout)
	}()

	agentDone := p.agentDone()
	select {
	case err := <-agentDone:
		if err != nil {
			t.Errorf("agent exited with error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("timeout waiting for agent to terminate")
	}

	p.markDone()
}

func TestIntegration_NonACPAgent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	nonACPAgent := createTempScript(t, "#!/bin/sh\necho 'not jsonrpc'\nexit 0\n")
	defer os.Remove(nonACPAgent)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	p := NewProxy()

	tmpDir := t.TempDir()
	if err := p.Start(ctx, nonACPAgent, nil, tmpDir); err != nil {
		t.Fatalf("failed to start proxy: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- p.Forward()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Logf("non-ACP agent returned error: %v (expected)", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for non-ACP agent")
	}

	if sid := p.SessionID(); sid != "" {
		t.Errorf("expected empty session ID for non-ACP agent, got %q", sid)
	}
}

func createMockACPAgent(t *testing.T, validACP bool) string {
	t.Helper()

	var script string
	if validACP {
		script = `#!/bin/sh
# Mock ACP agent for integration testing
while IFS= read -r line; do
    # Parse the JSON-RPC request
    method=$(echo "$line" | grep -o '"method":"[^"]*"' | cut -d'"' -f4)
    id=$(echo "$line" | grep -o '"id":[0-9]*' | cut -d: -f2)

    case "$method" in
        initialize)
            echo '{"jsonrpc":"2.0","id":'$id',"result":{"protocolVersion":1,"capabilities":{}}}'
            ;;
        session/new)
            echo '{"jsonrpc":"2.0","id":'$id',"result":{"sessionId":"test-session-12345"}}'
            ;;
        session/prompt)
            echo '{"jsonrpc":"2.0","id":'$id',"result":{}}'
            ;;
        *)
            echo '{"jsonrpc":"2.0","id":'$id',"result":{}}'
            ;;
    esac
done
`
	} else {
		script = "#!/bin/sh\necho 'not a valid ACP response'\n"
	}

	return createTempScript(t, script)
}

// MockAgent simulates an ACP-compatible agent for testing.
// It reads JSON-RPC messages from In and writes responses to Out.
type MockAgent struct {
	t            *testing.T
	sessionID    string
	protocolVer  int
	capabilities map[string]any
	in           *bufio.Reader
	out          io.Writer
	err          io.Writer

	// mu protects the fields below
	mu sync.Mutex
	// Handlers can be provided to override default behavior for specific methods
	Handlers map[string]func(*JSONRPCMessage) *JSONRPCMessage
	// MessagesReceived captures all messages received by the agent
	MessagesReceived []*JSONRPCMessage
	// ResponsesSent captures all responses sent by the agent
	ResponsesSent []*JSONRPCMessage
}

func NewMockAgent(t *testing.T, in io.Reader, out io.Writer) *MockAgent {
	return &MockAgent{
		t:            t,
		sessionID:    "mock-session-id-12345",
		protocolVer:  1,
		capabilities: make(map[string]any),
		in:           bufio.NewReader(in),
		out:          out,
		Handlers:     make(map[string]func(*JSONRPCMessage) *JSONRPCMessage),
	}
}

func (m *MockAgent) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			err := m.Step()
			if err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
		}
	}
}

func (m *MockAgent) Step() error {
	line, err := m.in.ReadString('\n')
	if err != nil {
		return err
	}

	var msg JSONRPCMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return fmt.Errorf("unmarshal request: %w", err)
	}

	m.mu.Lock()
	m.MessagesReceived = append(m.MessagesReceived, &msg)
	handler, ok := m.Handlers[msg.Method]
	m.mu.Unlock()

	var resp *JSONRPCMessage
	if ok {
		resp = handler(&msg)
	} else {
		resp = m.defaultHandler(&msg)
	}

	if resp != nil {
		m.mu.Lock()
		m.ResponsesSent = append(m.ResponsesSent, resp)
		m.mu.Unlock()

		data, err := json.Marshal(resp)
		if err != nil {
			return fmt.Errorf("marshal response: %w", err)
		}
		if _, err := fmt.Fprintf(m.out, "%s\n", data); err != nil {
			return fmt.Errorf("write response: %w", err)
		}
	}

	return nil
}

func (m *MockAgent) defaultHandler(msg *JSONRPCMessage) *JSONRPCMessage {
	switch msg.Method {
	case "initialize":
		return &JSONRPCMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result:  json.RawMessage(fmt.Sprintf(`{"protocolVersion":%d,"capabilities":{}}`, m.protocolVer)),
		}
	case "session/new":
		return &JSONRPCMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result:  json.RawMessage(fmt.Sprintf(`{"sessionId":"%s"}`, m.sessionID)),
		}
	case "session/prompt":
		return &JSONRPCMessage{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result:  json.RawMessage(`{}`),
		}
	case "session/cancel":
		return nil // Notifications don't get responses
	default:
		// For requests with IDs, send a generic successful result if not handled
		if msg.ID != nil {
			return &JSONRPCMessage{
				JSONRPC: "2.0",
				ID:      msg.ID,
				Result:  json.RawMessage(`{}`),
			}
		}
		return nil
	}
}

func (m *MockAgent) GetMessages() []*JSONRPCMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	msgs := make([]*JSONRPCMessage, len(m.MessagesReceived))
	copy(msgs, m.MessagesReceived)
	return msgs
}

func setupProxyWithMockAgent(t *testing.T) (*Proxy, *MockAgent) {
	p := NewProxy()

	// Agent side pipes: Proxy's perspective
	// Proxy.agentStdin (io.WriteCloser) -> agentStdinR (io.Reader)
	// agentStdoutW (io.Writer) -> Proxy.agentStdout (io.ReadCloser)
	agentStdinR, agentStdinW := io.Pipe()
	agentStdoutR, agentStdoutW := io.Pipe()

	p.agentStdin = agentStdinW
	p.agentStdout = agentStdoutR

	m := NewMockAgent(t, agentStdinR, agentStdoutW)

	// Mock p.cmd by starting a real process so that isProcessAlive() returns true.
	// We use a command that doesn't do much and will be killed on shutdown.
	p.cmd = exec.CommandContext(context.Background(), "sleep", "60")
	p.setupProcessGroup() // Ensure it's in its own group
	if err := p.cmd.Start(); err != nil {
		t.Fatalf("failed to start mock command: %v", err)
	}

	return p, m
}

func TestProxy_HandshakeWithMockAgent(t *testing.T) {
	p, m := setupProxyWithMockAgent(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// UI side pipes - needed to avoid panics in forwardFromAgent when encoding to UI
	uiStdoutR, uiStdoutW := io.Pipe()
	p.setStreams(nil, uiStdoutW)
	go func() {
		_, _ = io.Copy(io.Discard, uiStdoutR)
	}()

	go func() {
		_ = m.Run(ctx)
	}()

	// Run forwardFromAgent to process responses from the mock agent
	p.wg.Add(1)
	go p.forwardFromAgent()

	// 1. Send initialize
	initReq := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":1}`),
	}
	if err := p.writeToAgent(initReq); err != nil {
		t.Fatalf("failed to write initialize: %v", err)
	}

	// 2. Send session/new
	sessionReq := &JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "session/new",
	}
	if err := p.writeToAgent(sessionReq); err != nil {
		t.Fatalf("failed to write session/new: %v", err)
	}

	// Wait for session ID to be extracted
	waitCtx, waitCancel := context.WithTimeout(ctx, 2*time.Second)
	defer waitCancel()
	err := p.WaitForSessionID(waitCtx)
	if err != nil {
		t.Fatalf("failed to get session ID: %v", err)
	}

	if sid := p.SessionID(); sid != "mock-session-id-12345" {
		t.Errorf("expected session ID mock-session-id-12345, got %q", sid)
	}

	p.Shutdown()
}

func TestIntegration_FullLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	mockAgent := createMockACPAgent(t, true)
	defer os.Remove(mockAgent)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	p := NewProxy()

	// Capture proxy's stdout (UI side)
	var uiStdout bytes.Buffer
	p.stdoutMux.Lock()
	p.setStreams(nil, &uiStdout)
	p.stdoutMux.Unlock()

	// Input from UI side - using a pipe to control EOF
	uiStdinR, uiStdinW := io.Pipe()
	p.stdin = uiStdinR

	tmpDir := t.TempDir()
	if err := p.Start(ctx, mockAgent, nil, tmpDir); err != nil {
		t.Fatalf("failed to start proxy: %v", err)
	}

	// 1. Send messages from the UI
	go func() {
		defer uiStdinW.Close()
		messages := []string{
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1}}`,
			`{"jsonrpc":"2.0","id":2,"method":"session/new"}`,
			`{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"prompt":[{"type":"text","text":"hello"}]}}`,
		}
		for _, m := range messages {
			fmt.Fprintln(uiStdinW, m)
			time.Sleep(50 * time.Millisecond) // Give time for processing
		}

		// Wait for session ID to be set (completes handshake)
		waitCtx, waitCancel := context.WithTimeout(ctx, 2*time.Second)
		defer waitCancel()
		if err := p.WaitForSessionID(waitCtx); err != nil {
			t.Errorf("failed to get session ID: %v", err)
			p.Shutdown()
			return
		}

		// Wait a bit more for the prompt response to flow through
		time.Sleep(500 * time.Millisecond)
		p.Shutdown()
	}()

	err := p.Forward()
	if err != nil && !strings.Contains(err.Error(), "signal: killed") && !strings.Contains(err.Error(), "context canceled") {
		t.Logf("Forward() returned error (expected during shutdown): %v", err)
	}

	// Verify outputs
	output := uiStdout.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// We expect 3 responses: initialize, session/new, session/prompt
	if len(lines) < 3 {
		t.Errorf("expected at least 3 responses, got %d. Output:\n%s", len(lines), output)
	}

	// Verify all responses reached the UI
	expectedIDs := map[float64]bool{1: false, 2: false, 3: false}
	for _, line := range lines {
		if line == "" {
			continue
		}
		var msg JSONRPCMessage
		if err := json.Unmarshal([]byte(line), &msg); err == nil {
			if id, ok := msg.ID.(float64); ok {
				expectedIDs[id] = true
			}
		}
	}

	for id, found := range expectedIDs {
		if !found {
			t.Errorf("did not find response for ID %.0f in output:\n%s", id, output)
		}
	}

	// Verify session ID was captured
	if sid := p.SessionID(); sid != "test-session-12345" {
		t.Errorf("expected session ID test-session-12345, got %q", sid)
	}
}

func createTempScript(t *testing.T, content string) string {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "mock-agent-*.sh")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	if err := os.Chmod(tmpFile.Name(), 0755); err != nil {
		t.Fatalf("failed to chmod script: %v", err)
	}

	return tmpFile.Name()
}
