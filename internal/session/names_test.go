package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMayorSessionName(t *testing.T) {
	want := "gt-mayor"
	got := MayorSessionName()
	if got != want {
		t.Errorf("MayorSessionName() = %q, want %q", got, want)
	}
}

func TestDeaconSessionName(t *testing.T) {
	want := "gt-deacon"
	got := DeaconSessionName()
	if got != want {
		t.Errorf("DeaconSessionName() = %q, want %q", got, want)
	}
}

func TestWitnessSessionName(t *testing.T) {
	tests := []struct {
		rig  string
		want string
	}{
		{"gastown", "gt-gastown-witness"},
		{"beads", "gt-beads-witness"},
		{"foo", "gt-foo-witness"},
	}
	for _, tt := range tests {
		t.Run(tt.rig, func(t *testing.T) {
			got := WitnessSessionName(tt.rig)
			if got != tt.want {
				t.Errorf("WitnessSessionName(%q) = %q, want %q", tt.rig, got, tt.want)
			}
		})
	}
}

func TestRefinerySessionName(t *testing.T) {
	tests := []struct {
		rig  string
		want string
	}{
		{"gastown", "gt-gastown-refinery"},
		{"beads", "gt-beads-refinery"},
		{"foo", "gt-foo-refinery"},
	}
	for _, tt := range tests {
		t.Run(tt.rig, func(t *testing.T) {
			got := RefinerySessionName(tt.rig)
			if got != tt.want {
				t.Errorf("RefinerySessionName(%q) = %q, want %q", tt.rig, got, tt.want)
			}
		})
	}
}

func TestCrewSessionName(t *testing.T) {
	tests := []struct {
		rig  string
		name string
		want string
	}{
		{"gastown", "max", "gt-gastown-crew-max"},
		{"beads", "alice", "gt-beads-crew-alice"},
		{"foo", "bar", "gt-foo-crew-bar"},
	}
	for _, tt := range tests {
		t.Run(tt.rig+"/"+tt.name, func(t *testing.T) {
			got := CrewSessionName(tt.rig, tt.name)
			if got != tt.want {
				t.Errorf("CrewSessionName(%q, %q) = %q, want %q", tt.rig, tt.name, got, tt.want)
			}
		})
	}
}

func TestPolecatSessionName(t *testing.T) {
	tests := []struct {
		rig  string
		name string
		want string
	}{
		{"gastown", "Toast", "gt-gastown-Toast"},
		{"gastown", "Furiosa", "gt-gastown-Furiosa"},
		{"beads", "worker1", "gt-beads-worker1"},
	}
	for _, tt := range tests {
		t.Run(tt.rig+"/"+tt.name, func(t *testing.T) {
			got := PolecatSessionName(tt.rig, tt.name)
			if got != tt.want {
				t.Errorf("PolecatSessionName(%q, %q) = %q, want %q", tt.rig, tt.name, got, tt.want)
			}
		})
	}
}

func TestPrefix(t *testing.T) {
	want := "gt-"
	if Prefix != want {
		t.Errorf("Prefix = %q, want %q", Prefix, want)
	}
}

func TestPropulsionNudgeForRole_WithSessionID(t *testing.T) {
	// Create temp directory with session_id file
	tmpDir := t.TempDir()
	runtimeDir := filepath.Join(tmpDir, ".runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatalf("creating runtime dir: %v", err)
	}

	sessionID := "test-session-abc123"
	if err := os.WriteFile(filepath.Join(runtimeDir, "session_id"), []byte(sessionID), 0644); err != nil {
		t.Fatalf("writing session_id: %v", err)
	}

	// Test that session ID is appended
	msg := PropulsionNudgeForRole("mayor", tmpDir)
	if !strings.Contains(msg, "[session:test-session-abc123]") {
		t.Errorf("PropulsionNudgeForRole(mayor, tmpDir) = %q, should contain [session:test-session-abc123]", msg)
	}
}

func TestPropulsionNudgeForRole_WithoutSessionID(t *testing.T) {
	// Use nonexistent directory
	msg := PropulsionNudgeForRole("mayor", "/nonexistent-dir-12345")
	if strings.Contains(msg, "[session:") {
		t.Errorf("PropulsionNudgeForRole(mayor, /nonexistent) = %q, should NOT contain session ID", msg)
	}
}

func TestPropulsionNudgeForRole_EmptyWorkDir(t *testing.T) {
	// Empty workDir should not crash and should not include session ID
	msg := PropulsionNudgeForRole("mayor", "")
	if strings.Contains(msg, "[session:") {
		t.Errorf("PropulsionNudgeForRole(mayor, \"\") = %q, should NOT contain session ID", msg)
	}
}

func TestPropulsionNudgeForRole_AllRoles(t *testing.T) {
	tests := []struct {
		role     string
		contains string
	}{
		{"polecat", "gt hook"},
		{"crew", "gt hook"},
		{"witness", "gt prime"},
		{"refinery", "gt prime"},
		{"deacon", "gt prime"},
		{"mayor", "gt prime"},
		{"unknown", "gt hook"},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			msg := PropulsionNudgeForRole(tt.role, "")
			if !strings.Contains(msg, tt.contains) {
				t.Errorf("PropulsionNudgeForRole(%q, \"\") = %q, should contain %q", tt.role, msg, tt.contains)
			}
		})
	}
}
