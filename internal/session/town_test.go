package session

import (
	"testing"
)

func TestTownSessions(t *testing.T) {
	sessions := TownSessions()

	if len(sessions) != 3 {
		t.Errorf("TownSessions() returned %d sessions, want 3", len(sessions))
	}

	// Verify order is correct (Mayor, Boot, Deacon)
	expectedOrder := []string{"Mayor", "Boot", "Deacon"}
	for i, s := range sessions {
		if s.Name != expectedOrder[i] {
			t.Errorf("TownSessions()[%d].Name = %q, want %q", i, s.Name, expectedOrder[i])
		}
		if s.SessionID == "" {
			t.Errorf("TownSessions()[%d].SessionID should not be empty", i)
		}
	}
}

func TestTownSessions_SessionIDFormats(t *testing.T) {
	sessions := TownSessions()

	for _, s := range sessions {
		if s.SessionID == "" {
			t.Errorf("TownSession %q has empty SessionID", s.Name)
		}
		// Session IDs should follow a pattern
		if len(s.SessionID) < 4 {
			t.Errorf("TownSession %q SessionID %q is too short", s.Name, s.SessionID)
		}
	}
}

func TestTownSession_StructFields(t *testing.T) {
	ts := TownSession{
		Name:      "Test",
		SessionID: "test-session",
	}

	if ts.Name != "Test" {
		t.Errorf("TownSession.Name = %q, want %q", ts.Name, "Test")
	}
	if ts.SessionID != "test-session" {
		t.Errorf("TownSession.SessionID = %q, want %q", ts.SessionID, "test-session")
	}
}

func TestTownSession_CanBeCreated(t *testing.T) {
	// Test that TownSession can be created with any values
	tests := []struct {
		name      string
		sessionID string
	}{
		{"Mayor", "hq-mayor"},
		{"Boot", "hq-boot"},
		{"Custom", "custom-session"},
	}

	for _, tt := range tests {
		ts := TownSession{
			Name:      tt.name,
			SessionID: tt.sessionID,
		}
		if ts.Name != tt.name {
			t.Errorf("TownSession.Name = %q, want %q", ts.Name, tt.name)
		}
		if ts.SessionID != tt.sessionID {
			t.Errorf("TownSession.SessionID = %q, want %q", ts.SessionID, tt.sessionID)
		}
	}
}

func TestTownSession_ShutdownOrder(t *testing.T) {
	// Verify that shutdown order is Mayor -> Boot -> Deacon
	// This is critical because Boot monitors Deacon
	sessions := TownSessions()

	if sessions[0].Name != "Mayor" {
		t.Errorf("First session should be Mayor, got %q", sessions[0].Name)
	}
	if sessions[1].Name != "Boot" {
		t.Errorf("Second session should be Boot, got %q", sessions[1].Name)
	}
	if sessions[2].Name != "Deacon" {
		t.Errorf("Third session should be Deacon, got %q", sessions[2].Name)
	}
}
