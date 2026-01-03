package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

func setupTownRoot(t *testing.T, townRoot string, routes []beads.Route) {
	t.Helper()
	writeTestRoutes(t, townRoot, routes)
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatalf("create mayor dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}
}

func TestAddressToAgentBeadID_UsesRigPrefix(t *testing.T) {
	townRoot := t.TempDir()
	setupTownRoot(t, townRoot, []beads.Route{
		{Prefix: "hw-", Path: "helloworld/mayor/rig"},
		{Prefix: "bd-", Path: "beads/mayor/rig"},
	})

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir town root: %v", err)
	}

	tests := []struct {
		address  string
		expected string
	}{
		{"mayor", "gt-mayor"},
		{"deacon", "gt-deacon"},
		{"helloworld/witness", "hw-helloworld-witness"},
		{"helloworld/refinery", "hw-helloworld-refinery"},
		{"helloworld/alpha", "hw-helloworld-polecat-alpha"},
		{"helloworld/crew/max", "hw-helloworld-crew-max"},
		{"beads/witness", "bd-beads-witness"},
		{"beads/beta", "bd-beads-polecat-beta"},
		// Invalid addresses should return empty string
		{"invalid", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			got := addressToAgentBeadID(tt.address)
			if got != tt.expected {
				t.Errorf("addressToAgentBeadID(%q) = %q, want %q", tt.address, got, tt.expected)
			}
		})
	}
}

func TestBuildAgentBeadID_UsesRigPrefix(t *testing.T) {
	townRoot := t.TempDir()
	setupTownRoot(t, townRoot, []beads.Route{
		{Prefix: "hw-", Path: "helloworld/mayor/rig"},
	})

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir town root: %v", err)
	}

	tests := []struct {
		identity string
		role     Role
		expected string
	}{
		{"helloworld/witness", RoleUnknown, "hw-helloworld-witness"},
		{"helloworld/refinery", RoleUnknown, "hw-helloworld-refinery"},
		{"helloworld/alpha", RolePolecat, "hw-helloworld-polecat-alpha"},
		{"helloworld/crew/max", RoleCrew, "hw-helloworld-crew-max"},
	}

	for _, tt := range tests {
		t.Run(tt.identity, func(t *testing.T) {
			got := buildAgentBeadID(tt.identity, tt.role)
			if got != tt.expected {
				t.Errorf("buildAgentBeadID(%q, %q) = %q, want %q", tt.identity, tt.role, got, tt.expected)
			}
		})
	}
}

func TestAgentIDToBeadID_UsesRigPrefix(t *testing.T) {
	townRoot := t.TempDir()
	setupTownRoot(t, townRoot, []beads.Route{
		{Prefix: "hw-", Path: "helloworld/mayor/rig"},
	})

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir town root: %v", err)
	}

	tests := []struct {
		agentID  string
		expected string
	}{
		{"mayor", "gt-mayor"},
		{"deacon", "gt-deacon"},
		{"helloworld/witness", "hw-helloworld-witness"},
		{"helloworld/refinery", "hw-helloworld-refinery"},
		{"helloworld/crew/max", "hw-helloworld-crew-max"},
		{"helloworld/polecats/rictus", "hw-helloworld-polecat-rictus"},
	}

	for _, tt := range tests {
		t.Run(tt.agentID, func(t *testing.T) {
			got := agentIDToBeadID(tt.agentID)
			if got != tt.expected {
				t.Errorf("agentIDToBeadID(%q) = %q, want %q", tt.agentID, got, tt.expected)
			}
		})
	}
}

func TestAgentAddressToIDs_UsesRigPrefix(t *testing.T) {
	townRoot := t.TempDir()
	setupTownRoot(t, townRoot, []beads.Route{
		{Prefix: "hw-", Path: "helloworld/mayor/rig"},
	})

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
	if err := os.Chdir(townRoot); err != nil {
		t.Fatalf("chdir town root: %v", err)
	}

	tests := []struct {
		address      string
		wantBeadID   string
		wantSession  string
		expectErrMsg string
	}{
		{"helloworld/witness", "hw-helloworld-witness", "gt-helloworld-witness", ""},
		{"helloworld/refinery", "hw-helloworld-refinery", "gt-helloworld-refinery", ""},
		{"helloworld/polecats/rictus", "hw-helloworld-polecat-rictus", "gt-helloworld-rictus", ""},
		{"helloworld/crew/max", "hw-helloworld-crew-max", "gt-helloworld-crew-max", ""},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			beadID, sessionName, err := agentAddressToIDs(tt.address)
			if err != nil {
				t.Fatalf("agentAddressToIDs(%q) error: %v", tt.address, err)
			}
			if beadID != tt.wantBeadID {
				t.Errorf("agentAddressToIDs(%q) beadID = %q, want %q", tt.address, beadID, tt.wantBeadID)
			}
			if sessionName != tt.wantSession {
				t.Errorf("agentAddressToIDs(%q) session = %q, want %q", tt.address, sessionName, tt.wantSession)
			}
		})
	}
}
