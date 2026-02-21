package cmd

import (
	"testing"
)

func TestParseRigSlashName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantRig string
		wantN   string
		wantOk  bool
	}{
		{"simple rig/name", "beads/emma", "beads", "emma", true},
		{"no slash", "emma", "", "emma", false},
		{"nested slash", "beads/crew/emma", "beads", "crew/emma", true},
		{"leading slash", "/emma", "", "emma", true},
		{"trailing slash", "beads/", "beads", "", true},
		{"just a slash", "/", "", "", true},
		{"empty string", "", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rig, name, ok := parseRigSlashName(tt.input)
			if rig != tt.wantRig || name != tt.wantN || ok != tt.wantOk {
				t.Errorf("parseRigSlashName(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.input, rig, name, ok, tt.wantRig, tt.wantN, tt.wantOk)
			}
		})
	}
}

func TestIsShellCommand(t *testing.T) {
	tests := []struct {
		cmd    string
		expect bool
	}{
		{"bash", true},
		{"zsh", true},
		{"sh", true},
		{"fish", true},
		{"tcsh", true},
		{"ksh", true},
		{"claude", false},
		{"node", false},
		{"", false},
		{"basher", false},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			got := isShellCommand(tt.cmd)
			if got != tt.expect {
				t.Errorf("isShellCommand(%q) = %v, want %v", tt.cmd, got, tt.expect)
			}
		})
	}
}
