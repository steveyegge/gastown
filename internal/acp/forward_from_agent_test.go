package acp

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestForwardFromAgent_StandardForwarding(t *testing.T) {
	p := NewProxy()
	p.done = make(chan struct{})

	// Pipes for agent output and proxy output
	agentStdoutReader, agentStdoutWriter, _ := os.Pipe()
	stdoutReader, stdoutWriter, _ := os.Pipe()

	p.agentStdout = agentStdoutReader
	p.stdout = stdoutWriter
	p.uiEncoder = json.NewEncoder(stdoutWriter)

	// Start forwarding
	p.wg.Add(1)
	go func() {
		p.forwardFromAgent()
		stdoutWriter.Close()
	}()

	// 1. Standard message (not propelled/injected)
	standardMsg := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "test/standard",
		Params:  json.RawMessage(`{"foo":"bar"}`),
	}
	standardMsgJSON, _ := json.Marshal(standardMsg)
	standardMsgJSON = append(standardMsgJSON, '\n')

	// 2. Injected response (propelled) - should NOT be forwarded
	injectedMsg := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      "gt-inject-test",
		Method:  "test/injected",
		Result:  json.RawMessage(`{}`),
	}
	injectedMsgJSON, _ := json.Marshal(injectedMsg)
	injectedMsgJSON = append(injectedMsgJSON, '\n')

	// 3. Non-injected message with string ID - should be forwarded
	stringIDMsg := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      "some-standard-id",
		Method:  "test/string-id",
		Result:  json.RawMessage(`{}`),
	}
	stringIDMsgJSON, _ := json.Marshal(stringIDMsg)
	stringIDMsgJSON = append(stringIDMsgJSON, '\n')

	// 4. Redacted thought message - should NOT be forwarded
	redactedMsg := JSONRPCMessage{
		JSONRPC: "2.0",
		Method:  "session/update",
		Params:  json.RawMessage(`{"update":{"sessionUpdate":"agent_thought_chunk","content":{"text":"[REDACTED]"}}}`),
	}
	redactedMsgJSON, _ := json.Marshal(redactedMsg)
	redactedMsgJSON = append(redactedMsgJSON, '\n')

	go func() {
		agentStdoutWriter.Write(standardMsgJSON)
		time.Sleep(100 * time.Millisecond)
		agentStdoutWriter.Write(injectedMsgJSON)
		time.Sleep(100 * time.Millisecond)
		agentStdoutWriter.Write(stringIDMsgJSON)
		time.Sleep(100 * time.Millisecond)
		agentStdoutWriter.Write(redactedMsgJSON)
		time.Sleep(100 * time.Millisecond)
		agentStdoutWriter.Close()
		// We don't close stdoutWriter here because forwardFromAgent might still be writing
	}()

	// Read received messages
	receivedMsgs := []JSONRPCMessage{}
	decoder := json.NewDecoder(stdoutReader)

	// Use a timeout for the entire reading process
	done := make(chan bool)
	go func() {
		for {
			var msg JSONRPCMessage
			if err := decoder.Decode(&msg); err != nil {
				break
			}
			receivedMsgs = append(receivedMsgs, msg)
		}
		done <- true
	}()

	select {
	case <-done:
		// Reading complete
	case <-time.After(3 * time.Second):
		t.Errorf("timeout waiting for messages")
	}

	if len(receivedMsgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(receivedMsgs))
		for i, msg := range receivedMsgs {
			t.Logf("msg[%d]: method=%q id=%v", i, msg.Method, msg.ID)
		}
	}

	foundStandard := false
	foundStringID := false
	for _, msg := range receivedMsgs {
		if msg.Method == "test/standard" {
			foundStandard = true
		}
		if msg.Method == "test/string-id" {
			foundStringID = true
		}
		if msg.Method == "test/injected" {
			t.Errorf("injected message should not have been forwarded")
		}
		if msg.Method == "session/update" {
			t.Errorf("redacted thought should not have been forwarded")
		}
	}

	if !foundStandard {
		t.Errorf("standard message not found")
	}
	if !foundStringID {
		t.Errorf("string-id message not found")
	}
}

func TestForwardFromAgent_PropulsionSuppression(t *testing.T) {
	p := NewProxy()
	p.done = make(chan struct{})
	p.Propelled.Store(true)

	// Pipes for agent output and proxy output
	agentStdoutReader, agentStdoutWriter, _ := os.Pipe()
	stdoutReader, stdoutWriter, _ := os.Pipe()

	p.agentStdout = agentStdoutReader
	p.stdout = stdoutWriter
	p.uiEncoder = json.NewEncoder(stdoutWriter)

	// Start forwarding
	p.wg.Add(1)
	go func() {
		p.forwardFromAgent()
		stdoutWriter.Close()
	}()

	// Standard message (should be suppressed because Propelled is true)
	standardMsg := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "test/standard",
		Params:  json.RawMessage(`{"foo":"bar"}`),
	}
	standardMsgJSON, _ := json.Marshal(standardMsg)
	standardMsgJSON = append(standardMsgJSON, '\n')

	go func() {
		agentStdoutWriter.Write(standardMsgJSON)
		time.Sleep(100 * time.Millisecond)
		agentStdoutWriter.Close()
	}()

	// Read received messages
	receivedMsgs := []JSONRPCMessage{}
	decoder := json.NewDecoder(stdoutReader)

	// Use a timeout
	done := make(chan bool)
	go func() {
		for {
			var msg JSONRPCMessage
			if err := decoder.Decode(&msg); err != nil {
				break
			}
			receivedMsgs = append(receivedMsgs, msg)
		}
		done <- true
	}()

	select {
	case <-done:
		// Reading complete
	case <-time.After(3 * time.Second):
		t.Errorf("timeout waiting for messages")
	}

	if len(receivedMsgs) != 0 {
		t.Errorf("expected 0 messages when propelled, got %d", len(receivedMsgs))
		for i, msg := range receivedMsgs {
			t.Logf("msg[%d]: method=%q id=%v", i, msg.Method, msg.ID)
		}
	}

	p.markDone()
}

