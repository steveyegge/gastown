package terminal

import (
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
