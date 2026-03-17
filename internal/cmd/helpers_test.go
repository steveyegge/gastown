package cmd

import (
	"os"
	"path/filepath"
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

func TestInferRigFromCrewName(t *testing.T) {
	// Set up a fake town structure in a temp dir
	townRoot := t.TempDir()

	// Create rigs with crew members
	mkdirAll := func(path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(townRoot, path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mkdirAll("rigA/crew/alice")
	mkdirAll("rigA/crew/bob")
	mkdirAll("rigB/crew/carol")
	mkdirAll("rigB/crew/bob")   // bob exists in both rigs
	mkdirAll(".hidden/crew/dave") // hidden dir, should be skipped

	// Create a non-dir entry at town root
	os.WriteFile(filepath.Join(townRoot, "README.md"), []byte("hi"), 0o644)

	tests := []struct {
		name    string
		crew    string
		wantRig string
		wantErr bool
	}{
		{"unique crew member", "alice", "rigA", false},
		{"unique in other rig", "carol", "rigB", false},
		{"ambiguous crew member", "bob", "", true},
		{"nonexistent crew member", "nobody", "", true},
		{"hidden dir skipped", "dave", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := inferRigFromCrewName(townRoot, tt.crew)
			if (err != nil) != tt.wantErr {
				t.Errorf("inferRigFromCrewName(%q) error = %v, wantErr %v", tt.crew, err, tt.wantErr)
				return
			}
			if got != tt.wantRig {
				t.Errorf("inferRigFromCrewName(%q) = %q, want %q", tt.crew, got, tt.wantRig)
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
