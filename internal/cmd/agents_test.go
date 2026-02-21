package cmd

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestAgentsCmd_DefaultRunE(t *testing.T) {
	// After the fix, `gt agents` (no subcommand) should run the list function,
	// not the interactive popup menu.
	if agentsCmd.RunE == nil {
		t.Fatal("agentsCmd.RunE is nil")
	}

	// Verify it points to runAgentsList by checking the Short description
	// which we update to match the list behavior.
	want := "List Gas Town agent sessions"
	if agentsCmd.Short != want {
		t.Errorf("agentsCmd.Short = %q, want %q", agentsCmd.Short, want)
	}
}

func TestAgentsMenuCmd_Exists(t *testing.T) {
	found := false
	for _, sub := range agentsCmd.Commands() {
		if sub.Use == "menu" {
			found = true
			break
		}
	}
	if !found {
		t.Error("agentsMenuCmd not registered as subcommand of agentsCmd")
	}
}

func TestAgentsMenuCmd_RunE(t *testing.T) {
	var menuCmd *cobra.Command
	for _, sub := range agentsCmd.Commands() {
		if sub.Use == "menu" {
			menuCmd = sub
			break
		}
	}
	if menuCmd == nil {
		t.Fatal("agentsMenuCmd not found")
	}
	if menuCmd.RunE == nil {
		t.Fatal("agentsMenuCmd.RunE is nil")
	}
}

func TestAgentsListCmd_StillRegistered(t *testing.T) {
	found := false
	for _, sub := range agentsCmd.Commands() {
		if sub.Use == "list" {
			found = true
			break
		}
	}
	if !found {
		t.Error("agentsListCmd not registered as subcommand of agentsCmd")
	}
}

func TestAgentsCmd_ShortDescription(t *testing.T) {
	if agentsCmd.Short == "Switch between Gas Town agent sessions" {
		t.Error("agentsCmd.Short still describes popup menu behavior; should describe listing")
	}
}

func TestCategorizeSession_AllTypes(t *testing.T) {
	setupCmdTestRegistry(t)
	tests := []struct {
		name     string
		input    string
		wantType AgentType
	}{
		{"mayor", "hq-mayor", AgentMayor},
		{"deacon", "hq-deacon", AgentDeacon},
		// Rig-level sessions require a registered prefix. Use "gt" which is
		// commonly registered in the default PrefixRegistry.
		{"witness", "gt-witness", AgentWitness},
		{"refinery", "gt-refinery", AgentRefinery},
		{"crew", "gt-crew-max", AgentCrew},
		{"polecat", "gt-furiosa", AgentPolecat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := categorizeSession(tt.input)
			if got == nil {
				t.Fatalf("categorizeSession(%q) = nil, want type %d", tt.input, tt.wantType)
			}
			if got.Type != tt.wantType {
				t.Errorf("categorizeSession(%q).Type = %d, want %d", tt.input, got.Type, tt.wantType)
			}
		})
	}
}

func TestCategorizeSession_InvalidName(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"random string", "not-a-gastown-session"},
		{"bare word", "foobar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := categorizeSession(tt.input)
			if got != nil {
				t.Errorf("categorizeSession(%q) = %+v, want nil", tt.input, got)
			}
		})
	}
}

func TestCategorizeSession_Overseer(t *testing.T) {
	got := categorizeSession("hq-overseer")
	if got != nil {
		t.Errorf("categorizeSession(%q) = %+v, want nil (overseer is not a display agent)", "hq-overseer", got)
	}
}

func TestCategorizeSession_EmptyString(t *testing.T) {
	got := categorizeSession("")
	if got != nil {
		t.Errorf("categorizeSession(%q) = %+v, want nil", "", got)
	}
}

func TestShortcutKey_Range(t *testing.T) {
	tests := []struct {
		index int
		want  string
	}{
		{0, "1"},
		{1, "2"},
		{8, "9"},
		{9, "a"},
		{10, "b"},
		{34, "z"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := shortcutKey(tt.index)
			if got != tt.want {
				t.Errorf("shortcutKey(%d) = %q, want %q", tt.index, got, tt.want)
			}
		})
	}
}

func TestShortcutKey_BeyondRange(t *testing.T) {
	tests := []int{35, 36, 100}
	for _, idx := range tests {
		got := shortcutKey(idx)
		if got != "" {
			t.Errorf("shortcutKey(%d) = %q, want empty string", idx, got)
		}
	}
}

