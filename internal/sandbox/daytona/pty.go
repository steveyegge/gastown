// Package daytona provides a Go client for the Daytona API.
package daytona

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// PtyResult contains the result of a PTY session.
type PtyResult struct {
	ExitCode int
	Error    string
}

// PtySize represents the dimensions of a PTY terminal.
type PtySize struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// PtyHandle manages a WebSocket connection to a PTY session.
// It provides methods for sending input, receiving output, and managing the session lifecycle.
type PtyHandle struct {
	conn      *websocket.Conn
	sessionID string
	sandboxID string
	client    *Client // Reference to parent client for resize/kill operations

	mu                    sync.RWMutex
	connected             bool
	connectionEstablished bool
	exitCode              *int
	lastError             string

	// Output buffer for CaptureOutput
	outputMu     sync.Mutex
	outputBuffer []byte
	maxBufferLen int
}

// controlMessage represents a control message from the PTY server.
type controlMessage struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// closeData represents the data sent in a WebSocket close frame.
type closeData struct {
	ExitCode   *int   `json:"exitCode,omitempty"`
	ExitReason string `json:"exitReason,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ConnectPty establishes a WebSocket connection to a PTY session.
func (c *Client) ConnectPty(ctx context.Context, sandboxID, sessionID string) (*PtyHandle, error) {
	// Build WebSocket URL using toolbox URL
	// Convert https:// to wss:// or http:// to ws://
	wsURL := c.toolboxURL
	if strings.HasPrefix(wsURL, "https://") {
		wsURL = "wss://" + strings.TrimPrefix(wsURL, "https://")
	} else if strings.HasPrefix(wsURL, "http://") {
		wsURL = "ws://" + strings.TrimPrefix(wsURL, "http://")
	}

	// PTY WebSocket endpoint - matches SDK path: /{sandboxId}/process/pty/{sessionId}/connect
	wsURL = fmt.Sprintf("%s/%s/process/pty/%s/connect",
		wsURL, url.PathEscape(sandboxID), url.PathEscape(sessionID))

	// Set up WebSocket dialer with auth header
	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.apiKey)
	if c.orgID != "" {
		header.Set("X-Daytona-Organization-ID", c.orgID)
	}

	// Dial WebSocket
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.DialContext(ctx, wsURL, header)
	if err != nil {
		return nil, fmt.Errorf("connecting to PTY websocket: %w", err)
	}

	handle := &PtyHandle{
		conn:         conn,
		sessionID:    sessionID,
		sandboxID:    sandboxID,
		client:       c,
		connected:    true,
		maxBufferLen: 1024 * 1024, // 1MB output buffer
	}

	// Start the message reader goroutine
	handle.StartMessageReader()

	return handle, nil
}

// SessionID returns the PTY session ID.
func (h *PtyHandle) SessionID() string {
	return h.sessionID
}

// ExitCode returns the exit code if the PTY has terminated.
func (h *PtyHandle) ExitCode() *int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.exitCode
}

// Error returns the last error message.
func (h *PtyHandle) Error() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastError
}

// IsConnected returns true if the WebSocket connection is active.
func (h *PtyHandle) IsConnected() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.connected && h.conn != nil
}

// StartMessageReader starts a goroutine that reads messages from the WebSocket.
// This must be called before WaitForConnection.
func (h *PtyHandle) StartMessageReader() {
	go h.readMessages()
}

// readMessages continuously reads messages and updates connection state.
func (h *PtyHandle) readMessages() {
	defer func() {
		if r := recover(); r != nil {
			h.mu.Lock()
			h.connected = false
			h.lastError = fmt.Sprintf("websocket panic: %v", r)
			h.mu.Unlock()
		}
	}()

	for {
		h.mu.RLock()
		if !h.connected || h.conn == nil {
			h.mu.RUnlock()
			return
		}
		conn := h.conn
		h.mu.RUnlock()

		// Read with no deadline - block until message arrives
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			h.mu.Lock()
			h.connected = false
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				h.lastError = err.Error()
			}
			h.mu.Unlock()
			return
		}

		// Handle text messages (control messages)
		if messageType == websocket.TextMessage {
			var ctrl controlMessage
			if err := json.Unmarshal(message, &ctrl); err == nil && ctrl.Type == "control" {
				h.handleControlMessage(&ctrl)
				continue
			}
		}

		// Buffer binary output data
		h.appendToBuffer(message)
	}
}

// WaitForConnection waits for the PTY connection to be established.
// StartMessageReader must be called before this.
func (h *PtyHandle) WaitForConnection(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		h.mu.RLock()
		if h.connectionEstablished {
			h.mu.RUnlock()
			return nil
		}
		if h.lastError != "" {
			err := h.lastError
			h.mu.RUnlock()
			return fmt.Errorf("connection failed: %s", err)
		}
		if !h.connected {
			h.mu.RUnlock()
			return fmt.Errorf("connection closed during setup")
		}
		h.mu.RUnlock()

		time.Sleep(50 * time.Millisecond)
	}

	return fmt.Errorf("PTY connection timeout")
}

// SendInput sends input data to the PTY.
func (h *PtyHandle) SendInput(data string) error {
	if !h.IsConnected() {
		return fmt.Errorf("PTY is not connected")
	}

	// Send as binary message (like Python implementation)
	if err := h.conn.WriteMessage(websocket.BinaryMessage, []byte(data)); err != nil {
		return fmt.Errorf("failed to send input: %w", err)
	}

	return nil
}

// SendInputBytes sends raw bytes to the PTY.
func (h *PtyHandle) SendInputBytes(data []byte) error {
	if !h.IsConnected() {
		return fmt.Errorf("PTY is not connected")
	}

	if err := h.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return fmt.Errorf("failed to send input: %w", err)
	}

	return nil
}

// Resize resizes the PTY terminal.
func (h *PtyHandle) Resize(size PtySize) (*PtySessionInfo, error) {
	if h.client == nil {
		return nil, fmt.Errorf("resize handler not available")
	}

	return h.client.ResizePtySession(context.Background(), h.sandboxID, h.sessionID, size.Cols, size.Rows)
}

// Kill terminates the PTY process.
func (h *PtyHandle) Kill() error {
	if h.client == nil {
		return fmt.Errorf("kill handler not available")
	}

	return h.client.DeletePtySession(context.Background(), h.sandboxID, h.sessionID)
}

// ReadOutput reads available output from the PTY.
// This is a non-blocking read that returns whatever data is available.
func (h *PtyHandle) ReadOutput(timeout time.Duration) (data []byte, err error) {
	// Recover from panics in websocket read
	defer func() {
		if r := recover(); r != nil {
			h.mu.Lock()
			h.connected = false
			h.mu.Unlock()
			err = fmt.Errorf("websocket panic: %v", r)
		}
	}()

	h.mu.RLock()
	if !h.connected || h.conn == nil {
		h.mu.RUnlock()
		return nil, fmt.Errorf("PTY is not connected")
	}
	conn := h.conn
	h.mu.RUnlock()

	// Clear any previous deadline first, then set new one
	conn.SetReadDeadline(time.Time{})
	conn.SetReadDeadline(time.Now().Add(timeout))
	messageType, message, err := conn.ReadMessage()
	if err != nil {
		// Check for timeout first (most common case)
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return nil, err // Return timeout error, let caller handle
		}
		if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
			h.handleClose(err)
			return nil, nil
		}
		// Check if it's a close error
		if netErr, ok := err.(*websocket.CloseError); ok {
			h.handleClose(netErr)
			return nil, nil
		}
		// Mark as disconnected on any other error
		h.mu.Lock()
		h.connected = false
		h.mu.Unlock()
		return nil, err
	}

	// Handle text messages (might be control messages)
	if messageType == websocket.TextMessage {
		var ctrl controlMessage
		if err := json.Unmarshal(message, &ctrl); err == nil && ctrl.Type == "control" {
			h.handleControlMessage(&ctrl)
			return nil, nil // Control message, no output data
		}
	}

	// Store in output buffer
	h.appendToBuffer(message)

	return message, nil
}

// StreamOutput continuously reads output and calls the callback for each chunk.
// This blocks until the connection is closed or context is canceled.
func (h *PtyHandle) StreamOutput(ctx context.Context, callback func(data []byte)) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if !h.IsConnected() {
			return nil
		}

		data, err := h.ReadOutput(100 * time.Millisecond)
		if err != nil {
			// Check if it's a timeout error (expected, continue reading)
			// Use net.Error interface for proper timeout detection
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			// Also check error string as fallback
			errStr := err.Error()
			if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "i/o timeout") {
				continue
			}
			// "not connected" error means we should stop gracefully
			if strings.Contains(errStr, "not connected") {
				return nil
			}
			// Any other error means connection is broken
			h.mu.Lock()
			h.connected = false
			h.mu.Unlock()
			return err
		}
		if data != nil && callback != nil {
			callback(data)
		}
	}
}

// Wait waits for the PTY process to exit.
func (h *PtyHandle) Wait(ctx context.Context, onData func(data []byte)) (*PtyResult, error) {
	for {
		select {
		case <-ctx.Done():
			return &PtyResult{
				ExitCode: -1,
				Error:    ctx.Err().Error(),
			}, ctx.Err()
		default:
		}

		if !h.IsConnected() {
			break
		}

		data, err := h.ReadOutput(100 * time.Millisecond)
		if err != nil {
			continue // Timeout, keep waiting
		}
		if data != nil && onData != nil {
			onData(data)
		}
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	exitCode := 0
	if h.exitCode != nil {
		exitCode = *h.exitCode
	}

	return &PtyResult{
		ExitCode: exitCode,
		Error:    h.lastError,
	}, nil
}

// GetBufferedOutput returns the buffered output data.
func (h *PtyHandle) GetBufferedOutput() []byte {
	h.outputMu.Lock()
	defer h.outputMu.Unlock()
	result := make([]byte, len(h.outputBuffer))
	copy(result, h.outputBuffer)
	return result
}

// ClearBuffer clears the output buffer.
func (h *PtyHandle) ClearBuffer() {
	h.outputMu.Lock()
	defer h.outputMu.Unlock()
	h.outputBuffer = nil
}

// Disconnect closes the WebSocket connection.
func (h *PtyHandle) Disconnect() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.conn != nil {
		h.conn.Close()
		h.conn = nil
	}
	h.connected = false
}

// handleControlMessage processes control messages from the PTY server.
func (h *PtyHandle) handleControlMessage(ctrl *controlMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()

	switch ctrl.Status {
	case "connected":
		h.connectionEstablished = true
	case "error":
		h.lastError = ctrl.Error
		if h.lastError == "" {
			h.lastError = "Unknown connection error"
		}
		h.connected = false
	}
}

// handleClose processes WebSocket close events.
func (h *PtyHandle) handleClose(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.connected = false

	// Try to extract close data from the error
	if closeErr, ok := err.(*websocket.CloseError); ok {
		// Parse structured exit data from close reason
		if closeErr.Text != "" {
			var data closeData
			if json.Unmarshal([]byte(closeErr.Text), &data) == nil {
				if data.ExitCode != nil {
					h.exitCode = data.ExitCode
				}
				if data.ExitReason != "" {
					h.lastError = data.ExitReason
				}
				if data.Error != "" {
					h.lastError = data.Error
				}
			}
		}

		// Default to exit code 0 for normal close
		if h.exitCode == nil && closeErr.Code == websocket.CloseNormalClosure {
			code := 0
			h.exitCode = &code
		}
	}
}

// appendToBuffer adds data to the output buffer.
func (h *PtyHandle) appendToBuffer(data []byte) {
	h.outputMu.Lock()
	defer h.outputMu.Unlock()

	h.outputBuffer = append(h.outputBuffer, data...)

	// Trim buffer if it exceeds max length
	if len(h.outputBuffer) > h.maxBufferLen {
		// Keep the last maxBufferLen bytes
		h.outputBuffer = h.outputBuffer[len(h.outputBuffer)-h.maxBufferLen:]
	}
}

// ResizePtySession resizes a PTY session via the REST API.
func (c *Client) ResizePtySession(ctx context.Context, sandboxID, sessionID string, cols, rows int) (*PtySessionInfo, error) {
	path := fmt.Sprintf("/%s/process/pty/%s/resize?cols=%d&rows=%d",
		sandboxID, sessionID, cols, rows)
	resp, err := c.doToolboxRequest(ctx, http.MethodPost, path, nil)
	if err != nil {
		return nil, err
	}

	var result PtySessionInfo
	if err := parseResponse(resp, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
