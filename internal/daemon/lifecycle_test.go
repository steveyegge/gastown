package daemon

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
)

// testDaemon creates a minimal Daemon for testing.
func testDaemon() *Daemon {
	return &Daemon{
		config: &Config{TownRoot: "/tmp/test"},
		logger: log.New(io.Discard, "", 0), // silent logger for tests
	}
}

// testDaemonWithTown creates a Daemon with a proper town setup for testing.
// Returns the daemon and a cleanup function.
func testDaemonWithTown(t *testing.T, townName string) (*Daemon, func()) {
	t.Helper()
	townRoot := t.TempDir()

	// Create mayor directory and town.json
	mayorDir := filepath.Join(townRoot, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("failed to create mayor dir: %v", err)
	}
	townJSON := filepath.Join(mayorDir, "town.json")
	content := `{"name": "` + townName + `"}`
	if err := os.WriteFile(townJSON, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write town.json: %v", err)
	}

	d := &Daemon{
		config: &Config{TownRoot: townRoot},
		logger: log.New(io.Discard, "", 0),
	}

	return d, func() {
		// Cleanup handled by t.TempDir()
	}
}

func TestParseLifecycleRequest_Cycle(t *testing.T) {
	d := testDaemon()

	tests := []struct {
		subject  string
		body     string
		expected LifecycleAction
	}{
		// JSON body format
		{"LIFECYCLE: requesting action", `{"action": "cycle"}`, ActionCycle},
		// Simple text body format
		{"LIFECYCLE: requesting action", "cycle", ActionCycle},
		{"lifecycle: action request", "action: cycle", ActionCycle},
	}

	for _, tc := range tests {
		msg := &BeadsMessage{
			Subject: tc.subject,
			Body:    tc.body,
			From:    "test-sender",
		}
		result := d.parseLifecycleRequest(msg)
		if result == nil {
			t.Errorf("parseLifecycleRequest(subject=%q, body=%q) returned nil, expected action %s", tc.subject, tc.body, tc.expected)
			continue
		}
		if result.Action != tc.expected {
			t.Errorf("parseLifecycleRequest(subject=%q, body=%q) action = %s, expected %s", tc.subject, tc.body, result.Action, tc.expected)
		}
	}
}

func TestParseLifecycleRequest_RestartAndShutdown(t *testing.T) {
	// Verify that restart and shutdown are correctly parsed using structured body.
	d := testDaemon()

	tests := []struct {
		subject  string
		body     string
		expected LifecycleAction
	}{
		{"LIFECYCLE: action", `{"action": "restart"}`, ActionRestart},
		{"LIFECYCLE: action", `{"action": "shutdown"}`, ActionShutdown},
		{"lifecycle: action", "stop", ActionShutdown},
		{"LIFECYCLE: action", "restart", ActionRestart},
	}

	for _, tc := range tests {
		msg := &BeadsMessage{
			Subject: tc.subject,
			Body:    tc.body,
			From:    "test-sender",
		}
		result := d.parseLifecycleRequest(msg)
		if result == nil {
			t.Errorf("parseLifecycleRequest(subject=%q, body=%q) returned nil", tc.subject, tc.body)
			continue
		}
		if result.Action != tc.expected {
			t.Errorf("parseLifecycleRequest(subject=%q, body=%q) action = %s, expected %s", tc.subject, tc.body, result.Action, tc.expected)
		}
	}
}

func TestParseLifecycleRequest_NotLifecycle(t *testing.T) {
	d := testDaemon()

	tests := []string{
		"Regular message",
		"HEARTBEAT: check rigs",
		"lifecycle without colon",
		"Something else: requesting cycle",
		"",
	}

	for _, title := range tests {
		msg := &BeadsMessage{
			Subject: title,
			From:    "test-sender",
		}
		result := d.parseLifecycleRequest(msg)
		if result != nil {
			t.Errorf("parseLifecycleRequest(%q) = %+v, expected nil", title, result)
		}
	}
}

func TestParseLifecycleRequest_UsesFromField(t *testing.T) {
	d := testDaemon()

	// Now that we use structured body, the From field comes directly from the message
	tests := []struct {
		subject      string
		body         string
		sender       string
		expectedFrom string
	}{
		{"LIFECYCLE: action", `{"action": "cycle"}`, "mayor", "mayor"},
		{"LIFECYCLE: action", "restart", "gastown-witness", "gastown-witness"},
		{"lifecycle: action", "shutdown", "my-rig-refinery", "my-rig-refinery"},
	}

	for _, tc := range tests {
		msg := &BeadsMessage{
			Subject: tc.subject,
			Body:    tc.body,
			From:    tc.sender,
		}
		result := d.parseLifecycleRequest(msg)
		if result == nil {
			t.Errorf("parseLifecycleRequest(body=%q) returned nil", tc.body)
			continue
		}
		if result.From != tc.expectedFrom {
			t.Errorf("parseLifecycleRequest() from = %q, expected %q", result.From, tc.expectedFrom)
		}
	}
}

func TestParseLifecycleRequest_AlwaysUsesFromField(t *testing.T) {
	d := testDaemon()

	// With structured body parsing, From always comes from message From field
	msg := &BeadsMessage{
		Subject: "LIFECYCLE: action",
		Body:    "cycle",
		From:    "the-sender",
	}
	result := d.parseLifecycleRequest(msg)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.From != "the-sender" {
		t.Errorf("parseLifecycleRequest() from = %q, expected 'the-sender'", result.From)
	}
}

func TestIdentityToSession_Mayor(t *testing.T) {
	d, cleanup := testDaemonWithTown(t, "ai")
	defer cleanup()

	// Mayor session name is now fixed (one per machine, uses hq- prefix)
	result := d.identityToSession("mayor")
	if result != "hq-mayor" {
		t.Errorf("identityToSession('mayor') = %q, expected 'hq-mayor'", result)
	}
}

func TestIdentityToSession_Witness(t *testing.T) {
	d := testDaemon()

	tests := []struct {
		identity string
		expected string
	}{
		{"gastown-witness", "gt-gastown-witness"},
		{"myrig-witness", "gt-myrig-witness"},
		{"my-rig-name-witness", "gt-my-rig-name-witness"},
	}

	for _, tc := range tests {
		result := d.identityToSession(tc.identity)
		if result != tc.expected {
			t.Errorf("identityToSession(%q) = %q, expected %q", tc.identity, result, tc.expected)
		}
	}
}

func TestIdentityToSession_Unknown(t *testing.T) {
	d := testDaemon()

	tests := []string{
		"unknown",
		"polecat",
		"refinery",
		"gastown", // rig name without -witness
		"",
	}

	for _, identity := range tests {
		result := d.identityToSession(identity)
		if result != "" {
			t.Errorf("identityToSession(%q) = %q, expected empty string", identity, result)
		}
	}
}

func TestBeadsMessage_Serialization(t *testing.T) {
	msg := BeadsMessage{
		ID:       "msg-123",
		Subject:  "Test Message",
		Body:     "A test message body",
		From:     "test-sender",
		To:       "test-recipient",
		Priority: "high",
		Type:     "message",
	}

	// Verify all fields are accessible
	if msg.ID != "msg-123" {
		t.Errorf("ID mismatch")
	}
	if msg.Subject != "Test Message" {
		t.Errorf("Subject mismatch")
	}
	if msg.From != "test-sender" {
		t.Errorf("From mismatch")
	}
}
