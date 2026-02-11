package sling

import (
	"strings"
	"testing"
)

func TestGetSessionPane_NoTmux(t *testing.T) {
	// Set PATH to empty so exec.LookPath("tmux") fails immediately.
	// This simulates a K8s pod where tmux is not installed.
	t.Setenv("PATH", "")

	_, err := GetSessionPane("gt-test-session")
	if err == nil {
		t.Fatal("GetSessionPane() should return error when tmux is not on PATH")
	}
	if !strings.Contains(err.Error(), "tmux not available") {
		t.Errorf("GetSessionPane() error = %q, want it to contain %q", err.Error(), "tmux not available")
	}
}

func TestGetSessionPane_Exported(t *testing.T) {
	// Verify GetSessionPane is exported and callable from external packages.
	// This is a compile-time check — the function signature is:
	//   func GetSessionPane(sessionName string) (string, error)
	var fn func(string) (string, error) = GetSessionPane
	if fn == nil {
		t.Fatal("GetSessionPane should be non-nil")
	}
}
