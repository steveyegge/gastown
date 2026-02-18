//go:build darwin

package quota

import (
	"os"
	"testing"
)

func TestKeychainServiceName_DefaultDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	// Both tilde and expanded forms of the default dir should produce the bare name
	tests := []struct {
		name string
		path string
	}{
		{"tilde form", "~/.claude"},
		{"expanded form", home + "/.claude"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := KeychainServiceName(tt.path)
			want := "Claude Code-credentials"
			if got != want {
				t.Errorf("KeychainServiceName(%q) = %q, want %q", tt.path, got, want)
			}
		})
	}
}

func TestKeychainServiceName_AccountDir(t *testing.T) {
	got := KeychainServiceName("/Users/testuser/.claude-accounts/work")
	// Should have the base name plus an 8-char hex suffix
	if len(got) != len("Claude Code-credentials-") + 8 {
		t.Errorf("expected service name with 8-char hex suffix, got %q (len=%d)", got, len(got))
	}
	if got[:len("Claude Code-credentials-")] != "Claude Code-credentials-" {
		t.Errorf("expected prefix 'Claude Code-credentials-', got %q", got)
	}
}

func TestKeychainServiceName_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}

	tildePath := "~/.claude-accounts/work"
	expandedPath := home + "/.claude-accounts/work"

	tildeResult := KeychainServiceName(tildePath)
	expandedResult := KeychainServiceName(expandedPath)

	if tildeResult != expandedResult {
		t.Errorf("tilde and expanded paths produced different service names:\n  ~/ form:    %q\n  expanded:   %q",
			tildeResult, expandedResult)
	}
}

func TestKeychainServiceName_DifferentDirs(t *testing.T) {
	a := KeychainServiceName("/Users/testuser/.claude-accounts/work")
	b := KeychainServiceName("/Users/testuser/.claude-accounts/personal")

	if a == b {
		t.Errorf("different dirs produced same service name: %q", a)
	}
}
