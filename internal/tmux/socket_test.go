package tmux

import (
	"testing"
)

func TestSetGetDefaultSocket(t *testing.T) {
	// Save and restore
	orig := defaultSocket
	defer func() { defaultSocket = orig }()

	// Initially empty
	SetDefaultSocket("")
	if got := GetDefaultSocket(); got != "" {
		t.Errorf("expected empty, got %q", got)
	}

	SetDefaultSocket("mytown")
	if got := GetDefaultSocket(); got != "mytown" {
		t.Errorf("expected %q, got %q", "mytown", got)
	}
}

func TestNewTmuxInheritsSocket(t *testing.T) {
	orig := defaultSocket
	defer func() { defaultSocket = orig }()

	SetDefaultSocket("testtown")
	tmx := NewTmux()
	if tmx.socketName != "testtown" {
		t.Errorf("NewTmux() socketName = %q, want %q", tmx.socketName, "testtown")
	}
}

func TestNewTmuxWithSocket(t *testing.T) {
	tmx := NewTmuxWithSocket("custom")
	if tmx.socketName != "custom" {
		t.Errorf("NewTmuxWithSocket() socketName = %q, want %q", tmx.socketName, "custom")
	}
}

func TestBuildCommandNoSocket(t *testing.T) {
	orig := defaultSocket
	defer func() { defaultSocket = orig }()

	SetDefaultSocket("")
	cmd := BuildCommand("list-sessions")
	args := cmd.Args
	// Should be: tmux -u list-sessions
	expected := []string{"tmux", "-u", "list-sessions"}
	if len(args) != len(expected) {
		t.Fatalf("args = %v, want %v", args, expected)
	}
	for i, a := range args {
		if a != expected[i] {
			t.Errorf("args[%d] = %q, want %q", i, a, expected[i])
		}
	}
}

func TestKillStaleSessionOnOtherSockets_NoSocket(t *testing.T) {
	// Should be a no-op when socketName is empty
	tmx := &Tmux{socketName: ""}
	tmx.KillStaleSessionOnOtherSockets("test-session")
	// No panic = pass
}

func TestKillStaleSessionOnOtherSockets_DefaultToDefault(t *testing.T) {
	orig := defaultSocket
	defer func() { defaultSocket = orig }()

	// When target is "default" and GetDefaultSocket is also "default", should no-op
	SetDefaultSocket("default")
	tmx := &Tmux{socketName: "default"}
	tmx.KillStaleSessionOnOtherSockets("test-session")
	// No panic = pass
}

func TestKillStaleSessionOnOtherSockets_ChecksDefaultWhenOnTownSocket(t *testing.T) {
	// When targeting "gt" socket, should check "default" socket
	tmx := &Tmux{socketName: "gt"}
	// This will try to contact the "default" tmux server. If no server
	// is running, HasSession returns false and nothing happens — that's fine.
	tmx.KillStaleSessionOnOtherSockets("nonexistent-session")
	// No panic = pass
}

func TestKillStaleSessionOnOtherSockets_ChecksTownWhenOnDefault(t *testing.T) {
	orig := defaultSocket
	defer func() { defaultSocket = orig }()

	SetDefaultSocket("gt")
	tmx := &Tmux{socketName: "default"}
	// Should check "gt" socket for stale sessions
	tmx.KillStaleSessionOnOtherSockets("nonexistent-session")
	// No panic = pass
}

func TestBuildCommandWithSocket(t *testing.T) {
	orig := defaultSocket
	defer func() { defaultSocket = orig }()

	SetDefaultSocket("mytown")
	cmd := BuildCommand("has-session", "-t", "hq-mayor")
	args := cmd.Args
	// Should be: tmux -u -L mytown has-session -t hq-mayor
	expected := []string{"tmux", "-u", "-L", "mytown", "has-session", "-t", "hq-mayor"}
	if len(args) != len(expected) {
		t.Fatalf("args = %v, want %v", args, expected)
	}
	for i, a := range args {
		if a != expected[i] {
			t.Errorf("args[%d] = %q, want %q", i, a, expected[i])
		}
	}
}
