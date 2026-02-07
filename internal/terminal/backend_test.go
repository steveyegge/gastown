package terminal

import (
	"fmt"
	"testing"
)

// TestTmuxBackendImplementsInterface verifies TmuxBackend satisfies Backend.
func TestTmuxBackendImplementsInterface(t *testing.T) {
	var _ Backend = (*TmuxBackend)(nil)
}

// TestSSHBackendImplementsInterface verifies SSHBackend satisfies Backend.
func TestSSHBackendImplementsInterface(t *testing.T) {
	var _ Backend = (*SSHBackend)(nil)
}

func TestSSHBackendArgs(t *testing.T) {
	b := NewSSHBackend(SSHConfig{
		Host:         "gt@pod.namespace.svc",
		Port:         2222,
		IdentityFile: "/tmp/id_rsa",
	})

	args := b.sshArgs()

	// Should contain port
	foundPort := false
	for i, a := range args {
		if a == "-p" && i+1 < len(args) && args[i+1] == "2222" {
			foundPort = true
		}
	}
	if !foundPort {
		t.Errorf("expected -p 2222 in args, got %v", args)
	}

	// Should contain identity file
	foundKey := false
	for i, a := range args {
		if a == "-i" && i+1 < len(args) && args[i+1] == "/tmp/id_rsa" {
			foundKey = true
		}
	}
	if !foundKey {
		t.Errorf("expected -i /tmp/id_rsa in args, got %v", args)
	}

	// Should end with host
	if args[len(args)-1] != "gt@pod.namespace.svc" {
		t.Errorf("expected host as last arg, got %s", args[len(args)-1])
	}
}

func TestSSHBackendDefaultPort(t *testing.T) {
	b := NewSSHBackend(SSHConfig{Host: "gt@pod"})
	if b.Port != 22 {
		t.Errorf("expected default port 22, got %d", b.Port)
	}
}

func TestSSHBackendProxyCommand(t *testing.T) {
	b := NewSSHBackend(SSHConfig{
		Host:         "gt@localhost",
		ProxyCommand: "kubectl port-forward pod/test 2222:22",
	})

	args := b.sshArgs()
	foundProxy := false
	for i, a := range args {
		if a == "-o" && i+1 < len(args) {
			if args[i+1] == "ProxyCommand=kubectl port-forward pod/test 2222:22" {
				foundProxy = true
			}
		}
	}
	if !foundProxy {
		t.Errorf("expected ProxyCommand in args, got %v", args)
	}
}

func TestParsePort(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"22", 22},
		{"2222", 2222},
		{"", 22},
		{"invalid", 22},
		{"-1", 22},
		{"0", 22},
	}

	for _, tt := range tests {
		got := parsePort(tt.input)
		if got != tt.expected {
			t.Errorf("parsePort(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestLocalBackend(t *testing.T) {
	b := LocalBackend()
	if _, ok := b.(*TmuxBackend); !ok {
		t.Errorf("LocalBackend() should return *TmuxBackend, got %T", b)
	}
}

// --- parseSSHConfig Tests ---

func TestParseSSHConfig_Empty(t *testing.T) {
	cfg, err := parseSSHConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil config for empty input")
	}
}

func TestParseSSHConfig_NoK8s(t *testing.T) {
	cfg, err := parseSSHConfig("backend: local\nssh_host: something")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil config when 'k8s' not present")
	}
}

