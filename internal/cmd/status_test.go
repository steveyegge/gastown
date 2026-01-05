package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/rig"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	os.Stdout = w

	fn()

	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	_ = r.Close()

	return buf.String()
}

func TestDiscoverRigAgents_UsesRigPrefix(t *testing.T) {
	townRoot := t.TempDir()
	writeTestRoutes(t, townRoot, []beads.Route{
		{Prefix: "bd-", Path: "beads/mayor/rig"},
	})

	r := &rig.Rig{
		Name:       "beads",
		Path:       filepath.Join(townRoot, "beads"),
		HasWitness: true,
	}

	allAgentBeads := map[string]*beads.Issue{
		"bd-beads-witness": {
			ID:         "bd-beads-witness",
			AgentState: "running",
			HookBead:   "bd-hook",
		},
	}
	allHookBeads := map[string]*beads.Issue{
		"bd-hook": {ID: "bd-hook", Title: "Pinned"},
	}

	agents := discoverRigAgents(map[string]bool{}, r, nil, allAgentBeads, allHookBeads, nil, true)
	if len(agents) != 1 {
		t.Fatalf("discoverRigAgents() returned %d agents, want 1", len(agents))
	}

	if agents[0].State != "running" {
		t.Fatalf("agent state = %q, want %q", agents[0].State, "running")
	}
	if !agents[0].HasWork {
		t.Fatalf("agent HasWork = false, want true")
	}
	if agents[0].WorkTitle != "Pinned" {
		t.Fatalf("agent WorkTitle = %q, want %q", agents[0].WorkTitle, "Pinned")
	}
}

func TestRenderAgentDetails_UsesRigPrefix(t *testing.T) {
	townRoot := t.TempDir()
	writeTestRoutes(t, townRoot, []beads.Route{
		{Prefix: "bd-", Path: "beads/mayor/rig"},
	})

	agent := AgentRuntime{
		Name:    "witness",
		Address: "beads/witness",
		Role:    "witness",
		Running: true,
	}

	output := captureStdout(t, func() {
		renderAgentDetails(agent, "", nil, townRoot)
	})

	if !strings.Contains(output, "bd-beads-witness") {
		t.Fatalf("output %q does not contain rig-prefixed bead ID", output)
	}
}

func TestTownStatus_CwdField(t *testing.T) {
	// Test that TownStatus includes the Cwd field
	status := TownStatus{
		Name:     "test-town",
		Location: "/path/to/town",
		Cwd:      "/path/to/town/gastown/polecats/nux",
	}

	if status.Cwd == "" {
		t.Error("TownStatus.Cwd should not be empty when set")
	}

	if status.Cwd != "/path/to/town/gastown/polecats/nux" {
		t.Errorf("TownStatus.Cwd = %q, want %q", status.Cwd, "/path/to/town/gastown/polecats/nux")
	}
}

func TestOutputStatusText_ShowsCwd(t *testing.T) {
	status := TownStatus{
		Name:     "test-town",
		Location: "/path/to/town",
		Cwd:      "/current/working/dir",
		Rigs:     []RigStatus{},
	}

	output := captureStdout(t, func() {
		_ = outputStatusText(status)
	})

	// Should show cwd in output
	if !strings.Contains(output, "cwd:") {
		t.Errorf("outputStatusText should include 'cwd:' line, got:\n%s", output)
	}
	if !strings.Contains(output, "/current/working/dir") {
		t.Errorf("outputStatusText should include the cwd path, got:\n%s", output)
	}
}

func TestOutputStatusText_NoCwdWhenEmpty(t *testing.T) {
	status := TownStatus{
		Name:     "test-town",
		Location: "/path/to/town",
		Cwd:      "", // Empty cwd
		Rigs:     []RigStatus{},
	}

	output := captureStdout(t, func() {
		_ = outputStatusText(status)
	})

	// Should NOT show cwd line when empty
	if strings.Contains(output, "cwd:") {
		t.Errorf("outputStatusText should not include 'cwd:' when Cwd is empty, got:\n%s", output)
	}
}