func TestDisplayLabel_AllTypes(t *testing.T) {
	tests := []struct {
		name        string
		agent       AgentSession
		wantContain string
	}{
		{"mayor", AgentSession{Name: "hq-mayor", Type: AgentMayor}, "Mayor"},
		{"deacon", AgentSession{Name: "hq-deacon", Type: AgentDeacon}, "Deacon"},
		{"witness", AgentSession{Name: "gt-witness", Type: AgentWitness, Rig: "gastown"}, "gastown/witness"},
		{"refinery", AgentSession{Name: "gt-refinery", Type: AgentRefinery, Rig: "gastown"}, "gastown/refinery"},
		{"crew", AgentSession{Name: "gt-crew-max", Type: AgentCrew, Rig: "gastown", AgentName: "max"}, "crew/max"},
		{"polecat", AgentSession{Name: "gt-furiosa", Type: AgentPolecat, Rig: "gastown", AgentName: "furiosa"}, "furiosa"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label := tt.agent.displayLabel()
			if label == "" {
				t.Errorf("displayLabel() for %s returned empty string", tt.name)
			}
			if !strings.Contains(label, tt.wantContain) {
				t.Errorf("displayLabel() = %q, want substring %q", label, tt.wantContain)
			}
		})
	}
}

// --- filterAndSortSessions tests ---

func TestFilterAndSortSessions_NoSessions(t *testing.T) {
	setupCmdTestRegistry(t)
	got := filterAndSortSessions(nil, true)
	if len(got) != 0 {
		t.Errorf("filterAndSortSessions(nil) returned %d agents, want 0", len(got))
	}

	got = filterAndSortSessions([]string{}, true)
	if len(got) != 0 {
		t.Errorf("filterAndSortSessions([]) returned %d agents, want 0", len(got))
	}
}

func TestFilterAndSortSessions_AllFiltered(t *testing.T) {
	setupCmdTestRegistry(t)
	input := []string{
		"my-tmux-session",
		"dev-workspace",
		"random-thing",
	}
	got := filterAndSortSessions(input, true)
	if len(got) != 0 {
		t.Errorf("filterAndSortSessions(non-gastown names) returned %d agents, want 0", len(got))
	}
}

func TestFilterAndSortSessions_PolecatFiltering(t *testing.T) {
	setupCmdTestRegistry(t)
	input := []string{
		"hq-mayor",
		"gt-furiosa", // polecat
		"gt-witness",
	}

	// With polecats excluded
	got := filterAndSortSessions(input, false)
	for _, a := range got {
		if a.Type == AgentPolecat {
			t.Errorf("polecat %q present when includePolecats=false", a.Name)
		}
	}
	if len(got) != 2 {
		t.Errorf("filterAndSortSessions(includePolecats=false) returned %d agents, want 2", len(got))
	}

	// With polecats included
	got = filterAndSortSessions(input, true)
	hasPolecat := false
	for _, a := range got {
		if a.Type == AgentPolecat {
			hasPolecat = true
		}
	}
	if !hasPolecat {
		t.Error("no polecat found when includePolecats=true")
	}
	if len(got) != 3 {
		t.Errorf("filterAndSortSessions(includePolecats=true) returned %d agents, want 3", len(got))
	}
}

func TestFilterAndSortSessions_BootSessionFiltered(t *testing.T) {
	setupCmdTestRegistry(t)
	input := []string{
		"hq-mayor",
		"hq-boot", // should always be excluded
		"hq-deacon",
	}

	got := filterAndSortSessions(input, true)
	for _, a := range got {
		if a.Name == "hq-boot" {
			t.Error("hq-boot session should be filtered out")
		}
	}
	if len(got) != 2 {
		t.Errorf("filterAndSortSessions with boot returned %d agents, want 2", len(got))
	}
}