func TestParseSSHConfig_FullConfig(t *testing.T) {
	input := `backend: k8s
ssh_host: gt@gt-gastown-toast.gastown.svc.cluster.local
ssh_port: 2222
ssh_key: /tmp/id_rsa`

	cfg, err := parseSSHConfig(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.Host != "gt@gt-gastown-toast.gastown.svc.cluster.local" {
		t.Errorf("Host = %q, want %q", cfg.Host, "gt@gt-gastown-toast.gastown.svc.cluster.local")
	}
	if cfg.Port != 2222 {
		t.Errorf("Port = %d, want %d", cfg.Port, 2222)
	}
	if cfg.IdentityFile != "/tmp/id_rsa" {
		t.Errorf("IdentityFile = %q, want %q", cfg.IdentityFile, "/tmp/id_rsa")
	}
}

func TestParseSSHConfig_DefaultPort(t *testing.T) {
	input := `backend: k8s
ssh_host: gt@pod.namespace.svc`

	cfg, err := parseSSHConfig(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 22 {
		t.Errorf("Port = %d, want 22 (default)", cfg.Port)
	}
}

func TestParseSSHConfig_MissingHost(t *testing.T) {
	input := `backend: k8s
ssh_port: 2222`

	_, err := parseSSHConfig(input)
	if err == nil {
		t.Fatal("expected error for missing ssh_host")
	}
}

func TestParseSSHConfig_SkipsInvalidLines(t *testing.T) {
	input := `backend: k8s
this has no colon
ssh_host: gt@pod.svc
just a single value`

	cfg, err := parseSSHConfig(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Host != "gt@pod.svc" {
		t.Errorf("Host = %q, want %q", cfg.Host, "gt@pod.svc")
	}
}

func TestParseSSHConfig_WhitespaceHandling(t *testing.T) {
	input := "  backend: k8s  \n  ssh_host:   gt@pod.svc  \n"

	cfg, err := parseSSHConfig(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Host != "gt@pod.svc" {
		t.Errorf("Host = %q, want %q", cfg.Host, "gt@pod.svc")
	}
}

func TestParseSSHConfig_UnknownFieldsIgnored(t *testing.T) {
	input := `backend: k8s
ssh_host: gt@pod.svc
pod_name: some-pod
pod_namespace: gastown
unknown_field: ignored`

	cfg, err := parseSSHConfig(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Host != "gt@pod.svc" {
		t.Errorf("Host = %q, want %q", cfg.Host, "gt@pod.svc")
	}
}

// --- TmuxBackend with mock tests ---

// mockLocalTmux is a test implementation of localTmux.
type mockLocalTmux struct {
	hasSessionResult bool
	hasSessionErr    error
	capturePaneResult string
	capturePaneErr   error
	nudgeErr         error
	sendKeysRawErr   error

	// Recorded calls
	hasSessionCalled   string
	capturedSession    string
	capturedLines      int
	nudgedSession      string
	nudgedMessage      string
	sentKeysSession    string
	sentKeys           string
}

func (m *mockLocalTmux) HasSession(name string) (bool, error) {
	m.hasSessionCalled = name
	return m.hasSessionResult, m.hasSessionErr
}

func (m *mockLocalTmux) CapturePane(session string, lines int) (string, error) {
	m.capturedSession = session
	m.capturedLines = lines
	return m.capturePaneResult, m.capturePaneErr
}

func (m *mockLocalTmux) NudgeSession(session, message string) error {
	m.nudgedSession = session
	m.nudgedMessage = message
	return m.nudgeErr
}

func (m *mockLocalTmux) SendKeysRaw(session, keys string) error {
	m.sentKeysSession = session
	m.sentKeys = keys
	return m.sendKeysRawErr
}

func TestTmuxBackend_HasSession(t *testing.T) {
	mock := &mockLocalTmux{hasSessionResult: true}
	b := &TmuxBackend{tmux: mock}

	result, err := b.HasSession("test-session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result {
		t.Error("expected true")
	}
	if mock.hasSessionCalled != "test-session" {
		t.Errorf("called with %q, want %q", mock.hasSessionCalled, "test-session")
	}
}

func TestTmuxBackend_HasSession_Error(t *testing.T) {
	mock := &mockLocalTmux{hasSessionErr: fmt.Errorf("tmux down")}
	b := &TmuxBackend{tmux: mock}

	_, err := b.HasSession("test-session")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTmuxBackend_CapturePane(t *testing.T) {
	mock := &mockLocalTmux{capturePaneResult: "output line 1\noutput line 2"}
	b := &TmuxBackend{tmux: mock}

	result, err := b.CapturePane("test-session", 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "output line 1\noutput line 2" {
		t.Errorf("result = %q, want expected output", result)
	}
	if mock.capturedSession != "test-session" {
		t.Errorf("session = %q, want %q", mock.capturedSession, "test-session")
	}
	if mock.capturedLines != 50 {
		t.Errorf("lines = %d, want 50", mock.capturedLines)
	}
}

func TestTmuxBackend_NudgeSession(t *testing.T) {
	mock := &mockLocalTmux{}
	b := &TmuxBackend{tmux: mock}

	err := b.NudgeSession("test-session", "hello agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.nudgedSession != "test-session" {
		t.Errorf("session = %q, want %q", mock.nudgedSession, "test-session")
	}
	if mock.nudgedMessage != "hello agent" {
		t.Errorf("message = %q, want %q", mock.nudgedMessage, "hello agent")
	}
}

func TestTmuxBackend_SendKeys(t *testing.T) {
	mock := &mockLocalTmux{}
	b := &TmuxBackend{tmux: mock}

	err := b.SendKeys("test-session", "some-keys")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.sentKeysSession != "test-session" {
		t.Errorf("session = %q, want %q", mock.sentKeysSession, "test-session")
	}
	if mock.sentKeys != "some-keys" {
		t.Errorf("keys = %q, want %q", mock.sentKeys, "some-keys")
	}
}

func TestTmuxBackend_SendKeys_Error(t *testing.T) {
	mock := &mockLocalTmux{sendKeysRawErr: fmt.Errorf("send failed")}
	b := &TmuxBackend{tmux: mock}

	err := b.SendKeys("test-session", "keys")
	if err == nil {
		t.Fatal("expected error")
	}
}
