package session

import (
	"testing"
)

func TestTownSessionInfos(t *testing.T) {
	sessions := TownSessionInfos()

	if len(sessions) != 3 {
		t.Errorf("TownSessionInfos() returned %d sessions, want 3", len(sessions))
	}

	// Verify order is correct (Mayor, Boot, Deacon)
	expectedOrder := []string{"Mayor", "Boot", "Deacon"}
	for i, s := range sessions {
		if s.Name != expectedOrder[i] {
			t.Errorf("TownSessionInfos()[%d].Name = %q, want %q", i, s.Name, expectedOrder[i])
		}
		if s.SessionID == "" {
			t.Errorf("TownSessionInfos()[%d].SessionID should not be empty", i)
		}
	}
}

func TestTownSessionInfos_SessionIDFormats(t *testing.T) {
	sessions := TownSessionInfos()

	for _, s := range sessions {
		if s.SessionID == "" {
			t.Errorf("TownSessionInfo %q has empty SessionID", s.Name)
		}
		// Session IDs should follow a pattern
		if len(s.SessionID) < 4 {
			t.Errorf("TownSessionInfo %q SessionID %q is too short", s.Name, s.SessionID)
		}
	}
}

func TestTownSessionInfo_StructFields(t *testing.T) {
	ts := TownSessionInfo{
		Name:      "Test",
		SessionID: "test-session",
	}

	if ts.Name != "Test" {
		t.Errorf("TownSessionInfo.Name = %q, want %q", ts.Name, "Test")
	}
	if ts.SessionID != "test-session" {
		t.Errorf("TownSessionInfo.SessionID = %q, want %q", ts.SessionID, "test-session")
	}
}

func TestTownSessionInfo_CanBeCreated(t *testing.T) {
	// Test that TownSessionInfo can be created with any values
	tests := []struct {
		name      string
		sessionID string
	}{
		{"Mayor", "hq-mayor"},
		{"Boot", "hq-boot"},
		{"Custom", "custom-session"},
	}

	for _, tt := range tests {
		ts := TownSessionInfo{
			Name:      tt.name,
			SessionID: tt.sessionID,
		}
		if ts.Name != tt.name {
			t.Errorf("TownSessionInfo.Name = %q, want %q", ts.Name, tt.name)
		}
		if ts.SessionID != tt.sessionID {
			t.Errorf("TownSessionInfo.SessionID = %q, want %q", ts.SessionID, tt.sessionID)
		}
	}
}

func TestTownSessionInfo_ShutdownOrder(t *testing.T) {
	// Verify that shutdown order is Mayor -> Boot -> Deacon
	// This is critical because Boot monitors Deacon
	sessions := TownSessionInfos()

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