func TestFilterAndSortSessions_SortOrder(t *testing.T) {
	setupCmdTestRegistry(t)
	input := []string{
		"gt-crew-zed",    // crew (gastown)
		"gt-witness",     // witness (gastown)
		"hq-deacon",      // deacon
		"gt-refinery",    // refinery (gastown)
		"hq-mayor",       // mayor
		"gt-furiosa",     // polecat (gastown)
		"mr-witness",     // witness (myrig)
		"gt-crew-alpha",  // crew (gastown)
	}

	got := filterAndSortSessions(input, true)

	// Expected order:
	// 1. mayor (town-level)
	// 2. deacon (town-level)
	// 3. gastown/refinery (rig "gastown" < "myrig", refinery before witness)
	// 4. gastown/witness
	// 5. gastown/crew/alpha (crew after witness, alpha < zed)
	// 6. gastown/crew/zed
	// 7. gastown/polecat/furiosa (polecat last within rig)
	// 8. myrig/witness
	wantOrder := []struct {
		wantType AgentType
		wantName string
	}{
		{AgentMayor, "hq-mayor"},
		{AgentDeacon, "hq-deacon"},
		{AgentRefinery, "gt-refinery"},
		{AgentWitness, "gt-witness"},
		{AgentCrew, "gt-crew-alpha"},
		{AgentCrew, "gt-crew-zed"},
		{AgentPolecat, "gt-furiosa"},
		{AgentWitness, "mr-witness"},
	}

	if len(got) != len(wantOrder) {
		t.Fatalf("filterAndSortSessions returned %d agents, want %d", len(got), len(wantOrder))
	}

	for i, want := range wantOrder {
		if got[i].Type != want.wantType {
			t.Errorf("position %d: type = %d, want %d (session %q)", i, got[i].Type, want.wantType, got[i].Name)
		}
		if got[i].Name != want.wantName {
			t.Errorf("position %d: name = %q, want %q", i, got[i].Name, want.wantName)
		}
	}
}

func TestFilterAndSortSessions_CombinedFiltering(t *testing.T) {
	setupCmdTestRegistry(t)
	input := []string{
		"hq-mayor",
		"hq-boot",        // boot: always filtered
		"gt-furiosa",     // polecat: filtered when includePolecats=false
		"random-session", // non-gastown: always filtered
		"gt-witness",
	}

	got := filterAndSortSessions(input, false)
	if len(got) != 2 {
		t.Fatalf("filterAndSortSessions(combined, polecats=false) returned %d agents, want 2 (mayor + witness)", len(got))
	}
	if got[0].Type != AgentMayor {
		t.Errorf("position 0: type = %d, want AgentMayor", got[0].Type)
	}
	if got[1].Type != AgentWitness {
		t.Errorf("position 1: type = %d, want AgentWitness", got[1].Type)
	}

	got = filterAndSortSessions(input, true)
	if len(got) != 3 {
		t.Fatalf("filterAndSortSessions(combined, polecats=true) returned %d agents, want 3 (mayor + witness + polecat)", len(got))
	}
}

func TestRunAgentsList_EmptyList_Output(t *testing.T) {
	setupCmdTestRegistry(t)

	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping stdout check")
	}

	// Exercise the real runAgentsList code path with stdout capture.
	// tmux binary exists but the server may not be running, in which
	// case runAgentsList returns an error and output is empty.
	var runErr error
	output := captureStdout(t, func() {
		runErr = runAgentsList(nil, nil)
	})

	if runErr != nil {
		// tmux server not running — nothing to assert on stdout
		return
	}

	// runAgentsList succeeded: output is either the empty-list message
	// or a real agent listing if gastown sessions happen to be running.
	if !strings.Contains(output, "No agent sessions running.") &&
		!strings.Contains(output, "Mayor") &&
		!strings.Contains(output, "Deacon") &&
		!strings.Contains(output, "witness") {
		t.Errorf("unexpected output from runAgentsList: %q", output)
	}
}

func TestGuessSessionFromWorkerDir(t *testing.T) {
	setupCmdTestRegistry(t)
	townRoot := "/town"

	tests := []struct {
		name      string
		workerDir string
		want      string
	}{
		{"crew worker", "/town/gastown/crew/max", "gt-crew-max"},
		{"polecat worker", "/town/gastown/polecats/furiosa", "gt-furiosa"},
		{"witness worker", "/town/gastown/witness/main", "gt-witness"},
		{"refinery worker", "/town/gastown/refinery/main", "gt-refinery"},
		{"unknown type", "/town/gastown/unknown/thing", ""},
		{"too few path parts", "/town/gastown", ""},
		{"different rig", "/town/myrig/crew/alpha", "mr-crew-alpha"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := guessSessionFromWorkerDir(tt.workerDir, townRoot)
			if got != tt.want {
				t.Errorf("guessSessionFromWorkerDir(%q, %q) = %q, want %q",
					tt.workerDir, townRoot, got, tt.want)
			}
		})
	}
}
