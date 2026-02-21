package mayor

import (
	"testing"
)

func TestNewManager(t *testing.T) {
	m := NewManager("/tmp/test-town")
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.townRoot != "/tmp/test-town" {
		t.Errorf("townRoot = %q, want %q", m.townRoot, "/tmp/test-town")
	}
}

func TestManager_mayorDir(t *testing.T) {
	m := NewManager("/tmp/test-town")
	got := m.mayorDir()
	want := "/tmp/test-town/mayor"
	if got != want {
		t.Errorf("mayorDir() = %q, want %q", got, want)
	}
}

func TestSessionName_ReturnsConsistentValue(t *testing.T) {
	name := SessionName()
	if name == "" {
		t.Error("SessionName() returned empty string")
	}
	// Verify idempotent
	if SessionName() != name {
		t.Error("SessionName() returned different values on subsequent calls")
	}
}

func TestManager_SessionName_MatchesPackageFunc(t *testing.T) {
	m := NewManager("/tmp/test-town")
	if m.SessionName() != SessionName() {
		t.Errorf("Manager.SessionName() = %q, SessionName() = %q — should match",
			m.SessionName(), SessionName())
	}
}

func TestManager_Errors(t *testing.T) {
	if ErrNotRunning.Error() != "mayor not running" {
		t.Errorf("ErrNotRunning = %q", ErrNotRunning)
	}
	if ErrAlreadyRunning.Error() != "mayor already running" {
		t.Errorf("ErrAlreadyRunning = %q", ErrAlreadyRunning)
	}
}
