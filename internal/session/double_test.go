package session

import (
	"testing"
)

// TestDouble_Conformance verifies the test double matches Sessions contract.
func TestDouble_Conformance(t *testing.T) {
	factory := func() Sessions {
		return NewDouble()
	}

	RunConformanceTests(t, factory, nil)
}

// TestDouble_SwitchTo verifies the SwitchTo method behavior.
func TestDouble_SwitchTo(t *testing.T) {
	d := NewDouble()

	// Create two sessions
	_, _ = d.Start("sess1", "/tmp", "cmd1")
	_, _ = d.Start("sess2", "/tmp", "cmd2")

	// Not inside any session - should fail
	err := d.SwitchTo("sess1")
	if err == nil {
		t.Error("SwitchTo should fail when not inside any session")
	}

	// Set current session (simulates being inside tmux)
	d.SetCurrentSession("sess1")

	// Switch to existing session should succeed
	err = d.SwitchTo("sess2")
	if err != nil {
		t.Errorf("SwitchTo failed: %v", err)
	}
	if d.GetCurrentSession() != "sess2" {
		t.Errorf("current session should be sess2, got %s", d.GetCurrentSession())
	}

	// Switch to same session (no-op) should succeed
	err = d.SwitchTo("sess2")
	if err != nil {
		t.Errorf("SwitchTo to same session should succeed: %v", err)
	}

	// Switch to non-existent session should fail
	err = d.SwitchTo("nonexistent")
	if err == nil {
		t.Error("SwitchTo should fail for non-existent session")
	}
}
