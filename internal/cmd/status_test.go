package cmd

import (
	"bytes"
	"encoding/json"
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

func TestRunStatusWatch_RejectsZeroInterval(t *testing.T) {
	oldInterval := statusInterval
	oldWatch := statusWatch
	defer func() {
		statusInterval = oldInterval
		statusWatch = oldWatch
	}()

	statusInterval = 0
	statusWatch = true

	err := runStatusWatch(nil, nil)
	if err == nil {
		t.Fatal("expected error for zero interval, got nil")
	}
	if !strings.Contains(err.Error(), "positive") {
		t.Errorf("error %q should mention 'positive'", err.Error())
	}
}

func TestRunStatusWatch_RejectsNegativeInterval(t *testing.T) {
	oldInterval := statusInterval
	oldWatch := statusWatch
	defer func() {
		statusInterval = oldInterval
		statusWatch = oldWatch
	}()

	statusInterval = -5
	statusWatch = true

	err := runStatusWatch(nil, nil)
	if err == nil {
		t.Fatal("expected error for negative interval, got nil")
	}
	if !strings.Contains(err.Error(), "positive") {
		t.Errorf("error %q should mention 'positive'", err.Error())
	}
}

func TestRunStatusWatch_RejectsJSONCombo(t *testing.T) {
	oldJSON := statusJSON
	oldWatch := statusWatch
	oldInterval := statusInterval
	defer func() {
		statusJSON = oldJSON
		statusWatch = oldWatch
		statusInterval = oldInterval
	}()

	statusJSON = true
	statusWatch = true
	statusInterval = 2

	err := runStatusWatch(nil, nil)
	if err == nil {
		t.Fatal("expected error for --json + --watch, got nil")
	}
	if !strings.Contains(err.Error(), "cannot be used together") {
		t.Errorf("error %q should mention 'cannot be used together'", err.Error())
	}
}

func TestBuildStatusIndicator_DisabledAgent(t *testing.T) {
	agent := AgentRuntime{
		Name:     "refinery",
		Role:     "refinery",
		Running:  true,
		Disabled: true,
	}

	indicator := buildStatusIndicator(agent)
	if !strings.Contains(indicator, "disabled") {
		t.Errorf("buildStatusIndicator() = %q, want to contain 'disabled'", indicator)
	}
}

func TestBuildStatusIndicator_EnabledAgent(t *testing.T) {
	agent := AgentRuntime{
		Name:     "refinery",
		Role:     "refinery",
		Running:  true,
		Disabled: false,
	}

	indicator := buildStatusIndicator(agent)
	if strings.Contains(indicator, "disabled") {
		t.Errorf("buildStatusIndicator() = %q, should not contain 'disabled'", indicator)
	}
}

func TestDiscoverRigAgents_RefineryDisabled(t *testing.T) {
	townRoot := t.TempDir()
	rigName := "testrig"
	rigPath := filepath.Join(townRoot, rigName)

	// Create rig directory structure
	if err := os.MkdirAll(filepath.Join(rigPath, "refinery", "rig"), 0755); err != nil {
		t.Fatalf("create refinery dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(rigPath, "settings"), 0755); err != nil {
		t.Fatalf("create settings dir: %v", err)
	}

	// Write settings with refinery disabled
	settings := map[string]interface{}{
		"type":    "rig-settings",
		"version": 1,
		"refinery": map[string]interface{}{
			"enabled": false,
		},
	}
	settingsData, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rigPath, "settings", "config.json"), settingsData, 0644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	// Write routes for prefix resolution
	writeTestRoutes(t, townRoot, []beads.Route{
		{Prefix: "bd-", Path: rigName + "/mayor/rig"},
	})

	r := &rig.Rig{
		Name:        rigName,
		Path:        rigPath,
		HasRefinery: true,
	}

	agents := discoverRigAgents(map[string]bool{}, r, nil, nil, nil, nil, true)
	if len(agents) != 1 {
		t.Fatalf("discoverRigAgents() returned %d agents, want 1", len(agents))
	}

	if agents[0].Role != "refinery" {
		t.Errorf("agent role = %q, want 'refinery'", agents[0].Role)
	}
	if !agents[0].Disabled {
		t.Errorf("agent Disabled = false, want true (refinery is disabled in settings)")
	}
}

func TestDiscoverRigAgents_RefineryEnabled(t *testing.T) {
	townRoot := t.TempDir()
	rigName := "testrig"
	rigPath := filepath.Join(townRoot, rigName)

	// Create rig directory structure
	if err := os.MkdirAll(filepath.Join(rigPath, "refinery", "rig"), 0755); err != nil {
		t.Fatalf("create refinery dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(rigPath, "settings"), 0755); err != nil {
		t.Fatalf("create settings dir: %v", err)
	}

	// Write settings with refinery enabled (default)
	settings := map[string]interface{}{
		"type":    "rig-settings",
		"version": 1,
	}
	settingsData, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	if err := os.WriteFile(filepath.Join(rigPath, "settings", "config.json"), settingsData, 0644); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	// Write routes for prefix resolution
	writeTestRoutes(t, townRoot, []beads.Route{
		{Prefix: "bd-", Path: rigName + "/mayor/rig"},
	})

	r := &rig.Rig{
		Name:        rigName,
		Path:        rigPath,
		HasRefinery: true,
	}

	agents := discoverRigAgents(map[string]bool{}, r, nil, nil, nil, nil, true)
	if len(agents) != 1 {
		t.Fatalf("discoverRigAgents() returned %d agents, want 1", len(agents))
	}

	if agents[0].Disabled {
		t.Errorf("agent Disabled = true, want false (refinery is enabled by default)")
	}
}