func TestForwardFromAgent_PropulsionTriggers(t *testing.T) {
	p := NewProxy()
	p.done = make(chan struct{})

	// Pipes for agent output and proxy output
	agentStdoutReader, agentStdoutWriter, _ := os.Pipe()
	stdoutReader, stdoutWriter, _ := os.Pipe()

	p.agentStdout = agentStdoutReader
	p.stdout = stdoutWriter
	p.uiEncoder = json.NewEncoder(stdoutWriter)

	// Start forwarding
	p.wg.Add(1)
	go func() {
		p.forwardFromAgent()
		stdoutWriter.Close()
	}()

	// Triggers
	triggers := []string{
		"## 🚨 AUTONOMOUS WORK MODE 🚨\n",
		"→ PROPULSION PRINCIPLE: Work is on your hook. RUN IT.\n",
		"→ EXECUTE THIS STEP NOW.\n",
	}

	for _, trigger := range triggers {
		// Reset Propelled for each trigger to test them independently
		p.Propelled.Store(false)

		agentStdoutWriter.Write([]byte(trigger))

		// Give it a moment to process
		time.Sleep(100 * time.Millisecond)

		if !p.Propelled.Load() {
			t.Errorf("trigger %q did not set Propelled to true", trigger)
		}
	}

	// Test multi-line trigger split across writes
	p.Propelled.Store(false)
	agentStdoutWriter.Write([]byte("AUTONOMOUS\n"))
	time.Sleep(100 * time.Millisecond)
	if p.Propelled.Load() {
		t.Error("Propelled was set prematurely")
	}
	agentStdoutWriter.Write([]byte("WORK MODE\n"))
	time.Sleep(100 * time.Millisecond)
	if !p.Propelled.Load() {
		t.Error("Multi-line trigger split across writes failed to set Propelled to true")
	}

	// Test JSON notification trigger
	p.Propelled.Store(false)
	jsonTrigger := JSONRPCMessage{
		JSONRPC: "2.0",
		Method:  "session/update",
		Params:  json.RawMessage(`{"update":{"sessionUpdate":"agent_message_chunk","content":{"text":"## 🚨 AUTONOMOUS WORK MODE 🚨"}}}`),
	}
	jsonTriggerBytes, _ := json.Marshal(jsonTrigger)
	jsonTriggerBytes = append(jsonTriggerBytes, '\n')
	agentStdoutWriter.Write(jsonTriggerBytes)
	time.Sleep(100 * time.Millisecond)
	if !p.Propelled.Load() {
		t.Error("JSON notification trigger did not set Propelled to true")
	}

	// Test reset on prompt response
	p.activePromptID = "test-prompt"
	responseMsg := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      "test-prompt",
		Result:  json.RawMessage(`{}`),
	}
	responseBytes, _ := json.Marshal(responseMsg)
	responseBytes = append(responseBytes, '\n')
	agentStdoutWriter.Write(responseBytes)
	time.Sleep(100 * time.Millisecond)
	if p.Propelled.Load() {
		t.Error("Propelled was not reset after prompt response")
	}

	// 1. Verify that output is forwarded again after reset
	p.activePromptID = "test-prompt-2"
	msgAfterReset := JSONRPCMessage{
		JSONRPC: "2.0",
		Method:  "test/after-reset",
	}
	msgAfterResetBytes, _ := json.Marshal(msgAfterReset)
	msgAfterResetBytes = append(msgAfterResetBytes, '\n')
	agentStdoutWriter.Write(msgAfterResetBytes)
	time.Sleep(100 * time.Millisecond)

	// Read received messages to verify forwarding
	receivedMsgs := []JSONRPCMessage{}
	decoder := json.NewDecoder(stdoutReader)

	// We need to read everything that was sent to stdoutWriter
	// Note: previous messages in this test were also written to agentStdoutWriter
	// but many were suppressed.

	go func() {
		for {
			var msg JSONRPCMessage
			if err := decoder.Decode(&msg); err != nil {
				return
			}
			receivedMsgs = append(receivedMsgs, msg)
		}
	}()
	time.Sleep(100 * time.Millisecond)

	foundAfterReset := false
	for _, m := range receivedMsgs {
		if m.Method == "test/after-reset" {
			foundAfterReset = true
			break
		}
	}
	if !foundAfterReset {
		t.Error("Message after propulsion reset was not forwarded")
	}

	p.markDone()
	agentStdoutWriter.Close()

	// Clean up reader
	var buf [1024]byte
	for {
		_, err := stdoutReader.Read(buf[:])
		if err != nil {
			break
		}
	}
}
